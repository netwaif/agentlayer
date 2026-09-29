#!/bin/sh
# company-letter — 회사 총괄(agentlayer AI 회사)에게 편지 보내기. agentlayer가 설치한다(remote setup).
# 사용: company-letter "제목" "본문" [업무ID]
# 편지 카드(담당 {{MAILBOX}})를 만들고, 지금 대화를 구독해 총괄의 답장이 이 대화에 뜨게 한다.
set -eu
if [ "${1:-}" = "" ] || [ "${1:-}" = "--help" ] || [ "${2:-}" = "" ]; then
  echo "usage: company-letter \"제목\" \"본문\" [업무ID]" >&2; exit 2
fi
title=$1; body=$2; task=${3:-}
[ -n "$task" ] && title="[$task] $title"
id=$(hermes kanban create "$title" --assignee {{MAILBOX}} "--body=$body" --created-by "${HERMES_PROFILE:-default}" --json \
     | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
chat=${HERMES_SESSION_CHAT_ID:-}
if [ -n "$chat" ]; then
  set -- --platform "${HERMES_SESSION_PLATFORM:-discord}" --chat-id "$chat"
  [ -n "${HERMES_SESSION_CHAT_TYPE:-}" ] && set -- "$@" --chat-type "$HERMES_SESSION_CHAT_TYPE"
  [ -n "${HERMES_SESSION_THREAD_ID:-}" ] && set -- "$@" --thread-id "$HERMES_SESSION_THREAD_ID"
  [ -n "${HERMES_SESSION_USER_ID:-}" ] && set -- "$@" --user-id "$HERMES_SESSION_USER_ID"
  hermes kanban notify-subscribe "$id" "$@" >/dev/null && sub="이 대화에 답장 알림 구독됨" || sub="구독 실패"
else
  sub="구독 안 됨(HERMES_SESSION_CHAT_ID 없음 — 이 카드를 notify-subscribe로 직접 구독)"
fi
echo "편지 $id 전송(회사 총괄 앞). $sub"
