// Phone session (transcript) prototype for `.design/mobile-session.md`.
// Same mock conversation as frontend/src/mocks/projectsHandlers.ts, rendered
// two ways so the density choice can be compared on one dataset:
//   variant=A  conversation-first: a whole run folded to one line, process in a sheet
//   variant=B  structure kept: one line per call, folded open on tap
// Deep links: ?s=work,session &variant=A|B &theme=dark &w=&h= &scr=<px> &kb=1 &open=c1
const SESSION = {
  id: 't-pool',
  title: 'Tune the DB connection pool',
  project: 'Payments API latency',
  ago: '4m',
  branch: 'projects/t-pool',
  model: 'claude-haiku-4-5 · 高',
  checklist: [
    { t: '查看连接池配置', s: 'done' },
    { t: '按峰值估算连接数', s: 'now' },
    { t: '在 staging 跑基准', s: 'todo' },
  ],
};
S.variant = (q.get('variant') || 'A').toUpperCase() === 'B' ? 'B' : 'A';

const sessChev = I('right', 's chev');
const callLine = (icon, name, val, tag, openKey) =>
  `<button class="call" ${openKey ? `data-toggle="${openKey}"` : 'data-sheet="sactivity"'}>${I(icon, 's')}<b>${name}</b><span class="mono">${val}</span>${tag || ''}${sessChev}</button>`;
const thumb = (ic) => `<span class="thumb">${I(ic || 'image', 's')}</span>`;

function sessChecklist() {
  const open = !!S.open.sprog;
  const now = SESSION.checklist.find((x) => x.s === 'now');
  const done = SESSION.checklist.filter((x) => x.s === 'done').length;
  const items = SESSION.checklist.map((x) =>
    `<div class="it ${x.s === 'done' ? 'done' : x.s === 'now' ? 'now' : ''}"><span class="dot"></span>${x.t}</div>`).join('');
  if (S.variant === 'A') {
    return `<div class="progress-wrap"><button class="progress" data-toggle="sprog"><b>进度 ${done}/${SESSION.checklist.length}</b><span class="cur">${now ? '· ' + now.t : ''}</span>${I(open ? 'up' : 'down', 's chev')}</button>${open ? `<div class="items">${items}</div>` : ''}</div>`;
  }
  // B keeps a card header, folded to one line until tapped.
  return `<div class="progress-wrap"><button class="progress" data-toggle="sprog"><b>进度 ${done}/${SESSION.checklist.length}</b><span class="cur">${now ? '· ' + now.t : ''}</span>${I(now && !open ? 'right' : open ? 'up' : 'down', 's chev')}</button>${open ? `<div class="items">${items}</div>` : ''}</div>`;
}

const meTurn = (text, at) =>
  `<div class="turn me"><div class="msg me">${text}</div><div class="meta">${at}<button class="morebtn" data-sheet="smsg" aria-label="消息操作">${I('dots')}</button></div></div>`;
const themTurn = (html, at) =>
  `<div class="turn"><div class="msg them">${html}</div><div class="meta">${at}<button class="morebtn" data-sheet="smsg" aria-label="消息操作">${I('dots')}</button></div></div>`;

function runA() {
  return `<button class="proc" data-sheet="sactivity">${I('bolt', 's')}<span class="sum"><b>过程</b> 读 1 · 搜 1 · 改 1 · <span class="wait">1 待批准</span><span class="mut"> · 3m</span></span>${sessChev}</button>`;
}

function runB() {
  const c1 = !!S.open.c1;
  return `<div class="grp">
    <div class="hd">${I('bolt', 's')} 这一轮的过程 · 4 步 · 3m</div>
    <div class="tline">
      ${callLine('info', '思考', '先确认连接池怎么建，再按峰值并发估算。', '<span class="ok">3s</span>', 'th1')}
      ${callLine('file', 'Read', 'internal/db/pool.go', '<span class="ok">42 行</span>', 'c1')}
      ${c1 ? `<div class="detail"><div class="k">输出</div><pre class="code" style="margin:0 0 8px">MaxOpenConns: 10\nMaxIdleConns: 2</pre><div class="actgrid"><button class="btn s">复制输出</button><button class="btn s">在轨迹中定位</button></div></div>` : ''}
      ${callLine('eye', 'Grep', 'SetMaxOpenConns', '<span class="ok">1 处</span>', 'c2')}
      ${callLine('sliders', 'Edit', 'internal/db/pool.go', '<span class="ok">+8 −4</span>', 'c3')}
      ${callLine('bolt', 'Bash', 'go test ./... -run Bench -staging', '<span class="wait">等待批准</span>')}
    </div>
  </div>`;
}

function subagent() {
  const rep = '峰值时连接池饱和：MaxOpenConns=10 低于峰值并发 38，建议 40 打开 / 10 空闲。';
  const cap = S.variant === 'A' ? 'Explore · 完成' : 'Explore · 完成 · 6 工具 · 40s';
  return `<button class="subcard ${S.variant === 'A' ? 'one' : ''}" data-sheet="ssubagent">
    <span class="top">${I('apps', 's')}<b>Find pooling hotspots</b><span class="cap">${cap}</span>${sessChev}</span>
    <span class="rep">${rep}</span>
  </button>`;
}

function filesRow() {
  return `<button class="filesrow" data-sheet="sfiles">
    <span class="thumbs">${thumb('image')}${thumb('image')}${thumb('file')}</span>
    <span class="sum">3 个文件<span>2 张图 · 1 个文档 · 1.1 MB</span></span>${sessChev}
  </button>`;
}

function settled() {
  return `<button class="settled" data-sheet="smsg">${I('check', 's')}<b style="color:var(--text)">允许 Edit</b><span class="mono">bench/results.md</span><span class="ok">已允许</span></button>`;
}
function compacted() {
  return `<div class="cmdline">${I('check', 's')}<b style="color:var(--text);font-weight:600">会话已压缩</b><span class="mono">98k → 31k tokens</span><span class="cap">自动 · 12s</span></div>`;
}
function errorLine() {
  return `<button class="errline" data-sheet="serror">${I('bolt', 's')}<b>Bash 超时</b><span class="mono">go test ./store/...</span><span class="err">120s</span>${sessChev}</button>`;
}

function sessBody() {
  const code = `<pre class="code">db.SetMaxOpenConns(40)\ndb.SetMaxIdleConns(10)\ndb.SetConnMaxLifetime(5 * time.Minute)</pre>`;
  return `<div class="daymark">今天 10:14</div>
  ${meTurn('Profile /charge and size the connection pool for peak load.', '10:14')}
  ${themTurn('Profiling /charge first, then I will size the pool from the numbers.', '10:15')}
  ${S.variant === 'A' ? runA() : runB()}
  ${themTurn('Raised the pool to 40 open / 10 idle. To confirm under load I need to run the benchmark against staging.', '10:17')}
  ${subagent()}
  ${filesRow()}
  ${themTurn(`<span class="lbl">报告</span>Pool change is in. Staging load test, 400 rps for 20 min:${code}<p>p99 for /charge drops from <b>410 ms to 260 ms</b>. Tests pass.</p>`, '10:18')}
  ${settled()}
  ${compacted()}
  ${errorLine()}`;
}

function sessDock() {
  if (!S.pending['t-pool']) return '';
  return `<div class="dock sess-dock">
    <div class="h">${I('bolt', 's')} 允许 Bash？</div>
    <div class="q">go test ./... -run Bench -staging</div>
    <div class="row2"><button class="btn p grow" data-resolve="t-pool">${I('check', 's')}允许</button><button class="btn grow" data-resolve="t-pool">${I('x', 's')}不允许</button></div>
    <button class="more" data-sheet="allowopts">本线程都允许 Bash…</button>
  </div>`;
}

const composerLine = () => `<div class="line"><button class="tap">${I('clip')}</button><div class="field" data-kb="1">给这个线程发消息…</div><button class="send stop">${I('stop', 's')}</button></div>`;

function sessComposer() {
  if (S.variant === 'A') {
    return `<div class="composer sess-comp">${composerLine()}</div>`;
  }
  return `<div class="composer sess-comp">
    <div class="chipbar"><button class="chip" data-sheet="ssettings">${I('sliders', 's')}haiku-4-5</button><button class="chip" data-sheet="ssettings">${I('lock', 's')}自动接受编辑</button><button class="chip" data-sheet="ssettings" aria-label="会话设置">${I('dots', 's')}</button></div>
    ${composerLine()}
  </div>`;
}

function sessionScreen() {
  const kb = S.kb;
  const kbd = kb ? `<div class="kbd">${[10, 9, 7].map((n) => `<div>${'<i></i>'.repeat(n)}</div>`).join('')}<div><i style="width:60px"></i><i style="width:160px"></i><i style="width:60px"></i></div></div>` : '';
  // Typing: the full dock folds to one 44 px line, still one tap from allow/deny.
  const waitLine = kb && S.pending['t-pool']
    ? `<button class="proc sess-wait" data-sheet="allowopts">${I('bolt', 's')}<span class="sum"><b>等待批准</b> Bash go test ./... -run Bench…</span>${sessChev}</button>`
    : '';
  return `<div class="bar sess-bar">
    <button class="tap pri" data-act="back">${I('back')}</button>
    <div class="ttl"><b>${SESSION.title}</b><span>${dot2()}等你处理 · #1 · ${SESSION.ago} 前</span></div>
    <button class="tap" data-sheet="rail" aria-label="详情">${I('info')}</button>
    <button class="tap" data-sheet="tmenu" aria-label="更多">${I('dots')}</button>
  </div>
  ${kb ? '' : sessChecklist()}
  <div class="screen"><div class="scroll sess-scroll">${sessBody()}</div></div>
  ${kb ? '' : sessDock()}
  ${waitLine}
  ${sessComposer()}
  ${kbd}`;
}
const dot2 = () => S.pending['t-pool'] ? '<span class="dot warn"></span>' : '<span class="dot ok"></span>';

// Sheets: process detail, subagent, files, message actions, settings, error, checklist.
Object.assign(SHEETS, {
  schecklist: () => `<h3>进度 · 1/3</h3><div class="body">${SESSION.checklist.map((x) => `<div class="kv"><span>${x.t}</span><span>${x.s === 'done' ? '完成' : x.s === 'now' ? '进行中' : '待办'}</span></div>`).join('')}<div class="note">上一组任务（1 组）在轨迹里查看。</div></div>`,
  sactivity: () => `<h3>这一轮的过程</h3><div class="body">
    <div class="note">对话默认只显示结果；每一步的输入和输出在这里。整段可复制，长输出在自己的框里横向滚动。</div>
    <div class="tline">
      ${callLine('info', '思考', '先确认连接池怎么建，再按峰值并发估算。', '<span class="ok">3s</span>')}
      ${callLine('file', 'Read', 'internal/db/pool.go', '<span class="ok">42 行</span>')}
      <div class="detail"><div class="k">输入</div><pre class="code" style="margin:0 0 8px">{"file_path": "internal/db/pool.go"}</pre><div class="k">输出</div><pre class="code" style="margin:0 0 8px">MaxOpenConns: 10\nMaxIdleConns: 2</pre><button class="btn s">复制输出</button></div>
      ${callLine('eye', 'Grep', 'SetMaxOpenConns', '<span class="ok">1 处</span>')}
      ${callLine('sliders', 'Edit', 'internal/db/pool.go', '<span class="ok">+8 −4</span>')}
      ${callLine('bolt', 'Bash', 'go test ./... -run Bench -staging', '<span class="wait">等待批准</span>')}
    </div>
    <div class="actgrid"><button class="btn grow">复制这一轮</button><button class="btn grow" data-sheet="ssubagent">子代理</button></div>
  </div>`,
  ssubagent: () => `<h3 style="display:flex;align-items:center;gap:8px">${I('apps', 's')} Find pooling hotspots</h3><div class="body">
    <div class="kv"><span>类型</span><span>Explore · 完成</span></div>
    <div class="kv"><span>用量</span><span>6 工具 · 12.0k tokens · 40s</span></div>
    <div class="sec" style="margin:12px 4px 6px">它做了什么</div>
    <div class="tline">${callLine('eye', 'Grep', 'pool|MaxOpenConns', '<span class="ok">4 处</span>')}${callLine('file', 'Read', 'internal/db/pool.go', '<span class="ok">42 行</span>')}</div>
    <div class="sec" style="margin:12px 4px 6px">报告</div>
    <div class="msg them">峰值时连接池饱和：MaxOpenConns=10 低于峰值并发 38，建议 40 打开 / 10 空闲。</div>
    <div class="actgrid"><button class="btn grow">在轨迹中定位</button></div>
  </div>`,
  sfiles: () => `<h3>发来的文件 · 3</h3><div class="body">${[['p99.png', '54 KB · 图'], ['throughput.png', '62 KB · 图'], ['report.md', '4.8 KB · 文档']].map(([n, m]) => `<button class="row">${thumb(n.endsWith('.png') ? 'image' : 'file')}<span class="main"><span class="t">${n}</span><span class="d">${m}</span></span>${sessChev}</button>`).join('')}<div class="note">点按预览；完整图片库在电脑上。</div></div>`,
  smsg: () => `<h3>消息</h3><div class="body msgacts"><button class="row" data-toast="已复制">${I('copy', 's')}<span class="t main">复制文本</span></button><button class="row" data-toast="已复制">${I('info', 's')}<span class="main"><span class="t">复制时间</span><span class="d">今天 10:18</span></span></button><button class="row">${I('chart', 's')}<span class="t main">在轨迹中定位</span></button></div>`,
  ssettings: () => `<h3>会话设置</h3><div class="body"><div class="kv"><span>模型</span><span>claude-haiku-4-5 · 高</span></div><div class="kv"><span>权限</span><span>自动接受编辑</span></div><div class="kv"><span>自动回答</span><span>5 分钟后用推荐答案</span></div><div class="kv"><span>分支</span><span>${SESSION.branch}</span></div><div class="kv"><span>上下文</span><span>123k / 200k</span></div><div class="note">这些设置只在手机版里改；默认值是项目给的。</div></div>`,
  serror: () => `<h3>Bash 超时</h3><div class="body"><div class="kv"><span>命令</span><span style="font-family:ui-monospace,Menlo,monospace">go test ./store/...</span></div><div class="kv"><span>退出码</span><span style="color:var(--err)">124</span></div><div class="kv"><span>耗时</span><span>120s</span></div><pre class="code" style="margin-top:12px">panic: test timed out after 2m0s\n\trunning tests:\n\t\tTestIdempotencyStore (2m0s)</pre><button class="btn p" style="width:100%;margin-top:8px">重试这一步</button></div>`,
});

// After the shell renders, position the transcript for a screenshot (?scr=).
setTimeout(() => {
  const el = document.querySelector('.sess-scroll');
  const n = q.get('scr');
  if (el && n !== null) el.scrollTop = +n;
}, 60);
