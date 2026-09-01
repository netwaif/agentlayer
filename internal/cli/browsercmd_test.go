package cli

import (
	"bytes"
	"strings"
	"testing"
)

// 플래그는 인자 위치와 무관하게 인식돼야 한다 — Go flag는 첫 비플래그
// 인자에서 멈추므로 `shot <url> --send`가 --send를 조용히 삼키면 안 된다.
func TestParseShotArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		url     string
		send    bool
		wantErr bool
	}{
		{"url 뒤 --send", []string{"https://example.com", "--send"}, "https://example.com", true, false},
		{"--send 뒤 url", []string{"--send", "https://example.com"}, "https://example.com", true, false},
		{"인자 없음", nil, "", false, false},
		{"url만", []string{"https://example.com"}, "https://example.com", false, false},
		{"잉여 인자는 에러", []string{"a", "b"}, "", false, true},
		{"모르는 플래그는 에러", []string{"--bogus"}, "", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			url, send, err := parseShotArgs(c.args)
			if c.wantErr {
				if err == nil {
					t.Fatalf("에러여야 함: url=%q send=%v", url, send)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if url != c.url || send != c.send {
				t.Errorf("got url=%q send=%v, want url=%q send=%v", url, send, c.url, c.send)
			}
		})
	}
}

// errors는 위치 인자가 없다 — --send만 인식하고 잉여 인자는 에러.
func TestParseErrorsArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		send    bool
		wantErr bool
	}{
		{"인자 없음", nil, false, false},
		{"--send", []string{"--send"}, true, false},
		{"잉여 인자는 에러", []string{"뭔가"}, false, true},
		{"잉여 인자 뒤 --send도 에러", []string{"뭔가", "--send"}, false, true},
		{"모르는 플래그는 에러", []string{"--bogus"}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			send, err := parseErrorsArgs(c.args)
			if c.wantErr {
				if err == nil {
					t.Fatalf("에러여야 함: send=%v", send)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if send != c.send {
				t.Errorf("got send=%v, want %v", send, c.send)
			}
		})
	}
}

// 미지 서브커맨드는 명확한 에러로 알린다 — Task 6~8이 case를 추가해도
// default 분기의 문구는 유지돼야 한다.
func TestRunBrowserUnknownSub(t *testing.T) {
	var buf bytes.Buffer
	err := RunBrowser(&buf, []string{"없는명령"})
	if err == nil || !strings.Contains(err.Error(), "없는명령") {
		t.Fatalf("미지 서브커맨드는 에러: %v", err)
	}
	if !strings.Contains(err.Error(), "help") {
		t.Errorf("에러에 help 안내가 없다: %v", err)
	}
}
