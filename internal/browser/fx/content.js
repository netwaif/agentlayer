// AgentLayer FX — 에이전트 브라우저 오버레이(스펙 3절).
// 신호 셋: data-agentlayer-fx(도구 구간 on/off, 프록시가 씀), data-agentlayer-owner(소유권 미러, 프록시가 씀),
// data-agentlayer-request(버튼 요청, 여기서 씀 → 프록시가 회수).
//
// 디자인: "AI 뷰파인더" — 화면 가장자리에 흐르는 웜 그라디언트 테두리와 그 위를 도는 빛줄기,
// 모서리 조준 브래킷, 가장자리만 어두운 비네트(가운데는 거의 그대로라 글이 읽힌다), 읽기 도구
// 구간엔 스캔 빔, 입력 도구 구간엔 혜성 꼬리 커서·클릭 버스트·포커스 락온. 전부 CSS 애니메이션
// (transform/opacity 위주)이고 JS는 상태 속성(data-mode/data-phase)만 바꾼다 — 프레임 루프 없음.
(() => {
  if (window.top !== window) return;
  const FX = 'data-agentlayer-fx', OWNER = 'data-agentlayer-owner', REQ = 'data-agentlayer-request';
  const LINGER = 2500, EXPIRY = 20000;
  const INPUT = new Set(['click', 'hover', 'drag', 'fill', 'fill_form', 'type_text', 'press_key', 'upload_file']);
  const LABELS = { click: '클릭', hover: '가리키는 중', fill: '입력 중', fill_form: '입력 중', type_text: '입력 중', press_key: '입력 중',
    upload_file: '입력 중', drag: '끌기', navigate_page: '이동 중', new_page: '이동 중', take_snapshot: '읽는 중', take_screenshot: '읽는 중',
    wait_for: '기다리는 중', evaluate_script: '확인 중' };

  const st = { owner: 'idle', agent: '', label: '', lastMs: 0, stopped: false, waiting: 0, target: false, ackMs: 0, title: '',
    fxOn: false, tool: '', cursor: null, offTimer: 0, lastFxVal: undefined };

  // 미러 형식: owner:agent:label:since_ms:last_ms:stopped:waiting:target:ack_ms:title
  // ack_ms는 프록시가 이미 반영한 버튼 요청의 ms(fx/sync.js 참고). title은 ':'를 품을 수
  // 있으므로 항상 맨 뒤 — 새 필드는 title 앞에 끼워 넣는다.
  const parseMirror = (v) => {
    if (!v) return null;
    const p = v.split(':');
    if (p.length < 10) return null;
    const [owner, agent, label, sinceMs, lastMs, stopped, waiting, target, ackMs] = p;
    if (!['agent', 'user', 'idle'].includes(owner)) return null;
    return { owner, agent, label, sinceMs: +sinceMs, lastMs: +lastMs, stopped: stopped === '1', waiting: +waiting || 0,
      target: target === '1', ackMs: +ackMs || 0, title: p.slice(9).join(':') };
  };

  // 유효 소유권 — 프록시가 전부 꺼지면 아무도 미러를 안 고치므로 스스로 만료를 계산한다.
  const effOwner = () => (st.owner === 'agent' && Date.now() - st.lastMs >= EXPIRY) ? 'idle' : st.owner;

  // ---- 스타일 — 섀도 루트 안에만 적용된다(페이지 CSS가 버튼·글꼴을 흔들지 못하고, 우리 키프레임도
  // 페이지로 새지 않는다). 팔레트: 테라코타 T · 앰버 A · 엠버 E · 크림 #faf9f5 · 잉크 #1f1e1d.
  const T = '217,119,87', A = '240,179,106', E = '232,100,74';
  const EASE = 'cubic-bezier(.2,.8,.2,1)', SPRING = 'cubic-bezier(.2,1.3,.3,1)';
  const STYLE = `
.al-stage{position:absolute;inset:0;font:500 12px/16px -apple-system,BlinkMacSystemFont,"Apple SD Gothic Neo","Pretendard","Noto Sans KR","Malgun Gothic",system-ui,sans-serif;
color:#faf9f5;letter-spacing:normal;text-transform:none;text-align:left;text-shadow:none;font-style:normal;visibility:visible;-webkit-font-smoothing:antialiased}
.al-stage *{box-sizing:border-box}
.al-stage i{display:block;font-style:normal}
.al-shield{position:absolute;inset:0;cursor:default}
.al-veil{position:absolute;inset:0;opacity:0;transition:opacity .7s ease;
background:radial-gradient(130% 95% at 50% 46%,rgba(22,14,10,.04) 38%,rgba(22,14,10,.26) 74%,rgba(20,11,7,.5) 100%),
radial-gradient(rgba(250,249,245,.045) 1px,transparent 1.3px) 0 0/16px 16px}
.al-stage[data-mode=agent] .al-veil{opacity:1}
.al-stage[data-phase=input] .al-veil{opacity:.7}
.al-scan{position:absolute;left:0;right:0;top:0;height:24vh;opacity:0;transition:opacity .5s ease;transform:translateY(-26vh);
background:linear-gradient(180deg,transparent,rgba(${T},.05) 55%,rgba(${A},.16) 94%,rgba(255,236,214,.65) 99.3%,transparent)}
.al-stage[data-phase=busy] .al-scan{opacity:1;animation:al-scan 2.9s cubic-bezier(.45,.05,.55,.95) infinite}
@keyframes al-scan{0%{transform:translateY(-26vh)}100%{transform:translateY(104vh)}}
.al-frame{position:absolute;inset:0;opacity:0;transition:opacity .55s ease}
.al-stage[data-mode=agent] .al-frame{opacity:1}
.al-aura,.al-hot{position:absolute;inset:0}
.al-aura{box-shadow:inset 0 0 0 1px rgba(${T},.5),inset 0 0 38px rgba(${T},.4),inset 0 0 120px rgba(${E},.18),inset 0 0 260px rgba(${A},.1);
animation:al-breathe 3.6s ease-in-out infinite}
.al-hot{opacity:0;transition:opacity .35s ease;box-shadow:inset 0 0 26px rgba(${A},.55),inset 0 0 90px rgba(${T},.35)}
.al-stage[data-phase=input] .al-hot{opacity:1}
.al-stage[data-phase=input] .al-aura{animation-duration:1.3s}
@keyframes al-breathe{0%,100%{opacity:.7}50%{opacity:1}}
.al-edge{position:absolute;overflow:hidden;box-shadow:0 0 10px rgba(${T},.85),0 0 22px rgba(${E},.35)}
.al-edge.t{left:0;right:0;top:0;height:3px;background:linear-gradient(90deg,#d97757,#f0b36a 50%,#e8644a)}
.al-edge.r{top:0;bottom:0;right:0;width:3px;background:linear-gradient(180deg,#e8644a,#d97757 50%,#f0b36a)}
.al-edge.b{left:0;right:0;bottom:0;height:3px;background:linear-gradient(270deg,#f0b36a,#e8644a 50%,#d97757)}
.al-edge.l{top:0;bottom:0;left:0;width:3px;background:linear-gradient(0deg,#d97757,#f0b36a 50%,#d97757)}
.al-beam{position:absolute;left:0;top:0}
.al-edge.t .al-beam,.al-edge.b .al-beam{width:34%;height:100%}
.al-edge.r .al-beam,.al-edge.l .al-beam{width:100%;height:34%}
.al-edge.t .al-beam{background:linear-gradient(90deg,transparent,rgba(255,240,224,.9) 75%,#fff 92%,transparent);animation:al-bx 5.2s linear infinite}
.al-edge.r .al-beam{background:linear-gradient(180deg,transparent,rgba(255,240,224,.9) 75%,#fff 92%,transparent);animation:al-by 5.2s linear 1.3s infinite;transform:translateY(-100%)}
.al-edge.b .al-beam{background:linear-gradient(270deg,transparent,rgba(255,240,224,.9) 75%,#fff 92%,transparent);animation:al-bxr 5.2s linear 2.6s infinite;transform:translateX(300%)}
.al-edge.l .al-beam{background:linear-gradient(0deg,transparent,rgba(255,240,224,.9) 75%,#fff 92%,transparent);animation:al-byr 5.2s linear 3.9s infinite;transform:translateY(300%)}
.al-stage[data-phase=input] .al-beam{animation-duration:2.2s}
.al-stage[data-phase=input] .al-edge.r .al-beam{animation-delay:.55s}
.al-stage[data-phase=input] .al-edge.b .al-beam{animation-delay:1.1s}
.al-stage[data-phase=input] .al-edge.l .al-beam{animation-delay:1.65s}
@keyframes al-bx{0%{transform:translateX(-100%)}25%,100%{transform:translateX(300%)}}
@keyframes al-bxr{0%{transform:translateX(300%)}25%,100%{transform:translateX(-100%)}}
@keyframes al-by{0%{transform:translateY(-100%)}25%,100%{transform:translateY(300%)}}
@keyframes al-byr{0%{transform:translateY(300%)}25%,100%{transform:translateY(-100%)}}
.al-corner{position:absolute;width:34px;height:34px;opacity:0;border:0 solid rgba(250,249,245,.95);filter:drop-shadow(0 0 5px rgba(${T},.95));
transition:transform .8s ${SPRING},opacity .35s ease,border-color .3s ease}
.al-corner.tl{top:16px;left:16px;border-top-width:2.5px;border-left-width:2.5px;border-top-left-radius:12px;transform:translate(-44px,-44px) scale(1.5)}
.al-corner.tr{top:16px;right:16px;border-top-width:2.5px;border-right-width:2.5px;border-top-right-radius:12px;transform:translate(44px,-44px) scale(1.5)}
.al-corner.bl{bottom:16px;left:16px;border-bottom-width:2.5px;border-left-width:2.5px;border-bottom-left-radius:12px;transform:translate(-44px,44px) scale(1.5)}
.al-corner.br{bottom:16px;right:16px;border-bottom-width:2.5px;border-right-width:2.5px;border-bottom-right-radius:12px;transform:translate(44px,44px) scale(1.5)}
.al-stage[data-mode=agent] .al-corner{opacity:1;transform:none}
.al-stage[data-mode=agent] .al-corner.tr{transition-delay:.06s}
.al-stage[data-mode=agent] .al-corner.br{transition-delay:.12s}
.al-stage[data-mode=agent] .al-corner.bl{transition-delay:.18s}
.al-stage[data-phase=input] .al-corner{border-color:#ffd9b5}
.al-stage[data-phase=input] .al-corner.tl{transform:translate(6px,6px)}
.al-stage[data-phase=input] .al-corner.tr{transform:translate(-6px,6px)}
.al-stage[data-phase=input] .al-corner.bl{transform:translate(6px,-6px)}
.al-stage[data-phase=input] .al-corner.br{transform:translate(-6px,-6px)}
.al-fxl{position:absolute;inset:0;overflow:hidden}
.al-boot .sweep{position:absolute;left:0;right:0;top:0;height:42vh;
background:linear-gradient(180deg,transparent,rgba(${T},.08),rgba(${A},.3) 90%,rgba(255,240,222,.95) 99%,transparent);animation:al-boot 1.05s cubic-bezier(.6,0,.25,1) forwards}
.al-boot .flash{position:absolute;inset:0;box-shadow:inset 0 0 0 2px rgba(255,236,214,.95),inset 0 0 140px rgba(${T},.65);animation:al-flash 1.1s ease-out forwards}
@keyframes al-boot{0%{transform:translateY(-46vh);opacity:1}80%{opacity:1}100%{transform:translateY(106vh);opacity:0}}
@keyframes al-flash{0%{opacity:0}22%{opacity:1}100%{opacity:0}}
.al-release{position:absolute;inset:0;border:2px solid rgba(250,249,245,.85);box-shadow:inset 0 0 40px rgba(${T},.5),0 0 30px rgba(${T},.6);
animation:al-release .8s ${EASE} forwards}
@keyframes al-release{0%{opacity:1;transform:scale(1)}100%{opacity:0;transform:scale(.93)}}
.al-burst{position:absolute;width:0;height:0}
.al-burst .ring{position:absolute;left:-22px;top:-22px;width:44px;height:44px;border-radius:50%;border:2px solid rgba(255,226,196,.95);
box-shadow:0 0 12px rgba(${T},.9),inset 0 0 10px rgba(${T},.6);animation:al-ring .65s ${EASE} both}
.al-burst .ring.two{border-color:rgba(${T},.9);animation-delay:.09s;animation-duration:.85s}
.al-burst .core{position:absolute;left:-8px;top:-8px;width:16px;height:16px;border-radius:50%;background:radial-gradient(circle,#fff,rgba(${A},.9) 40%,transparent 70%);animation:al-core .45s ease-out both}
.al-burst .spk{position:absolute;left:0;top:0;width:0;height:0}
.al-burst .spk i{position:absolute;left:9px;top:-1px;width:11px;height:2px;border-radius:2px;background:linear-gradient(90deg,#ffecd6,rgba(${T},0));animation:al-spark .55s ${EASE} both}
@keyframes al-ring{0%{transform:scale(.2);opacity:1}100%{transform:scale(1.9);opacity:0}}
@keyframes al-core{0%{transform:scale(.4);opacity:1}100%{transform:scale(2.3);opacity:0}}
@keyframes al-spark{0%{transform:translateX(0) scaleX(.4);opacity:1}100%{transform:translateX(24px) scaleX(1);opacity:0}}
.al-hl{position:absolute;opacity:0;transition:opacity .3s ease}
.al-lock{position:absolute;inset:-6px;border-radius:10px;box-shadow:0 0 0 1.5px rgba(${T},.9),0 0 22px rgba(${T},.45),inset 0 0 14px rgba(${T},.18);animation:al-lock .5s ${SPRING} both}
.al-lock i{position:absolute;width:11px;height:11px;border:0 solid #ffe3c7}
.al-lock i:nth-child(1){left:-4px;top:-4px;border-top-width:2px;border-left-width:2px;border-top-left-radius:6px}
.al-lock i:nth-child(2){right:-4px;top:-4px;border-top-width:2px;border-right-width:2px;border-top-right-radius:6px}
.al-lock i:nth-child(3){left:-4px;bottom:-4px;border-bottom-width:2px;border-left-width:2px;border-bottom-left-radius:6px}
.al-lock i:nth-child(4){right:-4px;bottom:-4px;border-bottom-width:2px;border-right-width:2px;border-bottom-right-radius:6px}
@keyframes al-lock{0%{transform:scale(1.3);opacity:0}100%{transform:scale(1);opacity:1}}
.al-pointer{position:absolute;inset:0;opacity:0;transition:opacity .3s ease}
.al-cursor,.al-tail{position:absolute;left:0;top:0;will-change:transform}
.al-cursor{transition:transform .2s ${EASE}}
.al-cursor-icon{position:absolute;left:-4px;top:-3px;width:28px;height:34px;filter:drop-shadow(0 2px 3px rgba(0,0,0,.45)) drop-shadow(0 0 8px rgba(${T},.75))}
.al-tail{width:12px;height:12px;margin:-6px 0 0 -6px;border-radius:50%;background:radial-gradient(circle,rgba(255,228,200,.95),rgba(${T},.55) 45%,transparent 70%)}
.al-tail.t1{transition:transform .32s ${EASE};opacity:.8}
.al-tail.t2{transition:transform .46s ${EASE};opacity:.5;width:10px;height:10px;margin:-5px 0 0 -5px}
.al-tail.t3{transition:transform .62s ${EASE};opacity:.28;width:8px;height:8px;margin:-4px 0 0 -4px}
.al-label{position:absolute;left:22px;top:27px;padding:4px 10px 4px 8px;border-radius:999px;white-space:nowrap;color:#fff;font-weight:700;font-size:11px;line-height:15px;
background:linear-gradient(135deg,#ee8a62,#d97757 55%,#b9583a);box-shadow:0 4px 12px rgba(0,0,0,.35),inset 0 0 0 1px rgba(255,255,255,.2),0 0 14px rgba(${T},.55)}
.al-label::before{content:"✦";display:inline-block;margin-right:5px;color:#ffe3c7;animation:al-twinkle 1.6s ease-in-out infinite}
.al-label:empty{display:none}
@keyframes al-twinkle{0%,100%{transform:scale(.8) rotate(0);opacity:.7}50%{transform:scale(1.15) rotate(90deg);opacity:1}}
@keyframes al-pressA{0%{transform:scale(1)}35%{transform:scale(.82)}100%{transform:scale(1)}}
@keyframes al-pressB{0%{transform:scale(1)}35%{transform:scale(.82)}100%{transform:scale(1)}}
.al-pill{position:absolute;left:50%;bottom:22px;max-width:calc(100vw - 32px);padding:1.5px;border-radius:20px;overflow:hidden;opacity:0;
transform:translate(-50%,18px) scale(.94);transition:opacity .35s ease,transform .6s ${SPRING};
box-shadow:0 14px 40px rgba(0,0,0,.5),0 0 0 1px rgba(0,0,0,.35),0 0 36px rgba(${T},.25)}
.al-stage[data-mode=agent] .al-pill,.al-stage[data-mode=user] .al-pill{opacity:1;transform:translate(-50%,0) scale(1)}
.al-pill-beam{position:absolute;left:50%;top:50%;width:680px;height:680px;margin:-340px 0 0 -340px;
background:conic-gradient(rgba(${T},.18),rgba(${T},.95) 16%,#ffe6cc 24%,rgba(${A},.9) 31%,rgba(${T},.18) 48%,rgba(${T},.18) 52%,rgba(${E},.85) 68%,#ffd9b8 75%,rgba(${T},.18) 92%);
animation:al-spin 3.4s linear infinite}
@keyframes al-spin{to{transform:rotate(360deg)}}
.al-stage[data-mode=user] .al-pill-beam{background:conic-gradient(rgba(250,249,245,.1),rgba(250,249,245,.55) 20%,rgba(156,197,161,.6) 28%,rgba(250,249,245,.1) 50%,rgba(250,249,245,.1));animation-duration:9s}
.al-stage[data-stopped="1"] .al-pill-beam{background:rgba(229,83,75,.45);animation:none}
.al-pill-body{position:relative;display:flex;align-items:center;gap:12px;padding:10px 10px 10px 14px;border-radius:18.5px;
background:linear-gradient(180deg,rgba(46,44,42,.97),rgba(31,30,29,.97))}
.al-orb{position:relative;flex:none;width:18px;height:18px;border-radius:50%;
background:radial-gradient(circle at 35% 30%,#fff4e8 0,#f0b36a 28%,#d97757 58%,#7a3520 100%);
box-shadow:0 0 0 3px rgba(${T},.18),0 0 14px rgba(${T},.8);animation:al-orb 2.2s ease-in-out infinite}
.al-orb::after{content:"";position:absolute;inset:-5px;border-radius:50%;border:1.5px solid transparent;border-top-color:rgba(255,230,205,.95);border-right-color:rgba(${T},.5);animation:al-spin 1.1s linear infinite}
.al-orb.sm{width:10px;height:10px}
.al-orb.sm::after{inset:-4px}
@keyframes al-orb{0%,100%{transform:scale(1)}50%{transform:scale(1.13)}}
.al-stage[data-mode=user] .al-pill .al-orb{background:radial-gradient(circle at 35% 30%,#f4fff6,#9cc5a1 45%,#4f7a5a);box-shadow:0 0 0 3px rgba(156,197,161,.18),0 0 12px rgba(156,197,161,.6);animation-duration:3.6s}
.al-stage[data-mode=user] .al-pill .al-orb::after{border-top-color:rgba(220,245,225,.8);border-right-color:transparent;animation-duration:3s}
.al-stage[data-stopped="1"] .al-pill .al-orb{background:radial-gradient(circle at 35% 30%,#ffe9e7,#e5534b 50%,#7d2420);box-shadow:0 0 0 3px rgba(229,83,75,.2),0 0 10px rgba(229,83,75,.55);animation:none}
.al-stage[data-stopped="1"] .al-pill .al-orb::after{display:none}
.al-txt{display:flex;flex-direction:column;gap:3px;min-width:0}
.al-title{font-weight:700;font-size:13px;line-height:17px;max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.al-row{display:flex;align-items:center;gap:8px;white-space:nowrap}
.al-status{font-weight:600;color:#cfcbc0}
.al-stage[data-mode=agent] .al-status{color:#f0b36a;background:linear-gradient(90deg,#f0b36a 0%,#f0b36a 35%,#fff0e0 50%,#f0b36a 65%,#e08a64 100%) 0 0/300% 100%;
-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent;animation:al-shine 3.2s linear infinite}
.al-stage[data-stopped="1"] .al-status{color:#f29a92}
@keyframes al-shine{0%{background-position:100% 0}100%{background-position:0 0}}
.al-chip{display:none;align-items:center;gap:5px;padding:1px 8px 1px 6px;border-radius:999px;background:rgba(${T},.16);box-shadow:inset 0 0 0 1px rgba(${T},.38);color:#ffd9bd;font-size:11px;font-weight:700}
.al-stage[data-phase=busy] .al-chip,.al-stage[data-phase=input] .al-chip{display:inline-flex}
.al-eq{display:inline-flex;align-items:flex-end;gap:1.5px;height:9px}
.al-eq i{width:2px;height:100%;border-radius:1px;background:#f0b36a;transform-origin:bottom;animation:al-eq .9s ease-in-out infinite}
.al-eq i:nth-child(2){animation-delay:-.3s}
.al-eq i:nth-child(3){animation-delay:-.6s}
@keyframes al-eq{0%,100%{transform:scaleY(.3)}50%{transform:scaleY(1)}}
.al-btns{display:flex;align-items:center;gap:6px;margin-left:4px;transition:opacity .25s ease,filter .25s ease}
.al-stage[data-phase=input] .al-btns{opacity:.4;filter:saturate(.3)}
.al-btn{appearance:none;border:0;margin:0;cursor:pointer;border-radius:999px;padding:7px 14px;font:inherit;font-weight:700;font-size:12px;line-height:16px;white-space:nowrap;
transition:transform .15s ease,box-shadow .2s ease,background .2s ease}
.al-btn:active{transform:scale(.95)}
.al-btn:focus-visible{outline:2px solid #f0b36a;outline-offset:2px}
.al-btn.primary{background:linear-gradient(180deg,#fffdf8,#ece7dc);color:#1f1e1d;box-shadow:inset 0 1px 0 rgba(255,255,255,.7),0 2px 8px rgba(0,0,0,.3)}
.al-btn.primary:hover{transform:translateY(-1px);box-shadow:inset 0 1px 0 rgba(255,255,255,.7),0 2px 8px rgba(0,0,0,.3),0 0 0 3px rgba(${T},.4)}
.al-btn.ghost{background:rgba(250,249,245,.08);color:#faf9f5;box-shadow:inset 0 0 0 1px rgba(250,249,245,.2)}
.al-btn.ghost:hover{background:rgba(229,83,75,.24);box-shadow:inset 0 0 0 1px rgba(229,83,75,.65)}
.al-btn.hand{background:linear-gradient(135deg,#ee8a62,#d97757 55%,#c4623f);color:#fff;box-shadow:inset 0 1px 0 rgba(255,255,255,.3),0 0 16px rgba(${T},.5)}
.al-btn.hand:hover{transform:translateY(-1px);box-shadow:inset 0 1px 0 rgba(255,255,255,.3),0 0 24px rgba(${T},.75)}
.al-topline{position:absolute;left:0;right:0;top:0;height:2px;overflow:hidden;opacity:0;transition:opacity .4s ease;
background:linear-gradient(90deg,transparent,rgba(${T},.9) 18%,rgba(${A},.95) 50%,rgba(${T},.9) 82%,transparent);box-shadow:0 0 10px rgba(${T},.7)}
.al-topline i{position:absolute;left:0;top:0;width:28%;height:100%;background:linear-gradient(90deg,transparent,#fff3e6,transparent);animation:al-sweep 3.4s ease-in-out infinite}
@keyframes al-sweep{0%{transform:translateX(-100%)}100%{transform:translateX(360%)}}
.al-banner{position:absolute;top:10px;left:50%;display:flex;align-items:center;gap:9px;padding:6px 15px 6px 11px;border-radius:999px;max-width:min(560px,calc(100vw - 32px));
font-weight:600;background:rgba(31,30,29,.93);box-shadow:0 6px 22px rgba(0,0,0,.4),inset 0 0 0 1px rgba(${T},.45),0 0 18px rgba(${T},.25);
opacity:0;transform:translate(-50%,-170%);transition:transform .6s ${SPRING},opacity .3s ease}
.al-banner-text{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.al-stage[data-mode=other] .al-banner{opacity:1;transform:translate(-50%,0)}
.al-stage[data-mode=other] .al-topline{opacity:1}
@media (prefers-reduced-motion:reduce){
.al-stage *,.al-stage *::before,.al-stage *::after{animation-duration:.01s!important;animation-iteration-count:1!important;transition-duration:.01s!important}
.al-scan,.al-beam,.al-boot .sweep,.al-topline i{display:none}}
`;
  const CURSOR_SVG =
    '<svg width="28" height="34" viewBox="0 0 22 26"><defs><linearGradient id="al-cg" x1="0" y1="0" x2="1" y2="1">' +
    '<stop offset="0" stop-color="#ffd8b5"/><stop offset=".45" stop-color="#e8845f"/><stop offset="1" stop-color="#c4583a"/></linearGradient></defs>' +
    '<path d="M3 2 L19 13 L11.5 14.5 L15.5 23 L12.5 24.2 L8.5 15.8 L3 21 Z" fill="url(#al-cg)" stroke="#faf9f5" stroke-width="1.5" stroke-linejoin="round"/></svg>';
  const FRAME_HTML = '<i class="al-aura"></i><i class="al-hot"></i>' +
    ['t', 'r', 'b', 'l'].map(k => `<i class="al-edge ${k}"><i class="al-beam"></i></i>`).join('') +
    ['tl', 'tr', 'bl', 'br'].map(k => `<i class="al-corner ${k}"></i>`).join('');
  const BURST_HTML = '<i class="ring"></i><i class="ring two"></i><i class="core"></i>' +
    [0, 45, 90, 135, 180, 225, 270, 315].map(a => `<i class="spk" style="transform:rotate(${a}deg)"><i></i></i>`).join('');
  const LOCK_HTML = '<i class="al-lock"><i></i><i></i><i></i><i></i></i>';

  // ---- DOM
  let host, stage, shield, fxl, hl, pointer, cursor, cursorIcon, cursorLabel, tails, pill, pillTitle, pillStatus, pillTool, pillBtns, bannerText;
  // render()의 재도색 캐시 — 값이 바뀌지 않으면 DOM에 다시 쓰지 않는다(버튼을 매 뮤테이션마다
  // 다시 만들면 mousedown~mouseup 사이에 버튼이 사라져 클릭이 씹힌다: 게이트 리뷰 지적).
  let cache = null;
  const resetCache = () => { cache = { mode: null, phase: null, stopped: null, shield: null, pillTitle: null, pillStatus: null, pillTool: null,
    buttonsKey: null, buttonsClickable: null, banner: null, cursorLabel: null, cursorOn: null, hlHidden: null }; };
  resetCache();
  const css = (e, s) => { e.style.cssText = s; return e; };
  const mk = (tag, cls, parent) => {
    const e = document.createElement(tag);
    if (cls) e.setAttribute('class', cls);
    if (parent) parent.appendChild(e);
    return e;
  };
  const BTN_KIND = { '내가 조작하기': 'primary', '중단': 'ghost', 'AI에게 돌려주기': 'hand' };
  const btn = (text) => {
    const b = mk('button', `al-btn ${BTN_KIND[text] || 'primary'}`);
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
    // 닫힌 섀도 루트: 페이지 CSS(button{}·div{} 규칙, 전역 transition 등)가 오버레이를 흔들지 못하고
    // 페이지 스크립트도 안을 못 들여다본다. 섀도가 없는 환경(테스트 가짜 DOM)은 host에 바로 붙인다.
    let layer = host;
    if (typeof host.attachShadow === 'function') { try { layer = host.attachShadow({ mode: 'closed' }); } catch (_) { layer = host; } }
    mk('style', '', layer).textContent = STYLE;
    stage = mk('div', 'al-stage', layer);
    shield = mk('div', 'al-shield', stage); // 가짜 DOM 테스트가 "pointerEvents를 가진 첫 DIV"로 찾는다 — 순서 유지
    mk('div', 'al-veil', stage);
    mk('div', 'al-scan', stage);
    mk('div', 'al-frame', stage).innerHTML = FRAME_HTML;
    fxl = mk('div', 'al-fxl', stage); // 부트 스윕·해제·클릭 버스트 같은 일회성 효과가 여기 잠깐 붙었다 떨어진다
    hl = mk('div', 'al-hl', stage);
    pointer = mk('div', 'al-pointer', stage);
    tails = ['t3', 't2', 't1'].map(k => mk('div', `al-tail ${k}`, pointer)); // 전이 시간이 서로 달라 혜성 꼬리가 된다
    cursor = mk('div', 'al-cursor', pointer);
    cursorIcon = mk('div', 'al-cursor-icon', cursor);
    cursorIcon.innerHTML = CURSOR_SVG;
    cursorLabel = mk('div', 'al-label', cursor);
    pill = mk('div', 'al-pill', stage);
    mk('div', 'al-pill-beam', pill);
    const body = mk('div', 'al-pill-body', pill);
    mk('div', 'al-orb', body);
    const txt = mk('div', 'al-txt', body);
    pillTitle = mk('div', 'al-title', txt);
    const row = mk('div', 'al-row', txt);
    pillStatus = mk('span', 'al-status', row);
    const chip = mk('span', 'al-chip', row);
    mk('span', 'al-eq', chip).innerHTML = '<i></i><i></i><i></i>';
    pillTool = mk('span', '', chip);
    // pillBtns 자체는 pointer-events를 명시하지 않는다(호스트로부터 none을 물려받는 게 기본) —
    // render()가 매번 buttonsClickable에 따라 auto/none을 명시적으로 써준다.
    pillBtns = mk('div', 'al-btns', body);
    mk('div', 'al-topline', stage).innerHTML = '<i></i>';
    const banner = mk('div', 'al-banner', stage);
    mk('div', 'al-orb sm', banner);
    bannerText = mk('span', 'al-banner-text', banner);
    (document.body || document.documentElement).appendChild(host);
    resetCache(); // 새 엘리먼트는 기본 상태이므로 이전 캐시값과 비교하면 안 됨 — 다음 render가 전부 다시 씀
  };
  // 일회성 효과: fxl에 붙였다가 ms 뒤 스스로 떨어진다(CSS 애니메이션이 붙는 순간 재생된다).
  const spawn = (cls, html, ms, x, y) => {
    const e = mk('div', cls, fxl);
    if (html) e.innerHTML = html;
    if (x !== undefined) { e.style.left = x + 'px'; e.style.top = y + 'px'; }
    setTimeout(() => e.remove(), ms);
    return e;
  };

  const request = (text) => {
    const kind = text === '내가 조작하기' ? 'user' : text === '중단' ? 'stop' : 'agent';
    document.documentElement.setAttribute(REQ, `${kind}:${Date.now()}`);
  };

  // ---- 표시 계산(테스트가 같은 함수를 본다) — 순수 함수, DOM을 건드리지 않는다
  const compute = () => {
    const owner = effOwner();
    const active = owner === 'agent' && st.target;
    // 입력 도구 구간은 "작업 탭에서 에이전트가 조작 중"일 때만이다. 소유권과 무관하게
    // 판정하면(예전 코드) 사용자 소유일 때 프록시가 쓴 fx 신호 때문에 알약 버튼이
    // 눌리지 않아 제어권을 못 돌려주는 상황이 생긴다.
    const inputPhase = active && st.fxOn && INPUT.has(st.tool);
    const s = { owner, target: st.target, shield: 'none', dim: false, pill: false, banner: '', buttons: [],
      pillStatus: '', cursorLabel: '', cursor: st.cursor, buttonsClickable: !inputPhase };
    if (active) {
      s.shield = inputPhase ? 'none' : 'auto';
      s.dim = true; s.pill = true;
      s.buttons = ['내가 조작하기', '중단'];
      s.pillStatus = `AI가 조작 중 · ${st.agent}`;
      s.cursorLabel = st.fxOn ? (LABELS[st.tool] || '작업 중') : '';
    } else if (owner === 'user') {
      // 사용자 소유면 target과 무관하게 모든 탭에 알약을 띄운다(최종 리뷰 CRITICAL 1).
      // target은 작업 탭을 아는 경우에만 1이 되는데, PageMap이 아직 비었거나 작업 탭이
      // 닫히면 모든 탭이 target=0이 된다. 그때 버튼이 없으면 게이트가 모든 호출을 막은
      // 채 제어권을 돌려줄 방법이 사라져(재시작해도 파일이 남아) 영구 잠금이 된다.
      s.pill = true;
      s.buttons = ['AI에게 돌려주기'];
      s.pillStatus = st.stopped ? '중단됨' : `내가 조작 중 · 대기 중인 호출 ${st.waiting}`;
    } else if (owner === 'agent') {
      // 작업 탭이 아닌 탭: 얇은 띠만. 제목이 비면 구분자(·)도 붙이지 않는다.
      s.banner = st.title ? `AI가 다른 탭에서 작업 중 · ${st.title}` : 'AI가 다른 탭에서 작업 중';
    }
    return s;
  };

  // render()는 멱등이어야 한다 — 프록시가 미러를 500ms~ 간격으로 계속 다시 쓰고,
  // 뮤테이션옵저버가 매번 이 함수를 부르므로, 값이 안 바뀐 항목은 DOM에 다시 쓰지 않는다.
  // 특히 버튼 컨테이너를 매번 비웠다 새로 만들면 mousedown~mouseup 사이에 버튼이
  // 사라져 클릭이 씹힌다(게이트 리뷰 지적 1) — buttonsKey가 같으면 pillBtns를 건드리지 않는다.
  // 시각 효과 대부분은 stage의 data-mode/data-phase/data-stopped 세 속성에 CSS가 반응해서 난다.
  const render = () => {
    ensure();
    const s = compute();
    const mode = s.dim ? 'agent' : s.owner === 'user' ? 'user' : s.banner ? 'other' : 'idle';
    const phase = mode !== 'agent' ? '' : !s.buttonsClickable ? 'input' : st.fxOn ? 'busy' : 'wait';
    const stopped = mode === 'user' && st.stopped ? '1' : '0';
    if (cache.mode !== mode) {
      const prev = cache.mode;
      stage.setAttribute('data-mode', mode);
      // 전환 연출: 진입은 위→아래 부트 스윕 + 테두리 섬광, 이탈은 테두리가 안쪽으로 접히며 사라진다.
      // prev===null(새 페이지 첫 그림·host 재생성)엔 재생하지 않는다 — 에이전트가 페이지를
      // 옮길 때마다 번쩍이면 시끄럽다. 그땐 CSS 전이로 조용히 페이드 인만 한다.
      if (mode === 'agent' && prev !== null) spawn('al-boot', '<i class="sweep"></i><i class="flash"></i>', 1200);
      if (prev === 'agent') spawn('al-release', '', 900);
      cache.mode = mode;
    }
    if (cache.phase !== phase) { stage.setAttribute('data-phase', phase); cache.phase = phase; }
    if (cache.stopped !== stopped) { stage.setAttribute('data-stopped', stopped); cache.stopped = stopped; }
    if (cache.shield !== s.shield) { shield.style.pointerEvents = s.shield; cache.shield = s.shield; }
    const title = st.label || '브라우저 작업';
    if (cache.pillTitle !== title) { pillTitle.textContent = title; cache.pillTitle = title; }
    if (cache.pillStatus !== s.pillStatus) { pillStatus.textContent = s.pillStatus; cache.pillStatus = s.pillStatus; }
    if (cache.pillTool !== s.cursorLabel) { if (s.cursorLabel) pillTool.textContent = s.cursorLabel; cache.pillTool = s.cursorLabel; } // 비울 땐 칩이 CSS로 사라지는 동안 글자를 남겨 둔다
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
    if (cache.banner !== s.banner) { if (s.banner) bannerText.textContent = s.banner; cache.banner = s.banner; } // 내려가는 동안 글자 유지
    if (cache.cursorLabel !== s.cursorLabel) { cursorLabel.textContent = s.cursorLabel; cache.cursorLabel = s.cursorLabel; }
    const cursorOn = !!(s.owner === 'agent' && st.target && st.cursor);
    if (cache.cursorOn !== cursorOn) { pointer.style.opacity = cursorOn ? '1' : '0'; cache.cursorOn = cursorOn; }
    if (!s.dim) { if (cache.hlHidden !== true) { hl.style.opacity = '0'; cache.hlHidden = true; } }
    else { cache.hlHidden = false; }
  };

  // moveTo는 render()를 부르지 않는다 — 마우스가 움직일 때마다(뮤테이션과 무관하게) 풀
  // render를 돌리면 버튼이 계속 다시 만들어져 클릭이 씹힌다(게이트 리뷰 지적 1). 커서
  // 위치·표시 여부만 직접 반영한다. 호출 시점엔 inputPhase()가 이미 참이므로(owner=agent,
  // target=true) 커서를 보이는 상태로 둔다. 꼬리 셋은 같은 좌표를 받되 전이 시간이 길어 뒤처진다.
  const moveTo = (x, y) => {
    st.cursor = { x, y }; ensure();
    const t = `translate(${x}px,${y}px)`;
    cursor.style.transform = t;
    tails.forEach(e => { e.style.transform = t; });
    pointer.style.opacity = '1'; cache.cursorOn = true;
  };
  let pressAlt = false;
  const ripple = (x, y) => {
    ensure();
    spawn('al-burst', BURST_HTML, 950, x, y);
    // 커서 눌림: 애니메이션 이름을 A/B로 번갈아 바꿔야 연속 클릭에도 매번 다시 재생된다
    pressAlt = !pressAlt;
    cursorIcon.style.animation = `${pressAlt ? 'al-pressA' : 'al-pressB'} .32s ease-out`;
  };
  let hlTimer = 0;
  const highlight = (el) => {
    if (!(typeof Element !== 'undefined' && el instanceof Element) || el === document.body || el === document.documentElement || el === host) return;
    const b = el.getBoundingClientRect();
    if (!b.width || !b.height) return;
    ensure();
    hl.style.left = b.left + 'px'; hl.style.top = b.top + 'px'; hl.style.width = b.width + 'px'; hl.style.height = b.height + 'px';
    hl.innerHTML = LOCK_HTML; // 새 노드라 락온 애니메이션이 매번 처음부터 재생된다
    hl.style.opacity = '1';
    clearTimeout(hlTimer);
    hlTimer = setTimeout(() => { hl.style.opacity = '0'; }, 1500);
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
