package browser

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

// resolveTarget — 작업 탭을 url 일치로 찾되, 페이지 이동으로 url이 어긋난 동안엔
// 기억한 타깃 id를 유지한다(fx.go stickyTarget 주석, 2026-09-23 실측 결함).
func TestResolveTargetStickyAcrossNavigation(t *testing.T) {
	ResetStickyTarget()
	t.Cleanup(ResetStickyTarget)
	a := webTarget{id: proto.TargetTargetID("A"), url: "https://ko.wikipedia.org/"}
	b := webTarget{id: proto.TargetTargetID("B"), url: "https://example.com/"}

	// ① 미상 — targetURL이 비면 없음
	if _, ok := resolveTarget([]webTarget{a, b}, ""); ok {
		t.Fatal("targetURL이 비면 작업 탭이 없어야 한다")
	}
	// ② url 일치 → A, 기억
	id, ok := resolveTarget([]webTarget{a, b}, a.url)
	if !ok || id != a.id {
		t.Fatalf("url 일치 탭 A여야 한다: %v %v", id, ok)
	}
	// ③ A가 클릭으로 다른 주소로 이동(PageMap은 옛 url) → 여전히 A
	a2 := webTarget{id: a.id, url: "https://ko.wikipedia.org/wiki/기계_학습"}
	id, ok = resolveTarget([]webTarget{a2, b}, a.url)
	if !ok || id != a.id {
		t.Fatalf("이동 뒤에도 기억한 A여야 한다: %v %v", id, ok)
	}
	// ④ 에이전트가 B로 옮김(새 목록의 url이 B와 일치) → B로 갱신
	id, ok = resolveTarget([]webTarget{a2, b}, b.url)
	if !ok || id != b.id {
		t.Fatalf("B로 옮겨야 한다: %v %v", id, ok)
	}
	// ⑤ B가 닫히고 url도 안 맞음 → 없음
	if _, ok := resolveTarget([]webTarget{a2}, b.url); ok {
		t.Fatal("기억한 탭이 닫히고 일치도 없으면 작업 탭이 없어야 한다")
	}
}

func TestResolveTargetPrefersRememberedAmongDuplicates(t *testing.T) {
	ResetStickyTarget()
	t.Cleanup(ResetStickyTarget)
	u := "https://example.com/"
	x := webTarget{id: proto.TargetTargetID("X"), url: u}
	y := webTarget{id: proto.TargetTargetID("Y"), url: u}
	// 처음엔 첫 번째(X)
	if id, _ := resolveTarget([]webTarget{x, y}, u); id != x.id {
		t.Fatalf("첫 일치 X여야 한다: %v", id)
	}
	// Y를 기억하게 만든 뒤(같은 url 탭이 Y만 남았다가 X가 다시 열린 상황)
	if id, _ := resolveTarget([]webTarget{y}, u); id != y.id {
		t.Fatalf("Y여야 한다: %v", id)
	}
	if id, _ := resolveTarget([]webTarget{x, y}, u); id != y.id {
		t.Fatalf("같은 url이 여럿이면 기억한 Y를 우선해야 한다: %v", id)
	}
}
