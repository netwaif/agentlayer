package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// CtxInfo는 한 작업 폴더의 최근 에이전트 세션 정보.
type CtxInfo struct {
	Model   string
	UsedPct *float64
	TS      time.Time
}

// SnapshotsDir는 statusline-command.sh가 남기는 Claude 세션 스냅샷 위치.
// usage-coach 생태계와 공유한다 (읽기만).
func SnapshotsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "usage-coach", "sessions")
}

// CodexSessionsRoot는 codex rollout 저장소.
func CodexSessionsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

type snapshot struct {
	CWD        string   `json:"cwd"`
	ProjectDir string   `json:"project_dir"`
	Model      string   `json:"model"`
	Used       *float64 `json:"used"`
	TS         int64    `json:"ts"`
}

// LoadSnapshots는 폴더(절대경로)별 최신 Claude 세션 정보를 모은다.
// project_dir(세션을 띄운 폴더) 우선 — 봇이 하위 폴더로 cd 해도 매칭이
// 끊기지 않는다. 파일 청소 같은 부수효과는 없다(그건 소유자인 discord_dash 몫).
func LoadSnapshots(dir string) map[string]CtxInfo {
	out := map[string]CtxInfo{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s snapshot
		if err := json.Unmarshal(b, &s); err != nil {
			continue
		}
		key := s.ProjectDir
		if key == "" {
			key = s.CWD
		}
		if key == "" {
			continue
		}
		ts := time.Unix(s.TS, 0)
		if prev, ok := out[key]; ok && !ts.After(prev.TS) {
			continue
		}
		out[key] = CtxInfo{Model: s.Model, UsedPct: s.Used, TS: ts}
	}
	return out
}

// GeminiDir는 Gemini CLI 생태계 루트(~/.gemini).
func GeminiDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini")
}

var geminiModelRe = regexp.MustCompile(`"model":"([^"]+)"`)

// GeminiLatest는 workdir에서 가장 최근 stock Gemini CLI 세션의 모델을 찾는다.
// 매핑은 ~/.gemini/projects.json(경로→tmp 폴더명), 정확 일치가 없으면 가장 긴
// 조상 경로로 폴백(봇이 하위 폴더로 cd 해도 안 끊긴다). 세션 파일 각 모델 턴에
// "model" 키가 있어 마지막 것을 쓴다. 컨텍스트 창 크기는 기록되지 않아 %는 없다.
// agy(Antigravity CLI) 세션은 파일에 모델을 안 남기므로 hook의 modelName이 담당.
func GeminiLatest(geminiDir, workdir string) CtxInfo {
	b, err := os.ReadFile(filepath.Join(geminiDir, "projects.json"))
	if err != nil {
		return CtxInfo{}
	}
	var pj struct {
		Projects map[string]string `json:"projects"`
	}
	if json.Unmarshal(b, &pj) != nil {
		return CtxInfo{}
	}
	name, best := "", -1
	for path, n := range pj.Projects {
		if workdir != path && !strings.HasPrefix(workdir, path+string(filepath.Separator)) {
			continue
		}
		if len(path) > best {
			name, best = n, len(path)
		}
	}
	if name == "" {
		return CtxInfo{}
	}
	files, _ := filepath.Glob(filepath.Join(geminiDir, "tmp", name, "chats", "session-*.jsonl"))
	var latest string
	var latestMod time.Time
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		if latest == "" || st.ModTime().After(latestMod) {
			latest, latestMod = f, st.ModTime()
		}
	}
	if latest == "" {
		return CtxInfo{}
	}
	// 꼬리 64KB에서 마지막 model을 찾는다 (세션이 길어도 최근 턴이면 충분)
	f, err := os.Open(latest)
	if err != nil {
		return CtxInfo{}
	}
	defer f.Close()
	st, _ := f.Stat()
	var tail []byte
	if st != nil {
		off := st.Size() - 65536
		if off < 0 {
			off = 0
		}
		tail = make([]byte, st.Size()-off)
		f.ReadAt(tail, off)
	}
	info := CtxInfo{TS: latestMod}
	if m := geminiModelRe.FindAllStringSubmatch(string(tail), -1); len(m) > 0 {
		info.Model = m[len(m)-1][1]
	}
	return info
}

// codex rollout에서 컨텍스트 % 계산 시 제외하는 기본 오버헤드 (codex TUI와 동일).
const codexBaseline = 12000

var codexModelRe = regexp.MustCompile(`"model":"([^"]+)"`)

// codexRolloutsByRecency는 rollout 파일을 최신순으로 나열한다 (최대 400개).
func codexRolloutsByRecency(root string) []string {
	files, _ := filepath.Glob(filepath.Join(root, "*", "*", "*", "*.jsonl"))
	sort.Slice(files, func(i, j int) bool {
		fi, _ := os.Stat(files[i])
		fj, _ := os.Stat(files[j])
		if fi == nil || fj == nil {
			return false
		}
		return fi.ModTime().After(fj.ModTime())
	})
	if len(files) > 400 {
		files = files[:400]
	}
	return files
}

var codexSessionIDRe = regexp.MustCompile(`"session_id":"([^"]+)"`)

// CodexSessionID는 workdir의 가장 최근 codex rollout에서 session_id를 찾는다.
// codex notify에는 세션 ID가 없어 resume은 이 경로로 얻는다.
func CodexSessionID(root, workdir string) string {
	needle := `"cwd":"` + workdir + `"`
	for _, path := range codexRolloutsByRecency(root) {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		head := make([]byte, 4096)
		n, _ := f.Read(head)
		f.Close()
		h := string(head[:n])
		if !strings.Contains(h, needle) {
			continue
		}
		if m := codexSessionIDRe.FindStringSubmatch(h); m != nil {
			return m[1]
		}
	}
	return ""
}

// CodexLatest는 workdir에서 가장 최근 활동한 codex 세션의 모델·컨텍스트%를
// 찾는다. rollout 첫 줄의 session_meta cwd로 판별하고, 파일 꼬리에서
// 마지막 token_count를 읽는다. 못 찾으면 빈 CtxInfo.
func CodexLatest(root, workdir string) CtxInfo {
	files := codexRolloutsByRecency(root)
	needle := `"cwd":"` + workdir + `"`
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		head := make([]byte, 4096)
		n, _ := f.Read(head)
		if !strings.Contains(string(head[:n]), needle) {
			f.Close()
			continue
		}
		st, _ := f.Stat()
		info := CtxInfo{}
		if st != nil {
			info.TS = st.ModTime()
		}
		// 꼬리 128KB에서 마지막 token_count와 model을 찾는다
		var tail []byte
		if st != nil {
			off := st.Size() - 131072
			if off < 0 {
				off = 0
			}
			tail = make([]byte, st.Size()-off)
			f.ReadAt(tail, off)
		}
		f.Close()
		if m := codexModelRe.FindAllStringSubmatch(string(tail), -1); len(m) > 0 {
			info.Model = m[len(m)-1][1]
		} else if whole, err := os.ReadFile(path); err == nil {
			// model 키는 초기 턴에 몰려 있어 큰 rollout에서는 tail 밖 — 전체 폴백
			if m := codexModelRe.FindAllStringSubmatch(string(whole), -1); len(m) > 0 {
				info.Model = m[len(m)-1][1]
			}
		}
		lines := strings.Split(string(tail), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if !strings.Contains(lines[i], `"token_count"`) {
				continue
			}
			if pct, ok := parseCodexTokenCount(lines[i]); ok {
				info.UsedPct = &pct
				break
			}
		}
		return info
	}
	return CtxInfo{}
}

func parseCodexTokenCount(line string) (float64, bool) {
	idx := strings.Index(line, "{")
	if idx < 0 {
		return 0, false
	}
	var rec struct {
		Payload struct {
			Info struct {
				LastTokenUsage struct {
					TotalTokens float64 `json:"total_tokens"`
				} `json:"last_token_usage"`
				ModelContextWindow float64 `json:"model_context_window"`
			} `json:"info"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(line[idx:]), &rec); err != nil {
		return 0, false
	}
	win := rec.Payload.Info.ModelContextWindow
	if win <= codexBaseline {
		return 0, false
	}
	used := rec.Payload.Info.LastTokenUsage.TotalTokens - codexBaseline
	if used < 0 {
		used = 0
	}
	return used / (win - codexBaseline) * 100, true
}
