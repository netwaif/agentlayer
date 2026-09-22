// AgentLayer FX — 에이전트 브라우저 오버레이(스펙 3절).
// 신호 셋: data-agentlayer-fx(도구 구간 on/off, 프록시가 씀), data-agentlayer-owner(소유권 미러, 프록시가 씀),
// data-agentlayer-request(버튼 요청, 여기서 씀 → 프록시가 회수).
(() => {
  if (window.top !== window) return;
  const FX = 'data-agentlayer-fx', OWNER = 'data-agentlayer-owner', REQ = 'data-agentlayer-request';
  const ACCENT = '217,119,87', TERRA = '#d97757', CREAM = '#faf9f5', INK = '#1f1e1d';
  const LINGER = 2500, EXPIRY = 20000;
  const INPUT = new Set(['click', 'hover', 'drag', 'fill', 'fill_form', 'type_text', 'press_key', 'upload_file']);
  const LABELS = { click: '클릭', hover: '가리키는 중', fill: '입력 중', fill_form: '입력 중', type_text: '입력 중', press_key: '입력 중',
    upload_file: '입력 중', drag: '끌기', navigate_page: '이동 중', new_page: '이동 중', take_snapshot: '읽는 중', take_screenshot: '읽는 중',
    wait_for: '기다리는 중', evaluate_script: '확인 중' };

  const st = { owner: 'idle', agent: '', label: '', lastMs: 0, stopped: false, waiting: 0, target: false, title: '',
    fxOn: false, tool: '', cursor: null, offTimer: 0, lastFxVal: undefined };

  const parseMirror = (v) => {
    if (!v) return null;
    const p = v.split(':');
    if (p.length < 9) return null;
    const [owner, agent, label, sinceMs, lastMs, stopped, waiting, target] = p;
    if (!['agent', 'user', 'idle'].includes(owner)) return null;
    return { owner, agent, label, sinceMs: +sinceMs, lastMs: +lastMs, stopped: stopped === '1', waiting: +waiting || 0, target: target === '1', title: p.slice(8).join(':') };
  };

  // 유효 소유권 — 프록시가 전부 꺼지면 아무도 미러를 안 고치므로 스스로 만료를 계산한다.
  const effOwner = () => (st.owner === 'agent' && Date.now() - st.lastMs >= EXPIRY) ? 'idle' : st.owner;

  // ---- DOM
  let host, shield, dim, glow, hl, cursor, cursorLabel, pill, pillTitle, pillStatus, pillBtns, banner;
  // render()의 재도색 캐시 — 값이 바뀌지 않으면 DOM에 다시 쓰지 않는다(버튼을 매 뮤테이션마다
  // 다시 만들면 mousedown~mouseup 사이에 버튼이 사라져 클릭이 씹힌다: 게이트 리뷰 지적).
  let cache = null;
  const resetCache = () => { cache = { shield: null, dim: null, pill: null, pillTitle: null, pillStatus: null, buttonsKey: null, buttonsClickable: null, banner: null, cursorLabel: null, cursorOn: null, hlHidden: null }; };
  resetCache();
  const css = (e, s) => { e.style.cssText = s; return e; };
  const btn = (text) => {
    const b = css(document.createElement('button'),
      `cursor:pointer;border:0;border-radius:999px;padding:6px 12px;margin-left:6px;` +
      `font:600 12px/16px -apple-system,system-ui,sans-serif;background:${text === '중단' ? 'rgba(255,255,255,.12)' : CREAM};color:${text === '중단' ? CREAM : INK}`);
    // pointer-events는 여기서 고정하지 않는다 — pillBtns의 값을 그대로 물려받아야
    // 입력 도구 구간에서 컨테이너를 none으로 내리면 버튼도 같이 막힌다(게이트 리뷰 지적:
    // 에이전트 CDP 클릭이 화면 하단 알약 버튼에 먹히던 문제).
    b.textContent = text;
    b.setAttribute('type', 'button');
    b.addEventListener('click', (e) => { e.stopPropagation(); e.preventDefault(); request(text); });
    return b;
  };
  const ensure = () => {
    if (host && host.isConnected) return; // 페이지가 host를 지워도(document.write 등) 다음 호출에서 되살린다
    host = null;
    host = css(document.createElement('div'), 'position:fixed;inset:0;pointer-events:none;z-index:2147483647;contain:strict;');
    host.id = '__agentlayer_fx';
    host.setAttribute('aria-hidden', 'true'); // take_snapshot에 안 섞이게. inert는 붙이지 않는다(버튼이 눌려야 함)
    shield = css(document.createElement('div'), 'position:absolute;inset:0;pointer-events:none;');
    dim = css(document.createElement('div'),
      `position:absolute;inset:0;opacity:0;transition:opacity .35s ease;background:` +
      `radial-gradient(rgba(255,255,255,.06) 1px, transparent 1.2px) 0 0/14px 14px, rgba(0,0,0,.35);`);
    glow = css(document.createElement('div'),
      `position:absolute;inset:0;opacity:0;transition:opacity .35s ease;` +
      `box-shadow:inset 0 0 0 3px rgba(${ACCENT},.95),inset 0 0 60px rgba(${ACCENT},.45),inset 0 0 160px rgba(${ACCENT},.2);`);
    hl = css(document.createElement('div'),
      `position:absolute;opacity:0;transition:opacity .25s ease;border-radius:6px;box-shadow:0 0 0 2px rgba(${ACCENT},.9),0 0 14px rgba(${ACCENT},.45);`);
    cursor = css(document.createElement('div'),
      'position:absolute;left:0;top:0;width:0;height:0;opacity:0;transition:transform .25s cubic-bezier(.2,.8,.2,1),opacity .3s ease;will-change:transform;');
    cursor.innerHTML =
      `<svg width="30" height="36" viewBox="0 0 22 26" style="position:absolute;left:-3px;top:-3px;filter:drop-shadow(0 1px 2px rgba(0,0,0,.45))">` +
      `<path d="M3 2 L19 13 L11.5 14.5 L15.5 23 L12.5 24.2 L8.5 15.8 L3 21 Z" fill="${CREAM}" stroke="${INK}" stroke-width="1.4" stroke-linejoin="round"/></svg>`;
    cursorLabel = css(document.createElement('div'),
      `position:absolute;left:24px;top:28px;padding:3px 9px;border-radius:999px;background:${TERRA};color:${CREAM};` +
      `font:700 11px/15px -apple-system,system-ui,sans-serif;box-shadow:0 1px 3px rgba(0,0,0,.4);white-space:nowrap`);
    cursor.appendChild(cursorLabel);
    pill = css(document.createElement('div'),
      `position:absolute;left:50%;bottom:22px;transform:translate(-50%,8px);opacity:0;transition:opacity .3s ease,transform .3s ease;` +
      `display:flex;align-items:center;gap:10px;padding:10px 12px 10px 16px;border-radius:16px;background:rgba(31,30,29,.92);color:${CREAM};` +
      `font:500 12px/16px -apple-system,system-ui,sans-serif;box-shadow:0 8px 28px rgba(0,0,0,.45);white-space:nowrap;backdrop-filter:blur(8px)`);
    const spin = css(document.createElement('div'),
      `width:10px;height:10px;border-radius:50%;background:${TERRA};box-shadow:0 0 0 4px rgba(${ACCENT},.25);animation:__alpulse 1.2s ease-in-out infinite`);
    const txt = css(document.createElement('div'), 'display:flex;flex-direction:column;gap:2px');
    pillTitle = css(document.createElement('div'), 'font-weight:700;font-size:13px');
    pillStatus = css(document.createElement('div'), `color:${TERRA};font-weight:600`);
    txt.append(pillTitle, pillStatus);
    // pillBtns 자체는 pointer-events를 명시하지 않는다(호스트로부터 none을 물려받는 게 기본) —
    // render()가 매번 buttonsClickable에 따라 auto/none을 명시적으로 써준다.
    pillBtns = css(document.createElement('div'), 'display:flex;align-items:center;margin-left:6px');
    pill.append(spin, txt, pillBtns);
    banner = css(document.createElement('div'),
      `position:absolute;left:0;right:0;top:0;height:28px;opacity:0;transition:opacity .3s ease;display:flex;align-items:center;justify-content:center;` +
      `background:${TERRA};color:${CREAM};font:600 12px/16px -apple-system,system-ui,sans-serif;box-shadow:0 2px 8px rgba(0,0,0,.35)`);
    const style = document.createElement('style');
    style.textContent = '@keyframes __alpulse{0%,100%{transform:scale(1)}50%{transform:scale(1.35)}}';
    host.append(style, shield, dim, glow, hl, cursor, pill, banner);
    (document.body || document.documentElement).appendChild(host);
    resetCache(); // 새 엘리먼트는 기본 상태이므로 이전 캐시값과 비교하면 안 됨 — 다음 render가 전부 다시 씀
  };

  const request = (text) => {
    const kind = text === '내가 조작하기' ? 'user' : text === '중단' ? 'stop' : 'agent';
    document.documentElement.setAttribute(REQ, `${kind}:${Date.now()}`);
  };

  // ---- 표시 계산(테스트가 같은 함수를 본다) — 순수 함수, DOM을 건드리지 않는다
  const compute = () => {
    const owner = effOwner();
    const active = owner === 'agent' && st.target;
    const inputPhase = st.fxOn && INPUT.has(st.tool);
    const s = { owner, target: st.target, shield: 'none', dim: false, pill: false, banner: '', buttons: [],
      pillStatus: '', cursorLabel: '', cursor: st.cursor, buttonsClickable: !inputPhase };
    if (active) {
      s.shield = inputPhase ? 'none' : 'auto';
      s.dim = true; s.pill = true;
      s.buttons = ['내가 조작하기', '중단'];
      s.pillStatus = `AI가 조작 중 · ${st.agent}`;
      s.cursorLabel = st.fxOn ? (LABELS[st.tool] || '작업 중') : '';
    } else if (owner === 'user' && st.target) {
      s.pill = true;
      s.buttons = ['AI에게 돌려주기'];
      s.pillStatus = st.stopped ? '중단됨' : `내가 조작 중 · 대기 중인 호출 ${st.waiting}`;
    } else if (owner !== 'idle' && !st.target) {
      s.banner = `AI가 다른 탭에서 작업 중 · ${st.title}`;
    }
    return s;
  };

  // render()는 멱등이어야 한다 — 프록시가 미러를 500ms~ 간격으로 계속 다시 쓰고,
  // 뮤테이션옵저버가 매번 이 함수를 부르므로, 값이 안 바뀐 항목은 DOM에 다시 쓰지 않는다.
  // 특히 버튼 컨테이너를 매번 비웠다 새로 만들면 mousedown~mouseup 사이에 버튼이
  // 사라져 클릭이 씹힌다(게이트 리뷰 지적 1) — buttonsKey가 같으면 pillBtns를 건드리지 않는다.
  const render = () => {
    ensure();
    const s = compute();
    if (cache.shield !== s.shield) { shield.style.pointerEvents = s.shield; cache.shield = s.shield; }
    if (cache.dim !== s.dim) {
      dim.style.opacity = s.dim ? '1' : '0';
      glow.style.opacity = s.dim ? '1' : '0';
      cache.dim = s.dim;
    }
    if (cache.pill !== s.pill) {
      pill.style.opacity = s.pill ? '1' : '0';
      pill.style.transform = s.pill ? 'translate(-50%,0)' : 'translate(-50%,8px)';
      cache.pill = s.pill;
    }
    const title = st.label || '브라우저 작업';
    if (cache.pillTitle !== title) { pillTitle.textContent = title; cache.pillTitle = title; }
    if (cache.pillStatus !== s.pillStatus) { pillStatus.textContent = s.pillStatus; cache.pillStatus = s.pillStatus; }
    const buttonsKey = s.buttons.join('|');
    if (cache.buttonsKey !== buttonsKey) {
      pillBtns.innerHTML = '';
      s.buttons.forEach(b => pillBtns.appendChild(btn(b)));
      cache.buttonsKey = buttonsKey;
    }
    // 입력 도구 구간엔 알약 버튼의 pointer-events를 내려 에이전트의 CDP 합성 클릭이
    // 화면 하단 버튼에 먹히지 않게 한다(게이트 리뷰 지적 2). 버튼 자신은 pointer-events를
    // 고정하지 않으므로(위 btn() 참고) 이 컨테이너 값을 그대로 물려받는다.
    if (cache.buttonsClickable !== s.buttonsClickable) {
      pillBtns.style.pointerEvents = s.buttonsClickable ? 'auto' : 'none';
      cache.buttonsClickable = s.buttonsClickable;
    }
    if (cache.banner !== s.banner) {
      banner.textContent = s.banner;
      banner.style.opacity = s.banner ? '1' : '0';
      cache.banner = s.banner;
    }
    if (cache.cursorLabel !== s.cursorLabel) { cursorLabel.textContent = s.cursorLabel; cache.cursorLabel = s.cursorLabel; }
    const cursorOn = !!(s.owner === 'agent' && st.target && st.cursor);
    if (cache.cursorOn !== cursorOn) { cursor.style.opacity = cursorOn ? '1' : '0'; cache.cursorOn = cursorOn; }
    if (!s.dim) { if (cache.hlHidden !== true) { hl.style.opacity = '0'; cache.hlHidden = true; } }
    else { cache.hlHidden = false; }
  };

  // moveTo는 render()를 부르지 않는다 — 마우스가 움직일 때마다(뮤테이션과 무관하게) 풀
  // render를 돌리면 버튼이 계속 다시 만들어져 클릭이 씹힌다(게이트 리뷰 지적 1). 커서
  // 위치·표시 여부만 직접 반영한다. 호출 시점엔 inputPhase()가 이미 참이므로(owner=agent,
  // target=true) 커서를 보이는 상태로 둔다.
  const moveTo = (x, y) => { st.cursor = { x, y }; ensure(); cursor.style.transform = `translate(${x}px,${y}px)`; cursor.style.opacity = '1'; cache.cursorOn = true; };
  const ripple = (x, y) => {
    ensure();
    const r = css(document.createElement('div'),
      `position:absolute;left:${x - 18}px;top:${y - 18}px;width:36px;height:36px;border-radius:50%;border:3px solid rgba(${ACCENT},.95);` +
      'transform:scale(.3);opacity:.95;transition:transform .6s ease-out,opacity .6s ease-out;');
    host.appendChild(r);
    requestAnimationFrame(() => { r.style.transform = 'scale(1.7)'; r.style.opacity = '0'; });
    setTimeout(() => r.remove(), 700);
  };
  const highlight = (el) => {
    if (!(typeof Element !== 'undefined' && el instanceof Element) || el === document.body || el === document.documentElement) return;
    const b = el.getBoundingClientRect();
    if (!b.width || !b.height) return;
    ensure();
    hl.style.left = b.left + 'px'; hl.style.top = b.top + 'px'; hl.style.width = b.width + 'px'; hl.style.height = b.height + 'px';
    hl.style.opacity = '1';
    setTimeout(() => { hl.style.opacity = '0'; }, 1500);
  };

  const root = document.documentElement;
  const readOwner = () => {
    const m = parseMirror(root.getAttribute(OWNER));
    if (!m) { st.owner = 'idle'; st.target = false; return; }
    Object.assign(st, m);
  };
  // readFx()는 FX 속성 "값이 실제로 바뀌었을 때만" on/off를 처리한다. OWNER 미러가
  // 500ms~마다 다시 쓰이면 뮤테이션옵저버가 매번 이 함수를 부르는데, 예전 코드는 FX 값이
  // 그대로여도 매번 clearTimeout+setTimeout으로 LINGER를 재무장해 입력 호출 간격이
  // 2.5초보다 짧으면 fxOn이 영원히 꺼지지 않았다(게이트 리뷰 지적 3).
  const readFx = () => {
    const v = root.getAttribute(FX) || '';
    if (v === st.lastFxVal) return;
    st.lastFxVal = v;
    clearTimeout(st.offTimer);
    if (v.startsWith('on:')) { st.fxOn = true; st.tool = v.split(':')[1] || ''; }
    else if (v.startsWith('off:')) { st.offTimer = setTimeout(() => { st.fxOn = false; st.tool = ''; render(); }, LINGER); }
  };
  new MutationObserver(() => { readOwner(); readFx(); render(); }).observe(root, { attributes: true, attributeFilter: [OWNER, FX] });
  readOwner(); readFx();

  const inputPhase = () => st.fxOn && INPUT.has(st.tool) && effOwner() === 'agent' && st.target;
  window.addEventListener('mousemove', (e) => { if (inputPhase()) moveTo(e.clientX, e.clientY); }, true);
  window.addEventListener('mousedown', (e) => { if (inputPhase()) { moveTo(e.clientX, e.clientY); ripple(e.clientX, e.clientY); } }, true);
  window.addEventListener('focusin', (e) => { if (inputPhase()) highlight(e.target); }, true);
  window.addEventListener('scroll', () => { if (hl) hl.style.opacity = '0'; }, true);
  // 만료 자체 계산 — 프록시가 없을 때도 20초 뒤 내려가게 5초마다 다시 그린다
  if (typeof setInterval === 'function') setInterval(() => { if (st.owner !== 'idle') render(); }, 5000);

  window.__agentlayerFx = {
    parseMirror,
    state: () => (readOwner(), readFx(), compute()),
    clickButton: (t) => request(t),
    render: () => (readOwner(), readFx(), render()), // 테스트가 render() 경로(DOM 반영)를 직접 검증할 수 있게
  };
})();
