// internal/board/board.go
package board

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 열 이름 — Hermes 칸반의 6열. 저장하는 값이 아니라 status에서 파생한 표시 이름이다.
const (
	ColTodo    = "todo"
	ColReady   = "ready"
	ColRunning = "running"
	ColBlocked = "blocked"
	ColReview  = "review"
	ColDone    = "done"
)

// Columns는 보드 열 순서.
var Columns = []string{ColTodo, ColReady, ColRunning, ColBlocked, ColReview, ColDone}

// Card는 보드에 놓이는 업무 하나.
type Card struct {
	ID      string
	Title   string
	Status  string // task.md 원문 status
	Column  string // 파생 열
	Session string // 등록 세션[:창], 없으면 ""
	State   string // 등록 세션의 상태 단어(idle·WORK·WAIT·DONE·ERR·dead), 없으면 ""
	Parents []string
	Updated time.Time // task.md mtime
	Ready   time.Time // ready 열 진입 시각(마지막 부모 done의 mtime, 부모 없으면 자기 mtime)
	LastLog string
	Unknown bool // status 값을 해석하지 못함(todo 열에 ? 배지)
}

// Link는 업무 등록의 board용 축약 — task 패키지가 board를 import하므로 역방향 의존을 피한다.
type Link struct{ TaskID, Session, Window, AgentID string }

// columnOf는 status → 열. ready는 부모가 전부 done일 때만.
func columnOf(status string, parents []string, done map[string]bool) (string, bool) {
	switch {
	case status == "pending":
		for _, p := range parents {
			if !done[p] {
				return ColTodo, false
			}
		}
		return ColReady, false
	case status == "in_progress":
		return ColRunning, false
	case strings.HasPrefix(status, "waiting_"):
		return ColBlocked, false
	case status == "reviewing":
		return ColReview, false
	case status == "done":
		return ColDone, false
	}
	return ColTodo, true
}

// Load는 root/tasks/*/task.md를 전부 읽어 카드로 만든다(ID 오름차순). tasks/가 없으면 빈 목록.
// states는 AgentID → 상태 단어.
func Load(root string, links []Link, states map[string]string, now time.Time) ([]Card, error) {
	entries, err := os.ReadDir(filepath.Join(root, "tasks"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := map[string]TaskFile{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		tf, err := ReadTaskFile(root, e.Name())
		if err != nil {
			continue // task.md 없는 폴더는 업무가 아니다
		}
		files[e.Name()] = tf
	}
	done := map[string]bool{}
	for id, tf := range files {
		done[id] = tf.Status == "done"
	}
	byTask := map[string]Link{}
	for _, l := range links {
		byTask[l.TaskID] = l
	}
	var cards []Card
	for id, tf := range files {
		col, unknown := columnOf(tf.Status, tf.Parents, done)
		c := Card{ID: id, Title: tf.Title, Status: tf.Status, Column: col, Parents: tf.Parents,
			Updated: tf.Updated, Ready: tf.Updated, LastLog: ReadLastLog(root, id), Unknown: unknown}
		if col == ColReady {
			for _, p := range tf.Parents {
				if pt := files[p]; pt.Updated.After(c.Ready) {
					c.Ready = pt.Updated
				}
			}
		}
		if l, ok := byTask[id]; ok {
			c.Session = l.Session
			if l.Window != "" {
				c.Session += ":" + l.Window
			}
			c.State = states[l.AgentID]
		}
		cards = append(cards, c)
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	return cards, nil
}

// Children은 parents에 parentID를 가진 카드들(입력 순서 유지).
func Children(cards []Card, parentID string) []Card {
	var out []Card
	for _, c := range cards {
		for _, p := range c.Parents {
			if p == parentID {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// StaleReady — ready 열은 Ready 시각, blocked 열은 Updated 시각 기준으로 limit을 넘겼는가.
func StaleReady(c Card, now time.Time, limit time.Duration) bool {
	switch c.Column {
	case ColReady:
		return now.Sub(c.Ready) > limit
	case ColBlocked:
		return now.Sub(c.Updated) > limit
	}
	return false
}

// Counts는 열별 카드 수.
func Counts(cards []Card) map[string]int {
	m := map[string]int{}
	for _, c := range cards {
		m[c.Column]++
	}
	return m
}

// InferRoot는 "<root>/runtime/inbox" 규약에서 root를 뽑는다. 규약 밖이면 "".
func InferRoot(inbox string) string {
	inbox = filepath.Clean(inbox)
	if filepath.Base(inbox) != "inbox" || filepath.Base(filepath.Dir(inbox)) != "runtime" {
		return ""
	}
	return filepath.Dir(filepath.Dir(inbox))
}

// Root는 설정값 우선, 없으면 inbox 목록에서 첫 유추 성공 값. 설정 0개로 동작하기 위한 단일 지점.
// 설정값이 "~" 또는 "~/…"로 시작하면 셸이 없는 환경(설정 파일 읽기)이라 OS가 대신 펼쳐주지 않으므로
// 여기서 os.UserHomeDir()로 직접 펼친다.
func Root(configured string, inboxes []string) string {
	if configured != "" {
		return expandHome(configured)
	}
	for _, in := range inboxes {
		if r := InferRoot(in); r != "" {
			return r
		}
	}
	return ""
}

// expandHome은 선행 "~" 또는 "~/…"를 os.UserHomeDir()로 펼친다. 홈을 못 얻거나 패턴이 아니면 그대로.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// CompanyName은 <root>/직원명부.json의 name. 없으면 폴더명.
func CompanyName(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "직원명부.json"))
	if err == nil {
		var v struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(b, &v) == nil && v.Name != "" {
			return v.Name
		}
	}
	return filepath.Base(root)
}
