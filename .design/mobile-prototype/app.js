// Throwaway prototype for the mobile TEP: static, clickable, mock data from frontend/src/mocks/projectsHandlers.ts.
const P = {
  back: 'M15 6l-6 6 6 6', right: 'M9 6l6 6-6 6', down: 'M6 9l6 6 6-6', up: 'M6 15l6-6 6 6', star: 'M12 17.3l-5.4 3.2 1.4-6.1-4.7-4.1 6.2-.5L12 4l2.5 5.8 6.2.5-4.7 4.1 1.4 6.1z',
  dots: 'M12 6h.01M12 12h.01M12 18h.01', clip: 'M15 7l-6.5 6.5a1.5 1.5 0 0 0 3 3L18 10a3 3 0 0 0-6-6L5.5 10.5a4.5 4.5 0 0 0 9 9L21 13',
  send: 'M12 19V5M6 11l6-6 6 6', stop: 'M7 7h10v10H7z', check: 'M5 12l5 5L20 7', x: 'M6 6l12 12M18 6L6 18', sliders: 'M4 6h10M18 6h2M4 12h4M12 12h8M4 18h12M20 18h0M14 4v4M8 10v4M16 16v4',
  plus: 'M12 5v14M5 12h14', apps: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z', users: 'M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM3 20c0-3.3 2.7-6 6-6s6 2.7 6 6M16 5a3 3 0 0 1 0 6M18 14c1.8.6 3 2.2 3 4',
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z', lock: 'M6 11h12v9H6zM8 11V8a4 4 0 0 1 8 0v3', file: 'M7 3h7l5 5v13H7zM14 3v5h5',
  info: 'M12 8h.01M11 12h1v5h1M3 12a9 9 0 1 0 18 0 9 9 0 0 0-18 0', bolt: 'M13 3L5 14h6l-1 7 8-11h-6z', key: 'M14 10a4 4 0 1 0-3 3.9L11 15h2v2h2v2h3v-3z', copy: 'M8 8h11v11H8zM5 15V5h10', eye: 'M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12zM12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z',
  branch: 'M6 4v12M18 8a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM6 20a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM18 8c0 5-6 4-12 8', more: 'M5 12h.01M12 12h.01M19 12h.01', spin: 'M12 3a9 9 0 1 0 9 9', chart: 'M4 20V10M10 20V4M16 20v-8M22 20H2',
};
const I = (n, c = '') => `<svg class="i ${c}" viewBox="0 0 24 24"><path d="${P[n]}"/></svg>`;
const $ = (s, r = document) => r.querySelector(s);
const q = new URLSearchParams(location.search);
if (q.get('theme') === 'dark') document.body.dataset.theme = 'dark';
if (q.get('frame') !== '0') document.body.classList.add('framed');

const S = { tab: 'work', ptab: 'chat', screen: 'main', thread: 't-pool', pending: { 't-pool': true, 't-backoff': true }, open: { 1: true }, sheet: null, view: 'claude' };
const waiting = () => Object.values(S.pending).filter(Boolean).length;
const THREADS = [
  { id: 't-sdk', t: 'Retry budget in the SDK', g: 'run', m: 'payments-sdk · 1 file changed', ago: '1m' },
  { id: 't-queued', t: 'Audit logging for refunds', g: 'run', m: 'Waiting for a free slot', ago: '6s' },
  { id: 't-pool', t: 'Tune the DB connection pool', g: 'wait', m: 'Allow Bash: go test ./... -run Bench -staging', ago: '4m', ap: ['Allow Bash?', 'go test ./... -run Bench -staging'] },
  { id: 't-backoff', t: 'Pick a retry backoff', g: 'wait', m: 'Which backoff should retries use?', ago: '2m', ap: ['Which backoff?', 'Exponential with jitter (Recommended)'] },
  { id: 't-cache', t: 'Cache idempotency keys in Redis', g: 'rev', m: 'Lookups moved to Redis. p99 410 → 260 ms.', ago: '18m' },
  { id: 't-trace', t: 'Add tracing spans to /charge', g: 'done', m: 'Merged', ago: '20h' },
];

function nav() {
  const t = (k, ic, l, b) => `<button class="${S.tab === k ? 'on' : ''}" data-tab="${k}">${I(ic)}${l}${b ? `<span class="badge">${b}</span>` : ''}</button>`;
  return `<nav class="nav">${t('agent', 'apps', '智能应用')}${t('team', 'users', '团队')}${t('work', 'folder', '工作区', waiting())}${t('cred', 'lock', '凭证')}${t('more', 'more', '更多')}</nav>`;
}
function workHeader() {
  return `<div class="bar"><button class="tap" data-act="list">${I('back')}</button><div class="ttl"><b>Payments API latency</b></div>
  <button class="tap" data-act="star" style="color:var(--primary)">${I('star')}</button><button class="tap" data-act="projmenu">${I('dots')}</button></div>`;
}
const strip = () => waiting() ? `<button class="strip" data-act="inbox"><span class="dot"></span><span style="flex:1">${waiting()} 个线程等你处理</span><span style="color:var(--text2)">${I('right', 's')}</span></button>` : '';
const ptabs = () => `<div class="tabs" style="margin-top:10px">${[['chat', '对话'], ['threads', '线程'], ['lib', '文件库']].map(([k, l]) => `<button class="${S.ptab === k ? 'on' : ''}" data-ptab="${k}">${l}${k === 'threads' && waiting() ? `<span class="n">${waiting()}</span>` : ''}</button>`).join('')}</div>`;

function chat() {
  return `<div class="scroll"><div class="msg" style="color:var(--text2);font-size:12px">来自 Telegram · Maya (phone)</div>
  <div class="msg">I will add a retry budget in the SDK so an outage cannot amplify traffic. The pool benchmark still needs your approval.</div>
  <button class="card tap-row" data-thread="t-sdk"><div class="t"><span class="dot run"></span>Retry budget in the SDK<span class="r">进行中</span></div></button>
  <button class="fold" data-thread="t-cache"><span class="dot ok"></span><b>Cache idempotency keys in Redis</b> 已完成<span style="margin-left:auto;color:var(--text3)">打开线程</span></button>
  <div class="msg me">Cross-session message: Redis validation finished; load-test details are in the thread report.</div>
  <button class="fold" data-thread="t-pool"><span class="dot"></span><b>Tune the DB connection pool</b> 需要你回复<span style="margin-left:auto;color:var(--text3)">打开线程</span></button>
  <div class="msg">The Redis change is ready for review, and the tracing work is merged. Next, approve or deny the staging benchmark in the pool thread.</div></div>
  <div class="composer"><div class="line"><button class="tap">${I('clip')}</button><div class="field">给协调者发消息…</div><button class="send">${I('send')}</button></div>
  <button class="cfg" data-sheet="cfg">${I('sliders', 's')}<span>默认配置 · sonnet · 中</span>${I('up', 's')}</button></div>`;
}
function threadList(only) {
  const grp = [['wait', '等你处理'], ['run', '进行中'], ['rev', '可以 review'], ['done', '已解决']];
  return `<div class="scroll" style="padding-top:4px">${grp.map(([g, l]) => {
    const items = THREADS.filter(x => x.g === g && (g !== 'wait' || S.pending[x.id]));
    if (!items.length) return '';
    return `<div class="grp">${g === 'wait' ? '<span class="dot"></span>' : ''}${l}<span style="color:var(--text3)">${items.length}</span></div>` + items.map(x => `<div class="card"><button style="width:100%;text-align:left" data-thread="${x.id}"><div class="t">${x.t}<span class="r">${x.ago}</span></div><div class="m">${x.m}</div></button>
      ${g === 'wait' ? `<div class="inline-act"><button class="btn p sm" data-resolve="${x.id}">${x.id === 't-pool' ? '允许' : '用推荐回答'}</button><button class="btn sm" data-thread="${x.id}">查看</button></div>` : ''}</div>`).join('');
  }).join('')}</div>`;
}
function lib() {
  const r = (n, ic) => `<div class="field-row">${I(ic, 's')}<span style="flex:1;margin-left:8px">${n}</span><button class="tap">${I('down', 's')}</button></div>`;
  return `<div class="scroll"><div class="m" style="color:var(--text2);margin-bottom:8px">你添加的文件每个线程都能读取。</div>${r('outputs', 'folder')}${r('uploads', 'folder')}${r('baseline-p99.csv', 'file')}${r('charge-flamegraph.svg', 'file')}</div>`;
}
function workMain() {
  return `${workHeader()}${strip()}${ptabs()}<div class="screen">${S.ptab === 'chat' ? chat() : S.ptab === 'threads' ? threadList() : lib()}</div>`;
}
function projList() {
  const c = (n, g, m) => `<button class="card tap-row" data-act="proj"><div class="t">${n}</div><div class="m">${g}</div><div class="m" style="margin-top:8px">${m}</div></button>`;
  return `<div class="bar"><h1>工作区</h1><button class="tap" style="color:var(--primary)">${I('plus')}</button></div><div class="scroll">${c('Payments API latency', 'Get p99 of /charge under 300 ms without dropping retries.', `<span class="dot" style="display:inline-block"></span> ${waiting()} 等你处理 · 1 可以 review · 2 进行中`)}${c('Docs refresh', 'Every public endpoint documented with a runnable example.', '1 可以 review · 1 进行中')}${c('Postgres 17 migration', 'Move every service off Postgres 13 with zero downtime.', '已暂停')}</div>`;
}
function threadScreen() {
  const th = THREADS.find(x => x.id === S.thread), pend = S.pending[S.thread] && th.ap;
  const body = S.thread === 't-pool' ? `<div class="fold">${I('info', 's')}读了 1 个文件，搜索了 1 次，改了 1 个文件 · 3m 0s</div><div class="msg">Raised the pool to 40 open / 10 idle. To confirm under load I need to run the benchmark against staging.</div><div class="fold">${I('bolt', 's')}<span class="code" style="padding:2px 8px">Bash go test ./... -run Bench -staging</span></div>`
    : S.thread === 't-backoff' ? `<div class="msg">The SDK retries immediately today. I need to pick how it backs off.</div>` : `<div class="msg me">Also keep the Postgres path as a fallback.</div><div class="msg">Lookups moved to Redis behind <span class="code" style="padding:1px 6px">IDEMPOTENCY_REDIS</span>. p99 for /charge drops from 410 ms to 260 ms in the load test.</div>`;
  const sub = `${th.g === 'wait' && S.pending[S.thread] ? '<span class="dot"></span>等你处理' : th.g === 'run' ? '<span class="dot run"></span>进行中' : '<span class="dot ok"></span>可以 review'} <span style="color:var(--text3)">· ${th.ago}前</span>`;
  const ap = pend ? `<div class="approval"><div class="h">${I('bolt', 's')}${th.ap[0]}</div><div class="q code" style="margin:6px 0 10px">${th.ap[1]}</div><div class="row"><button class="btn p" data-resolve="${th.id}">${S.thread === 't-pool' ? '允许' : '用推荐回答'}</button><button class="btn" data-resolve="${th.id}">${S.thread === 't-pool' ? '不允许' : '拒绝'}</button></div></div>` : '';
  return `<div class="bar"><button class="tap" data-act="back">${I('back')}</button><div class="ttl"><b>${th.t}</b><span class="sub" style="display:flex;gap:6px;align-items:center">${sub}</span></div><button class="tap" data-sheet="rail">${I('info')}</button><button class="tap" data-sheet="tmenu">${I('dots')}</button></div>
  <div class="screen"><div class="scroll">${body}</div>${ap}<div class="composer"><div class="line"><button class="tap">${I('clip')}</button><div class="field">给这个线程发消息…</div>${th.g === 'run' || pend ? `<button class="send stop">${I('stop', 's')}</button>` : `<button class="send">${I('send')}</button>`}</div><button class="cfg" data-sheet="cfg">${I('sliders', 's')}<span>项目配置方案 · haiku-4-5 · 高 · 自动接受编辑</span>${I('up', 's')}</button></div></div>`;
}
