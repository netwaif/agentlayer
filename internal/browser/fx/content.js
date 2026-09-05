// AgentLayer FX — 에이전트가 페이지를 조작하는 동안만 보이는 오버레이.
// 신호: agentlayer mcp-serve가 CDP로 <html data-agentlayer-fx="on:<tool>:<ts>|off:<ts>">를 쓴다.
// 좌표: CDP가 합성한 마우스 이벤트가 페이지에 그대로 도착하므로 capture 단계에서 읽는다.
(() => {
  if (window.top !== window) return;
  const ATTR = 'data-agentlayer-fx';
  const ACCENT = '217,119,87'; // 테라코타 #d97757 (하우스 팔레트)
  const CREAM = '#faf9f5';
  let host, glow, cursor, hl, pill, active = false, offTimer, hideTimer, hlTimer, hasPos = false;
  const LINGER = 2500; // 마지막 응답 뒤 효과를 유지하는 시간(ms) — 도구 호출 하나는 수백 ms라 즉시 끄면 눈에 안 띈다

  const css = (el, s) => { el.style.cssText = s; return el; };
  const ensure = () => {
    if (host) return;
    host = css(document.createElement('div'),
      'position:fixed;inset:0;pointer-events:none;z-index:2147483647;contain:strict;');
    host.id = '__agentlayer_fx';
    // 접근성 트리에서 제외 — 에이전트의 take_snapshot에 'AI' 뱃지가 섞이지 않게
    host.setAttribute('aria-hidden', 'true');
    host.setAttribute('inert', '');
    glow = css(document.createElement('div'),
      `position:absolute;inset:0;opacity:0;transition:opacity .35s ease;` +
      `box-shadow:inset 0 0 0 3px rgba(${ACCENT},.95),inset 0 0 28px rgba(${ACCENT},.5),inset 0 0 90px rgba(${ACCENT},.22);`);
    hl = css(document.createElement('div'),
      `position:absolute;opacity:0;transition:opacity .25s ease;border-radius:6px;` +
      `box-shadow:0 0 0 2px rgba(${ACCENT},.9),0 0 14px rgba(${ACCENT},.45);`);
    cursor = css(document.createElement('div'),
      'position:absolute;left:0;top:0;width:0;height:0;opacity:0;' +
      'transition:transform .25s cubic-bezier(.2,.8,.2,1),opacity .3s ease;will-change:transform;');
    cursor.innerHTML =
      `<svg width="30" height="36" viewBox="0 0 22 26" style="position:absolute;left:-3px;top:-3px;` +
      `filter:drop-shadow(0 1px 2px rgba(0,0,0,.45))">` +
      `<path d="M3 2 L19 13 L11.5 14.5 L15.5 23 L12.5 24.2 L8.5 15.8 L3 21 Z" fill="${CREAM}" stroke="#1f1e1d" stroke-width="1.4" stroke-linejoin="round"/></svg>` +
      `<div style="position:absolute;left:22px;top:26px;padding:2px 7px;border-radius:999px;` +
      `background:#d97757;color:${CREAM};font:700 11px/15px -apple-system,system-ui,sans-serif;letter-spacing:.4px;` +
      `box-shadow:0 1px 3px rgba(0,0,0,.4);white-space:nowrap">AI</div>`;
    pill = css(document.createElement('div'),
      `position:absolute;left:50%;top:10px;transform:translate(-50%,-6px);opacity:0;transition:opacity .3s ease,transform .3s ease;` +
      `padding:4px 12px;border-radius:999px;background:#d97757;color:${CREAM};` +
      `font:600 12px/16px -apple-system,system-ui,sans-serif;letter-spacing:.3px;box-shadow:0 2px 8px rgba(0,0,0,.35);white-space:nowrap`);
    pill.textContent = 'AI 조작 중';
    host.append(glow, hl, cursor, pill);
    (document.body || document.documentElement).appendChild(host);
  };

  const moveTo = (x, y) => {
    ensure();
    hasPos = true;
    cursor.style.transform = `translate(${x}px,${y}px)`;
    if (active) cursor.style.opacity = '1';
  };
  const ripple = (x, y) => {
    ensure();
    const r = css(document.createElement('div'),
      `position:absolute;left:${x - 18}px;top:${y - 18}px;width:36px;height:36px;border-radius:50%;` +
      `border:3px solid rgba(${ACCENT},.95);transform:scale(.3);opacity:.95;` +
      'transition:transform .6s ease-out,opacity .6s ease-out;');
    host.appendChild(r);
    requestAnimationFrame(() => { r.style.transform = 'scale(1.7)'; r.style.opacity = '0'; });
    setTimeout(() => r.remove(), 700);
  };
  const highlight = (el) => {
    if (!(el instanceof Element) || el === document.body || el === document.documentElement) return;
    ensure();
    const b = el.getBoundingClientRect();
    if (!b.width || !b.height) return;
    hl.style.left = b.left + 'px'; hl.style.top = b.top + 'px';
    hl.style.width = b.width + 'px'; hl.style.height = b.height + 'px';
    hl.style.opacity = '1';
    clearTimeout(hlTimer);
    hlTimer = setTimeout(() => { hl.style.opacity = '0'; }, 1500);
  };

  const show = (on) => {
    active = on;
    glow.style.opacity = on ? '1' : '0';
    pill.style.opacity = on ? '1' : '0';
    pill.style.transform = on ? 'translate(-50%,0)' : 'translate(-50%,-6px)';
    if (on) {
      if (hasPos) cursor.style.opacity = '1';
    } else {
      cursor.style.opacity = '0';
      hl.style.opacity = '0';
    }
  };
  // on은 즉시, off는 LINGER 뒤에 — 그 사이 새 on이 오면 취소(연속 호출 동안 깜빡이지 않는다)
  const setActive = (on) => {
    ensure();
    clearTimeout(offTimer);
    if (on) { show(true); return; }
    offTimer = setTimeout(() => show(false), LINGER);
  };

  const root = document.documentElement;
  new MutationObserver(() => {
    const v = root.getAttribute(ATTR) || '';
    if (v.startsWith('on:')) setActive(true);
    else if (v.startsWith('off:')) setActive(false);
  }).observe(root, { attributes: true, attributeFilter: [ATTR] });

  window.addEventListener('mousemove', (e) => { if (active) moveTo(e.clientX, e.clientY); }, true);
  window.addEventListener('mousedown', (e) => { if (active) { moveTo(e.clientX, e.clientY); ripple(e.clientX, e.clientY); } }, true);
  window.addEventListener('focusin', (e) => { if (active) highlight(e.target); }, true);
  // 조작 중 스크롤·리사이즈로 하이라이트가 어긋나면 감춘다 (좌표가 낡음)
  window.addEventListener('scroll', () => { if (hl) hl.style.opacity = '0'; }, true);
})();
