package wt

import (
	"regexp"
	"time"
)

// 에이전트 기동 직후 뜨는 "이 폴더를 신뢰합니까?" 류의 질문을 화면에서 보고 Enter로 넘긴다.
// 화면 파싱은 상태 판정에 쓰지 않는다는 원칙(2026-08-25)의 예외 — 판정이 아니라 기동
// 핸드셰이크이고, 창을 만든 직후 짧은 시간 동안만 본다. 설정 파일 사전 등록은 CLI마다
// 다르고(codex는 저장소 루트별, agy는 위치 불명) 새 저장소마다 다시 묻기 때문에 이 쪽이
// 3사 공통이다. worker에 질문이 남아 있으면 코디네이터가 보낸 지시가 그 질문에 먹힌다.
var (
	trustWordRe  = regexp.MustCompile(`(?i)trust|신뢰|승인`)
	folderWordRe = regexp.MustCompile(`(?i)folder|director|files|workspace|폴더|파일|디렉터리`)
)

// IsTrustPrompt는 pane 화면(마지막 수십 줄)에 폴더 신뢰 질문이 떠 있는지 — 신뢰 어휘와
// 폴더 어휘가 함께 있으면(어순 무관: 영어는 trust→folder, 한국어는 폴더→신뢰).
func IsTrustPrompt(screen string) bool {
	return trustWordRe.MatchString(screen) && folderWordRe.MatchString(screen)
}

// AcceptStartupPrompts는 timeout 동안 interval마다 화면을 보고, 신뢰 질문이 보이면
// Enter(기본 선택 = 예)를 보낸다. 질문이 사라지면 끝. 누른 횟수를 돌려준다.
// capture·enter는 tmux 주입점(테스트에서는 가짜).
func AcceptStartupPrompts(capture func(paneID string, lines int) (string, error), enter func(paneID string) error,
	paneID string, timeout, interval time.Duration, sleep func(time.Duration)) int {
	deadline := time.Now().Add(timeout)
	pressed := 0
	for time.Now().Before(deadline) {
		screen, err := capture(paneID, 40)
		if err != nil {
			return pressed // pane이 사라짐
		}
		if IsTrustPrompt(screen) {
			if enter(paneID) == nil {
				pressed++
			}
			if pressed >= 3 {
				return pressed // 같은 질문이 계속 남아 있으면 우리가 아는 형태가 아니다 — 포기
			}
			sleep(1500 * time.Millisecond) // 화면이 바뀔 시간
			continue
		}
		if pressed > 0 {
			return pressed // 질문이 사라졌다
		}
		sleep(interval)
	}
	return pressed
}
