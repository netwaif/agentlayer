# 디스코드 → 헤르메스 → Claude 앱 세션 (claude-letter) 설계

2026-10-01. baton 남은 방향(디스코드에서 헤르메스에게 말하면 Claude 앱 세션이 받고 답함)을 여는 최소 추가. 기존 동작 불변, 추가만.

## 왜 지금 구조로는 안 되나
- 헤르메스가 먼저 편지를 보내는 길은 서버의 `company-letter` 하나이고 담당자가 회사 총괄 편지함(`company-manager`)으로 고정이다(`internal/remote/setup.go`, `hermesside/`).
- 편지함만 다른 원격을 하나 더 등록해도 분리되지 않는다. 회사 총괄의 폴링(`task.PollRemotesOnce`)은 **등록된 모든 원격**의 편지함을 걷어 가므로 baton 편지도 회사 수신함으로 들어간다. `remote add`의 준비물 설치는 같은 이름(`company-letter`)을 덮어써 회사 쪽 담당자까지 바꿔 버린다.
- 서버 스킬 문구가 "회사 총괄에게 편지"라서 디스코드에서 "Claude에게 전해 줘"라고 하면 헤르메스가 스킬을 못 찾는다. 회사 없이 baton만 쓰는 사람도 같다.
- `inbox wait`가 내놓는 원격 편지에는 카드 ID가 없고, 답장(`task reply`)은 회사 수신함을 전제로 한다. 앱 세션은 답할 길이 없다.

## 추가하는 것 (넷)
1. **서버 준비물 한 벌 더**: 명령 `claude-letter "제목" "본문"` + 헤르메스 스킬 `claude-letter`. 담당자는 고정값 `claude-app`. 회사용 `company-letter`와 나란히 깔린다(`remote add`·`remote setup`이 둘 다 설치, 멱등). 스킬 설명은 "Claude(클로드, Claude Code, 데스크톱 앱 세션)에게 전달·요청·질문"에 걸리게 쓴다.
2. **`inbox wait --remote <이름> --app-mailbox`**: 그 원격의 `claude-app` 앞 편지를 받는다. 기존 `--mailbox`(등록된 편지함=회사)는 그대로. 회사 폴링은 `company-manager`만 보므로 경합 없음.
3. **편지 출력에 카드 ID**: `--app-mailbox`로 받은 편지는 `from: <원격>/<보낸이>` 다음 줄에 `letter: <원격>:<카드ID>`를 찍는다(그 뒤 빈 줄 + 본문). 다른 경로의 출력은 그대로.
4. **`inbox reply <원격>:<카드ID> [--file <경로>]... <답|->`**: 어댑터 `Answer`로 편지 카드를 답으로 닫는다(첨부는 기존 업로드 경로). 회사 수신함을 거치지 않는다. 헤르메스 쪽은 `claude-letter`가 대화를 구독해 두므로 디스코드에 `✔ … done — <답 첫 줄>` 알림이 뜬다.

## 흐름
디스코드에서 헤르메스에게 "Claude에게 썸네일 문구 3개 뽑아 달라고 해" → 헤르메스가 `claude-letter` 실행(카드, 담당 `claude-app`, 이 대화 구독) → Claude 앱 세션의 `inbox wait --remote <이름> --app-mailbox`가 `from:`·`letter:`·본문을 내놓고 끝남 → Claude가 일을 하고 `inbox reply <원격>:<카드ID> "답"` → 카드 완료 → 디스코드 그 대화에 답 알림.

## 범위 밖
- 헤르메스 → Claude 방향 파일 첨부(본문에 서버 경로를 적는 것까지만).
- Claude 앱 세션이 여러 개일 때 받는 세션 지정(먼저 기다리던 세션이 받는다).
- 디스코드 알림 뒤 헤르메스가 자동으로 대화를 이어 가는 것(알림만; 자동 wake는 헤르메스 쪽 조건이 따로 있음).
- baton 스킬 문구(설치·실기와 함께 이 레포 밖 `netwaif/baton`에서 고친다).
