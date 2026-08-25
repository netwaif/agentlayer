// Package starter는 multi-agent-starter(MultiAgent)의 활성 작업을 읽기
// 전용으로 요약한다. 정밀 관제는 mat의 영역 — 여기서는 관제탑 헤더에
// 띄울 "지금 뭐가 돌고 있나" 한 줄이 목적이다.
package starter

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Task는 starter 작업 요약.
type Task struct {
	Name      string
	Status    string
	UpdatedAt time.Time
}

// DefaultRoot는 starter 정본 후보. 없으면 빈 문자열(패널 생략).
func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	root := filepath.Join(home, "VSCodeWorkspace", "MultiAgent")
	if st, err := os.Stat(filepath.Join(root, "tasks")); err == nil && st.IsDir() {
		return root
	}
	return ""
}

// active는 mat과 같은 기준: in_progress | reviewing | waiting_*.
func active(status string) bool {
	return status == "in_progress" || status == "reviewing" || strings.HasPrefix(status, "waiting_")
}

// ActiveTasks는 활성 작업을 최근 수정 순으로 돌려준다.
func ActiveTasks(root string) []Task {
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "tasks"))
	if err != nil {
		return nil
	}
	var out []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		md := filepath.Join(root, "tasks", e.Name(), "task.md")
		st, err := os.Stat(md)
		if err != nil {
			continue
		}
		status := readStatus(md)
		if !active(status) {
			continue
		}
		out = append(out, Task{Name: e.Name(), Status: status, UpdatedAt: st.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

// readStatus는 task.md의 ```yaml 블록에서 status: 값만 뽑는다.
// 파싱 실패는 "unknown" — 관제탑을 멈추게 하지 않는다.
func readStatus(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	inYAML := false
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```yaml"):
			inYAML = true
		case strings.HasPrefix(trimmed, "```"):
			if inYAML {
				return "unknown" // 블록 끝났는데 status 없음
			}
		case inYAML && strings.HasPrefix(trimmed, "status:"):
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))
		}
	}
	return "unknown"
}
