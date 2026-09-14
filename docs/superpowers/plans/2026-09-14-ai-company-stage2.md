# AI 회사 2단계 — `netwaif/ai-company` 플러그인 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** "AI 회사 만들어줘" 한마디로 회사 루트·규칙·직원명부·총괄 절차를 생성하고, 기존 folder-bot 봇을 직원으로 등록하며, 점검·제거까지 되는 Claude 플러그인을 새 공개 레포 `netwaif/ai-company`로 만든다.

**Architecture:** folder-bot과 같은 구조 — 스킬 `configure-company`(SKILL.md, 절차 안내)가 결정적·멱등 엔진 `companyctl.py`(Python 3 표준 라이브러리만)를 호출한다. 정본은 `<회사 루트>/직원명부.json` 하나. 파일 조작은 전부 엔진이 하고, 지침 파일에는 마커 블록(`<!-- store:ai-company:start/end -->`)만 추가·제거한다. 총괄 절차는 회사 루트 CLAUDE.md의 마커 블록으로 들어가며 agentlayer v1.5.0의 `send`·`task` 명령과 folder-bot의 `bot-thread`를 호출한다.

**Tech Stack:** Python 3.9+(stdlib: argparse·json·pathlib·subprocess·shutil), pytest, Claude Code plugin(marketplace.json + plugin.json + skills/), MIT.

**Spec:** `docs/superpowers/specs/2026-09-14-ai-company-design.md` §2 (+ 아래 정정 1건)

**스펙 정정 (이 계획에서 확정):** §2.3의 "tasks/<업무ID>/ … agentlayer MultiAgent 패널에 표시"는 사실이 아니다 — agentlayer `internal/starter.DefaultRoot()`는 `~/VSCodeWorkspace/MultiAgent/tasks` 고정 경로만 읽는다. tasks/ 형식(task.md의 ```yaml status:)은 mat·starter 호환을 위해 유지하되 패널 표시는 약속하지 않는다(후속 후보: agentlayer `multiagent_roots` 설정). 계획 Task 7에서 스펙 문구를 정정한다.

## 3엔진 × 3OS 표 (사용자 규칙 2026-09-11)

| | macOS | Linux(VPS·컨테이너) | Windows(WSL2) |
|---|---|---|---|
| 총괄 = Claude Code | ○ 스킬·Monitor·SendMessage | ○ (agentlayer 리눅스 빌드) | ○ |
| 총괄 = Codex | × 범위 밖 — 총괄 절차가 Claude 도구(Monitor·AskUserQuestion·SendMessage)에 의존 | × | × |
| 총괄 = Gemini(agy) | × 동일 사유 | × | × |
| 직원 = Claude(folder-bot) | ○ 스레드 배정 | ○ | ○ |
| 직원 = Codex(브리지) | ○ 세션 직접 전달(스레드 배정은 후속) | ○ | ○ |
| 직원 = Gemini/agy(브리지) | ○ 세션 직접 전달 | ○ | ○ |

엔진 판정은 실물로 검증: `bot-thread`(folder-bot 0.1.9+ claude 엔진 전용 스레드 창), `agentlayer send`(3사 공통, `tmuxx.SendText`), codex/agy 지침 파일 위치는 folder-bot `_directive_target`과 동일(AGENTS.md / `.agents/rules/*.md`).

## Global Constraints

- 정본은 `<root>/직원명부.json` 하나. 전역 설정 파일 없음. 모든 명령은 `--root <회사 루트>`(기본 `.`)를 받는다.
- 멱등: 같은 명령을 몇 번 실행해도 결과 동일. 기존 파일은 마커 블록 추가 외에 수정하지 않는다. `SESSION.md`는 **없을 때만** 템플릿에서 생성하고 있으면 절대 건드리지 않는다.
- 마커: `MARK_START = "<!-- store:ai-company:start -->"`, `MARK_END = "<!-- store:ai-company:end -->"`. 블록이 이미 있으면 안쪽만 갱신(블록 밖 diff 0). 제거는 원문 복원, 블록만 있던 파일은 삭제.
- 엔진별 지침 파일: claude → `CLAUDE.md`, codex → `AGENTS.md`, agy(gemini) → `.agents/rules/ai-company.md`(헤더 `---\ntrigger: always_on\n---\n`).
- 비밀값(토큰) 절대 읽지도 쓰지도 않는다. `.discord-state/.env`는 열지 않는다. 채널 ID는 `.discord-state/access.json`의 `groups` 키에서만 읽는다.
- 외부 도구 호출은 `doctor`에서만(읽기 전용: `agentlayer version`, `agentlayer task list --json`, `command -v bot-thread bot-up tmux`). 테스트는 PATH 앞에 가짜 바이너리를 두어 실호출을 막는다.
- 사용자 문구·파일 내용은 한국어. 부서 기본값 9개: 경영기획실, 콘텐츠전략팀, 기술개발팀, 크리에이티브팀, 기술검증팀, 교육자료팀, 채널그로스팀, 커뮤니티·멤버십팀, 비즈니스운영팀.
- 직원 `mode`: `bot`(기존 folder-bot/브리지 봇), `folder`(새 폴더, 봇은 configure-bot으로 나중에), `on-demand`(호출형, 폴더·봇 없음). `tool`: `claude|codex|gemini`.
- 커밋 접두 `feat:`·`fix:`·`docs:`·`chore:`; 마지막에 attribution 두 줄.
- 레포: 로컬 `~/VSCodeWorkspace/ai-company`, GitHub `netwaif/ai-company`(public, MIT). 플러그인 이름 `ai-company`, 스킬 이름 `configure-company`, 첫 버전 0.1.0.

---

## File Structure

```
ai-company/
├── .claude-plugin/marketplace.json
├── .gitignore  LICENSE  README.md  CLAUDE.md
├── plugins/ai-company/
│   ├── .claude-plugin/plugin.json
│   └── skills/configure-company/
│       ├── SKILL.md                          스킬 절차(설치·직원 추가·점검·제거)
│       ├── generator/companyctl.py           결정적 엔진(단일 파일, 표준 라이브러리)
│       └── assets/
│           ├── company-block.md              회사 루트 CLAUDE.md 마커 본문(공동 규칙 + 총괄 절차), {ROOT}·{INBOX} 치환
│           ├── employee-block.md             직원 지침 마커 본문, {DEPT}·{NAME}·{ROOT} 치환
│           ├── SESSION.template.md           loadout 세션 이어가기 5절
│           ├── task.md  업무요청.md  log.md    tasks/<업무ID>/·업무요청/ 템플릿
└── tests/
    ├── conftest.py                           HOME 격리 + PATH 가짜 바이너리
    └── test_companyctl.py
```

| 파일 | 책임 |
|---|---|
| `generator/companyctl.py` | 명부 로드/저장, init/dept/employee/install/remove/doctor/list |
| `assets/*.md` | 사람이 읽는 본문 전부 — 엔진은 치환만 |
| `SKILL.md` | 질문·순서·수동 단계 안내, 엔진 호출 명령 |
| `tests/*` | 엔진 동작 검증(실호출 0) |

---

### Task 1: 레포 뼈대 + 테스트 하네스

**Files:**
- Create: `~/VSCodeWorkspace/ai-company/.claude-plugin/marketplace.json`, `plugins/ai-company/.claude-plugin/plugin.json`, `.gitignore`, `LICENSE`, `CLAUDE.md`, `README.md`(뼈대), `tests/conftest.py`, `tests/test_companyctl.py`(스모크 1개), `plugins/ai-company/skills/configure-company/generator/companyctl.py`(`--help`만)

**Interfaces:**
- Produces: `tests/conftest.py`의 픽스처 `env`(dict: HOME=tmp, PATH=shim 우선) 와 헬퍼 `run(env, *args) -> CompletedProcess`; 가짜 바이너리 `agentlayer`(`version` → `agentlayer v1.5.0 (commit x, 2026-09-14)`, `task list --json` → `[]`), `bot-thread`, `bot-up`, `tmux`(모두 exit 0).

- [ ] **Step 1: 디렉터리·git 초기화**

```bash
mkdir -p ~/VSCodeWorkspace/ai-company && cd ~/VSCodeWorkspace/ai-company && git init -q -b main
mkdir -p .claude-plugin plugins/ai-company/.claude-plugin plugins/ai-company/skills/configure-company/{generator,assets} tests
cp ~/VSCodeWorkspace/folder-bot/LICENSE LICENSE   # MIT, 저작권자 동일
printf '__pycache__/\n.pytest_cache/\n' > .gitignore
```

- [ ] **Step 2: 매니페스트**

`.claude-plugin/marketplace.json`:
```json
{
  "name": "ai-company",
  "description": "AI 회사 생성기 — 총괄 봇 하나와 기존 폴더 봇들을 부서·직원으로 묶어, 요청 한 번으로 사슬 업무(기획→제작→검증)가 돌게 한다. agentlayer 배관(send·task·훅 보고) 위에서 동작",
  "owner": { "name": "netwaif", "email": "netwaif@users.noreply.github.com" },
  "plugins": [
    {
      "name": "ai-company",
      "description": "\"AI 회사 만들어줘\" — 회사 폴더·공동 규칙·직원명부·총괄 절차를 결정적 엔진(companyctl)이 멱등 설치. 직원은 기존 folder-bot 봇 등록 / 새 폴더 신설 / 호출형 세 모드. 수동은 디스코드 포탈(총괄 봇)뿐.",
      "version": "0.1.0",
      "source": "./plugins/ai-company",
      "author": { "name": "netwaif" }
    }
  ]
}
```

`plugins/ai-company/.claude-plugin/plugin.json`:
```json
{
  "name": "ai-company",
  "version": "0.1.0",
  "description": "AI 회사 생성기. 직원명부(직원명부.json) 정본으로 회사 루트·공동 규칙·총괄 절차·직원 지침 블록을 멱등 관리. agentlayer ≥1.5.0(send·task·훅 보고)과 folder-bot(봇·스레드) 위에서 동작한다.",
  "author": { "name": "netwaif" }
}
```

`CLAUDE.md`(레포 규칙, folder-bot과 동일 문구):
```markdown
## 기능 정의 = 3엔진 × 3OS (사용자 지시 2026-09-11)
기능 하나는 Claude Code·Codex·Gemini(agy)에서 같은 동작이어야 하고 맥·리눅스·윈도우(WSL2)에서 돌아가야 한다.
착수 전에 9칸 표를 먼저 채워 보고하고, 안 되는 칸은 그때 이유와 범위 포함 여부를 말한다. 완료 보고도 이 표로만 한다.
총괄 역할은 Claude Code 전용(Monitor·SendMessage 의존) — 이 예외는 사용자 승인(2026-09-14 스펙).
외부 도구 능력은 실물(파일·바이너리·도움말)로 검증한 뒤 말한다.

## 비파괴 원칙
SESSION.md는 없을 때만 템플릿에서 만들고 있으면 절대 수정하지 않는다. 지침 파일은 마커 블록만 추가·제거한다. 토큰은 읽지도 쓰지도 않는다.
```

- [ ] **Step 3: 테스트 하네스**

`tests/conftest.py`:
```python
"""테스트 격리 — HOME을 임시 폴더로, PATH 앞에 가짜 agentlayer·bot-thread·bot-up·tmux.
실제 바이너리를 부르면 표식 파일이 남아 즉시 실패한다(folder-bot conftest 방식)."""
import os
import subprocess
import sys
from pathlib import Path

import pytest

COMPANYCTL = Path(__file__).parent.parent / "plugins/ai-company/skills/configure-company/generator/companyctl.py"

FAKES = {
    "agentlayer": '#!/bin/sh\ncase "$1" in\n  version) echo "agentlayer v1.5.0 (commit abc1234, 2026-09-14)";;\n  task) echo "[]";;\n  *) echo "fake agentlayer $*";;\nesac\n',
    "bot-thread": '#!/bin/sh\necho "fake bot-thread $*"\n',
    "bot-up": '#!/bin/sh\necho "fake bot-up $*"\n',
    "tmux": '#!/bin/sh\necho "fake tmux $*"\n',
}


@pytest.fixture
def env(tmp_path):
    home = tmp_path / "home"
    home.mkdir()
    shim = tmp_path / "shim"
    shim.mkdir()
    for name, body in FAKES.items():
        p = shim / name
        p.write_text(body)
        p.chmod(0o755)
    return {"HOME": str(home), "PATH": f"{shim}:{os.environ.get('PATH', '')}",
            "LANG": "en_US.UTF-8", "LC_ALL": "en_US.UTF-8"}


def run(env, *args):
    return subprocess.run([sys.executable, str(COMPANYCTL), *args], capture_output=True, text=True, env=env)


def write_bots_json(env, bots: dict):
    cfg = Path(env["HOME"]) / ".config/folder-bot"
    cfg.mkdir(parents=True, exist_ok=True)
    (cfg / "bots.json").write_text(__import__("json").dumps(bots, ensure_ascii=False))


def write_access(folder: Path, channel_ids):
    st = folder / ".discord-state"
    st.mkdir(parents=True, exist_ok=True)
    groups = {cid: {"requireMention": False, "allowFrom": []} for cid in channel_ids}
    (st / "access.json").write_text(__import__("json").dumps({"dmPolicy": "allowlist", "allowFrom": [], "groups": groups, "pending": {}}))
```

`tests/test_companyctl.py`(스모크):
```python
from conftest import run


def test_help_runs(env):
    r = run(env, "--help")
    assert r.returncode == 0
    assert "init" in r.stdout and "doctor" in r.stdout
```

`generator/companyctl.py`(뼈대):
```python
#!/usr/bin/env python3
"""companyctl — AI 회사 생성기의 결정적 엔진.
정본은 <root>/직원명부.json 하나. 지침 파일은 마커 블록만 추가·제거하고 SESSION.md는 없을 때만 만든다.
비밀값은 읽지도 쓰지도 않는다. 표준 라이브러리만 쓴다.
"""
import argparse
import sys


def main() -> None:
    p = argparse.ArgumentParser(prog="companyctl", description=__doc__)
    sub = p.add_subparsers(dest="cmd", required=True)
    for name, help_ in [("init", "회사 루트·직원명부 생성"), ("dept", "부서 추가/제거"), ("employee", "직원 등록/제거"),
                        ("install", "폴더·템플릿·지침 블록 설치"), ("remove", "지침 블록 제거(기록 보존)"),
                        ("doctor", "읽기 전용 점검"), ("list", "직원명부 출력")]:
        sub.add_parser(name, help=help_)
    a = p.parse_args()
    print("아직 구현되지 않은 명령:", a.cmd, file=sys.stderr)
    sys.exit(2)


if __name__ == "__main__":
    main()
```

- [ ] **Step 4: 실행**

Run: `cd ~/VSCodeWorkspace/ai-company && python3 -m pytest -q`
Expected: `1 passed`

- [ ] **Step 5: 커밋**

```bash
git add -A && git commit -m "chore: ai-company 플러그인 뼈대 — 매니페스트·라이선스·테스트 하네스"
```

---

### Task 2: 명부 정본 — init · dept · list

**Files:**
- Modify: `generator/companyctl.py`
- Test: `tests/test_companyctl.py`

**Interfaces:**
- Produces:
  ```python
  ROSTER = "직원명부.json"
  DEFAULT_DEPTS = ["경영기획실", "콘텐츠전략팀", "기술개발팀", "크리에이티브팀", "기술검증팀", "교육자료팀", "채널그로스팀", "커뮤니티·멤버십팀", "비즈니스운영팀"]
  def roster_path(root: Path) -> Path
  def load_roster(root: Path) -> dict          # 없으면 SystemExit("직원명부가 없습니다 — init 먼저")
  def save_roster(root: Path, r: dict) -> None  # temp+rename, ensure_ascii=False, indent 2
  # 명부 스키마: {"version":1,"name":str,"departments":[str],"employees":[{"dept","name","tool","mode","session","folder","bot","channel_id"}]}
  ```
  CLI: `init --root R [--name N] [--depts a,b]`, `dept add --root R --name D`, `dept remove --root R --name D`, `list --root R [--json]`

- [ ] **Step 1: 실패하는 테스트**

```python
import json
from pathlib import Path
from conftest import run


def test_init_creates_roster_and_dirs(env, tmp_path):
    root = tmp_path / "company"
    r = run(env, "init", "--root", str(root), "--name", "AI 치트키")
    assert r.returncode == 0, r.stderr
    roster = json.loads((root / "직원명부.json").read_text())
    assert roster["version"] == 1 and roster["name"] == "AI 치트키"
    assert roster["departments"][0] == "경영기획실" and len(roster["departments"]) == 9
    assert roster["employees"] == []
    for d in ["업무요청", "참고자료", "결과물", "docs", "tasks", "runtime/inbox/pending", "runtime/inbox/received", "runtime/inbox/quarantine"]:
        assert (root / d).is_dir(), d


def test_init_is_idempotent_and_keeps_employees(env, tmp_path):
    root = tmp_path / "c"
    run(env, "init", "--root", str(root), "--name", "회사")
    p = root / "직원명부.json"
    r = json.loads(p.read_text())
    r["employees"].append({"dept": "경영기획실", "name": "총괄", "tool": "claude", "mode": "bot", "session": "s", "folder": str(root), "bot": "company", "channel_id": "1"})
    p.write_text(json.dumps(r, ensure_ascii=False))
    assert run(env, "init", "--root", str(root)).returncode == 0
    assert len(json.loads(p.read_text())["employees"]) == 1
    assert json.loads(p.read_text())["name"] == "회사"


def test_init_custom_depts(env, tmp_path):
    root = tmp_path / "c"
    run(env, "init", "--root", str(root), "--depts", "경영기획실,학원운영팀")
    assert json.loads((root / "직원명부.json").read_text())["departments"] == ["경영기획실", "학원운영팀"]


def test_dept_add_remove_and_list(env, tmp_path):
    root = tmp_path / "c"
    run(env, "init", "--root", str(root))
    assert run(env, "dept", "add", "--root", str(root), "--name", "학원운영팀").returncode == 0
    assert run(env, "dept", "add", "--root", str(root), "--name", "학원운영팀").returncode == 0  # 멱등
    depts = json.loads((root / "직원명부.json").read_text())["departments"]
    assert depts.count("학원운영팀") == 1
    r = run(env, "list", "--root", str(root))
    assert "학원운영팀" in r.stdout and "경영기획실" in r.stdout
    assert run(env, "dept", "remove", "--root", str(root), "--name", "학원운영팀").returncode == 0
    assert "학원운영팀" not in json.loads((root / "직원명부.json").read_text())["departments"]
    r = run(env, "list", "--root", str(root), "--json")
    assert json.loads(r.stdout)["version"] == 1


def test_commands_require_init(env, tmp_path):
    r = run(env, "list", "--root", str(tmp_path / "nope"))
    assert r.returncode != 0 and "init" in r.stderr
```

- [ ] **Step 2: 실패 확인** — Run: `python3 -m pytest -q` → Expected: FAIL(미구현 명령 exit 2)

- [ ] **Step 3: 구현** (`companyctl.py`의 뼈대 main을 아래로 교체·확장)

```python
import argparse
import json
import os
import sys
from pathlib import Path

ROSTER = "직원명부.json"
DEFAULT_DEPTS = ["경영기획실", "콘텐츠전략팀", "기술개발팀", "크리에이티브팀", "기술검증팀",
                 "교육자료팀", "채널그로스팀", "커뮤니티·멤버십팀", "비즈니스운영팀"]
COMPANY_DIRS = ["업무요청", "참고자료", "결과물", "docs", "tasks",
                "runtime/inbox/pending", "runtime/inbox/received", "runtime/inbox/quarantine"]


def die(msg: str, code: int = 1) -> None:
    print(msg, file=sys.stderr)
    sys.exit(code)


def roster_path(root: Path) -> Path:
    return root / ROSTER


def load_roster(root: Path) -> dict:
    p = roster_path(root)
    if not p.exists():
        die(f"직원명부가 없습니다: {p} — 먼저 `companyctl init --root {root}`")
    return json.loads(p.read_text(encoding="utf-8"))


def save_roster(root: Path, r: dict) -> None:
    p = roster_path(root)
    tmp = p.with_name("." + p.name + ".tmp")
    tmp.write_text(json.dumps(r, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    os.replace(tmp, p)


def cmd_init(a) -> None:
    root = Path(a.root).expanduser().resolve()
    for d in COMPANY_DIRS:
        (root / d).mkdir(parents=True, exist_ok=True)
    p = roster_path(root)
    if p.exists():
        r = json.loads(p.read_text(encoding="utf-8"))
        changed = False
        if a.name and r.get("name") != a.name:
            r["name"] = a.name
            changed = True
        if changed:
            save_roster(root, r)
        print(f"직원명부 유지: {p} (직원 {len(r['employees'])}명, 부서 {len(r['departments'])}개)")
        return
    depts = [d.strip() for d in a.depts.split(",")] if a.depts else list(DEFAULT_DEPTS)
    r = {"version": 1, "name": a.name or "AI 회사", "departments": depts, "employees": []}
    save_roster(root, r)
    print(f"직원명부 생성: {p} (부서 {len(depts)}개)")


def cmd_dept(a) -> None:
    root = Path(a.root).expanduser().resolve()
    r = load_roster(root)
    if a.action == "add":
        if a.name not in r["departments"]:
            r["departments"].append(a.name)
            save_roster(root, r)
        print(f"부서: {', '.join(r['departments'])}")
    else:
        if any(e["dept"] == a.name for e in r["employees"]):
            die(f"부서 {a.name}에 직원이 있습니다 — 먼저 employee remove")
        r["departments"] = [d for d in r["departments"] if d != a.name]
        save_roster(root, r)
        print(f"부서: {', '.join(r['departments'])}")


def cmd_list(a) -> None:
    root = Path(a.root).expanduser().resolve()
    r = load_roster(root)
    if a.json:
        print(json.dumps(r, ensure_ascii=False, indent=2))
        return
    print(f"{r['name']} — {root}")
    for d in r["departments"]:
        emps = [e for e in r["employees"] if e["dept"] == d]
        if not emps:
            print(f"  {d}: (호출형/미배정)")
        for e in emps:
            where = e.get("session") or e.get("folder") or "-"
            print(f"  {d}: {e['name']} [{e['tool']}/{e['mode']}] {where}")


def main() -> None:
    p = argparse.ArgumentParser(prog="companyctl", description=__doc__)
    sub = p.add_subparsers(dest="cmd", required=True)

    ip = sub.add_parser("init", help="회사 루트·직원명부 생성")
    ip.add_argument("--root", default=".")
    ip.add_argument("--name")
    ip.add_argument("--depts", help="쉼표 구분 부서 목록(기본 9부서)")
    ip.set_defaults(fn=cmd_init)

    dp = sub.add_parser("dept", help="부서 추가/제거")
    dp.add_argument("action", choices=["add", "remove"])
    dp.add_argument("--root", default=".")
    dp.add_argument("--name", required=True)
    dp.set_defaults(fn=cmd_dept)

    lp = sub.add_parser("list", help="직원명부 출력")
    lp.add_argument("--root", default=".")
    lp.add_argument("--json", action="store_true")
    lp.set_defaults(fn=cmd_list)

    for name, help_ in [("employee", "직원 등록/제거"), ("install", "폴더·템플릿·지침 블록 설치"),
                        ("remove", "지침 블록 제거(기록 보존)"), ("doctor", "읽기 전용 점검")]:
        sub.add_parser(name, help=help_).set_defaults(fn=lambda a: die("아직 구현되지 않은 명령", 2))

    a = p.parse_args()
    a.fn(a)
```

- [ ] **Step 4: 통과 확인** — Run: `python3 -m pytest -q` → Expected: 모두 passed
- [ ] **Step 5: 커밋** — `git add -A && git commit -m "feat(companyctl): 직원명부 정본 — init·dept·list"`

---

### Task 3: 직원 등록 — employee add/remove (봇·폴더·호출형)

**Files:** Modify `generator/companyctl.py`; Test `tests/test_companyctl.py`

**Interfaces:**
- Consumes: `~/.config/folder-bot/bots.json`(`{name: {engine, folder, session, ...}}`), `<folder>/.discord-state/access.json`(`groups` 키 = 채널 ID)
- Produces: 직원 레코드 `{"dept","name","tool","mode","session","folder","bot","channel_id"}`; 함수 `bots_json_path() -> Path`, `read_bots() -> dict`, `channel_id_of(folder: Path) -> list[str]`, `tool_of_engine(engine) -> str`(agy→gemini)
- CLI: `employee add --root R --dept D --name N (--bot B | --folder F [--engine E] | --on-demand) [--channel-id C] [--replace]`, `employee remove --root R --name N`

- [ ] **Step 1: 실패하는 테스트**

```python
from conftest import write_bots_json, write_access


def _init(env, tmp_path):
    root = tmp_path / "c"
    run(env, "init", "--root", str(root))
    return root


def test_employee_add_from_bot(env, tmp_path):
    root = _init(env, tmp_path)
    folder = tmp_path / "work" / "collab"
    folder.mkdir(parents=True)
    write_bots_json(env, {"collab": {"engine": "claude", "folder": str(folder), "session": "collab-bot"}})
    write_access(folder, ["1542142326384754748"])
    r = run(env, "employee", "add", "--root", str(root), "--dept", "비즈니스운영팀", "--name", "사업운영 매니저", "--bot", "collab")
    assert r.returncode == 0, r.stderr
    e = json.loads((root / "직원명부.json").read_text())["employees"][0]
    assert e == {"dept": "비즈니스운영팀", "name": "사업운영 매니저", "tool": "claude", "mode": "bot",
                 "session": "collab-bot", "folder": str(folder), "bot": "collab", "channel_id": "1542142326384754748"}


def test_employee_add_bot_engine_agy_maps_to_gemini_and_needs_channel_when_ambiguous(env, tmp_path):
    root = _init(env, tmp_path)
    folder = tmp_path / "g"
    folder.mkdir()
    write_bots_json(env, {"community": {"engine": "agy", "folder": str(folder), "session": "community-agy"}})
    write_access(folder, ["100", "200"])
    r = run(env, "employee", "add", "--root", str(root), "--dept", "커뮤니티·멤버십팀", "--name", "시청자 지원", "--bot", "community")
    assert r.returncode != 0 and "--channel-id" in r.stderr
    r = run(env, "employee", "add", "--root", str(root), "--dept", "커뮤니티·멤버십팀", "--name", "시청자 지원", "--bot", "community", "--channel-id", "200")
    assert r.returncode == 0, r.stderr
    e = json.loads((root / "직원명부.json").read_text())["employees"][0]
    assert e["tool"] == "gemini" and e["channel_id"] == "200"


def test_employee_add_unknown_bot_or_dept_fails(env, tmp_path):
    root = _init(env, tmp_path)
    write_bots_json(env, {})
    r = run(env, "employee", "add", "--root", str(root), "--dept", "비즈니스운영팀", "--name", "x", "--bot", "nope")
    assert r.returncode != 0 and "bots.json" in r.stderr
    r = run(env, "employee", "add", "--root", str(root), "--dept", "없는팀", "--name", "x", "--on-demand")
    assert r.returncode != 0 and "부서" in r.stderr


def test_employee_add_folder_and_on_demand(env, tmp_path):
    root = _init(env, tmp_path)
    newf = tmp_path / "new-dept"
    r = run(env, "employee", "add", "--root", str(root), "--dept", "기술개발팀", "--name", "AI 개발자", "--folder", str(newf), "--engine", "codex")
    assert r.returncode == 0, r.stderr
    assert newf.is_dir()
    r = run(env, "employee", "add", "--root", str(root), "--dept", "기술검증팀", "--name", "QA 담당", "--on-demand")
    assert r.returncode == 0, r.stderr
    emps = json.loads((root / "직원명부.json").read_text())["employees"]
    assert emps[0]["mode"] == "folder" and emps[0]["tool"] == "codex" and emps[0]["session"] == ""
    assert emps[1] == {"dept": "기술검증팀", "name": "QA 담당", "tool": "claude", "mode": "on-demand", "session": "", "folder": "", "bot": "", "channel_id": ""}


def test_employee_add_duplicate_needs_replace_and_remove(env, tmp_path):
    root = _init(env, tmp_path)
    run(env, "employee", "add", "--root", str(root), "--dept", "기술검증팀", "--name", "QA 담당", "--on-demand")
    r = run(env, "employee", "add", "--root", str(root), "--dept", "기술검증팀", "--name", "QA 담당", "--on-demand")
    assert r.returncode != 0 and "--replace" in r.stderr
    r = run(env, "employee", "add", "--root", str(root), "--dept", "채널그로스팀", "--name", "QA 담당", "--on-demand", "--replace")
    assert r.returncode == 0
    emps = json.loads((root / "직원명부.json").read_text())["employees"]
    assert len(emps) == 1 and emps[0]["dept"] == "채널그로스팀"
    assert run(env, "employee", "remove", "--root", str(root), "--name", "QA 담당").returncode == 0
    assert json.loads((root / "직원명부.json").read_text())["employees"] == []
    assert run(env, "employee", "remove", "--root", str(root), "--name", "QA 담당").returncode != 0
```

- [ ] **Step 2: 실패 확인** — `python3 -m pytest -q` → FAIL

- [ ] **Step 3: 구현** (Task 2 코드 뒤에 추가, main의 employee 자리를 교체)

```python
def bots_json_path() -> Path:
    return Path.home() / ".config" / "folder-bot" / "bots.json"


def read_bots() -> dict:
    p = bots_json_path()
    if not p.exists():
        return {}
    return json.loads(p.read_text(encoding="utf-8"))


def channel_id_of(folder: Path) -> list:
    p = folder / ".discord-state" / "access.json"
    if not p.exists():
        return []
    try:
        return list(json.loads(p.read_text(encoding="utf-8")).get("groups", {}).keys())
    except (ValueError, AttributeError):
        return []


def tool_of_engine(engine: str) -> str:
    return {"claude": "claude", "codex": "codex", "agy": "gemini", "gemini": "gemini"}.get(engine, engine)


def cmd_employee(a) -> None:
    root = Path(a.root).expanduser().resolve()
    r = load_roster(root)
    if a.action == "remove":
        before = len(r["employees"])
        r["employees"] = [e for e in r["employees"] if e["name"] != a.name]
        if len(r["employees"]) == before:
            die(f"직원 {a.name}이 명부에 없습니다")
        save_roster(root, r)
        print(f"직원 제거: {a.name}")
        return
    if a.dept not in r["departments"]:
        die(f"부서 {a.dept}이 명부에 없습니다 — `companyctl dept add --name {a.dept}` 먼저")
    if sum(map(bool, [a.bot, a.folder, a.on_demand])) != 1:
        die("--bot <이름> | --folder <폴더> | --on-demand 중 하나만 지정")
    exists = [e for e in r["employees"] if e["name"] == a.name]
    if exists and not a.replace:
        die(f"직원 {a.name}이 이미 있습니다 (--replace로 교체)")
    e = {"dept": a.dept, "name": a.name, "tool": "claude", "mode": "on-demand",
         "session": "", "folder": "", "bot": "", "channel_id": ""}
    if a.bot:
        bots = read_bots()
        if a.bot not in bots:
            die(f"folder-bot bots.json에 {a.bot} 봇이 없습니다: {bots_json_path()}")
        b = bots[a.bot]
        folder = Path(b["folder"])
        ids = channel_id_of(folder)
        cid = a.channel_id or (ids[0] if len(ids) == 1 else "")
        if not cid:
            die(f"채널 ID를 정할 수 없습니다(access.json groups: {ids}) — --channel-id <ID>로 지정")
        e.update(tool=tool_of_engine(b.get("engine", "claude")), mode="bot", session=b.get("session", ""),
                 folder=str(folder), bot=a.bot, channel_id=cid)
    elif a.folder:
        folder = Path(a.folder).expanduser().resolve()
        folder.mkdir(parents=True, exist_ok=True)
        e.update(tool=tool_of_engine(a.engine), mode="folder", folder=str(folder))
    r["employees"] = [x for x in r["employees"] if x["name"] != a.name] + [e]
    save_roster(root, r)
    print(f"직원 등록: {e['dept']} / {e['name']} [{e['tool']}/{e['mode']}]" + (f" 채널 {e['channel_id']}" if e["channel_id"] else ""))
```

main에:
```python
    ep = sub.add_parser("employee", help="직원 등록/제거")
    ep.add_argument("action", choices=["add", "remove"])
    ep.add_argument("--root", default=".")
    ep.add_argument("--name", required=True)
    ep.add_argument("--dept")
    ep.add_argument("--bot", help="folder-bot bots.json의 봇 이름(기존 봇 등록)")
    ep.add_argument("--folder", help="새 부서 폴더(봇은 나중에 configure-bot으로)")
    ep.add_argument("--engine", choices=["claude", "codex", "agy", "gemini"], default="claude")
    ep.add_argument("--on-demand", action="store_true", help="호출형(폴더·봇 없음)")
    ep.add_argument("--channel-id")
    ep.add_argument("--replace", action="store_true")
    ep.set_defaults(fn=cmd_employee)
```
(`add`인데 `--dept`가 없으면 `die("--dept 필요")`.)

- [ ] **Step 4: 통과 확인** — `python3 -m pytest -q` → passed
- [ ] **Step 5: 커밋** — `git add -A && git commit -m "feat(companyctl): 직원 등록 — 기존 봇/새 폴더/호출형, 채널 ID는 access.json에서"`

---

### Task 4: install · remove — 템플릿·마커 블록

**Files:**
- Create: `assets/company-block.md`, `assets/employee-block.md`, `assets/SESSION.template.md`, `assets/task.md`, `assets/업무요청.md`, `assets/log.md`
- Modify: `generator/companyctl.py`; Test: `tests/test_companyctl.py`

**Interfaces:**
- Produces: `MARK_START/MARK_END`, `install_block(path: Path, body: str, header: str = "") -> str|None`, `remove_block(path: Path, header: str = "") -> str|None`, `directive_file(tool: str) -> tuple[str, str]`(파일명, 헤더), `render(text, mapping)`; CLI `install --root R`, `remove --root R`

- [ ] **Step 1: 에셋 작성**

`assets/company-block.md` (치환: `{ROOT}`, `{INBOX}`, `{NAME}`):
```markdown
## AI 회사 — 공동 업무 규칙 ({NAME})

- 회사 루트는 `{ROOT}`. `업무요청/`(목표·범위·완료 기준) · `참고자료/` · `결과물/`(산출물·검증 결과) · `docs/` · `tasks/<업무ID>/`(task.md·log.md) · `runtime/inbox/`(총괄 수신함). 정본 명부는 `직원명부.json`.
- 이 폴더의 세션이 **총괄**이다. 총괄은 요청을 업무로 정리해 담당 직원에게 배정하고, 산출물을 직접 검증해 대표(사용자)에게 보고한다. 다른 부서의 일을 겸임하지 않는다.
- 사슬이 없는 일(폴더 하나로 끝나는 일)은 회사 경유를 권하지 않는다 — 그 봇 채널에 직접 지시하라고 안내한다.
- 외부 게시·지출·계정 변경은 대표 승인 사항. 비밀값은 문서·메시지에 담지 않는다. 직원의 완료 주장은 산출물을 읽어 확인한 뒤에만 완료로 보고한다.

### 총괄 절차
1. **시작·재정박**: `SESSION.md` → `직원명부.json` → `agentlayer task list` 순으로 읽는다. Monitor 도구로 `agentlayer task watch {INBOX}`를 `persistent: true`로 띄운다(상주 수신 — 이벤트가 한 줄 JSON으로 온다).
2. **업무 등록**: 업무ID는 `[A-Za-z0-9._-]{1,64}`(예: `VIDEO-07-TOPICS`). `_templates/task.md`를 복사해 `tasks/<업무ID>/task.md`(status: in_progress)와 `log.md`를 만들고, `업무요청/<업무ID>.md`에 목표·입력·허용 범위·완료 기준·산출물 경로를 쓴다.
3. **배정**: 명부에서 직원을 고른다.
   - Claude 봇 직원: `bot-thread open <bot> <channel_id> <업무ID>` → 스레드 ID 출력 → 창 이름은 `t<스레드ID 끝 6자리>`. `agentlayer task assign <업무ID> <session>:<창> --inbox {INBOX}` 뒤 `agentlayer send <session>:<창> - < 업무요청/<업무ID>.md` (파일 경로 대신 본문을 보낸다 — 직원 폴더 밖 읽기 권한 프롬프트 회피).
   - Codex·Gemini 봇 직원: 스레드 없이 `agentlayer task assign <업무ID> <session> --inbox {INBOX}` → `agentlayer send <session> - < 업무요청/<업무ID>.md`.
   - 호출형 직원(폴더·봇 없음): 총괄이 직접 `claude -p` 또는 `agentlayer wt new`로 처리하고 결과를 `결과물/<업무ID>/`에 둔다.
   - `send`가 "작업 중"·"승인 대기"로 거부되면 기다렸다가 다시 보낸다. `--force`는 쓰지 않는다.
4. **수신**: Monitor 이벤트의 `to`가 `WAITING`이면 `ask` 문구를 그대로 대표에게 전달한다(승인 대행 금지). `DONE_UNREAD`면 산출물을 읽어 완료 기준과 대조해 다음 직원 배정 또는 대표 보고. `ERROR`면 대표 보고. `task list`에 `stale`·`gone`이 보이면 그 업무는 재배정 대상이다.
5. **마감**: `agentlayer task done <업무ID>`, `tasks/<업무ID>/task.md`의 status를 `done`으로, `log.md`에 한 줄, 결과물을 `결과물/<업무ID>/`에 복사·링크.
6. **하지 말 것**: 메인 채널 대화에 개입, 승인 대행, 봇끼리 디스코드 멘션, 명부에 없는 세션에 전송, 직원 폴더의 파일 직접 수정.
```

`assets/employee-block.md` (치환: `{DEPT}`, `{NAME}`, `{ROOT}`):
```markdown
## AI 회사 소속 — {DEPT} · {NAME}

- 이 폴더는 AI 회사(`{ROOT}`)의 {DEPT} 업무 공간이다. 회사 총괄이 배정한 업무는 **스레드**(또는 세션 직접 전달)로 온다. 메인 채널의 지시는 지금처럼 다이렉트 업무다.
- 총괄 업무 메시지에는 업무ID가 있다. 산출물 파일명·요약에 그 업무ID를 남기고, 산출물은 이 폴더 안에 둔다(폴더 밖 쓰기는 승인 프롬프트가 뜬다 — 대표가 결정한다).
- 보고 명령은 없다. 작업이 끝나거나(턴 종료), 승인이 필요하거나, 오류로 멈추면 agentlayer 훅이 총괄에게 자동으로 알린다. 총괄에게 보내려고 다른 채널·세션에 쓰지 않는다.
- 총괄의 답변·추가 지시는 같은 스레드(세션)로 온다.
```

`assets/SESSION.template.md` — loadout `SESSION.template.md`와 동일 5절(목표/현재 상태/다음 단계/결정 기록/파일 흔적, 각 절 아래 갱신 규칙 주석). `~/VSCodeWorkspace/discord-harness-installer/SESSION.template.md`를 그대로 복사한다.

`assets/task.md`(discord-multiagent 형식, 첫 줄 제목 치환 없음):
```markdown
# [업무ID — 업무명]

## 메타

```yaml
status: pending
# pending | in_progress | waiting_<직원> | reviewing | done
created: <YYYY-MM-DD>
updated: <YYYY-MM-DD>
priority: medium
```

## 목표
한 문장. 무엇이 되면 완료인가.

## 담당·순서
- 1단계: <부서/직원> → 산출물 경로
- 2단계: <부서/직원> → 산출물 경로

## 완료 기준
- [ ] 기준 1
```

`assets/업무요청.md`:
```markdown
# <업무ID> — <업무명>
담당: <부서> / <직원>. 총괄 수신함: <ROOT>/runtime/inbox. 업무ID: <업무ID>.

## 목표

## 입력 (참고자료 경로, 이전 단계 산출물)

## 허용 범위 (수정 가능한 폴더·파일, 금지 사항)

## 완료 기준

## 산출물 경로 (이 폴더 안)
```

`assets/log.md`:
```markdown
# Log — <업무ID>

<!-- append-only. [YYYY-MM-DD HH:MM] [TAG] 내용. TAG: DECISION | ASSIGN | REPORT | VERIFICATION | ERROR | COMPLETE -->
```

- [ ] **Step 2: 실패하는 테스트**

```python
MS, ME = "<!-- store:ai-company:start -->", "<!-- store:ai-company:end -->"


def test_install_creates_templates_blocks_and_session(env, tmp_path):
    root = _init(env, tmp_path)
    folder = tmp_path / "collab"
    folder.mkdir()
    (folder / "CLAUDE.md").write_text("# collab\n기존 규칙\n")
    write_bots_json(env, {"collab": {"engine": "claude", "folder": str(folder), "session": "collab-bot"}})
    write_access(folder, ["1"])
    run(env, "employee", "add", "--root", str(root), "--dept", "비즈니스운영팀", "--name", "사업운영", "--bot", "collab")
    r = run(env, "install", "--root", str(root))
    assert r.returncode == 0, r.stderr
    for f in ["_templates/task.md", "_templates/업무요청.md", "_templates/log.md", "SESSION.md", "CLAUDE.md"]:
        assert (root / f).exists(), f
    cm = (root / "CLAUDE.md").read_text()
    assert MS in cm and ME in cm and str(root) in cm and "runtime/inbox" in cm and "총괄 절차" in cm
    em = (folder / "CLAUDE.md").read_text()
    assert em.startswith("# collab\n기존 규칙\n") and MS in em and "비즈니스운영팀" in em and "사업운영" in em


def test_install_idempotent_and_never_touches_existing_session(env, tmp_path):
    root = _init(env, tmp_path)
    (root / "SESSION.md").write_text("내 기록\n")
    run(env, "install", "--root", str(root))
    first = (root / "CLAUDE.md").read_text()
    run(env, "install", "--root", str(root))
    assert (root / "CLAUDE.md").read_text() == first
    assert (root / "SESSION.md").read_text() == "내 기록\n"


def test_install_engine_specific_files(env, tmp_path):
    root = _init(env, tmp_path)
    cx, gy = tmp_path / "cx", tmp_path / "gy"
    cx.mkdir(); gy.mkdir()
    write_bots_json(env, {"cx": {"engine": "codex", "folder": str(cx), "session": "cx"},
                          "gy": {"engine": "agy", "folder": str(gy), "session": "gy"}})
    write_access(cx, ["1"]); write_access(gy, ["2"])
    run(env, "employee", "add", "--root", str(root), "--dept", "크리에이티브팀", "--name", "비주얼", "--bot", "cx")
    run(env, "employee", "add", "--root", str(root), "--dept", "커뮤니티·멤버십팀", "--name", "지원", "--bot", "gy")
    assert run(env, "install", "--root", str(root)).returncode == 0
    assert MS in (cx / "AGENTS.md").read_text()
    g = (gy / ".agents/rules/ai-company.md").read_text()
    assert g.startswith("---\ntrigger: always_on\n---\n") and MS in g


def test_remove_restores_originals(env, tmp_path):
    root = _init(env, tmp_path)
    folder = tmp_path / "collab"
    folder.mkdir()
    (folder / "CLAUDE.md").write_text("# collab\n기존 규칙\n")
    write_bots_json(env, {"collab": {"engine": "claude", "folder": str(folder), "session": "collab-bot"}})
    write_access(folder, ["1"])
    run(env, "employee", "add", "--root", str(root), "--dept", "비즈니스운영팀", "--name", "사업운영", "--bot", "collab")
    run(env, "install", "--root", str(root))
    r = run(env, "remove", "--root", str(root))
    assert r.returncode == 0, r.stderr
    assert (folder / "CLAUDE.md").read_text() == "# collab\n기존 규칙\n"
    assert not (root / "CLAUDE.md").exists()          # 블록만 있던 파일은 삭제
    assert (root / "직원명부.json").exists() and (root / "SESSION.md").exists()   # 기록 보존
```

- [ ] **Step 3: 실패 확인** — `python3 -m pytest -q` → FAIL

- [ ] **Step 4: 구현**

```python
ASSETS = Path(__file__).resolve().parent.parent / "assets"
MARK_START = "<!-- store:ai-company:start -->"
MARK_END = "<!-- store:ai-company:end -->"
AGY_HEADER = "---\ntrigger: always_on\n---\n"
TEMPLATES = ["task.md", "업무요청.md", "log.md"]


def render(text: str, mapping: dict) -> str:
    for k, v in mapping.items():
        text = text.replace(k, v)
    return text


def directive_file(tool: str) -> tuple:
    if tool == "codex":
        return ("AGENTS.md", "")
    if tool == "gemini":
        return (".agents/rules/ai-company.md", AGY_HEADER)
    return ("CLAUDE.md", "")


def install_block(path: Path, body: str, header: str = ""):
    cur = path.read_text(encoding="utf-8") if path.exists() else header
    body = body.rstrip()
    if MARK_START in cur and MARK_END in cur:
        pre, rest = cur.split(MARK_START, 1)
        old, post = rest.split(MARK_END, 1)
        if old.strip("\n") == body:
            return None
        path.write_text(f"{pre}{MARK_START}\n{body}\n{MARK_END}{post}", encoding="utf-8")
        return f"지침 블록 갱신: {path}"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(cur + f"\n{MARK_START}\n{body}\n{MARK_END}\n", encoding="utf-8")
    return f"지침 블록 설치: {path}"


def remove_block(path: Path, header: str = ""):
    if not path.exists():
        return None
    cur = path.read_text(encoding="utf-8")
    if MARK_START not in cur or MARK_END not in cur:
        return None
    pre, rest = cur.split(MARK_START, 1)
    _, post = rest.split(MARK_END, 1)
    out = pre.rstrip("\n") + ("\n" if pre.strip() else "") + post.lstrip("\n")
    if not out.strip() or out.strip() == header.strip():
        path.unlink()
        return f"지침 블록 제거 후 빈 파일 삭제: {path}"
    path.write_text(out, encoding="utf-8")
    return f"지침 블록 제거: {path}"


def cmd_install(a) -> None:
    root = Path(a.root).expanduser().resolve()
    r = load_roster(root)
    done = []
    for d in COMPANY_DIRS + ["_templates"]:
        (root / d).mkdir(parents=True, exist_ok=True)
    for t in TEMPLATES:
        dst = root / "_templates" / t
        if not dst.exists():
            dst.write_text((ASSETS / t).read_text(encoding="utf-8"), encoding="utf-8")
            done.append(f"템플릿: {dst}")
    sess = root / "SESSION.md"
    if not sess.exists():
        sess.write_text((ASSETS / "SESSION.template.md").read_text(encoding="utf-8"), encoding="utf-8")
        done.append(f"SESSION.md 생성(템플릿): {sess}")
    inbox = str(root / "runtime" / "inbox")
    body = render((ASSETS / "company-block.md").read_text(encoding="utf-8"),
                  {"{ROOT}": str(root), "{INBOX}": inbox, "{NAME}": r["name"]})
    msg = install_block(root / "CLAUDE.md", body)
    if msg:
        done.append(msg)
    for e in r["employees"]:
        if e["mode"] == "on-demand" or not e["folder"]:
            continue
        fname, header = directive_file(e["tool"])
        eb = render((ASSETS / "employee-block.md").read_text(encoding="utf-8"),
                    {"{DEPT}": e["dept"], "{NAME}": e["name"], "{ROOT}": str(root)})
        msg = install_block(Path(e["folder"]) / fname, eb, header)
        if msg:
            done.append(msg)
    print("\n".join(done) if done else "변경 없음(이미 설치됨)")


def cmd_remove(a) -> None:
    root = Path(a.root).expanduser().resolve()
    r = load_roster(root)
    done = []
    msg = remove_block(root / "CLAUDE.md")
    if msg:
        done.append(msg)
    for e in r["employees"]:
        if not e["folder"]:
            continue
        fname, header = directive_file(e["tool"])
        msg = remove_block(Path(e["folder"]) / fname, header)
        if msg:
            done.append(msg)
    done.append("보존: 직원명부.json · SESSION.md · 업무요청/ · 결과물/ · tasks/ · runtime/ (삭제는 사용자 몫)")
    print("\n".join(done))
```
main: `install`·`remove`에 `--root` 추가하고 `set_defaults(fn=cmd_install/cmd_remove)`.

- [ ] **Step 5: 통과 확인** — `python3 -m pytest -q` → passed
- [ ] **Step 6: 커밋** — `git add -A && git commit -m "feat(companyctl): install/remove — 템플릿·SESSION.md(없을 때만)·총괄/직원 마커 블록"`

---

### Task 5: doctor — 읽기 전용 점검

**Files:** Modify `generator/companyctl.py`; Test `tests/test_companyctl.py`

**Interfaces:**
- Produces: `check_agentlayer() -> tuple[bool, str]`(버전 ≥ 1.5.0 파싱), `which(name) -> str|None`, `cmd_doctor(a)`; 출력은 줄마다 `OK  `/`WARN`/`FAIL` 접두, FAIL 하나라도 있으면 exit 1.
- 점검 항목: agentlayer 버전 / `bot-thread`·`bot-up`·`tmux` 존재 / bots.json 존재 / 직원명부 스키마 / 부서마다 직원 유무(없으면 WARN 호출형) / bot 모드 직원의 폴더·지침 블록·bots.json 등록·채널 ID / folder 모드 직원은 WARN(봇 미연결) / 회사 CLAUDE.md 블록 / runtime/inbox 하위 3개 / `agentlayer task list --json`의 stale·gone 행 WARN / tasks/ 활성 업무 수(status in_progress·waiting_*·reviewing).

- [ ] **Step 1: 실패하는 테스트**

```python
def test_doctor_ok_after_install(env, tmp_path):
    root = _init(env, tmp_path)
    folder = tmp_path / "collab"; folder.mkdir()
    write_bots_json(env, {"collab": {"engine": "claude", "folder": str(folder), "session": "collab-bot"}})
    write_access(folder, ["1"])
    run(env, "employee", "add", "--root", str(root), "--dept", "비즈니스운영팀", "--name", "사업운영", "--bot", "collab")
    run(env, "install", "--root", str(root))
    r = run(env, "doctor", "--root", str(root))
    assert r.returncode == 0, r.stdout + r.stderr
    assert "OK   agentlayer v1.5.0" in r.stdout and "FAIL" not in r.stdout
    assert "WARN 콘텐츠전략팀" in r.stdout      # 직원 없는 부서는 경고(호출형)


def test_doctor_fails_on_old_agentlayer_and_missing_block(env, tmp_path):
    root = _init(env, tmp_path)
    run(env, "install", "--root", str(root))
    (root / "CLAUDE.md").unlink()
    shim = Path(env["PATH"].split(":")[0]) / "agentlayer"
    shim.write_text('#!/bin/sh\ncase "$1" in version) echo "agentlayer v1.4.5 (commit x, 2026-09-14)";; task) echo "[]";; esac\n')
    r = run(env, "doctor", "--root", str(root))
    assert r.returncode == 1
    assert "FAIL agentlayer" in r.stdout and "1.5.0" in r.stdout
    assert "FAIL 회사 CLAUDE.md" in r.stdout


def test_doctor_warns_stale_assignments(env, tmp_path):
    root = _init(env, tmp_path)
    run(env, "install", "--root", str(root))
    shim = Path(env["PATH"].split(":")[0]) / "agentlayer"
    shim.write_text('#!/bin/sh\ncase "$1" in version) echo "agentlayer v1.5.0 (commit x, 2026-09-14)";; task) echo \'[{"task_id":"T-1","session":"s","state":"stale"}]\';; esac\n')
    r = run(env, "doctor", "--root", str(root))
    assert "WARN 업무 T-1" in r.stdout and "stale" in r.stdout
```

- [ ] **Step 2: 실패 확인** — FAIL

- [ ] **Step 3: 구현**

```python
import re
import shutil
import subprocess


def which(name: str):
    return shutil.which(name)


def check_agentlayer() -> tuple:
    exe = which("agentlayer")
    if not exe:
        return (False, "agentlayer 없음 — brew install netwaif/tap/agentlayer && agentlayer init")
    try:
        out = subprocess.run([exe, "version"], capture_output=True, text=True, timeout=10).stdout
    except (OSError, subprocess.TimeoutExpired) as ex:
        return (False, f"agentlayer version 실행 실패: {ex}")
    m = re.search(r"v(\d+)\.(\d+)\.(\d+)", out)
    if not m:
        return (False, f"agentlayer 버전 해석 실패: {out.strip()}")
    ver = tuple(int(x) for x in m.groups())
    if ver < (1, 5, 0):
        return (False, f"agentlayer v{'.'.join(map(str, ver))} — 1.5.0 이상 필요(brew upgrade netwaif/tap/agentlayer && agentlayer init)")
    return (True, f"agentlayer v{'.'.join(map(str, ver))}")


def task_rows() -> list:
    exe = which("agentlayer")
    if not exe:
        return []
    try:
        out = subprocess.run([exe, "task", "list", "--json"], capture_output=True, text=True, timeout=10).stdout
        rows = json.loads(out or "[]")
        return rows if isinstance(rows, list) else []
    except (OSError, subprocess.TimeoutExpired, ValueError):
        return []


def active_tasks(root: Path) -> int:
    n = 0
    for d in (root / "tasks").glob("*/task.md"):
        m = re.search(r"^status:\s*(\S+)", d.read_text(encoding="utf-8"), re.M)
        if m and (m.group(1) in ("in_progress", "reviewing") or m.group(1).startswith("waiting_")):
            n += 1
    return n


def cmd_doctor(a) -> None:
    root = Path(a.root).expanduser().resolve()
    r = load_roster(root)
    lines = []
    fail = False

    def ok(m): lines.append("OK   " + m)
    def warn(m): lines.append("WARN " + m)
    def bad(m):
        nonlocal fail
        fail = True
        lines.append("FAIL " + m)

    good, msg = check_agentlayer()
    (ok if good else bad)(msg)
    for tool in ("bot-thread", "bot-up", "tmux"):
        (ok if which(tool) else bad)(f"{tool}: {which(tool) or '없음 — folder-bot 플러그인/tmux 설치'}")
    (ok if bots_json_path().exists() else warn)(f"folder-bot bots.json: {bots_json_path()}")
    if r.get("version") != 1 or not isinstance(r.get("employees"), list):
        bad("직원명부 스키마: version/employees 이상")
    else:
        ok(f"직원명부: 부서 {len(r['departments'])}개, 직원 {len(r['employees'])}명")
    (ok if (root / "CLAUDE.md").exists() and MARK_START in (root / "CLAUDE.md").read_text(encoding="utf-8") else bad)(
        f"회사 CLAUDE.md 총괄 블록: {root / 'CLAUDE.md'}")
    for sub in ("pending", "received", "quarantine"):
        p = root / "runtime" / "inbox" / sub
        (ok if p.is_dir() else bad)(f"수신함 {sub}/: {p}")
    bots = read_bots()
    for d in r["departments"]:
        emps = [e for e in r["employees"] if e["dept"] == d]
        if not emps:
            warn(f"{d}: 직원 없음(호출형으로 운영)")
        for e in emps:
            if e["mode"] == "on-demand":
                ok(f"{d}/{e['name']}: 호출형")
                continue
            if e["mode"] == "folder":
                warn(f"{d}/{e['name']}: 폴더만 있음({e['folder']}) — configure-bot으로 봇 연결 뒤 `employee add --bot`로 갱신")
                continue
            folder = Path(e["folder"])
            fname, _ = directive_file(e["tool"])
            block_ok = (folder / fname).exists() and MARK_START in (folder / fname).read_text(encoding="utf-8")
            reg = e["bot"] in bots
            (ok if folder.is_dir() and block_ok and reg and e["channel_id"] else bad)(
                f"{d}/{e['name']} [{e['tool']}] 폴더={'있음' if folder.is_dir() else '없음'} 지침블록={'있음' if block_ok else '없음'} "
                f"bots.json={'등록' if reg else '미등록'} 채널={e['channel_id'] or '없음'} 세션={e['session']}")
    for row in task_rows():
        if row.get("state") in ("stale", "gone"):
            warn(f"업무 {row.get('task_id')} 세션 {row.get('session')}: {row.get('state')} — 재배정 또는 `agentlayer task done`")
    ok(f"활성 업무(tasks/): {active_tasks(root)}건")
    print("\n".join(lines))
    sys.exit(1 if fail else 0)
```
main: `doctor`에 `--root`, `set_defaults(fn=cmd_doctor)`.

- [ ] **Step 4: 통과 확인** — `python3 -m pytest -q` → passed
- [ ] **Step 5: 커밋** — `git add -A && git commit -m "feat(companyctl): doctor — agentlayer 버전·도구·명부·블록·수신함·낡은 업무 점검"`

---

### Task 6: SKILL.md · README

**Files:** Create `plugins/ai-company/skills/configure-company/SKILL.md`; Modify `README.md`

- [ ] **Step 1: SKILL.md**

```markdown
---
name: configure-company
description: Use when the user wants to build or operate an "AI company" on top of their folder bots — create the company root, register departments and employees (existing folder-bot bots, new folders, or on-demand), install the manager (총괄) procedure, check or remove it. Triggers on "AI 회사 만들어줘", "회사에 직원 추가해줘", "부서 추가", "회사 점검해줘", "회사 제거해줘", "/configure-company". 결정적 엔진(companyctl.py)이 직원명부.json 정본으로 폴더·템플릿·지침 블록을 멱등 설치하고, 수동은 총괄 봇의 디스코드 포탈 단계뿐(folder-bot configure-bot에 위임).
---

# configure-company — AI 회사 생성기

총괄 봇 하나와 기존 폴더 봇들을 부서·직원으로 묶는다. 모든 파일 조작은 `generator/companyctl.py`가 한다 —
직접 명부·지침 파일을 손으로 쓰지 말 것. **비파괴 원칙**: SESSION.md는 없을 때만 만들고 있으면 절대 수정하지 않는다.
지침 파일은 마커 블록(`<!-- store:ai-company:start/end -->`)만 추가·제거한다. 토큰은 읽지도 쓰지도 않는다.

총괄 역할은 Claude Code 전용이다(Monitor·SendMessage 의존). 직원은 Claude·Codex·Gemini(agy) 봇 모두 가능.

## 회사 만들기

### 1. 전제 점검
```bash
python3 "<이 스킬 폴더>/generator/companyctl.py" doctor --root <회사 루트>   # 명부가 없으면 아래 init 뒤에 다시
```
- agentlayer ≥ 1.5.0(`agentlayer version`). 없거나 낮으면: `brew install netwaif/tap/agentlayer && agentlayer init`(리눅스: README의 install.sh). 그 뒤 반드시 `agentlayer init`.
- folder-bot 플러그인(`bot-thread`·`bot-up`가 `~/.local/bin`에 있음). 없으면 `/plugin marketplace add netwaif/folder-bot` → `/plugin install folder-bot` 안내 후 중단.
- tmux.

### 2. 질문 (AskUserQuestion 한 번에)
- 회사 루트(기본 `~/ai-folder/company`)
- 회사 이름(기본 "AI 회사")
- 부서 목록(기본 9부서 — 경영기획실·콘텐츠전략팀·기술개발팀·크리에이티브팀·기술검증팀·교육자료팀·채널그로스팀·커뮤니티·멤버십팀·비즈니스운영팀; 빼거나 추가 가능)
AskUserQuestion이 없는 환경에서는 채팅으로 같은 질문을 하고 답을 받는다. 기본값으로 질주하지 말 것.

### 3. 회사 루트 생성
```bash
python3 "<이 스킬 폴더>/generator/companyctl.py" init --root <루트> --name "<이름>" [--depts a,b,c]
```

### 4. 직원 등록 (부서마다 한 번씩 물어본다)
먼저 `python3 - <<'EOF'` 없이 `cat ~/.config/folder-bot/bots.json`으로 기존 봇 이름·폴더를 읽어 표로 보여 준다(토큰 없음, 읽어도 됨). 부서마다 세 가지 중 하나를 고르게 한다:
- **기존 봇**: `companyctl employee add --root <루트> --dept <부서> --name <직원명> --bot <bots.json 이름>` — 채널 ID는 그 봇 폴더의 `.discord-state/access.json`에서 자동으로 읽는다(둘 이상이면 `--channel-id`).
- **새 폴더 + 새 봇**: `companyctl employee add ... --folder <새 폴더> --engine claude|codex|agy` → 그 폴더에서 folder-bot의 **configure-bot** 스킬로 봇을 만든 뒤(포탈 수동 단계 포함) `companyctl employee add ... --bot <새 봇 이름> --replace`로 갱신.
- **호출형**: `companyctl employee add ... --on-demand` — 폴더·봇 없음, 총괄이 필요할 때 `claude -p`로 처리.
경영기획실(총괄)은 직원으로 등록하지 않는다 — 회사 루트 자체가 총괄 봇 폴더다.

### 5. 설치
```bash
python3 "<이 스킬 폴더>/generator/companyctl.py" install --root <루트>
```
출력을 그대로 보여 준다(템플릿·SESSION.md·총괄 블록·직원 블록).

### 6. 총괄 봇 (수동 단계 포함)
회사 루트에서 folder-bot의 **configure-bot** 스킬을 실행한다("이 폴더를 디스코드 봇으로 만들어줘", 봇 이름 예: `company`, 세션 `company-bot`). 디스코드 포탈·토큰·초대·페어링은 그 스킬의 안내를 따른다. 직원 채널들은 한 카테고리에 모아 두는 것을 권한다(채널 ID는 바뀌지 않으므로 이동은 자유).

### 7. 검증
```bash
python3 "<이 스킬 폴더>/generator/companyctl.py" doctor --root <루트>
```
FAIL이 없으면 무해한 사슬 한 번: 총괄 채널(또는 회사 루트의 claude 세션)에서 "직원 <이름>에게 '도구 없이 OK라고만 답해'를 업무 LAB-1로 보내고 보고를 기다려 줘". 총괄 절차대로 스레드 생성 → `task assign` → `send` → Monitor 이벤트 `to: DONE_UNREAD`가 오면 성공. 결과를 사용자에게 그대로 보여 준다.

## 직원 추가·제거 / 부서 추가·제거
`companyctl employee add|remove`, `companyctl dept add|remove` 뒤 반드시 `companyctl install`(블록 갱신). 직원 제거는 명부에서만 빼며 그 폴더의 블록은 `install`이 다시 돌 때 유지된다 — 블록까지 걷으려면 먼저 `companyctl remove`, 명부 수정, `install`.

## 점검
`companyctl doctor --root <루트>` — 읽기 전용. agentlayer 버전·도구·명부·총괄 블록·수신함·직원별 폴더/블록/등록/채널·낡은 업무(`stale`·`gone`)·활성 업무 수.

## 제거
`companyctl remove --root <루트>` — 총괄·직원 지침 블록만 걷어낸다. 직원명부·SESSION.md·업무요청·결과물·tasks·runtime은 보존(삭제는 사용자 몫). 총괄 봇 자체는 folder-bot의 제거 절차.
```

- [ ] **Step 2: README.md**

```markdown
# ai-company

AI 회사 생성기 — 총괄 봇 하나와 기존 폴더 봇들을 부서·직원으로 묶어, 요청 한 번으로 사슬 업무(기획 → 제작 → 검증)가 돌게 한다.
agentlayer의 배관(`send`·`task`·훅 자동 보고)과 folder-bot(봇·스레드) 위에서 동작한다.

## 설치

```
brew install netwaif/tap/agentlayer && agentlayer init        # 있으면 brew upgrade 뒤 init
/plugin marketplace add netwaif/folder-bot  →  /plugin install folder-bot
/plugin marketplace add netwaif/ai-company  →  /plugin install ai-company
```

## 사용

```
AI 회사 만들어줘
```

스킬(configure-company)이 전 과정을 이끈다 — 회사 루트·부서·직원(기존 봇 / 새 폴더 / 호출형)을 묻고, 결정적 엔진 `companyctl`이
폴더·템플릿·총괄 절차·직원 지침 블록을 멱등 설치한다. 수동은 총괄 봇의 디스코드 포탈 단계뿐(folder-bot의 configure-bot이 안내).

**총괄 역할은 Claude Code 전용**(Monitor·SendMessage 의존). 직원은 Claude·Codex·Gemini(agy) 봇 모두 가능. 맥·리눅스·WSL2.

## 구조

```
<회사 루트>/                 = 총괄 봇 폴더
├─ CLAUDE.md                공동 규칙 + 총괄 절차(마커 블록)
├─ SESSION.md               세션 이어가기(없을 때만 생성)
├─ 직원명부.json             정본
├─ 업무요청/ 참고자료/ 결과물/ docs/ tasks/<업무ID>/ _templates/
└─ runtime/inbox/{pending,received,quarantine}/   총괄 수신함(agentlayer 훅이 씀)
```

직원 폴더에는 소속 부서 마커 블록 한 개만 추가된다. 보고 명령은 없다 — 직원의 상태 전이(끝남·승인 대기·오류)가 곧 보고다.

## 명령 (스킬이 대신 실행)

```bash
companyctl.py init --root ~/ai-folder/company --name "AI 회사"
companyctl.py employee add --root … --dept 비즈니스운영팀 --name "사업운영 매니저" --bot collab
companyctl.py install|doctor|remove|list --root …
```

## 비파괴 보장
SESSION.md는 없을 때만 만든다. 지침 파일은 마커 블록만 추가·제거(원문 복원). 토큰은 읽지도 쓰지도 않는다.

## 라이선스
MIT
```

- [ ] **Step 3: 커밋** — `git add -A && git commit -m "docs: configure-company 스킬 절차와 README"`

---

### Task 7: 공개 · 설치 검증 · 스펙 정정 · SESSION.md

**Files:** GitHub 레포 생성·푸시·태그; agentlayer 레포 `docs/superpowers/specs/2026-09-14-ai-company-design.md`(§2.3 정정 1줄) + `SESSION.md`

- [ ] **Step 1: 전체 테스트** — `cd ~/VSCodeWorkspace/ai-company && python3 -m pytest -q` → passed
- [ ] **Step 2: 공개(사용자 확인 뒤)** 
```bash
gh repo create netwaif/ai-company --public --source . --push --description "AI 회사 생성기 — 총괄 봇 + 폴더 봇 직원, agentlayer 배관 위에서"
git tag -a v0.1.0 -m "v0.1.0 — configure-company 스킬 + companyctl" && git push origin v0.1.0
```
- [ ] **Step 3: 이 맥에서 설치 확인** — `claude` 안에서 `/plugin marketplace add netwaif/ai-company` → `/plugin install ai-company` 은 사용자 조작(안내만). 대신 파일 기준 검증: `python3 plugins/ai-company/skills/configure-company/generator/companyctl.py --help`와 `doctor --root /tmp/x`(init 전 → 에러 메시지에 init 안내).
- [ ] **Step 4: 스펙 정정** — agentlayer 레포 스펙 §2.3의 "(discord-multiagent 형식 → agentlayer MultiAgent 패널에 표시)"를 "(discord-multiagent·mat 호환 형식. agentlayer 패널은 `~/VSCodeWorkspace/MultiAgent` 고정 경로만 읽으므로 자동 표시되지 않음 — 후속: `multiagent_roots` 설정)"으로. 커밋 `docs: 스펙 §2.3 tasks/ 패널 표시 문구 정정`.
- [ ] **Step 5: SESSION.md(agentlayer)** — 현재 상태 "2단계 ai-company v0.1.0 공개", 다음 단계 1번 = 3단계(사용자 회사 설치: `AI 회사 만들어줘` → 기존 봇 등록 → 총괄 봇 포탈·페어링). 파일 흔적에 레포 경로·GitHub URL. 커밋·푸시.

---

## Self-Review

- 스펙 §2.1 스킬 단계 1~6: Task 6 SKILL.md ✓ / §2.2 엔진 명령 init·dept·employee·install·doctor·remove·list: Task 2~5 ✓ / §2.3 루트 구조: Task 2(디렉터리)·4(템플릿·SESSION·블록) ✓, 패널 문구는 정정 / §2.4 총괄 절차: `assets/company-block.md` ✓(send·task·bot-thread 실제 명령, WAIT는 대표에게, --force 금지) / §2.5 테스트: Task 1~5 ✓ / 비밀값 금지: `.env` 미접근, access.json groups 키만 ✓
- 타입 일관성: 직원 레코드 8키 고정(Task 3 테스트가 dict 동등 비교) — Task 4·5가 같은 키 사용 ✓; `directive_file(tool)` tool 값은 `claude|codex|gemini`(agy→gemini 매핑은 Task 3) ✓; `MARK_START` 문자열이 테스트 `MS`와 동일 ✓
- 플레이스홀더 없음. 외부 실호출은 doctor뿐이며 테스트는 shim ✓
