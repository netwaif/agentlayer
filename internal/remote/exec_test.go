package remote

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExecAdapterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	dispatch := writeScript(t, dir, "d.sh", `echo "dispatch $1 $2 $(cat "$3") $4 $5" >> `+log+`; echo '{"handle":"h-1"}'`)
	poll := writeScript(t, dir, "p.sh", `echo "poll $1" >> `+log+`; echo '{"status":"waiting","ask":"어느 파일?","seen":42}'`)
	reply := writeScript(t, dir, "r.sh", `echo "reply $1 $(cat "$2")" >> `+log)
	mailbox := writeScript(t, dir, "m.sh", `echo '[{"id":"m1","from":"oc","text":"안녕","task_id":"","at":"2026-09-25T10:00:00Z"}]'`)
	e := &Exec{Commands: map[string][]string{
		"dispatch": {dispatch, "{task_id}", "{title}", "{body_file}", "{parent}", "{attempt}"},
		"poll":     {poll, "{handle}"},
		"reply":    {reply, "{handle}", "{text_file}"},
		"mailbox":  {mailbox},
	}}
	ctx := context.Background()
	h, err := e.Dispatch(ctx, DispatchRequest{TaskID: "T-1", Title: "제목", Body: "본문 it's", Attempt: "a1"})
	if err != nil || h != "h-1" {
		t.Fatalf("Dispatch=%q %v", h, err)
	}
	s, err := e.Poll(ctx, "h-1")
	if err != nil || s.State != state.StateWaiting || s.Ask != "어느 파일?" || s.Seen != 42 {
		t.Fatalf("Poll=%+v %v", s, err)
	}
	if err := e.Reply(ctx, "h-1", "a.txt"); err != nil {
		t.Fatal(err)
	}
	letters, err := e.Mailbox(ctx)
	if err != nil || len(letters) != 1 || letters[0].From != "oc" || letters[0].Text != "안녕" || letters[0].At.Year() != 2026 {
		t.Fatalf("Mailbox=%+v %v", letters, err)
	}
	if err := e.Pull(ctx, "h-1", dir); err != nil {
		t.Errorf("pull 미정의는 no-op: %v", err)
	}
	if err := e.Finish(ctx, "h-1"); err != nil {
		t.Errorf("finish 미정의는 no-op: %v", err)
	}
	got, _ := os.ReadFile(log)
	want := "dispatch T-1 제목 본문 it's  a1\npoll h-1\nreply h-1 a.txt\n"
	if string(got) != want {
		t.Errorf("스크립트 호출 기록:\n%s\nwant:\n%s", got, want)
	}
}

func TestExecAdapterBadJSON(t *testing.T) {
	dir := t.TempDir()
	poll := writeScript(t, dir, "p.sh", `echo not-json`)
	e := &Exec{Commands: map[string][]string{"dispatch": {"true"}, "poll": {poll, "{handle}"}}}
	if _, err := e.Poll(context.Background(), "h"); err == nil {
		t.Error("불량 JSON은 에러")
	}
	unknown := writeScript(t, dir, "u.sh", `echo '{"status":"weird"}'`)
	e.Commands["poll"] = []string{unknown}
	if _, err := e.Poll(context.Background(), "h"); err == nil {
		t.Error("알 수 없는 status는 에러")
	}
}

func TestOpenExec(t *testing.T) {
	ad, err := Open(Remote{Name: "oc", Kind: "exec", Commands: map[string][]string{"dispatch": {"true"}, "poll": {"true"}}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ad.(*Exec); !ok {
		t.Errorf("Open(exec)는 *Exec: %T", ad)
	}
}

func TestExecReplyAndResumeRequireCommands(t *testing.T) {
	e := &Exec{Commands: map[string][]string{"dispatch": {"true"}, "poll": {"true"}}}
	if err := e.Reply(context.Background(), "h", "답"); err == nil {
		t.Error("commands.reply가 없으면 답변이 어디에도 안 간다 — 에러여야 함")
	}
	if err := e.Resume(context.Background(), "h"); err == nil {
		t.Error("commands.resume가 없으면 에러")
	}
	if err := e.Answer(context.Background(), "m1", "답"); err == nil {
		t.Error("commands.answer가 없으면 에러")
	}
}
