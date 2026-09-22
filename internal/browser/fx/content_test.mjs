// internal/browser/fx/content_test.mjs — node로 DOM을 최소 흉내 내어 콘텐츠 스크립트를 검사한다.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import assert from 'node:assert/strict';

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'content.js'), 'utf8');

// ---- 최소 DOM 스텁: 속성·자식·스타일·이벤트만
function el(tag) {
  const e = { tagName: tag.toUpperCase(), style: {}, children: [], attrs: {}, listeners: {}, textContent: '', innerHTML: '' };
  e.setAttribute = (k, v) => { e.attrs[k] = String(v); observers.forEach(o => o.target === e && o.cb()); };
  e.getAttribute = (k) => (k in e.attrs ? e.attrs[k] : null);
  e.removeAttribute = (k) => { delete e.attrs[k]; };
  e.appendChild = (c) => { e.children.push(c); c.parentElement = e; return c; };
  e.append = (...cs) => cs.forEach(e.appendChild);
  e.remove = () => { if (e.parentElement) e.parentElement.children = e.parentElement.children.filter(c => c !== e); };
  e.addEventListener = (t, cb) => { (e.listeners[t] ||= []).push(cb); };
  e.click = () => (e.listeners.click || []).forEach(cb => cb({ stopPropagation() {}, preventDefault() {} }));
  e.getBoundingClientRect = () => ({ left: 0, top: 0, width: 10, height: 10 });
  Object.defineProperty(e.style, 'cssText', { set(v) { v.split(';').forEach(kv => { const [k, val] = kv.split(':'); if (k) e.style[k.trim()] = (val || '').trim(); }); }, get() { return ''; } });
  return e;
}
const observers = [];
globalThis.MutationObserver = class { constructor(cb) { this.cb = cb; } observe(t) { this.target = t; observers.push(this); } };
const html = el('html'), body = el('body'); html.appendChild(body);
globalThis.document = { documentElement: html, body, createElement: el, getElementById: () => null };
globalThis.window = { top: null, addEventListener: (t, cb) => { (window.listeners ||= {})[t] ||= []; window.listeners[t].push(cb); }, innerWidth: 1000, innerHeight: 800 };
window.top = window;
globalThis.requestAnimationFrame = (cb) => cb();
globalThis.setTimeout = (cb) => { cb(); return 0; }; // 타이머는 즉시 — LINGER·만료는 별도 함수로 검사
globalThis.clearTimeout = () => {};
// content.js의 `typeof setInterval === 'function'` 가드는 브라우저 스텁에 setInterval이
// 없는 경우를 막으려는 것이지만, Node 자체는 전역 setInterval을 갖고 있어 가드를 통과한다.
// 실제 타이머를 걸면 프로세스가 안 끝나므로(5초 간격) 무동작으로 덮어써 스텁을 완결한다.
globalThis.setInterval = () => 0;
globalThis.clearInterval = () => {};
globalThis.Date = class extends Date { static now() { return 1_000_000; } };

new Function(src)();
const fx = window.__agentlayerFx;
assert.ok(fx, 'content.js가 window.__agentlayerFx를 노출해야 함');

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

// ---- 입력 도구 구간: 방패 내림, 커서 라벨
html.setAttribute('data-agentlayer-fx', 'on:click:1');
s = fx.state();
assert.equal(s.shield, 'none', '클릭 구간엔 방패를 내려 CDP 입력이 닿게');
assert.equal(s.cursorLabel, '클릭');
html.setAttribute('data-agentlayer-fx', 'on:take_snapshot:2');
s = fx.state();
assert.equal(s.shield, 'auto', '읽기 도구 구간엔 방패 유지');
assert.equal(s.cursorLabel, '읽는 중');
html.setAttribute('data-agentlayer-fx', 'off:3');

// ---- 커서는 입력 구간의 마우스 이벤트만 따라감
html.setAttribute('data-agentlayer-fx', 'on:click:4');
window.listeners.mousemove.forEach(cb => cb({ clientX: 50, clientY: 60 }));
assert.deepEqual(fx.state().cursor, { x: 50, y: 60 });
html.setAttribute('data-agentlayer-fx', 'off:5');
window.listeners.mousemove.forEach(cb => cb({ clientX: 500, clientY: 600 }));
assert.deepEqual(fx.state().cursor, { x: 50, y: 60 }, '구간 밖(사용자 마우스)은 무시');

// ---- 버튼 클릭 → 요청 속성
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

// ---- 자체 만료: last_ms + 20초 지나면 idle로 본다(프록시가 다 꺼진 경우)
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:900000:0:0:1:제목'); // now=1,000,000 → 100초 경과
s = fx.state();
assert.equal(s.owner, 'idle'); assert.equal(s.shield, 'none'); assert.equal(s.pill, false);

// ---- idle
html.setAttribute('data-agentlayer-owner', 'idle:::0:0:0:0:0:');
s = fx.state();
assert.equal(s.owner, 'idle'); assert.equal(s.banner, '');

console.log('content_test.mjs OK');
