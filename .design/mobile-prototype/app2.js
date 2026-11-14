// part 2: agent page, sheets, render + events
const RULES = [
  { id: 1, name: 'claude-sonnet-5', conds: [['agent.claude_code = subagent', 'glm-5.1'], ['agent.claude_code = compact', 'deepseek-v4-pro'], ['token ≥ 60000', 'qwen3.5-plus']], other: 'claude-sonnet-5 (official)' },
  { id: 2, name: 'claude-opus-4-8', conds: [['无条件 · 跳过', '']], other: 'claude-opus-4-8 (official)' },
  { id: 3, name: 'claude-haiku-4-5', conds: [], other: 'glm-5.1' },
];
function agentPage() {
  const acc = (title, inner, k) => `<div class="card"><button class="acc" data-acc="${k}">${title}${I(S.open[k] ? 'up' : 'down', 's')}</button>${S.open[k] ? inner : ''}</div>`;
  const conn = `<div class="field-row"><span class="url">http://localhost:12580/tingly/claude_code</span><button class="tap">${I('copy', 's')}</button><div class="seg"><button class="on">Local</button><button>Docker</button></div></div>
    <div class="field-row"><span class="url" style="color:var(--text)">mock-model-token</span><button class="tap">${I('copy', 's')}</button><button class="tap">${I('eye', 's')}</button></div>
    <div class="field-row"><span class="url">npm i -g @anthropic-ai/claude-code</span><button class="tap">${I('copy', 's')}</button></div>`;
  const rules = RULES.map(r => `<div class="card rule"><button class="hd" data-rule="${r.id}"><b>${r.name}</b><span class="chip">1M: Off</span>${I(S.open['r' + r.id] ? 'up' : 'down', 's')}</button>${S.open['r' + r.id] ? `<div class="steps">${r.conds.map((c, i) => `<div class="step"><span class="no ${c[1] ? '' : 'o'}">${i + 1}</span><div class="c"><b>如果</b> ${c[0]}${c[1] ? `<br><span class="to">→ ${c[1]}</span>` : '<small>没有条件，已跳过</small>'}</div></div>`).join('')}
    <div class="step"><span class="no o">·</span><div class="c"><b>其他</b><br><span class="to">→ ${r.other}</span></div></div><button class="btn sm" style="width:100%;margin-top:6px">${I('plus', 's')} 添加智能条件</button></div>` : ''}</div>`).join('');
  return `<div class="bar"><h1>Claude Code<span class="sub"><span class="dot ok" style="display:inline-block;margin-right:6px"></span>已接入 · 今日 1842 次 · 错误率 0.7%</span></h1><button class="tap">${I('info')}</button><button class="tap">${I('dots')}</button></div>
  <div class="screen"><div class="scroll"><div class="chip" style="margin-bottom:10px;color:var(--warn);border-color:var(--warn)">配置已过期 · 1 处差异 <button style="color:var(--primary);margin-left:6px;font-weight:600">自动配置</button></div>
  ${acc('接入信息', conn, 0)}<div class="card"><div class="grp" style="margin:0 0 6px;justify-content:space-between"><span style="font-size:15px;color:var(--text)">模型规则</span><button class="tap" style="min-width:44px">${I('chart')}</button></div>${rules}</div></div>
  <button class="fab">${I('plus')}新建模型</button></div>`;
}
function simple(title) {
  return `<div class="bar"><h1>${title}</h1></div><div class="scroll"><div class="card"><div class="t">${title}</div><div class="m">本原型只展示 工作区 与 智能应用。</div></div></div>`;
}
function moreSheet() {
  const row = (ic, l) => `<button class="menu-row">${I(ic)}${l}</button>`;
  return `<h3>更多</h3><div class="body">${row('chart', '仪表盘')}${row('apps', 'Image')}${row('bolt', '扩展')}${row('info', '帮助')}${row('star', 'Star')}${row('sliders', '设置 · 主题 · 语言')}</div>`;
}
const SHEETS = {
  cfg: () => `<h3>线程设置</h3><div class="body"><div class="kv"><span>配置方案</span><span>项目配置方案 · 默认配置</span></div><div class="kv"><span>模型</span><span>claude-haiku-4-5</span></div><div class="kv"><span>推理力度</span><span>高</span></div><div class="kv"><span>权限</span><span>自动接受编辑 (下一轮生效)</span></div></div>`,
  rail: () => `<h3>线程详情</h3><div class="body"><div class="kv"><span>文件夹</span><span>…/threads/${S.thread}</span></div><div class="kv"><span>分支</span><span>projects/${S.thread}</span></div><div class="kv"><span>变更</span><span>1 个提交 · 2 个文件</span></div><div class="kv"><span>上下文</span><span>123k / 200k</span></div><div class="kv"><span>Tokens in / out</span><span>182k / 9.4k</span></div><div class="kv"><span>轮次</span><span>9</span></div></div>`,
  tmenu: () => `<h3>${THREADS.find(x => x.id === S.thread).t}</h3><div class="body">${['查看任务', '查看轨迹', '展开所有工具调用', '子代理', '标记为已解决'].map(l => `<button class="menu-row">${l}</button>`).join('')}</div>`,
  more: moreSheet,
  projmenu: () => `<h3>项目</h3><div class="body"><button class="menu-row">${I('sliders')}项目设置</button><button class="menu-row">${I('users')}已绑定 IM · Telegram · Maya (phone)</button><button class="menu-row">${I('plus')}绑定聊天</button></div>`,
};
function render() {
  let main;
  if (S.tab === 'work') main = S.screen === 'list' ? projList() : S.screen === 'thread' ? threadScreen() : workMain();
  else if (S.tab === 'agent') main = agentPage();
  else main = simple(S.tab);
  const showNav = !(S.tab === 'work' && S.screen === 'thread');
  $('#phone').innerHTML = `<div class="screen">${main}</div>${showNav ? nav() : ''}<div class="scrim ${S.sheet ? 'on' : ''}" data-act="close"></div><div class="sheet ${S.sheet ? 'on' : ''}"><div class="grab"></div>${S.sheet ? SHEETS[S.sheet]() : ''}</div>`;
}
document.addEventListener('click', (e) => {
  const el = e.target.closest('[data-tab],[data-ptab],[data-thread],[data-act],[data-sheet],[data-resolve],[data-acc],[data-rule]'); if (!el) return;
  const d = el.dataset;
  if (d.tab) { if (d.tab === 'more') S.sheet = 'more'; else { S.tab = d.tab; S.screen = d.tab === 'work' ? 'main' : 'main'; } }
  else if (d.ptab) S.ptab = d.ptab;
  else if (d.thread) { S.thread = d.thread; S.screen = 'thread'; }
  else if (d.resolve) { S.pending[d.resolve] = false; }
  else if (d.sheet) S.sheet = d.sheet;
  else if (d.acc !== undefined) S.open[d.acc] = !S.open[d.acc];
  else if (d.rule) S.open['r' + d.rule] = !S.open['r' + d.rule];
  else if (d.act === 'close') S.sheet = null;
  else if (d.act === 'inbox') S.ptab = 'threads';
  else if (d.act === 'back') S.screen = 'main';
  else if (d.act === 'list') S.screen = 'list';
  else if (d.act === 'proj') S.screen = 'main';
  else if (d.act === 'projmenu') S.sheet = 'projmenu';
  render();
});
// deep-link states for screenshots: ?s=tab,ptab,screen,thread,sheet
const st = q.get('s'); if (st) { const [tab, ptab, screen, thread, sheet] = st.split(','); Object.assign(S, { tab: tab || S.tab, ptab: ptab || S.ptab, screen: screen || S.screen, thread: thread || S.thread, sheet: sheet || null }); if (q.get('open')) q.get('open').split(',').forEach(k => S.open[k] = true); }
render();
