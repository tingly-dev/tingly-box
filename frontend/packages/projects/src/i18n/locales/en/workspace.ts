// Workspace shell: landing page, navigation, sidebar/rail, layout chrome,
// shared labels.
export const workspace = {
    common: {
        cancel: 'Cancel', save: 'Save', close: 'Close', delete: 'Delete', edit: 'Edit', rename: 'Rename',
        copy: 'Copy', copied: 'Copied', more: 'More', star: 'Star', unstar: 'Unstar', retry: 'Retry',
        tokensApprox: '≈ {{value}} tokens', turnOn: 'Turn on', turnOff: 'Turn off',
    },
    // Desktop notifications while the tab is in the background (notifications/engine.ts).
    notifications: {
        waiting: 'Waits on your input',
        failed: 'The turn failed',
        finished: 'Turn finished',
        decision: 'Wants your decision',
    },
    nav: {
        newChat: 'New chat', chats: 'Chats', projects: 'Projects', recentChats: 'Recent chats',
        settings: 'Settings', noChats: 'No chats yet', collapse: 'Close sidebar', expand: 'Open sidebar',
    },
    rail: { title: 'Thread details', environment: 'Environment', folder: 'Folder', branch: 'Branch', model: 'Model', lastModel: 'Last model', permissions: 'Permissions', usage: 'Usage', tokens: 'Tokens in / out', turns: 'Turns', changes: 'Changes', noCommits: 'No commits yet', finished_one: '{{count}} finished', finished_other: '{{count}} finished', hide: 'Hide details', show: 'Show details', status: 'Status', state: 'State', context: 'Context', contextEmpty: 'No turn yet', contextTitle: 'How full the context was on the last model call', lastTurn: 'Last turn', lastActive: 'Last active', cache: 'Cache read / write', cacheTitle: 'Prompt tokens served from the cache / written to it', cacheHitLabel: 'Cache hit', cacheHitTitle: 'Share of prompt tokens served from the cache', session: 'Session', queued: 'Queued', queuedTitle: 'Messages, thread reports and settings changes waiting behind the running turn', threads: 'Threads', noThreads: 'No threads yet', effort: 'Effort', coordinatorHide: 'Hide coordinator status', backgroundTasks: 'Background tasks', tasksInThreads_one: '{{tasks}} running in {{count}} thread', tasksInThreads_other: '{{tasks}} running in {{count}} threads' },
    sidebar: { showOlder_one: 'Show {{count}} older…', showOlder_other: 'Show {{count}} older…', fold: 'Fold {{name}}', unfold: 'Unfold {{name}}', activity: '{{running}} running · {{waiting}} waiting', running_one: '{{count}} running', running_other: '{{count}} running', recent: 'Recent sessions', noWorkdirs: 'No working folders yet', sessionsWaiting_one: '{{count}} session waits on you', sessionsWaiting_other: '{{count}} sessions wait on you', noProjects: 'No projects yet', pausedTip: 'Paused · resume in settings', waiting_one: '{{count}} thread waits on you', waiting_other: '{{count}} threads wait on you' },
    theme: { system: 'System', light: 'Light', dark: 'Dark', sunlit: 'Sunlit', claude: 'Claude', ds: 'DeepSeek' },
    home: {
        greetingMorning: 'Good morning', greetingAfternoon: 'Good afternoon', greetingEvening: 'Good evening', greetingNight: 'Burning the midnight oil?',
        placeholder: 'How can I help you today?',
        projectHint: 'A quick chat with the model. For ongoing work that should run as threads, start a project.',
    },
    workspace: {
        title: 'Workspace',
        intro: 'Claude Code on your machine: hand a project a goal and let a coordinator split the work, or open a folder and work in it directly.',
        projectsBody: 'For work with a goal that outlasts one session. A coordinator runs each task in its own thread.',
        projectsStep1: 'Create a project with a goal and, optionally, the repositories it touches.',
        projectsStep2: 'Describe the work. The coordinator starts a thread for each task, each on its own branch.',
        projectsStep3: 'Come back to review what is ready and answer what is waiting on you.',
        viewAllProjects: 'All projects',
        workdirBody: 'For quick, hands-on work: open a folder on this machine and run Claude Code right in it.',
        workdirStep1: 'Add a folder on this machine, such as a repository you are working in.',
        workdirStep2: 'Start a session and describe a task. Claude Code works in the folder itself.',
        workdirStep3: 'Pick any past session back up from the sidebar.',
        viewAllWorkdirs: 'All workdirs',
        imBody: 'Reach your projects from your chat apps: bind a chat to a project or folder, and its updates and replies arrive right in the chat.',
        imStep1: 'Connect a chat bot — Telegram, Feishu or another chat app.',
        imStep2: 'Message the bot from the chat you want to use and grant it access.',
        imStep3: 'Bind that chat to a project or folder, then talk to its threads from there.',
        imAction: 'Bind a chat',
        imHintTitle: 'Work from your chat apps',
        imHintBody: 'IM bots live down here: bind a chat to a project or folder and message its threads from Telegram, Feishu or another chat app.',
        imHintGotIt: 'Got it',
    },
    placement: {
        main: 'Main area (switch)',
        title: 'Layout', overview: 'Overview sits in', thread: 'Threads open in', side: 'Side pane', window: 'Window', top: 'Top bar', float: 'Floating window', overviewFloat: 'Floating, from the Overview',
        lockedTop: 'With the Overview on top, a thread opens below it.', remember: 'Remembered in this browser.',
    },
    window: {
        dock: 'Dock to the side pane', float: 'Pop out as a window', minimize: 'Minimize', restore: 'Restore', close: 'Close',
    },
    band: { collapse: 'Collapse', expand: 'Expand' },
};
