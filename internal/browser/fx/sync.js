// fx/sync.js — 프록시가 탭마다 한 번 Eval하는 스크립트(browser/fx.go가 embed해서 쓴다).
// 한 왕복에 셋을 묶는다: 소유권 미러 쓰기 · (있으면) fx 신호 쓰기 · 사용자 버튼 요청 회수.
//
// ack 규칙(최종 리뷰 IMPORTANT 2): 요청을 "읽으면서 지우지" 않는다. 프록시가 이미 반영한
// 요청의 ms(ackMs)를 같이 내려보내, 그보다 새 요청만 돌려주고 ack된(reqMs <= ackMs) 요청만
// 지운다. 느린 탭의 응답이 예산을 넘겨 버려져도 요청은 DOM에 남아 다음 왕복에서 다시 잡힌다.
(mirror, ownerAttr, requestAttr, fxAttr, fxValue, ackMs) => {
  const h = document.documentElement;
  h.setAttribute(ownerAttr, mirror);
  if (fxValue) h.setAttribute(fxAttr, fxValue);
  const r = h.getAttribute(requestAttr) || '';
  if (!r) return '';
  const ms = Number(r.split(':')[1]) || 0;
  if (ms <= ackMs) { h.removeAttribute(requestAttr); return ''; }
  return r;
}
