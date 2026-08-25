package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultDir는 상태 정본 디렉터리. AGENTLAYER_STATE_DIR로 오버라이드 가능.
func DefaultDir() string {
	if d := os.Getenv("AGENTLAYER_STATE_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agentlayer")
	}
	return filepath.Join(home, ".local", "state", "agentlayer")
}

// Store는 디렉터리 기반 상태 저장소. 에이전트 1개 = agents/<id>.json 1개.
// 데몬 없이 여러 프로세스(hook, TUI, CLI)가 동시에 써도 temp→rename
// 원자적 쓰기로 파일이 깨지지 않는다.
type Store struct {
	Dir string
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0o755); err != nil {
		return nil, fmt.Errorf("상태 디렉터리 생성: %w", err)
	}
	return &Store{Dir: dir}, nil
}

func (s *Store) path(id string) string {
	// ID가 경로 구분자를 품어도 파일명 하나로 남게 한다.
	safe := strings.NewReplacer("/", "_", string(filepath.Separator), "_").Replace(id)
	return filepath.Join(s.Dir, "agents", safe+".json")
}

// Save는 레코드를 원자적으로 기록한다.
func (s *Store) Save(a *Agent) error {
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Join(s.Dir, "agents"), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, s.path(a.ID))
}

func (s *Store) Load(id string) (*Agent, error) {
	b, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var a Agent
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("레코드 파싱 %s: %w", id, err)
	}
	return &a, nil
}

// List는 전체 레코드를 Priority → StateSince 순으로 반환한다.
// 깨진 파일은 건너뛴다 — 관제탑은 일부가 깨져도 나머지를 보여줘야 한다.
func (s *Store) List() ([]*Agent, error) {
	entries, err := os.ReadDir(filepath.Join(s.Dir, "agents"))
	if err != nil {
		return nil, err
	}
	var out []*Agent
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, "agents", e.Name()))
		if err != nil {
			continue
		}
		var a Agent
		if err := json.Unmarshal(b, &a); err != nil {
			continue
		}
		out = append(out, &a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := out[i].State.Priority(), out[j].State.Priority()
		if pi != pj {
			return pi < pj
		}
		return out[i].StateSince.Before(out[j].StateSince)
	})
	return out, nil
}

// MarkRead는 DONE_UNREAD를 IDLE로 바꾼다. 다른 상태면 아무것도 안 한다 —
// 읽음 처리는 사용자 행동이며 다른 전이를 덮어쓰면 안 된다.
func (s *Store) MarkRead(id string, now time.Time) error {
	a, err := s.Load(id)
	if err != nil {
		return err
	}
	if a.State != StateDoneUnread {
		return nil
	}
	a.Transition(StateIdle, now)
	return s.Save(a)
}

func (s *Store) Delete(id string) error {
	return os.Remove(s.path(id))
}
