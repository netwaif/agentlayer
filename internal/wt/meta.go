// Package wt는 태스크별 git worktree lifecycle을 소유한다.
// 원칙: 생성 전 기록, 삭제 전 검사. 미커밋·미병합이 있으면 절대 지우지 않는다.
package wt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Meta는 worktree 태스크 하나의 정본 기록.
// worktree를 만들기 전에 먼저 저장한다 — 무엇을 어디에 만들었는지의
// 증거가 파일시스템 조작보다 항상 앞서야 한다.
type Meta struct {
	Task      string     `json:"task"`
	Repo      string     `json:"repo"`   // target repo 루트 (절대경로)
	Base      string     `json:"base"`   // base branch
	Branch    string     `json:"branch"` // agent/<task>
	Path      string     `json:"path"`   // worktree 절대경로
	Agent     string     `json:"agent"`  // claude | codex | gemini
	TestCmd   string     `json:"test_cmd,omitempty"`
	TestPass  *bool      `json:"test_pass,omitempty"`
	TestAt    *time.Time `json:"test_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func metaDir(stateDir string) string {
	return filepath.Join(stateDir, "worktrees")
}

func metaPath(stateDir, task string) string {
	safe := strings.NewReplacer("/", "_", string(filepath.Separator), "_").Replace(task)
	return filepath.Join(metaDir(stateDir), safe+".json")
}

func SaveMeta(stateDir string, m *Meta) error {
	if err := os.MkdirAll(metaDir(stateDir), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := metaPath(stateDir, m.Task) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, metaPath(stateDir, m.Task))
}

func LoadMeta(stateDir, task string) (*Meta, error) {
	b, err := os.ReadFile(metaPath(stateDir, task))
	if err != nil {
		return nil, fmt.Errorf("태스크 %q 메타 없음 — wt list로 확인하세요", task)
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func DeleteMeta(stateDir, task string) error {
	return os.Remove(metaPath(stateDir, task))
}

func ListMetas(stateDir string) ([]*Meta, error) {
	entries, err := os.ReadDir(metaDir(stateDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Meta
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(metaDir(stateDir), e.Name()))
		if err != nil {
			continue
		}
		var m Meta
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		out = append(out, &m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
