// Package board는 AI 회사 루트의 tasks/<ID>/task.md·log.md를 읽고 쓰는 유일한 곳이다.
// yaml 파서를 쓰지 않고 줄 단위로 status:·parents:만 다룬다(starter.readStatus와 같은 원칙 —
// 파싱 실패로 관제탑이 멈추면 안 된다). starter 패키지는 다른 루트(MultiAgent)·다른 목적이라 건드리지 않는다.
package board

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ErrNoStatus — task.md에 ```yaml 블록의 status: 줄이 없어 상태를 쓸 수 없다.
var ErrNoStatus = errors.New("task.md에 ```yaml 블록의 status: 줄이 없습니다")

// ErrBadID — 업무ID가 ValidID 규칙(영숫자·점·밑줄·하이픈, 1~64자)을 어겨 경로 조작(예: "../x") 위험이
// 있다. task.md·log.md를 건드리는 함수는 전부 이 검사를 먼저 한다(방어적 이중화 — task.ValidID가
// 이미 걸러도, 이 패키지가 filesystem을 직접 만지는 유일한 곳이라 여기서도 막는다).
var ErrBadID = errors.New("업무ID 형식 오류")

var idRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidID는 업무ID 규칙(영숫자·점·밑줄·하이픈, 1~64자) — task.ValidID가 위임하는 정본. 문자 집합에
// "."이 있어 정규식만으로는 "."·".."을 걸러내지 못하므로(둘 다 패턴 자체는 통과) 경로 조작 방지용
// 명시 검사를 더한다: 빈 문자열·"."·".."·filepath.Base(id) != id(구분자 포함) 전부 거부.
func ValidID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	if filepath.Base(id) != id {
		return false
	}
	return idRe.MatchString(id)
}

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
	if !ValidID(id) {
		return TaskFile{}, ErrBadID
	}
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
			tf.Parents = append(tf.Parents, stripComment(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
		case strings.HasPrefix(trimmed, "status:"):
			inParents = false
			tf.Status = stripComment(strings.TrimSpace(strings.TrimPrefix(trimmed, "status:")))
		case strings.HasPrefix(trimmed, "parents:"):
			rest := stripComment(strings.TrimSpace(strings.TrimPrefix(trimmed, "parents:")))
			tf.Parents = parseInlineList(rest)
			inParents = rest == "" // 여러 줄 리스트가 이어진다
		default:
			inParents = false
		}
	}
	return tf, nil
}

// stripComment는 " #…"(공백+해시 → 줄 끝) 형태의 인라인 yaml 주석을 잘라낸다. status:·parents:
// 값과 여러 줄 "- item"에서 쓴다. yaml 파서가 아니라 줄 단위 규칙이므로 값 문자열 안에 " #"가
// 들어 있는 경우까지는 다루지 않는다(이 파일들의 실제 사용 범위에서는 없다).
func stripComment(s string) string {
	if i := strings.Index(s, " #"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
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
// 다른 바이트는 그대로 — 총괄이 쓴 본문을 훅이 망치면 안 된다. status: 줄 자체는 통째로
// "status: <새 값>"으로 교체하므로, 그 줄에 " #…" 인라인 주석이 있었다면 함께 사라진다(그
// 한 줄만의 트레이드오프 — ReadTaskFile은 어차피 주석을 잘라내고 읽으므로 무해하다).
func SetStatus(root, id, status string, now time.Time) error {
	if !ValidID(id) {
		return ErrBadID
	}
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
	if !ValidID(id) {
		return ErrBadID
	}
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
	if !ValidID(id) {
		return ""
	}
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

// ReadLog는 log.md의 비어 있지 않은 줄 전부(파일 순서). 없으면 nil. 보드 상세 패널이 쓴다.
func ReadLog(root, id string) []string {
	if !ValidID(id) {
		return nil
	}
	b, err := os.ReadFile(logPath(root, id))
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if s := strings.TrimSpace(line); s != "" && !strings.HasPrefix(s, "<!--") && !strings.HasPrefix(s, "# ") {
			out = append(out, s)
		}
	}
	return out
}

// ReadBody는 task.md에서 첫 "# " 제목 줄과 ```yaml 메타 블록을 뺀 본문(목표·담당·완료 기준 …).
// 없으면 "". 보드 상세 패널이 원문 그대로 보여 주는 용도 — 파싱하지 않는다.
func ReadBody(root, id string) string {
	if !ValidID(id) {
		return ""
	}
	b, err := os.ReadFile(taskPath(root, id))
	if err != nil {
		return ""
	}
	var out []string
	inYAML, titleSkipped := false, false
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !titleSkipped && strings.HasPrefix(trimmed, "# "):
			titleSkipped = true
			continue
		case strings.HasPrefix(trimmed, "```yaml"):
			inYAML = true
			continue
		case inYAML && strings.HasPrefix(trimmed, "```"):
			inYAML = false
			continue
		case inYAML:
			continue
		}
		out = append(out, strings.TrimRight(line, " \t"))
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

// writeAtomic은 temp→rename. 훅·총괄이 동시에 써도 반쪽 파일이 없다. 원래 파일의 권한 비트를
// 보존한다(stat 실패 시 0644로 폴백) — temp 파일은 os.CreateTemp가 0600으로 만들기 때문에
// chmod 없이 rename하면 task.md 권한이 조용히 바뀐다.
func writeAtomic(p string, b []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(p); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
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
