package main

import (
	"testing"

	"github.com/netwaif/agentlayer/internal/usage"
)

// 카드 2차 게시(usage 갱신 반영)가 필요한지 판정 — TS가 바뀌었을 때만.
func TestUsagePayloadChanged(t *testing.T) {
	a := &usage.Payload{TS: "2026-09-01T00:00:00Z"}
	b := &usage.Payload{TS: "2026-09-01T00:05:00Z"}
	cases := []struct {
		name     string
		old, new *usage.Payload
		want     bool
	}{
		{"갱신 실패(nil)는 재게시 없음", a, nil, false},
		{"캐시 없다가 생기면 재게시", nil, a, true},
		{"TS 동일이면 재게시 없음", a, a, false},
		{"TS 바뀌면 재게시", a, b, true},
		{"둘 다 nil이면 없음", nil, nil, false},
	}
	for _, c := range cases {
		if got := usagePayloadChanged(c.old, c.new); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}
