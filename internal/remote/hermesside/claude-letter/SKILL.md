---
name: claude-letter
description: "Use when the user asks to tell, send, forward, request or ask something to Claude (클로드, Claude Code, the Claude desktop app session, 'Claude에게/클로드한테 전해/물어봐/부탁해'), or asks whether Claude replied / what Claude answered."
version: 1.0.0
author: netwaif
license: MIT
metadata:
  hermes:
    tags: [claude, agentlayer, kanban, letter, 편지, 클로드]
---

# Claude에게 편지 보내기

사용자의 Claude 데스크톱 앱 세션은 이 Hermes의 칸반을 우체통으로 쓴다. 담당자 `{{MAILBOX}}` 앞으로 카드를 만들면 Claude 쪽(`agentlayer inbox wait --remote … --app-mailbox`)이 가져가고, Claude의 답은 이 대화에 알림으로 온다.

## 보내기 (한 번만 실행)

터미널에서:

```bash
claude-letter "제목" "본문"
```

- 카드 생성과 "답 오면 이 대화에 알려줘" 구독을 한꺼번에 한다. 출력의 카드 ID(`t_…`)를 사용자에게 알린다.
- 출력에 "구독 안 됨"이 있으면 `hermes kanban notify-subscribe <카드ID> --platform discord --chat-id <이 스레드 ID> --chat-type thread --thread-id <이 스레드 ID>`로 직접 구독한다.
- 사용자가 전해 달라고 한 말을 본문에 그대로 담는다. 파일을 함께 보내야 하면 서버 경로를 본문에 적는다(첨부 업로드는 Claude → Hermes 방향만 있다).

## 답장 받기

- Claude가 답하면 이 대화에 `✔ … Kanban <카드ID> done — <제목> / <답 첫 줄>`이 뜬다. 전문은 `hermes kanban show <카드ID>`의 `result`.
- 답에 "첨부: <경로>"가 있으면 파일은 그 경로(`{{ATTACH_ROOT}}/<카드ID>/`)에 있다. `hermes kanban attachments <카드ID>`로도 본다. 사용자가 원하는 폴더로 옮겨 준다.

## 하지 말 것

- 회사 총괄에게 보내는 `company-letter`와 다르다 — Claude(클로드)에게 전할 때만 이 스킬이다.
- 이메일·브리지·MCP 같은 다른 경로를 제안하지 않는다.
- 카드를 직접 `complete`하지 않는다(닫는 것은 Claude의 답장이다).
