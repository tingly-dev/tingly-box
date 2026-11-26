// Workspace-first home, folder session, size handling, render and events.
T['t-folder1'] = { t: 'Add dark mode to settings', p: 'notes-app', g: 'run', d: 'Editing src/Settings.tsx…', ago: '2m' };
T['t-folder2'] = { t: 'Fix flaky sync test', p: 'notes-app', g: 'rev', d: '1 file changed', ago: '1h' };

function workHome() {
  const wait = ['t-pool', 't-backoff'].filter(id => S.pending[id]);
  const act = (id) => `<div class="acts"><button class="btn p grow" data-resolve="${id}">${id === 't-pool' ? '允许' : '用推荐回答'}</button><button class="btn grow" data-thread="${id}">查看</button></div>`;
  const fold = (key, label, ids) => `<div class="group" style="margin-top:10px"><button class="row" data-toggle="${key}"><span class="t main">${label} · ${ids.length}</span>${I(S.open[key] ? 'up' : 'down', 's chev')}</button>${S.open[key] ? ids.map(id => tRow(id)).join('') : ''}</div>`;
  const r = (n, g, m, bad, act = 'proj') => `<button class="row" data-act="${act}"><span class="main"><span class="t">${n}</span><span class="d">${g}</span></span>${bad ? `<span class="badge" style="position:static">${bad}</span>` : ''}<span class="r">${m}</span>${chev}</button>`;
  return `<div class="large"><div><h1>工作区</h1><div class="sub">${waiting() ? `${waiting()} 件需要你` : '没有需要你的事'}</div></div><button class="tap pri" data-sheet="newtask" aria-label="新任务">${I('plus')}</button></div>
  <div class="scroll">${wait.length ? `<div class="sec"><span class="dot"></span>需要你回答 · ${wait.length}</div><div class="group">${wait.map(id => tRow(id) + act(id)).join('')}</div>` : ''}
  ${fold('rev', '可以 review', S.resolved ? ['t-docs'] : ['t-cache', 't-docs'])}${fold('run', '进行中', ['t-sdk', 't-queued'])}
  <div class="sec">项目</div><div class="group">${r('Payments API latency', 'p99 of /charge under 300 ms', '4m', waiting())}${r('Docs refresh', 'Documented endpoints with examples', '3h', 0)}${r('Postgres 17 migration', '已暂停', '2d', 0)}</div>
  <div class="sec">工作目录</div><div class="group">${r('notes-app', '/Users/me/code/notes-app', '2m', 0, 'folder')}${r('website', '/Users/me/code/website', '1m', 0, 'folder')}</div></div>`;
}
function folderScreen() {
  return `<div class="bar"><button class="tap pri" data-act="back" ${S.wide ? 'hidden' : ''}>${I('back')}</button><div class="ttl"><b>notes-app</b><span>/Users/me/code/notes-app</span></div><button class="tap" data-sheet="tmenu">${I('dots')}</button></div>
  <div class="screen"><div class="scroll" style="padding-top:8px"><div class="sec">会话</div><div class="group">${tRow('t-folder1', false)}${tRow('t-folder2', false)}</div><div class="note">在这个文件夹里直接开一个会话，改动直接落在目录上。</div></div></div>
  <div class="composer"><div class="line"><button class="tap">${I('clip')}</button><div class="field" data-kb="1">描述一个任务…</div><button class="send">${I('send')}</button></div></div>`;
}
Object.assign(SHEETS, {
  newtask: () => `<h3>新任务</h3><div class="body"><div class="group"><button class="row"><span class="glyph">${I('folder', 's')}</span><span class="main"><span class="cap">在</span><span class="t">Payments API latency</span></span>${I('down', 's chev')}</button></div>
    <div class="code" style="margin:12px 0;white-space:normal;color:var(--text3);min-height:88px">描述工作、粘贴报错…</div><div style="display:flex;gap:8px"><button class="btn grow" data-act="close">取消</button><button class="btn p grow" data-act="close">交给协调者</button></div><div class="note">线程的模型和权限用项目默认；要改，开始后在线程菜单里设置。</div></div>`,
  preview: () => `<h3 style="font-family:ui-monospace,Menlo,monospace">baseline-p99.csv</h3><div class="body"><div class="code" style="white-space:pre">ts,route,p99_ms\n10:00,/charge,412\n10:05,/charge,398\n10:10,/refund,205\n10:15,/charge,421</div><div class="note">只读预览；下载或编辑请在电脑上进行。</div></div>`,
});

function fit() {
  const p = $('#phone'), w = p.clientWidth - (document.body.classList.contains('framed') ? 16 : 0), h = p.clientHeight - (document.body.classList.contains('framed') ? 16 : 0);
  S.wide = w >= 600; S.land = !S.wide && w > h;
  p.dataset.size = w < 360 ? 'compact' : w >= 428 && w < 600 ? 'large' : 'regular';
  p.dataset.short = h < 700 && !S.land ? '1' : '0';
  p.dataset.flat = h < 500 ? '1' : '0';
  p.dataset.mode = S.wide ? 'wide' : S.land ? 'land' : 'portrait';
}
function render() {
  fit();
  const t = S.tab, sc = S.screen;
  let list, detail;
  if (t === 'work') {
    list = workHome();
    detail = sc === 'thread' ? threadScreen() : sc === 'project' ? projectScreen() : sc === 'folder' ? folderScreen() : '';
  } else if (t === 'agent') { list = agentList(); detail = sc === 'detail' ? agentDetail() : ''; }
  else { list = me(); detail = sc === 'team' ? teamPage() : sc === 'cred' ? credPage() : sc.startsWith('fb:') ? fallbackPage(sc.slice(3)) : ''; }
  let main;
  if (S.wide) main = `<div class="split"><div class="pane list">${list}</div><div class="pane detail">${detail || '<div class="empty">选择一个线程</div>'}</div></div>`;
  else main = `<div class="screen">${detail || list}</div>`;
  const hideNav = !S.wide && (S.kb || (detail && sc !== 'root' && !(S.land && false)));
  $('#phone').innerHTML = `${hideNav ? '' : nav()}${main}<div class="scrim ${S.sheet ? 'on' : ''}" data-act="close"></div><div class="sheet ${S.sheet ? 'on' : ''}"><div class="grab"></div>${S.sheet ? SHEETS[S.sheet]() : ''}</div>${S.toast ? `<div class="toast">${S.toast}</div>` : ''}`;
}
document.addEventListener('click', (e) => {
  const el = e.target.closest('[data-tab],[data-ptab],[data-thread],[data-act],[data-sheet],[data-resolve],[data-agent],[data-rule],[data-toast],[data-kb],[data-toggle],[data-page]'); if (!el) return;
  const d = el.dataset; S.toast = '';
  if (d.tab) { S.tab = d.tab; S.screen = 'root'; S.kb = false; }
  else if (d.page) S.screen = d.page;
  else if (d.toggle) S.open[d.toggle] = !S.open[d.toggle];
  else if (d.ptab) S.ptab = d.ptab;
  else if (d.thread) { S.thread = d.thread; S.screen = 'thread'; S.kb = false; S.sheet = null; }
  else if (d.resolve) { S.pending[d.resolve] = false; S.sheet = null; S.toast = d.resolve === 't-pool' ? '已允许 · 线程继续运行' : '已回答'; }
  else if (d.sheet) S.sheet = d.sheet;
  else if (d.agent) S.screen = 'detail';
  else if (d.rule !== undefined) { S.rule = +d.rule; S.sheet = 'rule'; }
  else if (d.toast) S.toast = d.toast;
  else if (d.kb) S.kb = !S.kb;
  else if (d.act === 'close') S.sheet = null;
  else if (d.act === 'proj') S.screen = 'project';
  else if (d.act === 'folder') S.screen = 'folder';
  else if (d.act === 'done') { S.resolved = true; S.screen = 'root'; S.toast = '已标记为已解决'; }
  else if (d.act === 'back') { if (S.kb) S.kb = false; else S.screen = (S.screen === 'thread' && S.fromProject) ? 'project' : 'root'; }
  if (d.act === 'proj') S.fromProject = true; else if (d.tab || d.act === 'folder') S.fromProject = false;
  render();
});
window.addEventListener('resize', render);
// size and deep-link states for screenshots: ?w=&h= (phone size), ?s=tab,screen,thread,sheet,ptab (&kb=1 &toast=…)
const W = +q.get('w'), H = +q.get('h');
if (W && H) { const f = document.body.classList.contains('framed') ? 16 : 0; Object.assign($('#phone').style, { width: (W + f) + 'px', height: (H + f) + 'px' }); }
const st = q.get('s');
if (st) {
  const [tab, screen, thread, sheet, ptab] = st.split(',');
  Object.assign(S, { tab: tab === 'inbox' ? 'work' : tab || S.tab, screen: screen || 'root', thread: thread || S.thread, sheet: sheet || null, ptab: ptab || S.ptab });
  S.fromProject = screen === 'thread' && ptab === 'threads';
  if (q.get('kb')) S.kb = true; if (q.get('toast')) S.toast = q.get('toast'); if (q.get('open')) q.get('open').split(',').forEach(k => S.open[k] = true);
}
render();
