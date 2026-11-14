// part 2: agent, me, sheets, render + events
const AGENTS = [['claude', 'Claude Code', '已接入', '1842 次', 'ok'], ['openai', 'OpenAI SDK', '已接入', '310 次', 'ok'], ['codex', 'Codex', '未接入', '—', 'idle'], ['opencode', 'OpenCode', '已接入', '57 次', 'ok'], ['cursor', 'Cursor', '未接入', '—', 'idle']];
const RULES = [
  { n: 'claude-sonnet-5', s: '默认 → claude-sonnet-5 · 3 条智能规则', conds: [['agent.claude_code = subagent', 'glm-5.1'], ['agent.claude_code = compact', 'deepseek-v4-pro'], ['token ≥ 60000', 'qwen3.5-plus']], other: 'claude-sonnet-5 (official)' },
  { n: 'claude-opus-4-8', s: '默认 → claude-opus-4-8', conds: [], other: 'claude-opus-4-8 (official)' },
  { n: 'claude-haiku-4-5', s: '默认 → glm-5.1', conds: [], other: 'glm-5.1' },
];
function agentList() {
  return `<div class="large"><div><h1>智能应用</h1><div class="sub">今日 2209 次请求</div></div></div><div class="scroll"><div class="group">${AGENTS.map(a => `<button class="row" data-agent="${a[0]}"><span class="glyph">${I('apps', 's')}</span><span class="main"><span class="t">${a[1]}</span><span class="d"><span class="dot ${a[4]}"></span> ${a[2]} · ${a[3]}</span></span>${chev}</button>`).join('')}</div></div>`;
}
function agentDetail() {
  const c = (k, v, ic = 'copy') => `<button class="copy" data-toast="已复制"><span style="flex:1;min-width:0"><span class="k">${k}</span><br><span class="v">${v}</span></span>${I(ic, 's')}</button>`;
  return `<div class="bar"><button class="tap pri" data-act="back">${I('back')}</button><div class="ttl"><b>Claude Code</b><span><span class="dot ok"></span>已接入 · 最近请求 7 分钟前</span></div><button class="tap" data-sheet="agentmenu">${I('dots')}</button></div>
  <div class="screen"><div class="scroll" style="padding-top:12px">
  <div class="group"><div class="stat"><div><b>1842</b><span>今日请求</span></div><div><b style="color:var(--ok)">0.7%</b><span>错误率</span></div><div><b>260ms</b><span>首字节</span></div></div><button class="row" data-sheet="logs"><span class="glyph">${I('chart', 's')}</span><span class="t main">最近请求与日志</span>${chev}</button></div>
  <div style="margin-top:12px;display:flex;align-items:center;gap:8px;color:var(--warn);font-size:14px;padding:0 4px">配置已过期 · 1 处差异<button class="btn s" style="margin-left:auto;color:var(--primary)">自动配置</button></div>
  <div class="sec">接入信息 <span style="font-weight:400;color:var(--text3)">· 点按复制</span></div><div class="group">${c('Base URL', 'http://localhost:12580/tingly/claude_code')}<div style="border-top:1px solid var(--divider)">${c('API Key', '••••••••••••••••  mock-model-token')}</div><div style="border-top:1px solid var(--divider)">${c('安装', 'npm i -g @anthropic-ai/claude-code')}</div></div>
  <div class="sec">模型规则</div><div class="group">${RULES.map((r, i) => `<button class="row" data-rule="${i}"><span class="main"><span class="t" style="font-family:ui-monospace,Menlo,monospace;font-size:15px">${r.n}</span><span class="d">${r.s}</span></span>${chev}</button>`).join('')}</div>
  <div class="note">在手机上可以查看规则、更换目标模型、启停；拖拽排序和流程图编辑请在电脑上进行。</div></div></div>`;
}
function me() {
  const r = (ic, l, extra = '') => `<button class="row"><span class="glyph">${I(ic, 's')}</span><span class="t main">${l}</span>${extra}${chev}</button>`;
  return `<div class="large"><div><h1>我的</h1><div class="sub">mock-dev · 智能，尽在掌握。</div></div></div><div class="scroll"><div class="group">${r('users', '团队')}${r('lock', '凭证')}${r('chart', '仪表盘')}${r('image', 'Image')}${r('bolt', '扩展')}</div>
  <div class="sec">偏好</div><div class="group">${r('sliders', '主题与语言')}${r('msg', '通知', '<span class="r">Telegram</span>')}${r('info', '帮助')}</div>
  <div class="sec">界面</div><div class="group"><button class="row"><span class="t main">使用桌面版界面</span><span class="r">自动 · 手机版</span></button></div></div>`;
}
const SHEETS = {
  rail: () => `<h3>线程详情</h3><div class="body"><div class="kv"><span>分支</span><span>projects/${S.thread}</span></div><div class="kv"><span>变更</span><span>1 个提交 · 2 个文件</span></div><div class="kv"><span>模型</span><span>claude-haiku-4-5 · 高</span></div><div class="kv"><span>权限</span><span>自动接受编辑</span></div><div class="kv"><span>上下文</span><span>123k / 200k</span></div><div class="kv"><span>Tokens in / out</span><span>182k / 9.4k</span></div></div>`,
  changes: () => `<h3>变更 · 6 个文件</h3><div class="body">${['internal/idem/store.go', 'internal/idem/redis.go', 'internal/idem/store_test.go', 'cmd/server/main.go', 'bench/results.md', 'go.mod'].map((f, i) => `<div class="kv"><span style="color:var(--text);font-family:ui-monospace,Menlo,monospace;font-size:14px">${f}</span><span style="color:var(--ok)">+${40 - i * 5}</span></div>`).join('')}<div class="note">只读摘要；完整 diff 在电脑上查看。</div></div>`,
  tmenu: () => `<h3>线程</h3><div class="body">${['查看任务', '查看轨迹', '展开所有工具调用', '子代理', '设置（配置方案 · 模型 · 权限）'].map(l => `<button class="row" style="min-height:52px">${l}</button>`).join('')}</div>`,
  allowopts: () => `<h3>允许 Bash？</h3><div class="body"><button class="row" style="min-height:56px" data-resolve="t-pool"><span class="t main">仅这一次</span></button><button class="row" style="min-height:56px" data-resolve="t-pool"><span class="t main">本线程都允许 Bash</span></button><button class="row" style="min-height:56px" data-resolve="t-pool"><span class="t main" style="color:var(--err)">不允许</span></button></div>`,
  askopts: () => `<h3>Which backoff should retries use?</h3><div class="body">${[['Exponential with jitter', '推荐 · 避免重试同时到达'], ['Fixed 200 ms', '最简单，但重试集中'], ['Linear', '短故障的折中']].map(([a, b]) => `<button class="row" style="min-height:64px" data-resolve="t-backoff"><span class="main"><span class="t">${a}</span><span class="d">${b}</span></span></button>`).join('')}<div class="note">或者用你自己的话回答 — 在线程里输入。</div></div>`,
  projmenu: () => `<h3>Payments API latency</h3><div class="body">${['项目设置', '已绑定 IM · Telegram · Maya (phone)', '绑定聊天', '收藏'].map(l => `<button class="row" style="min-height:52px">${l}</button>`).join('')}</div>`,
  filter: () => `<h3>收件箱</h3><div class="body"><div class="kv"><span>显示项目</span><span>全部</span></div><div class="kv"><span>推送与 IM 通知</span><span>需要你时</span></div></div>`,
  agentmenu: () => `<h3>Claude Code</h3><div class="body"><div class="row" style="min-height:56px"><span class="t main">已启用</span><span class="toggle"></span></div>${['测试全部模型', '连接 AI（添加密钥）', '快速开始'].map(l => `<button class="row" style="min-height:52px">${l}</button>`).join('')}</div>`,
  logs: () => `<h3>最近请求</h3><div class="body">${[['200', 'claude-sonnet-5', '7m', '1.2s'], ['200', 'glm-5.1 (subagent)', '8m', '0.9s'], ['429', 'qwen3.5-plus', '15m', '—'], ['200', 'claude-sonnet-5', '16m', '2.0s']].map(([s, m, a, l]) => `<div class="kv"><span style="color:${s === '200' ? 'var(--ok)' : 'var(--err)'}">${s}</span><span style="flex:1;text-align:left;margin-left:12px;color:var(--text)">${m}</span><span>${a} · ${l}</span></div>`).join('')}</div>`,
  rule: () => { const r = RULES[S.rule || 0]; return `<h3 style="font-family:ui-monospace,Menlo,monospace">${r.n}</h3><div class="body">${r.conds.map((c, i) => `<div class="step"><span class="no">${i + 1}</span><div><b>如果</b> ${c[0]}<br><span class="to">→ ${c[1]}</span></div></div>`).join('')}<div class="step"><span class="no o">·</span><div><b>其他</b><br><span class="to">→ ${r.other}</span></div></div><button class="btn p" style="width:100%;margin-top:12px">更换目标模型</button></div>`; },
};
function render() {
  let main;
  if (S.tab === 'inbox') main = S.screen === 'thread' ? threadScreen() : inbox();
  else if (S.tab === 'work') main = S.screen === 'thread' ? threadScreen() : S.screen === 'project' ? projectScreen() : workList();
  else if (S.tab === 'agent') main = S.screen === 'detail' ? agentDetail() : agentList();
  else main = me();
  const pushed = S.screen !== 'root' && !(S.tab === 'work' && S.screen === 'project') && !(S.tab === 'agent' && S.screen === 'detail') ? S.screen === 'thread' : (S.screen !== 'root');
  const hideNav = S.screen === 'thread' || S.kb;
  $('#phone').innerHTML = `<div class="screen">${main}</div>${hideNav ? '' : nav()}<div class="scrim ${S.sheet ? 'on' : ''}" data-act="close"></div><div class="sheet ${S.sheet ? 'on' : ''}"><div class="grab"></div>${S.sheet ? SHEETS[S.sheet]() : ''}</div>${S.toast ? `<div class="toast">${S.toast}</div>` : ''}`;
}
document.addEventListener('click', (e) => {
  const el = e.target.closest('[data-tab],[data-ptab],[data-thread],[data-act],[data-sheet],[data-resolve],[data-agent],[data-rule],[data-toast],[data-kb]'); if (!el) return;
  const d = el.dataset; S.toast = '';
  if (d.tab) { S.tab = d.tab; S.screen = 'root'; S.kb = false; }
  else if (d.ptab) S.ptab = d.ptab;
  else if (d.thread) { S.thread = d.thread; S.screen = 'thread'; S.kb = false; }
  else if (d.resolve) { S.pending[d.resolve] = false; S.sheet = null; S.toast = d.resolve === 't-pool' ? '已允许 · 线程继续运行' : '已回答'; }
  else if (d.sheet) S.sheet = d.sheet;
  else if (d.agent) { S.screen = 'detail'; }
  else if (d.rule !== undefined) { S.rule = +d.rule; S.sheet = 'rule'; }
  else if (d.toast) S.toast = d.toast;
  else if (d.kb) S.kb = !S.kb;
  else if (d.act === 'close') S.sheet = null;
  else if (d.act === 'proj') { S.screen = 'project'; }
  else if (d.act === 'done') { S.resolved = true; S.screen = 'root'; S.toast = '已标记为已解决'; }
  else if (d.act === 'back') { if (S.kb) S.kb = false; else S.screen = (S.screen === 'thread' && S.tab === 'work') ? 'project' : 'root'; }
  render();
});
// deep-link states for screenshots: ?s=tab,screen,thread,sheet,ptab,kb  (&kb=1)
const st = q.get('s');
if (st) { const [tab, screen, thread, sheet, ptab] = st.split(','); Object.assign(S, { tab: tab || S.tab, screen: screen || 'root', thread: thread || S.thread, sheet: sheet || null, ptab: ptab || S.ptab }); if (q.get('kb')) S.kb = true; if (q.get('toast')) S.toast = q.get('toast'); if (q.get('rule')) S.rule = +q.get('rule'); }
render();
