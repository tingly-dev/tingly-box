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
  const r = (ic, l, d, page, extra = '') => `<button class="row" data-page="${page}"><span class="glyph">${I(ic, 's')}</span><span class="main"><span class="t">${l}</span><span class="d">${d}</span></span>${extra}${chev}</button>`;
  return `<div class="large"><div><h1>更多</h1><div class="sub">mock-dev · 智能，尽在掌握。</div></div></div><div class="scroll"><div class="sec">管理</div><div class="group">${r('users', '团队', '3 个成员 · 2 把密钥', 'team')}${r('lock', '凭证', '6 个 provider · 1 个异常', 'cred', '<span class="badge" style="position:static">1</span>')}${r('image', 'Image', '画廊 · 提示词 · API', 'fb:Image')}${r('chart', '仪表盘', '今日 5,662 次 · 错误率 0.53%', 'fb:仪表盘')}${r('bolt', '扩展', 'Skills · MCP', 'fb:扩展')}${r('msg', 'IM 机器人', '5 个机器人', 'fb:IM 机器人')}</div>
  <div class="sec">偏好</div><div class="group">${r('sliders', '主题与语言', '跟随系统 · 中文', 'fb:主题与语言')}${r('msg', '通知', 'Telegram · 需要你时', 'fb:通知')}${r('info', '帮助', '', 'fb:帮助')}</div>
  <div class="sec">界面</div><div class="group"><button class="row"><span class="t main">使用桌面版界面</span><span class="r">自动 · 手机版</span></button></div></div>`;
}
function teamPage() {
  const c = (k, v) => `<button class="copy" data-toast="已复制"><span style="flex:1;min-width:0"><span class="k">${k}</span><br><span class="v">${v}</span></span>${I('copy', 's')}</button>`;
  return `<div class="bar"><button class="tap pri" data-act="back" ${S.wide ? 'hidden' : ''}>${I('back')}</button><div class="ttl"><b>团队</b><span>3 个成员 · 2 把密钥</span></div><button class="tap pri" aria-label="新建密钥">${I('plus')}</button></div>
  <div class="screen"><div class="scroll" style="padding-top:12px"><div class="sec" style="margin-top:0">密钥</div><div class="group"><div class="row"><span class="main"><span class="t">alice-laptop</span><span class="d">今日 612 次 · 额度剩余 71%</span></span><span class="toggle"></span></div><div style="border-top:1px solid var(--divider)">${c('Base URL', 'http://localhost:12580/tingly/team/alice')}</div><div style="border-top:1px solid var(--divider)">${c('Key', 'tk-••••••••8f2a')}</div></div>
  <div class="group" style="margin-top:10px"><div class="row"><span class="main"><span class="t">ci-runner</span><span class="d">今日 41 次 · 额度剩余 92%</span></span><span class="toggle"></span></div></div>
  <div class="sec">成员</div><div class="group">${['Alice', 'Bo', 'Chen'].map(n => `<div class="row"><span class="glyph">${I('user', 's')}</span><span class="t main">${n}</span><span class="r">成员</span></div>`).join('')}</div>
  <div class="note">手机上可以查看、复制、启停、新建密钥；成员权限和路由模板请在电脑上编辑。</div></div></div>`;
}
function credPage() {
  return `<div class="bar"><button class="tap pri" data-act="back" ${S.wide ? 'hidden' : ''}>${I('back')}</button><div class="ttl"><b>凭证</b><span>6 个 provider</span></div><button class="tap pri" aria-label="连接 AI">${I('plus')}</button></div>
  <div class="screen"><div class="scroll" style="padding-top:12px"><div class="group">${[['Anthropic', '已连接 · 3 个模型', 'ok'], ['OpenAI', '已连接 · 5 个模型', 'ok'], ['DeepSeek', '已连接 · 2 个模型', 'ok'], ['Qwen', '密钥过期 · 重新授权', 'warn'], ['GLM', '已连接 · 1 个模型', 'ok'], ['Kimi', '已连接 · 2 个模型', 'ok']].map(([n, s, g]) => `<button class="row"><span class="glyph ${g}">${I(g === 'ok' ? 'check' : 'bolt', 's')}</span><span class="main"><span class="t">${n}</span><span class="d">${s}</span></span>${chev}</button>`).join('')}</div>
  <div class="note">手机上可以看状态、重新授权、启停、测试；批量编辑和导入请在电脑上进行。</div></div></div>`;
}
function fallbackPage(name) {
  return `<div class="bar"><button class="tap pri" data-act="back" ${S.wide ? 'hidden' : ''}>${I('back')}</button><div class="ttl"><b>${name}</b><span>桌面页面 · 手机自适应</span></div></div><div class="screen"><div class="scroll"><div class="fallback">这个页面还没有手机专用版本，先用桌面页面在手机框架里显示（单列、触控目标放大、不横向溢出）。常用后再逐个做成手机版。<div class="mock"></div></div></div></div>`;
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
// render + events: app3.js
