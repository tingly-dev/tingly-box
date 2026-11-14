import type { Dict } from '../../dict';
import type { workspace as workspaceEn } from '../en/workspace';

// Workspace shell: landing page, navigation, sidebar/rail, layout chrome,
// shared labels.
export const workspace: Dict<typeof workspaceEn> = {
    common: {
        cancel: '取消', save: '保存', close: '关闭', delete: '删除', edit: '编辑', rename: '重命名',
        copy: '复制', copied: '已复制', more: '更多', star: '收藏', unstar: '取消收藏', retry: '重试',
        tokensApprox: '≈ {{value}} tokens', turnOn: '开启', turnOff: '关闭',
    },
    // 标签页在后台时的桌面通知（notifications/engine.ts）。
    notifications: {
        waiting: '等待你的输入',
        failed: '本轮运行失败',
        finished: '本轮已完成',
        decision: '等待你拍板',
    },
    needsYou: { title: '需要你处理', open: '打开', failed: '失败', more_one: '再显示 {{count}} 个', more_other: '再显示 {{count}} 个', fewer: '收起' },
    notifyControl: {
        label: '通知',
        scope: '标签页在后台时，线程需要你处理（提问、审批、失败、完成、决策）就弹出系统通知；点击后直达对应位置。',
        enable: '开启',
        blocked: '被浏览器拦截',
        howTo: '怎么解除',
        blockedTitle: '浏览器已拦截本站通知',
        blockedBody: '在浏览器的站点设置（地址栏左侧图标）里允许“通知”，允许后会自动开启。',
        unsupported: '此浏览器不支持',
    },
    nav: {
        newChat: '新对话', chats: '对话', projects: '项目', recentChats: '最近对话',
        settings: '设置', noChats: '还没有对话', collapse: '收起侧栏', expand: '展开侧栏',
    },
    rail: { coordinatorStatus: '协调者状态', coordinatorStatusShow: '展开协调者状态', coordinatorStatusHide: '收起协调者状态', title: '线程详情', environment: '环境', folder: '目录', branch: '分支', model: '模型', lastModel: '上次模型', permissions: '权限', usage: '用量', tokens: 'Token 输入 / 输出', turns: '轮数', changes: '改动', noCommits: '还没有提交', finished_one: '{{count}} 个已完成', finished_other: '{{count}} 个已完成', hide: '隐藏详情', show: '显示详情', close: '关闭详情', status: '状态', state: '当前', context: '上下文', contextEmpty: '还没有回合', contextTitle: '最近一次模型调用时上下文的占用', lastTurn: '上一轮耗时', lastActive: '最近活动', cache: '缓存读 / 写', cacheTitle: '从缓存读取 / 写入缓存的提示词 token', cacheHitLabel: '缓存命中', cacheHitTitle: '提示词 token 中由缓存提供的比例', session: '会话', queued: '排队中', queuedTitle: '在当前回合后面等待的消息、线程汇报和设置变更', threads: '线程', noThreads: '还没有线程', effort: '推理强度', coordinatorHide: '隐藏协调者状态', backgroundTasks: '后台任务', tasksInThreads_one: '{{tasks}} 个运行中，分布在 {{count}} 个线程', tasksInThreads_other: '{{tasks}} 个运行中，分布在 {{count}} 个线程' },
    links: { title: '议题与 PR', open: '打开', draft: '草稿', merged: '已合并', closed: '已关闭', checksFailing: '检查失败', checksPending: '检查进行中', checksPassing: '检查通过', conflicts: '有冲突', behind: '落后于 base', blocked: '被阻塞', ready: '可合并', stale: '刷新失败 · 读取于 {{age}}', readTitle: '上次向 GitHub 查询的时间', titleCount: '议题与 PR · {{count}}', tabOpen: '议题与 PR · {{count}} 个打开', filter: '显示', filterOpen: '打开 · {{count}}', filterClosed: '已结束 · {{count}}', prs: '拉取请求', issues: '议题', noneOpen: '没有打开的。', noneClosed: '还没有已合并或已关闭的。', empty: '线程打开或涉及的议题与 PR 会列在这里，并标明各自的进展。', openThread: '打开线程“{{title}}”' },
    sidebar: { showOlder_one: '显示更早的 {{count}} 个…', showOlder_other: '显示更早的 {{count}} 个…', fold: '收起 {{name}}', unfold: '展开 {{name}}', activity: '{{running}} 个运行中 · {{waiting}} 个在等你', running_one: '{{count}} 个运行中', running_other: '{{count}} 个运行中', recent: '最近的会话', noWorkdirs: '还没有工作目录', sessionsWaiting_one: '{{count}} 个会话在等你', sessionsWaiting_other: '{{count}} 个会话在等你', noProjects: '还没有项目', pausedTip: '已暂停 · 在设置中恢复', waiting_one: '{{count}} 个线程在等你', waiting_other: '{{count}} 个线程在等你' },
    theme: { system: '跟随系统', light: '浅色', dark: '深色', claude: 'Claude', ds: 'DeepSeek' },
    home: {
        greetingMorning: '早上好', greetingAfternoon: '下午好', greetingEvening: '晚上好', greetingNight: '夜深了，还在忙？',
        placeholder: '今天想聊点什么？',
        projectHint: '这里是和模型的快速对话。需要以线程方式持续推进的工作，请新建一个项目。',
    },
    workspace: {
        title: '工作区',
        intro: '在你的机器上使用 Claude Code：给项目一个目标，让协调者拆分工作；或者打开一个文件夹，直接在其中工作。',
        projectsBody: '适合目标超出一次会话的工作。协调者会让每个任务在各自的线程中运行。',
        projectsStep1: '创建项目，写下目标，也可以加上涉及的代码仓库。',
        projectsStep2: '描述工作内容。协调者为每个任务开一个线程，各自在独立分支上进行。',
        projectsStep3: '回来审阅已完成的内容，处理在等你的事项。',
        viewAllProjects: '全部项目',
        workdirBody: '适合快速、亲自动手的工作：打开本机上的文件夹，直接在其中运行 Claude Code。',
        workdirStep1: '添加本机上的一个文件夹，例如你正在开发的仓库。',
        workdirStep2: '开始一个会话并描述任务，Claude Code 会直接在该文件夹中工作。',
        workdirStep3: '在侧边栏中随时接着之前的会话继续。',
        viewAllWorkdirs: '全部工作目录',
        imBody: '从聊天应用直达项目：把聊天绑定到项目或目录，更新和回复会直接发到聊天里。',
        imStep1: '接入一个聊天机器人——Telegram、飞书或其他聊天应用。',
        imStep2: '从你想用的聊天里给机器人发消息，并授予访问权限。',
        imStep3: '把这个聊天绑定到项目或目录，之后就能在聊天里直接对话线程。',
        imAction: '绑定聊天',
        imBound_one: '{{count}} 个聊天已绑定', imBound_other: '{{count}} 个聊天已绑定', imManage: '管理',
        imHintTitle: '在聊天应用里工作',
        imHintBody: 'IM 机器人在页面下方这里接入：把聊天绑定到项目或目录，就能从 Telegram、飞书等聊天应用直接对话线程。',
        imHintGotIt: '知道了',
    },
    placement: {
        main: '主区域（切换）',
        title: '布局', overview: 'Overview 放在', thread: '线程打开在', side: '侧栏', window: '独立窗口', top: '顶栏', float: '浮窗', overviewFloat: '浮窗（从 Overview 点开时）', reasonTop: '在顶栏下方打开', reasonSide: '需要 Overview 在侧栏', remember: '记在这个浏览器里。',
    },
    window: {
        dock: '停靠到侧栏', float: '浮起为窗口', minimize: '最小化', restore: '还原', close: '关闭',
    },
    band: { collapse: '折起', expand: '展开' },
};
