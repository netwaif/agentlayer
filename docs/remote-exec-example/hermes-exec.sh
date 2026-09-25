#!/bin/sh
# docs/remote-exec-example/hermes-exec.sh — exec 어댑터 규격으로 Hermes 칸반 CLI를 감싼 예시.
# 내장 hermes 어댑터가 하는 일을 셸로 보여 준다: 다른 실행기(OpenClaw 등)는 이 네 동사만 같은 규격으로 구현하면 된다.
# 환경변수: HERMES_SSH(기본 hostinger), HERMES_EXEC(기본 "docker exec -i -u hermes hermes-agent-iqxn-hermes-agent-1"),
#           HERMES_PROFILE(기본 tech-qa), HERMES_WS(기본 /opt/data/ai-company/결과물)
set -eu
SSH_HOST=${HERMES_SSH:-hostinger}
EXEC=${HERMES_EXEC:-"docker exec -i -u hermes hermes-agent-iqxn-hermes-agent-1"}
PROFILE=${HERMES_PROFILE:-tech-qa}
WS=${HERMES_WS:-/opt/data/ai-company/결과물}
q() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }
remote() { ssh -o BatchMode=yes "$SSH_HOST" "$EXEC $*"; }

case "$1" in
  dispatch)
    task=$2; title=$3; body=$(cat "$4"); parent=$5
    remote mkdir -p "$(q "$WS/$task")" >/dev/null
    extra=""; [ -n "$parent" ] && extra="--parent $(q "$parent")"
    id=$(remote hermes kanban create "$(q "$task $title")" --assignee "$(q "$PROFILE")" --idempotency-key "$(q "agentlayer:$task:$parent")" \
         --created-by agentlayer --workspace "$(q "dir:$WS/$task")" --max-runtime 2h $extra --body "$(q "$body")" --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
    remote hermes kanban dispatch --max 3 --json | python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if any(s['task_id']=='$id' for s in d['spawned']) else 1)"
    printf '{"handle":"%s"}\n' "$id" ;;
  poll)
    remote hermes kanban show "$(q "$2")" --json | python3 -c '
import json,sys
c=json.load(sys.stdin); t=c["task"]; st=t["status"]
m={"running":"working","scheduled":"working","blocked":"waiting","done":"done","crashed":"error","timed_out":"error","gave_up":"error"}
out={"status":m.get(st,"idle"),"summary":(c.get("latest_summary") or t.get("result") or "").split("\n")[0][:120],"ask":"","error":"","seen":(c["events"][-1]["created_at"] if c["events"] else 0)}
if st=="blocked":
    ev=[e for e in c["events"] if e["kind"]=="blocked"]
    out["ask"]=(ev[-1]["payload"] or {}).get("reason","") if ev else "입력 대기"
print(json.dumps(out, ensure_ascii=False))' ;;
  reply)
    remote hermes kanban unblock "$(q "$2")" --reason "$(q "$(cat "$3")")" >/dev/null
    remote hermes kanban dispatch --max 3 --json >/dev/null ;;
  finish)
    remote hermes kanban archive "$(q "$2")" >/dev/null ;;
  *) echo "usage: $0 dispatch|poll|reply|finish …" >&2; exit 2 ;;
esac
