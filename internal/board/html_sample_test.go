package board

import (
	"os"
	"strings"
	"testing"
	"time"
)

// 눈으로 보는 표본 — BOARD_SAMPLE_OUT=<경로>를 주면 6열이 다 찬 보드를 그 파일에 쓴다.
// 평소 go test에선 건너뛴다(디자인 확인용, 회귀 검사는 html_test.go).
func TestWriteSampleBoard(t *testing.T) {
	out := os.Getenv("BOARD_SAMPLE_OUT")
	if out == "" {
		t.Skip("BOARD_SAMPLE_OUT 없음")
	}
	body := "## 목표\n협업 제안 3건을 골라 초안을 쓴다.\n\n## 담당·순서\n- 1단계: 비즈니스운영팀 / 사업운영 매니저(collab-bot)\n- 2단계: 콘텐츠전략팀 / 콘텐츠 PD(search-youtube-bot)\n\n## 완료 기준\n- [x] 후보 10건 수집\n- [ ] 3건 선정 사유 정리\n- [ ] 결과물/PROP-3/초안.md 작성\n\n```\n참고: 참고자료/제안서-양식.md\n```"
	logs := []string{
		"[2026-09-16 09:12] [DECISION] 후보 수집 범위를 최근 30일로 한정",
		"[2026-09-16 09:14] [ASSIGN] collab-bot:t415938",
		"[2026-09-16 09:14] [SEND] # PROP-3 — 협업 제안 초안⏎담당: 비즈니스운영팀…⏎⏎## 목표⏎3건 선정 (412자)",
		"[2026-09-16 09:31] [ASK] 후보 중 유료 협업도 포함할까요?",
		"[2026-09-16 09:40] [SEND] 포함하되 별도 표기",
		"[2026-09-16 10:02] [REPORT] DONE: 초안 3건 작성 완료",
	}
	cards := []Card{
		{ID: "MAN-7", Title: "매뉴얼 v2 배포 (설치 안내 갱신 후)", Status: "pending", Column: ColTodo, Parents: []string{"MAN-6"}, Updated: now.Add(-3 * time.Hour), Body: body, Log: logs[:1]},
		{ID: "MAN-6", Title: "설치 안내 갱신", Status: "pending", Column: ColReady, Ready: now.Add(-52 * time.Minute), Updated: now.Add(-52 * time.Minute), Body: body, Log: logs[:1]},
		{ID: "YT-12", Title: "이번 주 구독 채널 인기 영상 10선", Status: "pending", Column: ColReady, Ready: now.Add(-6 * time.Minute), Updated: now.Add(-6 * time.Minute), Parents: []string{"YT-11"}, Body: body},
		{ID: "PROP-3", Title: "협업 제안 3건 초안", Status: "in_progress", Column: ColRunning, Session: "collab-bot:t415938", State: "WORK", Updated: now.Add(-4 * time.Minute), LastLog: logs[4], Body: body, Log: logs[:5]},
		{ID: "COM-2", Title: "코칭방 이번 달 멤버 질문 정리", Status: "waiting_haendaechacne-bot", Column: ColBlocked, Session: "haendaechacne-bot:t201133", State: "WAIT", Updated: now.Add(-41 * time.Minute), LastLog: "[2026-09-16 08:50] [ASK] 질문 원문을 그대로 실어도 될까요?", Body: body, Log: logs[:4]},
		{ID: "VIS-1", Title: "썸네일 3안", Status: "reviewing", Column: ColReview, Session: "codex-live", State: "DONE", Updated: now.Add(-12 * time.Minute), LastLog: logs[5], Body: body, Log: logs},
		{ID: "LAB-1", Title: "수신·응답 경로 점검(도구 없이 OK 응답)", Status: "done", Column: ColDone, Updated: now.Add(-25 * time.Hour), LastLog: "[2026-09-16 09:30] [COMPLETE] task done — 결과물/LAB-1/ 에 기록 복사.", Body: body, Log: logs},
		{ID: "YT-11", Title: "채널 다이제스트 노트북 갱신", Status: "done", Column: ColDone, Updated: now.Add(-2 * 24 * time.Hour), LastLog: "[2026-09-14 18:00] [COMPLETE] 채널 다이제스트 노트북 갱신", Body: body, Log: logs},
		{ID: "ZZ-0", Title: "상태값 이상", Status: "weird", Column: ColTodo, Unknown: true, Updated: now.Add(-30 * time.Hour)},
	}
	h := HTML("AI 치트키 회사", cards, now, 30*time.Minute)
	if !strings.Contains(string(h), "PROP-3") {
		t.Fatal("표본 렌더 실패")
	}
	if err := os.WriteFile(out, h, 0o644); err != nil {
		t.Fatal(err)
	}
}
