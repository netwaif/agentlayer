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
const m = fx.parseMirror('agent:claude-%1:JustWatch∶ 신작:1000:2000:0:1:1:탭 제목');
assert.equal(m.owner, 'agent'); assert.equal(m.agent, 'claude-%1'); assert.equal(m.label, 'JustWatch∶ 신작');
assert.equal(m.lastMs, 2000); assert.equal(m.stopped, false); assert.equal(m.waiting, 1); assert.equal(m.target, true); assert.equal(m.title, '탭 제목');
assert.equal(fx.parseMirror('garbage'), null);

// ---- 작업 탭: agent 소유 → 방패·디밍·알약 켜짐, 버튼 문구
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999000:0:0:1:제목');
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
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999001:0:0:1:제목'); // 프록시가 미러를 다시 쓴 것과 같은 상황(같은 버튼 구성)
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
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999002:0:0:1:제목'); // FX는 안 건드리고 OWNER만 다시 씀
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999003:0:0:1:제목'); // 한 번 더(프록시가 여러 번 다시 쓰는 상황)
__advance(1500); // 최초 무장 시점 기준 총 2500ms 경과 — 재무장됐다면(버그) 아직 안 꺼져 있어야 한다
assert.equal(fx.state().shield, 'auto', 'OWNER만 다시 써도 LINGER가 재무장되면 안 된다 — 최초 off 시점 기준으로 꺼져야 한다');

// ---- 버튼 클릭 → 요청 속성(clickButton 훅 경유)
fx.clickButton('내가 조작하기');
assert.match(html.getAttribute('data-agentlayer-request'), /^user:\d+$/);
fx.clickButton('중단');
assert.match(html.getAttribute('data-agentlayer-request'), /^stop:\d+$/);

// ---- user 소유: 방패 없음, 돌려주기 버튼, 대기 수
html.setAttribute('data-agentlayer-owner', 'user:claude-%1:검색:1000:999000:0:2:1:제목');
s = fx.state();
assert.equal(s.shield, 'none'); assert.equal(s.dim, false);
assert.deepEqual(s.buttons, ['AI에게 돌려주기']);
assert.equal(s.pillStatus, '내가 조작 중 · 대기 중인 호출 2');
html.setAttribute('data-agentlayer-owner', 'user:claude-%1:검색:1000:999000:1:0:1:제목');
assert.equal(fx.state().pillStatus, '중단됨');
fx.clickButton('AI에게 돌려주기');
assert.match(html.getAttribute('data-agentlayer-request'), /^agent:\d+$/);

// ---- 타 탭(target=0): 띠만
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999000:0:0:0:JustWatch 신작');
s = fx.state();
assert.equal(s.shield, 'none'); assert.equal(s.dim, false); assert.equal(s.pill, false);
assert.equal(s.banner, 'AI가 다른 탭에서 작업 중 · JustWatch 신작');

// ---- 자체 만료: last_ms + 20초 지나면 idle로 본다(프록시가 다 꺼진 경우) — 갭이 100초로
// 넉넉해 지금까지의 __advance 누적(수 초)과 무관하게 항상 만료 조건을 넘는다.
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:900000:0:0:1:제목');
s = fx.state();
assert.equal(s.owner, 'idle'); assert.equal(s.shield, 'none'); assert.equal(s.pill, false);

// ---- idle
html.setAttribute('data-agentlayer-owner', 'idle:::0:0:0:0:0:');
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

console.log('content_test.mjs OK');
