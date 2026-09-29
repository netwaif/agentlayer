---
name: company-letter
description: "Use when the user asks to send a letter/message/request to the company manager (회사 총괄, agentlayer AI 회사), or asks how to send one, or asks about a reply/attachment from the company manager."
version: 1.1.0
author: netwaif
license: MIT
metadata:
  hermes:
    tags: [company, agentlayer, kanban, letter, 편지, 총괄]
---

# 회사 총괄에게 편지 보내기

agentlayer AI 회사의 총괄은 이 Hermes의 칸반을 우체통으로 쓴다. 담당자 `{{MAILBOX}}` 앞으로 카드를 만들면 총괄 쪽이 30초 안에 가져가고, 총괄의 답은 이 대화에 알림으로 온다.

## 보내기 (한 번만 실행)

터미널에서:

```bash
company-letter "제목" "본문" [업무ID]
```

- 카드 생성과 "답 오면 이 대화에 알려줘" 구독을 한꺼번에 한다. 출력의 카드 ID(`t_…`)를 사용자에게 알린다.
- 출력에 "구독 안 됨"이 있으면 `hermes kanban notify-subscribe <카드ID> --platform discord --chat-id <이 스레드 ID> --chat-type thread --thread-id <이 스레드 ID>`로 직접 구독한다.
- 업무ID는 회사 쪽 업무와 이어질 때만(예: `VIDEO-07`). 모르면 생략.

## 답장 받기

- 총괄이 답하면 이 대화에 `✔ … Kanban <카드ID> done — <제목> / <답 첫 줄>`이 뜬다. 전문은 `hermes kanban show <카드ID>`의 `result`.
- 답에 "첨부: <경로>"가 있으면 파일은 그 경로(`{{ATTACH_ROOT}}/<카드ID>/`)에 있다. `hermes kanban attachments <카드ID>`로도 본다. 사용자가 원하는 폴더로 옮겨 준다.

## 하지 말 것

- 이메일·브리지·MCP 같은 다른 경로를 제안하지 않는다. 회사 총괄과의 편지는 이 스킬 하나다.
- 카드를 직접 `complete`하지 않는다(닫는 것은 총괄의 답장이다).
