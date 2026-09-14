// internal/task/assign.go
// Package task는 AI 회사 배관 — 세션에 업무를 등록하고, 상태 전이를 총괄 수신함에 보고하고,
// 수신함을 상주 감시한다. 정본은 파일이며 데몬이 없다(agentlayer 원칙).
package task

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

// Assignment는 세션(pane) 하나 ↔ 업무 하나. 파일은 <state>/tasks/<agent-id>.json.
type Assignment struct {
	TaskID     string    `json:"task_id"`
	AgentID    string    `json:"agent_id"`
	Session    string    `json:"session"`
	Window     string    `json:"window,omitempty"`
	Pane       string    `json:"pane"`
	Inbox      string    `json:"inbox"`
	AssignedAt time.Time `json:"assigned_at"`
}

var ErrAlreadyAssigned = errors.New("이 세션에는 이미 업무가 있습니다 (--replace로 교체)")

var idRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidID는 업무ID 규칙(영숫자·점·밑줄·하이픈, 1~64자).
func ValidID(id string) bool { return idRe.MatchString(id) }

// validAgentID는 에이전트 ID가 경로 조작 없이 파일명 한 조각으로 쓰일 수 있는지 확인한다.
func validAgentID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id
}

// Dir은 업무 등록 파일 디렉터리.
func Dir(stateDir string) string { return filepath.Join(stateDir, "tasks") }

func path(stateDir, agentID string) string {
	return filepath.Join(Dir(stateDir), agentID+".json")
}

// Assign은 등록 파일을 원자적으로 쓴다. 같은 에이전트에 이미 있으면 replace가 아닌 한 거부.
func Assign(stateDir string, as Assignment, replace bool) error {
	if !ValidID(as.TaskID) {
		return fmt.Errorf("업무ID 형식 오류: %q (영숫자·점·밑줄·하이픈 1~64자)", as.TaskID)
	}
	if !validAgentID(as.AgentID) {
		return fmt.Errorf("에이전트 ID 형식 오류: %q", as.AgentID)
	}
	if _, ok, err := Load(stateDir, as.AgentID); err != nil {
		return err
	} else if ok && !replace {
		return ErrAlreadyAssigned
	}
	if err := os.MkdirAll(Dir(stateDir), 0o700); err != nil {
		return err
	}
	return writeAtomic(path(stateDir, as.AgentID), as)
}

// Load는 에이전트의 등록을 읽는다. 없으면 ok=false, 에러 없음.
func Load(stateDir, agentID string) (*Assignment, bool, error) {
	if !validAgentID(agentID) {
		return nil, false, fmt.Errorf("에이전트 ID 형식 오류: %q", agentID)
	}
	b, err := os.ReadFile(path(stateDir, agentID))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var as Assignment
	if err := json.Unmarshal(b, &as); err != nil {
		return nil, false, fmt.Errorf("등록 파일 손상 %s: %w", path(stateDir, agentID), err)
	}
	return &as, true, nil
}

// List는 모든 등록을 TaskID 오름차순으로. 디렉터리가 없으면 빈 목록.
func List(stateDir string) ([]Assignment, error) {
	entries, err := os.ReadDir(Dir(stateDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Assignment
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(Dir(stateDir), e.Name()))
		if err != nil {
			continue
		}
		var as Assignment
		if json.Unmarshal(b, &as) == nil {
			out = append(out, as)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskID < out[j].TaskID })
	return out, nil
}

// Done은 업무ID로 등록을 해제한다. 보고 파일은 건드리지 않는다.
func Done(stateDir, taskID string) (bool, error) {
	list, err := List(stateDir)
	if err != nil {
		return false, err
	}
	found := false
	for _, as := range list {
		if as.TaskID != taskID {
			continue
		}
		if err := os.Remove(path(stateDir, as.AgentID)); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		found = true
	}
	return found, nil
}

// writeAtomic은 temp→rename. 여러 프로세스(hook·CLI)가 동시에 써도 반쪽 파일이 안 생긴다.
func writeAtomic(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
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
	if err := tmp.Chmod(0o600); err != nil {
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
