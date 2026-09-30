# 스킬 초안 v2 — `baton` (앱 간 대화 연결), 사용자 확정 전

2026-09-30 21:40 개정. 이름 `baton`(플러그인·스킬·레포 `netwaif/baton`). v1의 "요청 한 번" 모델을 **"대화 연결"** 모델로 바꿈(사용자 그림: 한 번 연결하면 그 뒤로는 귀찮은 과정 없이 두 세션이 협력하듯 이어진다).

대상: 데스크톱 앱(Claude Code)만 쓰는 초보. 설치·실행 전부 앱 안에서. tmux·훅·총괄 전제 없음.
배포: 플러그인 마켓플레이스 `netwaif/baton`. 앱의 Claude Code에서 `/plugin marketplace add netwaif/baton` → `/plugin install baton@baton`.
바이너리: 첫 실행에 `agentlayer` 1.12.0+ 확인, 없으면 `brew install netwaif/tap/agentlayer`를 앱 안 Bash로.
노드: Claude(앱) ↔ 코덱스(앱, 세션 ID) ↔ 헤르메스(**같은 맥의 로컬 설치가 기본**, `remote add … --local`; VPS는 `--ssh` 변형) ↔ 디스코드(헤르메스 채널 게시로 확인).

## 연결 모델
- **연결(한 번)**: Claude가 `agentlayer inbox open --name <별칭>`으로 고유 주소 ID(`al-xxxxxx`)를 받고, 짝 정보(코덱스 세션 ID·작업 폴더)를 `.baton/pair.json`에 저장한다. 첫 메시지 끝에 "회신은 셸에서 `agentlayer send al-xxxxxx \"…\"`"를 넣어 보낸다. 코덱스는 대화 맥락에 주소가 남으므로 이후 스스로 회신한다.
- **이어감**: Claude는 답을 받아 처리한 뒤 곧바로 `inbox wait --name <별칭> --timeout 2h`를 다시 켠다(백그라운드 Bash). 대기가 잠깐 꺼진 사이에 온 편지도 큐에 남아 다음 wait가 집는다(agentlayer 연결 모드). 사용자가 "코덱스에게 이것도"라고 하면 저장된 ID로 바로 보낸다.
- **끝**: 사용자가 "연결 끊어"라고 하면 `inbox close`, 또는 2시간 무통신이면 wait가 끝나고 스킬이 "연결이 쉬고 있음"을 알린다(주소는 남아 있어 다시 wait만 켜면 이어짐).

## SKILL.md (초안)

```markdown
---
name: baton
description: 이 Claude 세션과 다른 앱의 에이전트(코덱스·헤르메스)를 대화로 연결해 일을 넘기고 답을 받는다. "코덱스 <세션ID>와 연결해", "코덱스에 (이미지 만들어 달라고) 시켜", "헤르메스에게 이 파일 보내", "메시지 받을 준비해", "연결 끊어", "/baton" 에 사용. 터미널 없이 데스크톱 앱 안에서만.
---

# baton — 다른 앱과 대화 연결

준비(한 번): `agentlayer version` 1.12.0 이상인지 확인. 없거나 낮으면 `brew install netwaif/tap/agentlayer`(있으면 `brew upgrade netwaif/tap/agentlayer`) 실행 후 재확인.
플래그(`--json`·`--file`·`--cwd`)는 반드시 대상 앞에 쓴다.

## 연결하기 — "코덱스 <세션ID>와 연결해"
사용자가 세션 ID(UUID)를 주지 않았으면 한 번만 안내한다: "코덱스 앱에서 '세션 번호 알려줘'라고 물어 나온 ID를 붙여 주세요".
1. `ID=$(agentlayer inbox open --name <폴더명>)` — 고유 주소. `.baton/pair.json`에 {codex_session, cwd, my_address, my_name}을 저장.
2. 백그라운드 Bash: `agentlayer inbox wait --name <폴더명> --timeout 2h`
3. 첫 메시지: `agentlayer send --json --cwd <작업폴더> <코덱스 세션 ID> "<본문>

   이 대화의 회신 주소: al-xxxxxx. 결과(파일 경로 포함)는 셸에서 agentlayer send al-xxxxxx \"<결과>\" 로 보내라. 이후 이 대화의 모든 회신도 같은 주소로."`
4. 연결됐다고 사용자에게 한 줄로 알린다.

## 시키기 — "코덱스에 … 시켜" (연결된 뒤)
`.baton/pair.json`의 세션 ID로 `agentlayer send --json --cwd <작업폴더> <ID> "<요청> (회신 주소 al-xxxxxx)"`. 대기가 꺼져 있으면(직전 wait가 끝났으면) 다시 켠다.

## 답 받기 (백그라운드 wait가 끝났을 때)
출력의 `from:` 다음 본문이 답이다. 파일 경로가 있으면 열어 확인하고 사용자에게 보고한다. 처리 뒤 **곧바로** `inbox wait --name <폴더명> --timeout 2h`를 다시 켠다. `답 없음(2h)`이면 "연결이 2시간 동안 조용해 대기를 멈췄습니다. 다시 기다릴까요?"라고 묻는다(주소는 남아 있다).

## 헤르메스 — "헤르메스에게 이 파일(스킬) 보내"
1. `agentlayer remote list`로 이름 확인(하나면 그것). 없으면 매뉴얼의 등록 절(`agentlayer remote add <이름> --kind hermes --local --profile <프로필> --workspace-root <경로>`)을 안내하고 멈춘다.
2. 파일마다 `--file`(폴더는 zip 하나로): `agentlayer send --json --file <경로> <원격이름> "<본문>

   받았으면 네 디스코드 채널에 '수신 확인 — <첫 줄 요약>' 한 줄을 올려라. 회신이 필요하면 셸에서 agentlayer send al-xxxxxx \"<회신>\"."`
3. `files`(원격 경로)·`handle`을 보고한다. 디스코드 게시는 사용자가 확인한다.

## 받을 준비만 — "메시지 받을 준비해"
`inbox open` + `inbox wait`(위 1·2)만 하고 주소 ID를 사용자에게 알린다: "상대에게 `agentlayer send al-xxxxxx \"…\"`로 보내라고 하세요".

## 끊기 — "연결 끊어"
`agentlayer inbox close --name <폴더명>`, `.baton/pair.json` 삭제. 백그라운드 wait가 있으면 그대로 두어도 된다(다음 편지 없이 타임아웃으로 끝난다).
```

## 확정된 결정(2026-09-30)
- 회신 주소는 고유 ID(al-6자), 이름은 별칭. 코덱스 세션 ID는 `.baton/pair.json`에 저장.
- 헤르메스는 기본 흐름(로컬 설치). 등록은 매뉴얼.
- 이름 `baton`. 레포 공개(`gh repo create netwaif/baton`)는 사용자 확인 뒤.

## 남은 확인
- 코덱스 앱 세션이 `agentlayer send al-xxxxxx …`를 셸로 실행할 때 PATH에 agentlayer가 있는가(어제 실측에서 `send agentlayer-dev`는 됐음 → 있음).
- 코덱스 세션 ID 직송(A) 실기 — 밤에.
