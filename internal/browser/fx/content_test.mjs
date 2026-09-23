// internal/browser/fx/content_test.mjs — node로 DOM을 최소 흉내 내어 콘텐츠 스크립트를 검사한다.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import assert from 'node:assert/strict';

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'content.js'), 'utf8');

// ---- 최소 DOM 스텁: 속성·자식·스타일·이벤트만
function el(tag) {
  const e = { tagName: tag.toUpperCase(), style: {}, children: [], attrs: {}, listeners: {}, textContent: '', parentElement: null };
  e.setAttribute = (k, v) => { e.attrs[k] = String(v); observers.forEach(o => o.target === e && o.cb()); };
  e.getAttribute = (k) => (k in e.attrs ? e.attrs[k] : null);
  e.removeAttribute = (k) => { delete e.attrs[k]; };
  e.appendChild = (c) => { e.children.push(c); c.parentElement = e; return c; };
  e.append = (...cs) => cs.forEach(e.appendChild);
  // remove()는 부모의 children 배열에서 빼는 것뿐 아니라 자기 parentElement도 끊어야 한다 —
  // 안 그러면 isConnected가 끊긴 뒤에도 옛 부모 체인을 타고 올라가 true로 잘못 본다.
  e.remove = () => { if (e.parentElement) { e.parentElement.children = e.parentElement.children.filter(c => c !== e); e.parentElement = null; } };
  e.addEventListener = (t, cb) => { (e.listeners[t] ||= []).push(cb); };
  e.click = () => (e.listeners.click || []).forEach(cb => cb({ stopPropagation() {}, preventDefault() {} }));
  e.getBoundingClientRect = () => ({ left: 0, top: 0, width: 10, height: 10 });
  Object.defineProperty(e.style, 'cssText', { set(v) { v.split(';').forEach(kv => { const [k, val] = kv.split(':'); if (k) e.style[k.trim()] = (val || '').trim(); }); }, get() { return ''; } });
  // innerHTML: content.js는 두 가지 용도로 쓴다 — (a) 정적 SVG 마크업을 한 번 박아 넣기(문자열
  // 보관만 하면 됨, 자식 파싱은 스텁이 안 함), (b) 버튼 컨테이너를 비우기(''를 대입) — 실DOM은
  // innerHTML=''로 자식이 실제로 사라지지만, 이 스텁은 DOM 파서가 없으므로 ''일 때만 children을
  // 비워 같은 효과를 낸다(게이트 리뷰가 짚은, production 코드에서 children.length=0을 뺀 대신
  // 스텁이 이 몫을 진다).
  let _html = '';
  Object.defineProperty(e, 'innerHTML', {
    get() { return _html; },
    set(v) { _html = v; if (v === '') e.children = []; },
  });
  // isConnected — documentElement까지 parentElement 체인을 타고 올라가면 붙어 있는 것으로 본다.
  Object.defineProperty(e, 'isConnected', {
    get() { let n = e; while (n) { if (n === document.documentElement) return true; n = n.parentElement; } return false; },
  });
  return e;
}
const observers = [];
globalThis.MutationObserver = class { constructor(cb) { this.cb = cb; } observe(t) { this.target = t; observers.push(this); } };
const html = el('html'), body = el('body'); html.appendChild(body);
globalThis.document = { documentElement: html, body, createElement: el, getElementById: () => null };
globalThis.window = { top: null, addEventListener: (t, cb) => { (window.listeners ||= {})[t] ||= []; window.listeners[t].push(cb); }, innerWidth: 1000, innerHeight: 800 };
window.top = window;
globalThis.requestAnimationFrame = (cb) => cb();

// ---- 가짜 타이머 — LINGER(2.5초)·만료(20초) 검사를 실제로 하려면 시간을 손으로 넘길 수 있어야
// 한다. 이전엔 setTimeout이 즉시 실행돼 LINGER가 사실상 0이었고, "off 뒤 사용자 마우스 무시"
// 검증이 우연히 통과했었다(게이트 리뷰 지적 6). now를 __advance로만 움직이는 가짜 시계로 바꾼다.
let now = 1_000_000;
let nextTimerId = 1;
const timers = new Map(); // id -> { at, cb }
globalThis.setTimeout = (cb, ms = 0) => { const id = nextTimerId++; timers.set(id, { at: now + ms, cb }); return id; };
globalThis.clearTimeout = (id) => { timers.delete(id); };
globalThis.setInterval = () => 0; // content.js의 5초 재도색 인터벌은 테스트에서 쓰지 않는다(무동작)
globalThis.clearInterval = () => {};
globalThis.Date = class extends Date { static now() { return now; } };
// now를 ms만큼 전진시키고, 그 사이 만기된 타이머를 (스케줄된 순서로) 실행한다. 콜백이 새
// 타이머를 또 거는 경우(readFx의 재무장 등)까지 반영되도록 만기 타이머가 없을 때까지 돈다.
globalThis.__advance = (ms) => {
  now += ms;
  let fired = true;
  while (fired) {
    fired = false;
    for (const [id, t] of [...timers.entries()].sort((a, b) => a[1].at - b[1].at)) {
      if (t.at <= now) { timers.delete(id); t.cb(); fired = true; }
    }
  }
};

new Function(src)();
const fx = window.__agentlayerFx;
assert.ok(fx, 'content.js가 window.__agentlayerFx를 노출해야 함');
assert.equal(typeof fx.render, 'function', 'content.js가 render()도 훅에 노출해야 함(렌더 경로 직접 검증용)');

// ---- render()가 실제로 그린 DOM을 들여다보기 위한 헬퍼(스텁 트리를 훑는다)
const findByTag = (node, tag, acc = []) => {
  if (!node) return acc;
  if (node.tagName === tag) acc.push(node);
  (node.children || []).forEach(c => findByTag(c, tag, acc));
  return acc;
};
const hostEl = () => body.children.find(c => c.id === '__agentlayer_fx');
const buttonEls = () => findByTag(hostEl(), 'BUTTON');
const pillBtnsEl = () => buttonEls()[0]?.parentElement;

// ---- parseMirror
const m = fx.parseMirror('agent:claude-%1:JustWatch∶ 신작:1000:2000:0:1:1:0:탭 제목');
assert.equal(m.owner, 'agent'); assert.equal(m.agent, 'claude-%1'); assert.equal(m.label, 'JustWatch∶ 신작');
assert.equal(m.lastMs, 2000); assert.equal(m.stopped, false); assert.equal(m.waiting, 1); assert.equal(m.target, true); assert.equal(m.title, '탭 제목');
assert.equal(m.ackMs, 0, '미러의 9번째 필드는 ack_ms(반영된 버튼 요청의 ms)');
assert.equal(fx.parseMirror('agent:a:l:1:2:0:0:1:777:제목: 콜론').ackMs, 777);
assert.equal(fx.parseMirror('agent:a:l:1:2:0:0:1:777:제목: 콜론').title, '제목: 콜론', 'title은 항상 맨 뒤 — 나머지를 다 이어 붙인다');
assert.equal(fx.parseMirror('garbage'), null);
assert.equal(fx.parseMirror('agent:a:l:1:2:0:0:1:제목'), null, 'ack_ms 없는 옛 형식은 안 받는다');

// ---- 작업 탭: agent 소유 → 방패·디밍·알약 켜짐, 버튼 문구
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999000:0:0:1:0:제목');
let s = fx.state();
assert.equal(s.owner, 'agent'); assert.equal(s.target, true);
assert.equal(s.shield, 'auto', '입력 도구 구간 밖에서는 방패가 실제 마우스를 삼킨다');
assert.equal(s.dim, true);
assert.deepEqual(s.buttons, ['내가 조작하기', '중단']);
assert.equal(s.pillStatus, 'AI가 조작 중 · claude-%1');
assert.equal(s.buttonsClickable, true, '입력 도구 구간 밖에선 버튼을 누를 수 있어야 한다');

// ---- render() 경로: 실제로 그려진 DOM이 state()의 계산과 일치하는지
fx.render();
assert.equal(shieldStyle(), 'auto');
assert.deepEqual(buttonEls().map(b => b.textContent), ['내가 조작하기', '중단'], '알약에 실제 버튼 엘리먼트가 문구대로 생겨야 한다');
assert.equal(pillBtnsEl().style.pointerEvents, 'auto', '입력 구간 밖에선 버튼 컨테이너가 클릭을 받아야 한다');
function shieldStyle() { return findByTag(hostEl(), 'DIV').find(d => d.style && 'pointerEvents' in d.style && d !== pillBtnsEl())?.style.pointerEvents; }

// ---- render()를 같은 상태로 두 번 불러도 버튼 엘리먼트를 다시 만들지 않는다(게이트 리뷰 지적 1) —
// 그래야 CDP mousedown~mouseup 사이에 프록시가 미러를 다시 써도 버튼이 안 사라진다.
const before1 = buttonEls();
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999001:0:0:1:0:제목'); // 프록시가 미러를 다시 쓴 것과 같은 상황(같은 버튼 구성)
fx.render();
const after1 = buttonEls();
assert.equal(before1.length, 2); assert.equal(after1.length, 2);
assert.equal(before1[0], after1[0], '버튼 목록이 안 바뀌면 같은 엘리먼트를 유지해야 한다(클릭 씹힘 방지)');
assert.equal(before1[1], after1[1], '버튼 목록이 안 바뀌면 같은 엘리먼트를 유지해야 한다(클릭 씹힘 방지)');

// ---- 실제 버튼 엘리먼트를 클릭하면(스텁의 click()이 리스너를 돈다) 요청 속성이 써진다
after1[1].click(); // '중단'
assert.match(html.getAttribute('data-agentlayer-request'), /^stop:\d+$/, '실제 DOM 버튼 클릭이 리스너를 태워야 한다');

// ---- 입력 도구 구간: 방패 내림, 커서 라벨, 그리고 알약 버튼은 클릭을 막는다(게이트 리뷰 지적 2 —
// 에이전트의 CDP 합성 클릭이 화면 하단 버튼에 먹히면 안 된다)
html.setAttribute('data-agentlayer-fx', 'on:click:1');
s = fx.state();
assert.equal(s.shield, 'none', '클릭 구간엔 방패를 내려 CDP 입력이 닿게');
assert.equal(s.cursorLabel, '클릭');
assert.equal(s.buttonsClickable, false, '입력 구간엔 알약 버튼이 클릭을 막아야 한다');
fx.render();
assert.equal(pillBtnsEl().style.pointerEvents, 'none', '입력 구간엔 버튼 컨테이너의 pointer-events가 none이어야 한다');

html.setAttribute('data-agentlayer-fx', 'on:take_snapshot:2');
s = fx.state();
assert.equal(s.shield, 'auto', '읽기 도구 구간엔 방패 유지');
assert.equal(s.cursorLabel, '읽는 중');
assert.equal(s.buttonsClickable, true, '읽기 도구는 입력 구간이 아니므로 버튼을 다시 누를 수 있다');
html.setAttribute('data-agentlayer-fx', 'off:3');

// ---- 커서는 입력 구간(LINGER 포함)의 마우스 이벤트만 따라가고, LINGER가 끝난 뒤엔 무시한다
// (게이트 리뷰 지적 6 — 예전엔 setTimeout이 즉시 실행돼 이 구분이 테스트되지 않았다)
html.setAttribute('data-agentlayer-fx', 'on:click:4');
window.listeners.mousemove.forEach(cb => cb({ clientX: 50, clientY: 60 }));
assert.deepEqual(fx.state().cursor, { x: 50, y: 60 });

html.setAttribute('data-agentlayer-fx', 'off:5'); // LINGER(2.5초) 타이머가 걸릴 뿐, fxOn은 아직 유지된다
window.listeners.mousemove.forEach(cb => cb({ clientX: 70, clientY: 80 }));
assert.deepEqual(fx.state().cursor, { x: 70, y: 80 }, 'LINGER가 끝나기 전(off 직후)엔 아직 입력 구간으로 보고 커서가 따라간다');

__advance(2500); // LINGER 만료 → fxOn 꺼짐
window.listeners.mousemove.forEach(cb => cb({ clientX: 500, clientY: 600 }));
assert.deepEqual(fx.state().cursor, { x: 70, y: 80 }, 'LINGER가 끝난 뒤엔 구간 밖(사용자 마우스)이므로 무시한다');

// ---- OWNER 미러가 (FX 값은 그대로 둔 채) 다시 쓰여도 LINGER 타이머가 재무장되면 안 된다
// (게이트 리뷰 지적 3 — 프록시가 500ms~마다 미러를 다시 쓰는데, 호출 간격이 2.5초보다 짧으면
// 예전 코드는 매번 clearTimeout+setTimeout으로 다시 무장해 fxOn이 영원히 안 꺼졌다)
html.setAttribute('data-agentlayer-fx', 'on:click:6');
assert.equal(fx.state().shield, 'none', '입력 구간 진입 확인');
html.setAttribute('data-agentlayer-fx', 'off:7'); // LINGER 타이머 1회 무장(이 시점 기준 +2500ms에 꺼져야 한다)
__advance(1000); // 아직 LINGER 안(1초 경과) — 이 사이에 프록시가 미러를 여러 번 다시 쓴다고 가정
// 재무장 버그가 있었다면 아래 두 줄이 "무장 시점"을 이 시점(+1000ms)으로 밀어놓아, 이후
// +1500ms만 더 지나도(총 2500ms) 아직 안 꺼진 것처럼 보인다 — 그래서 시간차를 둬야 버그와
// 수정을 구분할 수 있다(동시에 다시 쓰면 우연히 같은 시각에 만료돼 버그를 못 잡는다).
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999002:0:0:1:0:제목'); // FX는 안 건드리고 OWNER만 다시 씀
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999003:0:0:1:0:제목'); // 한 번 더(프록시가 여러 번 다시 쓰는 상황)
__advance(1500); // 최초 무장 시점 기준 총 2500ms 경과 — 재무장됐다면(버그) 아직 안 꺼져 있어야 한다
assert.equal(fx.state().shield, 'auto', 'OWNER만 다시 써도 LINGER가 재무장되면 안 된다 — 최초 off 시점 기준으로 꺼져야 한다');

// ---- 버튼 클릭 → 요청 속성(clickButton 훅 경유)
fx.clickButton('내가 조작하기');
assert.match(html.getAttribute('data-agentlayer-request'), /^user:\d+$/);
fx.clickButton('중단');
assert.match(html.getAttribute('data-agentlayer-request'), /^stop:\d+$/);

// ---- user 소유: 방패 없음, 돌려주기 버튼, 대기 수
html.setAttribute('data-agentlayer-owner', 'user:claude-%1:검색:1000:999000:0:2:1:0:제목');
s = fx.state();
assert.equal(s.shield, 'none'); assert.equal(s.dim, false);
assert.deepEqual(s.buttons, ['AI에게 돌려주기']);
assert.equal(s.pillStatus, '내가 조작 중 · 대기 중인 호출 2');
html.setAttribute('data-agentlayer-owner', 'user:claude-%1:검색:1000:999000:1:0:1:0:제목');
assert.equal(fx.state().pillStatus, '중단됨');
fx.clickButton('AI에게 돌려주기');
assert.match(html.getAttribute('data-agentlayer-request'), /^agent:\d+$/);

// ---- user 소유인데 작업 탭을 모를 때(target=0): 그래도 모든 탭에 알약·「AI에게 돌려주기」가
// 떠야 한다(최종 리뷰 CRITICAL 1 — 예전엔 띠만 떠서 제어권을 돌려줄 방법이 없었다).
html.setAttribute('data-agentlayer-owner', 'user:claude-%1:검색:1000:999000:0:1:0:0:');
s = fx.state();
assert.equal(s.pill, true, 'user 소유면 target과 무관하게 알약이 뜬다');
assert.deepEqual(s.buttons, ['AI에게 돌려주기']);
assert.equal(s.banner, '', 'user 소유엔 "다른 탭에서 작업 중" 띠를 띄우지 않는다');
assert.equal(s.buttonsClickable, true, 'user 소유면 언제나 버튼을 누를 수 있어야 한다');
fx.render();
assert.deepEqual(buttonEls().map(b => b.textContent), ['AI에게 돌려주기'], '실제 DOM에도 돌려주기 버튼이 있어야 한다');
buttonEls()[0].click();
assert.match(html.getAttribute('data-agentlayer-request'), /^agent:\d+$/, 'target=0 탭에서도 돌려주기가 눌린다');
// fx 신호가 남아 있어도(입력 도구 구간처럼 보여도) user 소유에선 버튼을 막지 않는다
html.setAttribute('data-agentlayer-fx', 'on:click:8');
assert.equal(fx.state().buttonsClickable, true, 'user 소유 중 fx 신호가 와도 버튼은 살아 있어야 한다');
html.setAttribute('data-agentlayer-fx', 'off:9');
__advance(2600);

// ---- 타 탭(target=0): 띠만
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999000:0:0:0:0:JustWatch 신작');
s = fx.state();
assert.equal(s.shield, 'none'); assert.equal(s.dim, false); assert.equal(s.pill, false);
assert.equal(s.banner, 'AI가 다른 탭에서 작업 중 · JustWatch 신작');

// ---- 제목을 모르는 경우(pageId 없는 호출): 구분자(·)가 덩그러니 남으면 안 된다(최종 리뷰 MINOR a)
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999000:0:0:0:0:');
assert.equal(fx.state().banner, 'AI가 다른 탭에서 작업 중', '제목이 비면 구분자도 붙이지 않는다');

// ---- 자체 만료: last_ms + 20초 지나면 idle로 본다(프록시가 다 꺼진 경우) — 갭이 100초로
// 넉넉해 지금까지의 __advance 누적(수 초)과 무관하게 항상 만료 조건을 넘는다.
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:900000:0:0:1:0:제목');
s = fx.state();
assert.equal(s.owner, 'idle'); assert.equal(s.shield, 'none'); assert.equal(s.pill, false);

// ---- idle
html.setAttribute('data-agentlayer-owner', 'idle:::0:0:0:0:0:0:');
s = fx.state();
assert.equal(s.owner, 'idle'); assert.equal(s.banner, '');

// ---- 페이지가 host를 지워도(document.write, body.innerHTML= 등) 다음 render에서 되살아난다
// (게이트 리뷰 지적 4)
const oldHost = hostEl();
assert.ok(oldHost, '오버레이 host가 붙어 있어야 함');
oldHost.remove();
assert.equal(body.children.includes(oldHost), false, 'host가 body에서 제거됨');
assert.equal(oldHost.isConnected, false, '제거된 host는 isConnected가 false여야 한다');
fx.render();
const newHost = hostEl();
assert.ok(newHost, 'render 후 host가 다시 붙어야 함');
assert.notEqual(newHost, oldHost, '이전 host를 재사용하지 않고 새로 만들어야 한다');

// ---- 연출 상태 속성: CSS가 반응하는 stage의 data-mode/data-phase/data-stopped와 일회성 효과
const byClass = (node, cls, acc = []) => {
  if (!node) return acc;
  if ((node.attrs?.class || '').split(' ').includes(cls)) acc.push(node);
  (node.children || []).forEach(c => byClass(c, cls, acc));
  return acc;
};
const stageEl = () => byClass(hostEl(), 'al-stage')[0];
const fxlEl = () => byClass(hostEl(), 'al-fxl')[0];
assert.ok(stageEl(), 'host 안에 stage가 있어야 한다');
assert.equal(stageEl().getAttribute('data-mode'), 'idle');

html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999100:0:0:1:0:제목');
assert.equal(stageEl().getAttribute('data-mode'), 'agent');
assert.equal(stageEl().getAttribute('data-phase'), 'wait', '도구 호출 사이엔 wait');
assert.equal(byClass(fxlEl(), 'al-boot').length, 1, 'idle→agent 전환엔 부트 스윕이 한 번 재생된다');
__advance(1300);
assert.equal(byClass(fxlEl(), 'al-boot').length, 0, '부트 스윕은 끝나면 스스로 떨어진다');

// 멱등: 같은 상태로 미러가 다시 써지면 stage 속성을 다시 쓰지 않는다
let stageWrites = 0;
const origSet = stageEl().setAttribute;
stageEl().setAttribute = (k, v) => { stageWrites++; origSet(k, v); };
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999101:0:0:1:0:제목');
fx.render();
assert.equal(stageWrites, 0, '값이 안 바뀌면 stage 속성도 다시 쓰지 않는다');
stageEl().setAttribute = origSet;

html.setAttribute('data-agentlayer-fx', 'on:take_snapshot:10');
assert.equal(stageEl().getAttribute('data-phase'), 'busy', '읽기 도구 구간은 busy(스캔 빔)');
html.setAttribute('data-agentlayer-fx', 'on:click:11');
assert.equal(stageEl().getAttribute('data-phase'), 'input', '입력 도구 구간은 input');

// 커서 꼬리: 커서와 같은 좌표를 받는다(전이 시간 차로 뒤처지는 건 CSS 몫)
window.listeners.mousemove.forEach(cb => cb({ clientX: 120, clientY: 140 }));
const tailEls = byClass(hostEl(), 'al-tail');
assert.equal(tailEls.length, 3);
assert.equal(byClass(hostEl(), 'al-cursor')[0].style.transform, 'translate(120px,140px)');
tailEls.forEach(t => assert.equal(t.style.transform, 'translate(120px,140px)', '꼬리도 같은 목표 좌표로 간다'));
assert.equal(byClass(hostEl(), 'al-pointer')[0].style.opacity, '1');

// 클릭 버스트: mousedown마다 하나씩 생겼다가 사라진다
window.listeners.mousedown.forEach(cb => cb({ clientX: 120, clientY: 140 }));
const bursts = byClass(fxlEl(), 'al-burst');
assert.equal(bursts.length, 1, 'mousedown이면 클릭 버스트가 생긴다');
assert.equal(bursts[0].style.left, '120px');
__advance(1000);
assert.equal(byClass(fxlEl(), 'al-burst').length, 0, '버스트는 끝나면 떨어진다');

// agent→user: 테두리 해제 연출, 모드 전환, 중단 표시
html.setAttribute('data-agentlayer-owner', 'user:claude-%1:검색:1000:999100:1:0:1:0:제목');
assert.equal(stageEl().getAttribute('data-mode'), 'user');
assert.equal(stageEl().getAttribute('data-phase'), '');
assert.equal(stageEl().getAttribute('data-stopped'), '1');
assert.equal(byClass(fxlEl(), 'al-release').length, 1, 'agent를 떠날 땐 해제 연출');
html.setAttribute('data-agentlayer-fx', 'off:12');
__advance(3000);

// 다른 탭 배너, idle
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999100:0:0:0:0:JustWatch 신작');
assert.equal(stageEl().getAttribute('data-mode'), 'other');
assert.equal(byClass(hostEl(), 'al-banner-text')[0].textContent, 'AI가 다른 탭에서 작업 중 · JustWatch 신작');
html.setAttribute('data-agentlayer-owner', 'idle:::0:0:0:0:0:0:');
assert.equal(stageEl().getAttribute('data-mode'), 'idle');
assert.equal(byClass(fxlEl(), 'al-boot').length, 0, 'agent가 아닌 전환엔 부트 스윕이 없다');

// 파일 크기 상한(콘텐츠 스크립트는 모든 페이지에 주입된다)
assert.ok(Buffer.byteLength(src) <= 40 * 1024, `content.js는 40KB 이하여야 함(현재 ${Buffer.byteLength(src)}B)`);

// ---- fx/sync.js — 프록시가 탭마다 한 번 Eval하는 스크립트(미러 쓰기 + fx 신호 + 요청 회수).
// ack 규칙(최종 리뷰 IMPORTANT 2): 읽으면서 지우지 않는다 — ack된 요청만 지우고, ack보다
// 새 요청만 돌려준다. 그래야 예산을 넘겨 버려진 탭의 클릭이 DOM에서만 사라지는 일이 없다.
const syncSrc = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'sync.js'), 'utf8');
// rod가 Eval에 넘길 때와 같은 모양으로 감싸 본다(page_eval.go formatToJSFunc) —
// 앞머리 주석 때문에 구문이 깨지면 실브라우저에서만 터지므로 여기서 잡는다.
const sync = new Function('return function() { return (' + syncSrc.trim() + ').apply(this, arguments) }')();
const OWNER_ATTR = 'data-agentlayer-owner', REQ_ATTR = 'data-agentlayer-request', FX_ATTR = 'data-agentlayer-fx';
const ackMirror = 'user:claude-%1:검색:1000:999000:0:1:1:500:제목';

html.setAttribute(REQ_ATTR, 'agent:400'); // ack(500)보다 오래된 = 이미 반영된 요청
let ret = sync(ackMirror, OWNER_ATTR, REQ_ATTR, FX_ATTR, '', 500);
assert.equal(ret, '', 'ack된 요청은 다시 돌려주지 않는다');
assert.equal(html.getAttribute(REQ_ATTR), null, 'ack된 요청만 DOM에서 지운다');
assert.equal(fx.parseMirror(html.getAttribute(OWNER_ATTR)).ackMs, 500, '미러가 ack를 싣고 내려간다');

html.setAttribute(REQ_ATTR, 'agent:900'); // ack보다 새 요청
ret = sync(ackMirror, OWNER_ATTR, REQ_ATTR, FX_ATTR, 'on:click:9', 500);
assert.equal(ret, 'agent:900', 'ack보다 새 요청은 돌려준다');
assert.equal(html.getAttribute(REQ_ATTR), 'agent:900', '아직 반영 전이므로 지우지 않는다(느린 탭이어도 다음 왕복에 다시 잡힌다)');
assert.equal(html.getAttribute(FX_ATTR), 'on:click:9', 'fx 신호도 같은 왕복에서 쓴다(왕복 1회)');

ret = sync(ackMirror, OWNER_ATTR, REQ_ATTR, FX_ATTR, '', 900); // 프록시가 반영해 ack가 올라감
assert.equal(ret, '', '반영된 뒤엔 같은 요청을 두 번 주지 않는다');
assert.equal(html.getAttribute(REQ_ATTR), null, 'ack가 따라잡으면 지운다');
html.removeAttribute(REQ_ATTR);

console.log('content_test.mjs OK');
