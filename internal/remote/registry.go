// Package remote는 다른 실행기(호스팅어 Hermes 등)를 AI 회사 직원으로 붙이는 배관이다.
// 등록 파일은 <state>/remotes/<이름>.json — 에이전트 저장소(agents/)와 분리해 scan.Sync의 DEAD 처리를 피한다.
package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Remote는 원격 직원 하나의 등록. Kind가 hermes면 SSH·Exec·Profile·WorkspaceRoot·Mailbox·MaxRuntime을,
// exec면 Commands를 쓴다.
type Remote struct {
	Name    string    `json:"name"`
	Kind    string    `json:"kind"` // hermes | exec
	Poll    string    `json:"poll,omitempty"`
	AddedAt time.Time `json:"added_at"`

	SSH           string   `json:"ssh,omitempty"`
	Exec          []string `json:"exec,omitempty"`
	Profile       string   `json:"profile,omitempty"`
	Board         string   `json:"board,omitempty"`
	WorkspaceRoot string   `json:"workspace_root,omitempty"`
	Mailbox       string   `json:"mailbox,omitempty"`
	MaxRuntime    string   `json:"max_runtime,omitempty"`

	Commands map[string][]string `json:"commands,omitempty"` // dispatch·poll·reply·pull·mailbox·finish·check
}

const (
	DefaultMailbox    = "imac-manager"
	DefaultMaxRuntime = "2h"
	DefaultPoll       = 5 * time.Second
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidName — 파일명 한 조각이고 ':'가 없다('<세션>:<창>' 파싱과 충돌 금지).
func ValidName(name string) bool {
	return nameRe.MatchString(name) && name != "." && name != ".."
}

// PollInterval은 poll 설정(1초 이상)이거나 기본 5초.
func (r Remote) PollInterval() time.Duration {
	if d, err := time.ParseDuration(r.Poll); err == nil && d >= time.Second {
		return d
	}
	return DefaultPoll
}

// Validate는 종류별 필수 필드를 확인한다.
func (r Remote) Validate() error {
	if !ValidName(r.Name) {
		return fmt.Errorf("이름 형식 오류: %q (영숫자·점·밑줄·하이픈 1~64자, ':' 불가)", r.Name)
	}
	switch r.Kind {
	case "hermes":
		if r.SSH == "" || r.Profile == "" || r.WorkspaceRoot == "" {
			return errors.New("hermes 원격은 --ssh, --profile, --workspace-root가 필수")
		}
		if !strings.HasPrefix(r.WorkspaceRoot, "/") {
			return errors.New("--workspace-root는 원격 절대경로")
		}
	case "exec":
		if len(r.Commands["dispatch"]) == 0 || len(r.Commands["poll"]) == 0 {
			return errors.New("exec 원격은 commands.dispatch와 commands.poll이 필수")
		}
	default:
		return fmt.Errorf("알 수 없는 kind: %q (hermes | exec)", r.Kind)
	}
	if r.Poll != "" {
		if _, err := time.ParseDuration(r.Poll); err != nil {
			return fmt.Errorf("poll 형식 오류: %w", err)
		}
	}
	return nil
}

// Dir은 등록 파일 디렉터리.
func Dir(stateDir string) string { return filepath.Join(stateDir, "remotes") }

func path(stateDir, name string) string { return filepath.Join(Dir(stateDir), name+".json") }

// Save는 기본값(Mailbox·MaxRuntime)을 채우고 원자적으로 쓴다.
func Save(stateDir string, r Remote) error {
	if r.Kind == "hermes" {
		if r.Mailbox == "" {
			r.Mailbox = DefaultMailbox
		}
		if r.MaxRuntime == "" {
			r.MaxRuntime = DefaultMaxRuntime
		}
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.AddedAt.IsZero() {
		r.AddedAt = time.Now()
	}
	if err := os.MkdirAll(Dir(stateDir), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(Dir(stateDir), "."+r.Name+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path(stateDir, r.Name))
}

// Load — 없으면 ok=false, 에러 없음. 이름이 규칙에 어긋나면 에러.
func Load(stateDir, name string) (*Remote, bool, error) {
	if !ValidName(name) {
		return nil, false, fmt.Errorf("원격 이름 형식 오류: %q", name)
	}
	b, err := os.ReadFile(path(stateDir, name))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var r Remote
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, false, fmt.Errorf("등록 파일 손상 %s: %w", path(stateDir, name), err)
	}
	return &r, true, nil
}

// List는 등록 전부를 이름순으로. 디렉터리가 없으면 빈 목록.
func List(stateDir string) ([]Remote, error) {
	entries, err := os.ReadDir(Dir(stateDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Remote
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(Dir(stateDir), e.Name()))
		if err != nil {
			continue
		}
		var r Remote
		if json.Unmarshal(b, &r) == nil {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Delete는 등록을 지운다. 없었으면 found=false.
func Delete(stateDir, name string) (bool, error) {
	if !ValidName(name) {
		return false, fmt.Errorf("원격 이름 형식 오류: %q", name)
	}
	err := os.Remove(path(stateDir, name))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
