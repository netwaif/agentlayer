package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// Hermes는 `hermes kanban` CLI 위의 어댑터. 카드 하나 = 업무 하나(후속 지시는 --parent로 새 카드).
type Hermes struct {
	R               Runner
	Profile         string
	Board           string
	WorkspaceRoot   string
	MailboxAssignee string
	MaxRuntime      string
	Now             func() time.Time
}

// Card는 `kanban show --json`에서 읽는 부분만.
type Card struct {
	Task struct {
		ID            string  `json:"id"`
		Title         string  `json:"title"`
		Status        string  `json:"status"`
		Result        *string `json:"result"`
		WorkspacePath string  `json:"workspace_path"`
		CreatedBy     string  `json:"created_by"`
	} `json:"task"`
	LatestSummary *string `json:"latest_summary"`
	Comments      []struct {
		Author    string `json:"author"`
		Body      string `json:"body"`
		CreatedAt int64  `json:"created_at"`
	} `json:"comments"`
	Events []struct {
		Kind      string          `json:"kind"`
		Payload   json.RawMessage `json:"payload"`
		CreatedAt int64           `json:"created_at"`
	} `json:"events"`
	Runs []struct {
		Outcome string  `json:"outcome"`
		Error   *string `json:"error"`
		Summary string  `json:"summary"`
	} `json:"runs"`
}

// listItem은 `kanban list --json`의 원소.
type listItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Assignee  string `json:"assignee"`
	Status    string `json:"status"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

func firstLine120(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
}

func isFailedOutcome(o string) bool { return o == "crashed" || o == "timed_out" || o == "gave_up" }

// StatusOfCard — 스펙 3절의 상태 대응표.
func StatusOfCard(c Card) Status {
	var s Status
	if n := len(c.Events); n > 0 {
		s.Seen = c.Events[n-1].CreatedAt
	}
	switch c.Task.Status {
	case "running", "scheduled":
		s.State = state.StateWorking
	case "blocked":
		s.State = state.StateWaiting
		s.Ask = blockedReason(c)
	case "done":
		s.State = state.StateDoneUnread
		if c.LatestSummary != nil && *c.LatestSummary != "" {
			s.Summary = firstLine120(*c.LatestSummary)
		} else if c.Task.Result != nil {
			s.Summary = firstLine120(*c.Task.Result)
		}
	case "crashed", "timed_out", "gave_up":
		s.State = state.StateError
		s.Error = lastRunError(c)
	default: // todo · ready · triage · archived
		s.State = state.StateIdle
		// 마지막 실행이 실패로 끝나 ready로 돌아온 카드(재시도 대기)도 총괄이 알아야 한다.
		if n := len(c.Runs); n > 0 && isFailedOutcome(c.Runs[n-1].Outcome) {
			s.State = state.StateError
			s.Error = lastRunError(c)
		}
	}
	return s
}

func blockedReason(c Card) string {
	for i := len(c.Events) - 1; i >= 0; i-- {
		if c.Events[i].Kind != "blocked" {
			continue
		}
		var p struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(c.Events[i].Payload, &p) == nil && p.Reason != "" {
			return p.Reason
		}
		break
	}
	for i := len(c.Comments) - 1; i >= 0; i-- {
		if strings.HasPrefix(c.Comments[i].Body, "BLOCKED:") {
			return strings.TrimSpace(strings.TrimPrefix(c.Comments[i].Body, "BLOCKED:"))
		}
	}
	return "입력 대기"
}

func lastRunError(c Card) string {
	for i := len(c.Runs) - 1; i >= 0; i-- {
		if c.Runs[i].Error != nil && *c.Runs[i].Error != "" {
			return firstLine120(*c.Runs[i].Error)
		}
	}
	return c.Task.Status
}

func (h *Hermes) kanban(args ...string) []string {
	base := []string{"hermes", "kanban"}
	if h.Board != "" {
		base = append(base, "--board", h.Board)
	}
	return append(base, args...)
}

func (h *Hermes) run(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return h.R.Run(ctx, nil, args...)
}

type dispatchResult struct {
	Spawned []struct {
		TaskID string `json:"task_id"`
	} `json:"spawned"`
	SkippedUnassigned       []string `json:"skipped_unassigned"`
	SkippedNonspawnable     []string `json:"skipped_nonspawnable"`
	SkippedPerProfileCapped []string `json:"skipped_per_profile_capped"`
}

// dispatch는 ready 카드를 띄우고 want가 실제로 떴는지 확인한다.
func (h *Hermes) dispatch(ctx context.Context, want Handle) error {
	out, err := h.run(ctx, TimeoutDispatch, h.kanban("dispatch", "--max", "3", "--json")...)
	if err != nil {
		return err
	}
	var d dispatchResult
	if err := json.Unmarshal(bytes.TrimSpace(out), &d); err != nil {
		return fmt.Errorf("dispatch 응답 파싱: %w", err)
	}
	for _, s := range d.Spawned {
		if s.TaskID == want {
			return nil
		}
	}
	reason := "spawned 목록에 없음"
	for name, ids := range map[string][]string{"skipped_unassigned": d.SkippedUnassigned, "skipped_nonspawnable": d.SkippedNonspawnable,
		"skipped_per_profile_capped": d.SkippedPerProfileCapped} {
		for _, id := range ids {
			if id == want {
				reason = name
			}
		}
	}
	return fmt.Errorf("카드 %s 기동 실패: %s (다음 send로 재시도)", want, reason)
}

func (h *Hermes) Dispatch(ctx context.Context, taskID, title, body string, parent Handle) (Handle, error) {
	ws := h.WorkspaceRoot + "/" + taskID
	if _, err := h.run(ctx, TimeoutQuery, "mkdir", "-p", ws); err != nil {
		return "", err
	}
	full := taskID
	if title != "" {
		full = taskID + " " + title
	}
	key := "agentlayer:" + taskID
	if parent != "" {
		key += ":" + parent
	}
	args := []string{"create", full, "--assignee", h.Profile, "--idempotency-key", key, "--created-by", "agentlayer",
		"--workspace", "dir:" + ws, "--max-runtime", h.MaxRuntime}
	if parent != "" {
		args = append(args, "--parent", parent)
	}
	args = append(args, "--body", body, "--json")
	out, err := h.run(ctx, TimeoutDispatch, h.kanban(args...)...)
	if err != nil {
		return "", err
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &created); err != nil || created.ID == "" {
		return "", fmt.Errorf("create 응답에 id 없음: %s", firstLine120(string(out)))
	}
	if err := h.dispatch(ctx, created.ID); err != nil {
		return created.ID, err
	}
	return created.ID, nil
}

func (h *Hermes) show(ctx context.Context, id Handle) (Card, error) {
	out, err := h.run(ctx, TimeoutQuery, h.kanban("show", id, "--json")...)
	if err != nil {
		return Card{}, err
	}
	var c Card
	if err := json.Unmarshal(bytes.TrimSpace(out), &c); err != nil {
		return Card{}, fmt.Errorf("show 응답 파싱: %w", err)
	}
	return c, nil
}

func (h *Hermes) Poll(ctx context.Context, id Handle) (Status, error) {
	c, err := h.show(ctx, id)
	if err != nil {
		return Status{}, err
	}
	return StatusOfCard(c), nil
}

func (h *Hermes) Reply(ctx context.Context, id Handle, text string) error {
	if _, err := h.run(ctx, TimeoutDispatch, h.kanban("unblock", id, "--reason", text)...); err != nil {
		return err
	}
	return h.dispatch(ctx, id)
}

func (h *Hermes) Pull(ctx context.Context, id Handle, destDir string) error {
	c, err := h.show(ctx, id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	if c.Task.WorkspacePath != "" {
		ctx2, cancel := context.WithTimeout(ctx, TimeoutPull)
		defer cancel()
		out, err := h.R.Run(ctx2, nil, "tar", "-C", c.Task.WorkspacePath, "-cf", "-", ".")
		if err != nil {
			return fmt.Errorf("산출물 tar: %w", err)
		}
		if err := Untar(destDir, bytes.NewReader(out), MaxPullBytes); err != nil {
			return err
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n- 카드: %s\n- 상태: %s\n- 작업 폴더(원격): %s\n- 회수 시각: %s\n\n", c.Task.Title, c.Task.ID, c.Task.Status,
		c.Task.WorkspacePath, h.Now().Format(time.RFC3339))
	if c.Task.Result != nil {
		fmt.Fprintf(&b, "## result\n\n%s\n\n", *c.Task.Result)
	}
	if c.LatestSummary != nil {
		fmt.Fprintf(&b, "## summary\n\n%s\n\n", *c.LatestSummary)
	}
	for _, r := range c.Runs {
		e := ""
		if r.Error != nil {
			e = " error=" + *r.Error
		}
		fmt.Fprintf(&b, "- run outcome=%s%s\n", r.Outcome, e)
	}
	return os.WriteFile(filepath.Join(destDir, "RESULT.md"), []byte(b.String()), 0o644)
}

// Mailbox — 예약 담당자 앞으로 온 todo·ready 카드를 편지로 돌려주고 claim→complete로 수신 확인한다.
func (h *Hermes) Mailbox(ctx context.Context) ([]Letter, error) {
	if h.MailboxAssignee == "" {
		return nil, nil
	}
	out, err := h.run(ctx, TimeoutQuery, h.kanban("list", "--json")...)
	if err != nil {
		return nil, err
	}
	var items []listItem
	if err := json.Unmarshal(bytes.TrimSpace(out), &items); err != nil {
		return nil, fmt.Errorf("list 응답 파싱: %w", err)
	}
	var letters []Letter
	for _, it := range items {
		if it.Assignee != h.MailboxAssignee || (it.Status != "todo" && it.Status != "ready") {
			continue
		}
		if _, err := h.run(ctx, TimeoutQuery, h.kanban("claim", it.ID)...); err != nil {
			continue
		}
		ack := "received by agentlayer " + h.Now().UTC().Format(time.RFC3339)
		if _, err := h.run(ctx, TimeoutQuery, h.kanban("complete", it.ID, "--result", ack)...); err != nil {
			continue
		}
		l := Letter{ID: it.ID, From: it.CreatedBy, Text: it.Body, At: time.Unix(it.CreatedAt, 0)}
		if strings.HasPrefix(it.Title, "[") {
			if end := strings.IndexByte(it.Title, ']'); end > 1 {
				l.TaskID = it.Title[1:end]
			}
		}
		if l.Text == "" {
			l.Text = it.Title
		}
		letters = append(letters, l)
	}
	return letters, nil
}

func (h *Hermes) Finish(ctx context.Context, id Handle) error {
	_, err := h.run(ctx, TimeoutQuery, h.kanban("archive", id)...)
	return err
}

func (h *Hermes) Check(ctx context.Context) (Info, error) {
	start := time.Now()
	out, err := h.run(ctx, TimeoutQuery, "hermes", "--version")
	if err != nil {
		return Info{}, err
	}
	info := Info{Version: firstLine120(string(out)), RoundTrip: time.Since(start)}
	out, err = h.run(ctx, TimeoutQuery, h.kanban("assignees")...)
	if err != nil {
		return info, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == h.Profile {
			info.ProfileOK = f[1] == "yes"
			info.Detail = line
		}
	}
	if !info.ProfileOK {
		return info, errors.New("프로필 " + h.Profile + "이 서버에 없습니다(hermes kanban assignees)")
	}
	return info, nil
}
