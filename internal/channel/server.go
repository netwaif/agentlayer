// Package channel은 Claude Code 채널(MCP 서버가 notifications/claude/channel로 세션에 이벤트를 미는 구조)의
// stdio 서버다. 외부 의존 없이 줄 단위 JSON-RPC 2.0만 구현한다 — initialize·ping·tools/list와 알림 전송.
// stdout은 프로토콜 전용이고, 사람용 로그는 Log 콜백(stderr)로만 나간다.
package channel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Name은 서버 이름 — 등록명·source= 값·`server:agentlayer` 플래그가 전부 이 값이다.
const Name = "agentlayer"

// Notification은 세션에 밀어 넣는 이벤트 한 건. Meta 키는 Claude 규칙(^[a-zA-Z_][a-zA-Z0-9_]*$)을 지켜야
// 하며 어긋난 키는 Notify가 버리고 로그를 남긴다.
type Notification struct {
	Content string
	Meta    map[string]string
}

var metaKey = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type Server struct {
	in           io.Reader
	out          io.Writer
	version      string
	instructions string
	mu           sync.Mutex // ready/queue 보호 + initialize 응답→ready→큐 방출의 순서 보장
	wmu          sync.Mutex // out 쓰기 직렬화(mu 안에서 잡을 수 있다 — 역순 금지)
	ready        bool
	queue        []Notification
	inited       chan struct{} // initialize 응답을 보낸 뒤 닫힌다
	initOnce     sync.Once
	fail         chan error // 첫 stdout 쓰기 실패 — Run이 이 오류로 즉시 끝난다(편지 소모 방지)
	Log          func(string)
}

func New(in io.Reader, out io.Writer, version, instructions string) *Server {
	if version == "" {
		version = "dev"
	}
	return &Server{in: in, out: out, version: version, instructions: instructions, inited: make(chan struct{}), fail: make(chan error, 1), Log: func(string) {}}
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type line struct {
	b   []byte
	err error
}

// Run은 stdin을 한 줄씩 읽어 처리한다. EOF면 nil, ctx가 끝나면 ctx.Err().
func (s *Server) Run(ctx context.Context) error {
	ch := make(chan line)
	go func() {
		r := bufio.NewReaderSize(s.in, 1<<20) // 편지는 16KiB 상한이라 넉넉
		for {
			b, err := r.ReadBytes('\n')
			if len(bytes.TrimSpace(b)) > 0 {
				select {
				case ch <- line{b: b}:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				select {
				case ch <- line{err: err}:
				case <-ctx.Done():
				}
				return
			}
		}
	}()
	// 리더 고루틴은 ctx 취소 뒤에도 ReadBytes에 막혀 남을 수 있다 — 프로세스 종료와 함께 사라지므로 의도된 누수.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-s.fail:
			return err
		case l := <-ch:
			if l.err != nil {
				if errors.Is(l.err, io.EOF) {
					return nil
				}
				return l.err
			}
			s.handle(l.b)
		}
	}
}

func (s *Server) handle(raw []byte) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		s.Log("잘못된 JSON 줄 무시: " + err.Error())
		return
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return // 알림(notifications/initialized 등)은 응답하지 않는다
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2024-11-05"
		}
		// 응답 → ready → 큐 방출을 한 임계구역에서: 응답 직후 도착한 Notify가 큐에 갇히거나 큐보다 앞서지 않게.
		s.mu.Lock()
		s.reply(req.ID, map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities": map[string]any{
				"tools":        map[string]any{},
				"experimental": map[string]any{"claude/channel": map[string]any{}},
			},
			"serverInfo":   map[string]any{"name": Name, "version": s.version},
			"instructions": s.instructions,
		})
		s.ready = true
		s.initOnce.Do(func() { close(s.inited) })
		q := s.queue
		s.queue = nil
		for _, n := range q {
			s.send(n)
		}
		s.mu.Unlock()
	case "ping":
		s.reply(req.ID, map[string]any{})
	case "tools/list":
		s.reply(req.ID, map[string]any{"tools": []any{}})
	default:
		s.write(map[string]any{"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": -32601, "message": "method not found: " + req.Method}})
	}
}

// Initialized는 클라이언트가 initialize를 마치면 닫히는 채널 — 수신함 소비를 그 뒤로 미루는 데 쓴다.
func (s *Server) Initialized() <-chan struct{} { return s.inited }

// Notify는 편지 1건을 알림으로 보낸다. initialize 전이면 큐에 쌓아 두었다가 응답 직후 순서대로 보낸다.
func (s *Server) Notify(n Notification) {
	n = s.cleanMeta(n)
	s.mu.Lock()
	if !s.ready {
		s.queue = append(s.queue, n)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.send(n)
}

func (s *Server) cleanMeta(n Notification) Notification {
	meta := map[string]string{}
	var bad []string
	for k, v := range n.Meta {
		if metaKey.MatchString(k) {
			meta[k] = v
		} else {
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		s.Log("meta 키 규칙 위반으로 버림: " + strings.Join(bad, ", "))
	}
	n.Meta = meta
	return n
}

func (s *Server) send(n Notification) {
	s.write(map[string]any{"jsonrpc": "2.0", "method": "notifications/claude/channel",
		"params": map[string]any{"content": n.Content, "meta": n.Meta}})
}

func (s *Server) reply(id json.RawMessage, result any) {
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *Server) write(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		s.Log("직렬화 실패: " + err.Error())
		return
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if _, err := s.out.Write(append(b, '\n')); err != nil {
		s.Log(fmt.Sprintf("stdout 쓰기 실패(세션 종료?): %v", err))
		select {
		case s.fail <- err:
		default:
		}
	}
}
