package usage

import (
	"fmt"
	"math"
)

// classify는 usage-coach coach.py classify()를 그대로 옮긴 판단(등급·권고·이유) — 새 경로도 coach --json과 같은 문장을 낸다.
// 판정 순서(먼저 걸리는 것): #1 7일 빠듯 🔴 · #2 소진형(5h/일일) 넉넉 🟢 · #3 부족+초기화 임박 ⏳ · #4 부족+페이스 뒤처짐 🟡 · #5 그 외 ⚪.
// 소진형 창이 없으면 7일 페이스만으로 🔴/🟢/⚪. 상수도 coach.py와 같다.
const (
	paceMargin = 0.10 // M
	tBig       = 0.40 // T_BIG
	tSoon      = 30.0 // T_SOON(분)
)

type classWin struct {
	wm       float64 // 창 길이(분)
	left     float64 // 남은 몫 0~1
	mins     float64 // 재설정까지 분
	timeLeft float64 // mins / wm
}

func pickWin(ws []classWin, wm float64) *classWin {
	var best *classWin
	for i := range ws {
		if ws[i].wm == wm && (best == nil || ws[i].left < best.left) {
			best = &ws[i]
		}
	}
	return best
}

// durText는 coach.py _dur — 남은 분 → 「4일 2시간」·「3시간 53분」·「12분」.
func durText(mins float64) string {
	m := int(math.Round(mins))
	d, rem := m/1440, m%1440
	h, mm := rem/60, rem%60
	switch {
	case d > 0 && h > 0:
		return fmt.Sprintf("%d일 %d시간", d, h)
	case d > 0:
		return fmt.Sprintf("%d일", d)
	case h > 0 && mm > 0:
		return fmt.Sprintf("%d시간 %d분", h, mm)
	case h > 0:
		return fmt.Sprintf("%d시간", h)
	}
	return fmt.Sprintf("%d분", mm)
}

// classify → (level, action, reason). level: red|green|wait|yellow|white, 판정 불가면 "".
func classify(ws []classWin) (string, string, string) {
	seven := pickWin(ws, 10080)
	short := pickWin(ws, 300)
	if short == nil {
		short = pickWin(ws, 1440)
	}
	pct := func(f float64) int { return int(math.Round(f * 100)) }
	if short == nil {
		if seven == nil {
			return "", "데이터 부족", "표시할 한도 윈도우가 없어요."
		}
		wp := pct(seven.left)
		if seven.left < seven.timeLeft-paceMargin {
			return "red", "큰 작업은 미루세요", fmt.Sprintf("주간 한도가 빠듯해서(남은 %d%%) 지금 크게 쓰면 주 후반에 막혀요. 꼭 필요한 것만 돌리세요.", wp)
		}
		if seven.left >= tBig {
			return "green", "큰 작업 돌려도 돼요", fmt.Sprintf("주간 한도가 %d%% 남아 페이스에 여유 있어요. 평소보다 크게 써도 괜찮아요.", wp)
		}
		return "white", "평소대로 쓰세요", fmt.Sprintf("주간 한도 %d%% 남았고 페이스에 맞게 쓰는 중이라 여유 있어요.", wp)
	}
	sp := pct(short.left)
	if seven != nil && seven.left < seven.timeLeft-paceMargin {
		return "red", "큰 작업은 미루세요", fmt.Sprintf("7일 한도가 가장 빠듯해서(남은 %d%%) 지금 크게 쓰면 주 후반에 막혀요. 꼭 필요한 작은 것만 돌리세요.", pct(seven.left))
	}
	if short.left >= tBig {
		longOK := ""
		if seven != nil {
			longOK = " 장기도 여유라"
		}
		return "green", "지금 큰 작업 돌리세요", fmt.Sprintf("단기 한도 %d%% 남고%s 끊길 걱정 없어요. 남기면 초기화로 사라지니 지금이 적기예요.", sp, longOK)
	}
	if short.mins <= tSoon {
		return "wait", "조금만 기다렸다 큰 작업", fmt.Sprintf("단기 한도가 %d%%뿐이지만 약 %s 뒤 초기화되면 풀로 쓸 수 있어요.", sp, durText(short.mins))
	}
	if short.left < short.timeLeft-paceMargin {
		return "yellow", "작은 작업으로 돌리세요", fmt.Sprintf("단기 한도가 %d%%라 큰 작업은 중간에 끊겨요. 초기화까지 %s 남았어요.", sp, durText(short.mins))
	}
	return "white", "평소대로 쓰세요", fmt.Sprintf("단기·장기 모두 페이스에 맞게(단기 %d%%) 쓰는 중이라 여유 있어요. 아주 큰 작업만 한 번 더 확인하세요.", sp)
}
