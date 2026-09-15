// Package board는 AI 회사 루트의 tasks/<ID>/task.md·log.md를 읽고 쓰는 유일한 곳이다.
// yaml 파서를 쓰지 않고 줄 단위로 status:·parents:만 다룬다(starter.readStatus와 같은 원칙 —
// 파싱 실패로 관제탑이 멈추면 안 된다). starter 패키지는 다른 루트(MultiAgent)·다른 목적이라 건드리지 않는다.
package board

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrNoStatus — task.md에 ```yaml 블록의 status: 줄이 없어 상태를 쓸 수 없다.
var ErrNoStatus = errors.New("task.md에 ```yaml 블록의 status: 줄이 없습니다")

// TaskFile은 task.md 한 장의 요약.
type TaskFile struct {
	ID      string
	Title   string    // 첫 "# " 제목, 없으면 ID
	Status  string    // yaml status:, 없으면 "unknown"
	Parents []string  // yaml parents: [A, B] 또는 여러 줄 리스트
	Updated time.Time // task.md mtime
}

// TaskDir은 업무 폴더 경로.
func TaskDir(root, id string) string { return filepath.Join(root, "tasks", id) }

func taskPath(root, id string) string { return filepath.Join(TaskDir(root, id), "task.md") }
func logPath(root, id string) string  { return filepath.Join(TaskDir(root, id), "log.md") }

// ReadTaskFile은 task.md를 읽는다. 없으면 os.IsNotExist가 참인 에러.
func ReadTaskFile(root, id string) (TaskFile, error) {
	p := taskPath(root, id)
	st, err := os.Stat(p)
	if err != nil {
		return TaskFile{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return TaskFile{}, err
	}
	tf := TaskFile{ID: id, Title: id, Status: "unknown", Updated: st.ModTime()}
	inYAML, inParents, titleSet := false, false, false
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !titleSet && strings.HasPrefix(trimmed, "# "):
			tf.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			titleSet = true
		case strings.HasPrefix(trimmed, "```yaml"):
			inYAML = true
		case strings.HasPrefix(trimmed, "```"):
			inYAML, inParents = false, false
		case !inYAML:
		case inParents && strings.HasPrefix(trimmed, "- "):
			tf.Parents = append(tf.Parents, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		case strings.HasPrefix(trimmed, "status:"):
			inParents = false
			tf.Status = strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))
		case strings.HasPrefix(trimmed, "parents:"):
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "parents:"))
			tf.Parents = parseInlineList(rest)
			inParents = rest == "" // 여러 줄 리스트가 이어진다
		default:
			inParents = false
		}
	}
	return tf, nil
}

// parseInlineList는 "[A, B]" 또는 "A, B"를 항목 목록으로. 빈 값·"[]"는 nil.
func parseInlineList(s string) []string {
	s = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "["), "]")
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SetStatus는 ```yaml 블록의 status: 줄만 바꾸고(updated: 줄이 있으면 오늘 날짜로) 원자적으로 쓴다.
// 다른 바이트는 그대로 — 총괄이 쓴 본문을 훅이 망치면 안 된다.
func SetStatus(root, id, status string, now time.Time) error {
	p := taskPath(root, id)
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	inYAML, found := false, false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		switch {
		case strings.HasPrefix(trimmed, "```yaml"):
			inYAML = true
		case strings.HasPrefix(trimmed, "```"):
			inYAML = false
		case inYAML && strings.HasPrefix(trimmed, "status:"):
			lines[i] = indent + "status: " + status
			found = true
		case inYAML && strings.HasPrefix(trimmed, "updated:"):
			lines[i] = indent + "updated: " + now.Format("2006-01-02")
		}
	}
	if !found {
		return ErrNoStatus
	}
	return writeAtomic(p, []byte(strings.Join(lines, "\n")))
}

// AppendLog는 "[YYYY-MM-DD HH:MM] [TAG] text" 한 줄을 log.md 끝에 붙인다. 개행은 ⏎로 접는다.
func AppendLog(root, id, tag, text string, now time.Time) error {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "⏎")
	f, err := os.OpenFile(logPath(root, id), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("[" + now.Format("2006-01-02 15:04") + "] [" + tag + "] " + text + "\n")
	return err
}

// ReadLastLog는 log.md의 마지막 비어 있지 않은 줄. 없으면 "".
func ReadLastLog(root, id string) string {
	b, err := os.ReadFile(logPath(root, id))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

// writeAtomic은 temp→rename. 훅·총괄이 동시에 써도 반쪽 파일이 없다.
func writeAtomic(p string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
