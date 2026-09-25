package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// Exec는 사용자가 등록 파일에 적은 명령 템플릿으로 아무 실행기나 붙이는 어댑터. 본문·답변은 임시 파일로 넘긴다.
type Exec struct {
	Commands map[string][]string
	Dir      string // 작업 폴더(비면 현재 폴더)
}

func (e *Exec) argv(name string, vars map[string]string) ([]string, bool) {
	tmpl := e.Commands[name]
	if len(tmpl) == 0 {
		return nil, false
	}
	out := make([]string, len(tmpl))
	for i, a := range tmpl {
		for k, v := range vars {
			a = strings.ReplaceAll(a, "{"+k+"}", v)
		}
		out[i] = a
	}
	return out, true
}

func (e *Exec) run(ctx context.Context, timeout time.Duration, name string, vars map[string]string) ([]byte, bool, error) {
	argv, ok := e.argv(name, vars)
	if !ok {
		return nil, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = e.Dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), true, fmt.Errorf("%s: %w: %s", name, err, firstLine120(errb.String()))
	}
	return out.Bytes(), true, nil
}

func tempText(text string) (string, func(), error) {
	f, err := os.CreateTemp("", "agentlayer-remote-*.txt")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Chmod(0o600)
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

func (e *Exec) Dispatch(ctx context.Context, req DispatchRequest) (Handle, error) {
	p, cleanup, err := tempText(req.Body)
	if err != nil {
		return "", err
	}
	defer cleanup()
	out, ok, err := e.run(ctx, TimeoutDispatch, "dispatch", map[string]string{"task_id": req.TaskID, "title": req.Title, "body_file": p,
		"parent": req.Parent, "attempt": req.Attempt})
	if !ok {
		return "", errors.New("commands.dispatch가 없습니다")
	}
	if err != nil {
		return "", err
	}
	var r struct {
		Handle string `json:"handle"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &r); err != nil || r.Handle == "" {
		return "", fmt.Errorf("dispatch 응답에 handle 없음: %s", firstLine120(string(out)))
	}
	return r.Handle, nil
}

func (e *Exec) Poll(ctx context.Context, h Handle) (Status, error) {
	out, ok, err := e.run(ctx, TimeoutQuery, "poll", map[string]string{"handle": h})
	if !ok {
		return Status{}, errors.New("commands.poll이 없습니다")
	}
	if err != nil {
		return Status{}, err
	}
	var r struct {
		Status  string `json:"status"`
		Summary string `json:"summary"`
		Ask     string `json:"ask"`
		Error   string `json:"error"`
		Seen    int64  `json:"seen"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &r); err != nil {
		return Status{}, fmt.Errorf("poll 응답 파싱: %w", err)
	}
	s := Status{Summary: firstLine120(r.Summary), Ask: r.Ask, Error: firstLine120(r.Error), Seen: r.Seen}
	switch r.Status {
	case "idle":
		s.State = state.StateIdle
	case "working":
		s.State = state.StateWorking
	case "waiting":
		s.State = state.StateWaiting
	case "done":
		s.State = state.StateDoneUnread
	case "error":
		s.State = state.StateError
	default:
		return Status{}, fmt.Errorf("poll 응답 status 값 불명: %q (idle|working|waiting|done|error)", r.Status)
	}
	return s, nil
}

func (e *Exec) Reply(ctx context.Context, h Handle, text string) error {
	p, cleanup, err := tempText(text)
	if err != nil {
		return err
	}
	defer cleanup()
	_, ok, err := e.run(ctx, TimeoutDispatch, "reply", map[string]string{"handle": h, "text_file": p})
	if !ok {
		return errors.New("commands.reply가 없습니다 — 답변을 전달할 길이 없음")
	}
	return err
}

// Resume — commands.resume(재기동). 없으면 에러: 조용히 성공하면 총괄이 기다리기만 한다.
func (e *Exec) Resume(ctx context.Context, h Handle) error {
	_, ok, err := e.run(ctx, TimeoutDispatch, "resume", map[string]string{"handle": h})
	if !ok {
		return errors.New("commands.resume가 없습니다 — 'task assign --replace' 뒤 다시 보내세요")
	}
	return err
}

func (e *Exec) Pull(ctx context.Context, h Handle, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	_, _, err := e.run(ctx, TimeoutPull, "pull", map[string]string{"handle": h, "dest_dir": destDir})
	return err
}

func (e *Exec) Mailbox(ctx context.Context) ([]Letter, error) {
	out, ok, err := e.run(ctx, TimeoutQuery, "mailbox", nil)
	if !ok || err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	var items []struct {
		ID     string `json:"id"`
		From   string `json:"from"`
		Text   string `json:"text"`
		TaskID string `json:"task_id"`
		At     string `json:"at"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &items); err != nil {
		return nil, fmt.Errorf("mailbox 응답 파싱: %w", err)
	}
	letters := make([]Letter, 0, len(items))
	for _, it := range items {
		at, _ := time.Parse(time.RFC3339, it.At)
		if at.IsZero() {
			at = time.Now()
		}
		letters = append(letters, Letter{ID: it.ID, From: it.From, Text: it.Text, TaskID: it.TaskID, At: at})
	}
	return letters, nil
}

func (e *Exec) Answer(ctx context.Context, id, text string, files []string) error {
	p, cleanup, err := tempText(text)
	if err != nil {
		return err
	}
	defer cleanup()
	lf, cleanup2, err := tempText(strings.Join(files, "\n"))
	if err != nil {
		return err
	}
	defer cleanup2()
	_, ok, err := e.run(ctx, TimeoutPull, "answer", map[string]string{"letter": id, "text_file": p, "files_file": lf})
	if !ok {
		return errors.New("commands.answer가 없습니다 — 답장을 전달할 길이 없음")
	}
	return err
}

func (e *Exec) Finish(ctx context.Context, h Handle) error {
	_, _, err := e.run(ctx, TimeoutQuery, "finish", map[string]string{"handle": h})
	return err
}

func (e *Exec) Check(ctx context.Context) (Info, error) {
	start := time.Now()
	out, ok, err := e.run(ctx, TimeoutQuery, "check", nil)
	if !ok {
		return Info{ProfileOK: true, Detail: "check 명령 없음(생략)"}, nil
	}
	if err != nil {
		return Info{}, err
	}
	return Info{ProfileOK: true, RoundTrip: time.Since(start), Detail: firstLine120(string(out))}, nil
}
