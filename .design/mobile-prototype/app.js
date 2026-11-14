// Throwaway prototype v2 for the mobile TEP: a phone-native style (inbox-first, grouped lists, docked actions, sheets).
// Static, clickable, mock data from frontend/src/mocks/projectsHandlers.ts.
const P = {
  back: 'M15 6l-6 6 6 6', right: 'M9 6l6 6-6 6', down: 'M6 9l6 6 6-6', up: 'M6 15l6-6 6 6', star: 'M12 17.3l-5.4 3.2 1.4-6.1-4.7-4.1 6.2-.5L12 4l2.5 5.8 6.2.5-4.7 4.1 1.4 6.1z',
  dots: 'M12 6h.01M12 12h.01M12 18h.01', clip: 'M15 7l-6.5 6.5a1.5 1.5 0 0 0 3 3L18 10a3 3 0 0 0-6-6L5.5 10.5a4.5 4.5 0 0 0 9 9L21 13',
  send: 'M12 19V5M6 11l6-6 6 6', stop: 'M7 7h10v10H7z', check: 'M5 12l5 5L20 7', x: 'M6 6l12 12M18 6L6 18', sliders: 'M4 6h10M18 6h2M4 12h4M12 12h8M4 18h12M20 18h0M14 4v4M8 10v4M16 16v4',
  plus: 'M12 5v14M5 12h14', apps: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z', users: 'M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM3 20c0-3.3 2.7-6 6-6s6 2.7 6 6M16 5a3 3 0 0 1 0 6M18 14c1.8.6 3 2.2 3 4',
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z', lock: 'M6 11h12v9H6zM8 11V8a4 4 0 0 1 8 0v3', file: 'M7 3h7l5 5v13H7zM14 3v5h5',
  info: 'M12 8h.01M11 12h1v5h1M3 12a9 9 0 1 0 18 0 9 9 0 0 0-18 0', bolt: 'M13 3L5 14h6l-1 7 8-11h-6z', copy: 'M8 8h11v11H8zM5 15V5h10', eye: 'M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12zM12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z',
  branch: 'M6 4v12M18 8a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM6 20a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM18 8c0 5-6 4-12 8', more: 'M5 12h.01M12 12h.01M19 12h.01', chart: 'M4 20V10M10 20V4M16 20v-8M22 20H2',
  inbox: 'M4 13l2-8h12l2 8M4 13v6h16v-6M4 13h4l1 2h6l1-2h4', user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21c0-4 3.6-7 8-7s8 3 8 7', msg: 'M4 5h16v11H9l-5 4z', pause: 'M8 6v12M16 6v12', image: 'M4 5h16v14H4zM8 10h.01M4 17l5-5 4 4 3-3 4 4',
};
const I = (n, c = '') => `<svg class="i ${c}" viewBox="0 0 24 24"><path d="${P[n]}"/></svg>`;
const $ = (s, r = document) => r.querySelector(s);
const q = new URLSearchParams(location.search);
if (q.get('theme') === 'dark') document.body.dataset.theme = 'dark';
if (q.get('frame') !== '0') document.body.classList.add('framed');

const S = { tab: 'work', open: {}, screen: 'root', ptab: 'chat', thread: 't-pool', pending: { 't-pool': true, 't-backoff': true }, sheet: null, toast: '', kb: false, resolved: false, agent: 'claude' };
const waiting = () => Object.values(S.pending).filter(Boolean).length;
const T = {
  't-pool': { t: 'Tune the DB connection pool', p: 'Payments API latency', g: 'wait', d: 'Allow Bash: go test ./... -run Bench -staging', ago: '4m', kind: 'approval', ap: ['Allow Bash?', 'go test ./... -run Bench -staging'] },
  't-backoff': { t: 'Pick a retry backoff', p: 'Payments API latency', g: 'wait', d: 'Which backoff should retries use?', ago: '2m', kind: 'ask', ap: ['Which backoff should retries use?', 'Exponential with jitter (Recommended)'] },
  't-cache': { t: 'Cache idempotency keys in Redis', p: 'Payments API latency', g: 'rev', d: 'p99 410 → 260 ms · 3 commits · 6 files', ago: '18m' },
  't-sdk': { t: 'Retry budget in the SDK', p: 'Payments API latency', g: 'run', d: 'Running tests…', ago: '1m' },
  't-queued': { t: 'Audit logging for refunds', p: 'Payments API latency', g: 'run', d: 'Waiting for a free slot', ago: '6s' },
  't-docs': { t: 'Document the /refund endpoint', p: 'Docs refresh', g: 'rev', d: 'Example added · 1 file', ago: '3h' },
};
const nav = () => {
  const t = (k, ic, l, bd) => `<button class="${S.tab === k ? 'on' : ''}" data-tab="${k}">${I(ic)}<span>${l}</span>${bd ? `<span class="badge">${bd}</span>` : ''}</button>`;
  return `<nav class="nav">${t('work', 'folder', '工作区', waiting())}${t('agent', 'apps', '智能应用')}${t('me', 'more', '更多')}</nav>`;
};
const chev = I('right', 's chev');
const glyph = (g) => g === 'wait' ? `<span class="glyph warn">${I('bolt', 's')}</span>` : g === 'rev' ? `<span class="glyph ok">${I('check', 's')}</span>` : `<span class="glyph run">${I('more', 's')}</span>`;
const tRow = (id, proj = true) => { const x = T[id]; return `<button class="row" data-thread="${id}">${glyph(x.g)}<span class="main">${proj ? `<span class="cap">${x.p}</span>` : ''}<span class="t">${x.t}</span><span class="d">${x.d}</span></span><span class="r">${x.ago}</span></button>`; };

function chatBody() {
  return `<div class="msg" style="color:var(--text2);font-size:13px">来自 Telegram · Maya (phone)</div>
  <div class="msg them">I will add a retry budget in the SDK so an outage cannot amplify traffic. The pool benchmark still needs your approval.</div>
  <button class="pill" data-thread="t-sdk"><span class="glyph run">${I('more', 's')}</span><b style="flex:1">Retry budget in the SDK</b><span style="color:var(--text3);font-size:13px">进行中</span></button>
  <button class="pill" data-thread="t-cache"><span class="glyph ok">${I('check', 's')}</span><b style="flex:1">Cache idempotency keys in Redis</b><span style="color:var(--text3);font-size:13px">可 review</span></button>
  <div class="msg me">Also keep the Postgres path as a fallback.</div>
  <div class="msg them">The Redis change is ready for review, and the tracing work is merged. Next, approve or deny the staging benchmark in the pool thread.</div>`;
}
function projectScreen() {
  const body = S.ptab === 'chat' ? `<div class="scroll">${chatBody()}</div>` : S.ptab === 'threads'
    ? `<div class="scroll"><div class="sec"><span class="dot"></span>等你处理</div><div class="group">${['t-pool', 't-backoff'].filter(i => S.pending[i]).map(i => tRow(i, false)).join('') || '<div class="row"><span class="d">没有</span></div>'}</div><div class="sec">可以 review</div><div class="group">${tRow('t-cache', false)}</div><div class="sec">进行中</div><div class="group">${tRow('t-sdk', false)}${tRow('t-queued', false)}</div></div>`
    : `<div class="scroll"><div class="sec">文件库</div><div class="group">${['outputs', 'uploads'].map(n => `<div class="row"><span class="glyph">${I('folder', 's')}</span><span class="t main">${n}</span>${chev}</div>`).join('')}${['baseline-p99.csv', 'charge-flamegraph.svg'].map(n => `<div class="row"><span class="glyph">${I('file', 's')}</span><span class="t main">${n}</span><span class="r">预览</span></div>`).join('')}</div></div>`;
  const comp = S.ptab === 'chat' ? `<div class="composer"><div class="line"><button class="tap">${I('clip')}</button><div class="field" data-kb="1">给协调者发消息…</div><button class="send">${I('send')}</button></div></div>` : '';
  return `<div class="bar"><button class="tap pri" data-act="back">${I('back')}</button><div class="ttl"><b>Payments API latency</b><span>${waiting() ? `<span class="dot"></span>${waiting()} 等你处理 · ` : ''}2 进行中</span></div><button class="tap" data-sheet="projmenu">${I('dots')}</button></div>
  <div class="seg">${[['chat', '对话'], ['threads', '线程'], ['lib', '文件库']].map(([k, l]) => `<button class="${S.ptab === k ? 'on' : ''}" data-ptab="${k}">${l}${k === 'threads' && waiting() ? `<span class="n">${waiting()}</span>` : ''}</button>`).join('')}</div>
  <div class="screen">${body}</div>${comp}`;
}
function threadScreen() {
  const id = S.thread, x = T[id], pend = S.pending[id] && x.ap;
  const status = pend ? '<span class="dot"></span>等你处理' : x.g === 'rev' ? '<span class="dot ok"></span>可以 review' : '<span class="dot run"></span>进行中';
  let body, dock = '';
  if (id === 't-cache') {
    body = `<div class="msg them"><b>报告</b><br>Lookups moved to Redis behind <span class="code" style="padding:1px 6px;display:inline">IDEMPOTENCY_REDIS</span>. p99 for /charge drops from 410 ms to 260 ms in the load test. Tests pass.</div>
    <div class="group" style="margin-top:12px"><button class="row" data-sheet="changes"><span class="glyph">${I('branch', 's')}</span><span class="main"><span class="t">变更</span><br><span class="d">3 个提交 · 6 个文件 · projects/t-cache</span></span>${chev}</button><button class="row" data-sheet="rail"><span class="glyph">${I('info', 's')}</span><span class="main"><span class="t">详情</span><br><span class="d">上下文 · 用量 · 模型</span></span>${chev}</button></div>
    <div class="act" style="margin-top:8px">${I('bolt', 's')}搜索了 1 次，执行了 1 条命令，改了 1 个文件 · 10m</div>`;
    dock = `<div class="dock"><div class="row2"><button class="btn p grow" data-act="done">${I('check', 's')}标记为已解决</button><button class="btn grow" data-kb="1">继续追问</button></div></div>`;
  } else if (id === 't-pool') {
    body = `<div class="act">${I('info', 's')}读了 1 个文件，搜索了 1 次，改了 1 个文件 · 3m</div><div class="msg them">Raised the pool to 40 open / 10 idle. To confirm under load I need to run the benchmark against staging.</div><div class="act">${I('bolt', 's')}<span class="code" style="padding:2px 8px">Bash go test ./... -run Bench -staging</span></div>`;
  } else body = `<div class="msg them">The SDK retries immediately today. I need to pick how it backs off before wiring it in.</div>`;
  if (pend) dock = `<div class="dock"><div class="h">${I(id === 't-pool' ? 'bolt' : 'msg', 's')}${x.ap[0]}</div><div class="q ${id === 't-pool' ? 'code' : ''}">${x.ap[1]}</div>
    <div class="row2"><button class="btn p grow" data-resolve="${id}">${id === 't-pool' ? '允许' : '用推荐回答'}</button><button class="btn grow" data-resolve="${id}">${id === 't-pool' ? '不允许' : '拒绝'}</button></div><button class="more" data-sheet="${id === 't-pool' ? 'allowopts' : 'askopts'}">${id === 't-pool' ? '本线程都允许 Bash…' : '其他选项…'}</button></div>`;
  const run = x.g === 'run' || pend;
  const comp = `<div class="composer"><div class="line"><button class="tap">${I('clip')}</button><div class="field" data-kb="1">给这个线程发消息…</div>${run ? `<button class="send stop">${I('stop', 's')}</button>` : `<button class="send">${I('send')}</button>`}</div></div>`;
  const kb = S.kb ? `<div class="kbd">${[10, 9, 7].map(n => `<div>${'<i></i>'.repeat(n)}</div>`).join('')}<div><i style="width:60px"></i><i style="width:160px"></i><i style="width:60px"></i></div></div>` : '';
  return `<div class="bar"><button class="tap pri" data-act="back">${I('back')}</button><div class="ttl"><b>${x.t}</b><span>${status}<span style="color:var(--text3)">· ${x.ago}前</span></span></div><button class="tap" data-sheet="rail">${I('info')}</button><button class="tap" data-sheet="tmenu">${I('dots')}</button></div>
  <div class="screen"><div class="scroll" style="padding-top:8px">${body}</div></div>${S.kb ? '' : dock}${comp}${kb}`;
}
