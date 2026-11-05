import { http, HttpResponse } from 'msw'
import { resolveMockDataProfile } from './mockConfig'

// Mock-mode data for Projects (/api/v1/projects and /api/v1/threads,
// @tingly/projects): a few projects in different states, one of them with a
// conversation (its main thread's events), threads in every status (one
// waiting on the user, so the rail shows its dot), a small Library, and
// working folders, which are projects of type folder with plain threads.
// Mutations answer with plausible objects; nothing persists.

const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString()

const repo = (path: string) => ({ path, base_branch: 'main' })

type Proj = {
    id: string; type: 'project' | 'folder'; path?: string; main_thread_id?: string
    name: string; goal: string; instructions: string; paused: boolean; starred: boolean
    repos: { path: string; base_branch: string }[]; coordinator_model: string; thread_model: string
    coordinator_effort: string; thread_effort: string
    thread_permission_mode: string; max_running_threads: number; created_at: string; updated_at: string; archived_at?: string
}

const projects: Proj[] = [
    {
        id: 'p-latency', type: 'project', main_thread_id: 'main-p-latency', name: 'Payments API latency', goal: 'Get p99 of /charge under 300 ms without dropping retries.',
        instructions: 'Keep changes behind the existing feature flags.', paused: false, starred: true,
        repos: [repo('/Users/me/code/payments-api'), repo('/Users/me/code/payments-sdk')],
        coordinator_model: 'sonnet', thread_model: 'claude-haiku-4-5', coordinator_effort: 'medium', thread_effort: 'high', thread_permission_mode: 'acceptEdits', max_running_threads: 4,
        created_at: ago(60 * 26), updated_at: ago(4),
    },
    {
        id: 'p-docs', type: 'project', main_thread_id: 'main-p-docs', name: 'Docs refresh', goal: 'Every public endpoint documented with a runnable example.',
        instructions: '', paused: false, starred: false, repos: [repo('/Users/me/code/website')],
        coordinator_model: 'claude-sonnet-4-5', thread_model: 'claude-haiku-4-5', coordinator_effort: '', thread_effort: '', thread_permission_mode: 'default', max_running_threads: 4,
        created_at: ago(60 * 24 * 5), updated_at: ago(60 * 3),
    },
    {
        id: 'p-migrate', type: 'project', main_thread_id: 'main-p-migrate', name: 'Postgres 17 migration', goal: 'Move every service off Postgres 13 with zero downtime.',
        instructions: '', paused: true, starred: false, repos: [repo('/Users/me/code/infra')],
        coordinator_model: 'claude-sonnet-4-5', thread_model: 'claude-sonnet-4-5', coordinator_effort: '', thread_effort: 'max', thread_permission_mode: 'default', max_running_threads: 2,
        created_at: ago(60 * 24 * 12), updated_at: ago(60 * 24 * 2),
    },
]

// Working folders: projects of type folder (no goal, memory or main thread).
const folderProject = (id: string, path: string, min: number): Proj => ({
    id, type: 'folder', path, name: path.split('/').pop() ?? path, goal: '', instructions: '', paused: false, starred: false, repos: [],
    coordinator_model: '', thread_model: '', coordinator_effort: '', thread_effort: '', thread_permission_mode: '', max_running_threads: 0, created_at: ago(60 * 24 * 3), updated_at: ago(min),
})
projects.push(
    folderProject('f-notes', '/Users/me/code/notes-app', 2),
    folderProject('f-website', '/Users/me/code/website', 1),
    folderProject('f-scratch', '/Users/me/scratch', 60 * 30),
)

// What a thread's turns spent, derived from its turn count and tokens so every mock thread carries a plausible status.
const usageOf = (o: { turns?: unknown; input_tokens?: unknown; updated_at?: unknown }) => {
    const turns = Number(o.turns ?? 0)
    const input = Number(o.input_tokens ?? 0)
    return {
        cache_read_tokens: Math.round(input * 0.72), cache_write_tokens: Math.round(input * 0.04),
        context_tokens: turns > 0 ? Math.min(188_000, 24_000 + turns * 11_000) : 0, context_window: 200_000,
        last_turn_ms: turns > 0 ? 2_400 + turns * 1_300 : 0, last_turn_at: turns > 0 ? (o.updated_at as string) : undefined,
    }
}

const thread = (o: Partial<Record<string, unknown>> & { id: string; title: string; status: string; task: string }) => ({
    project_id: 'p-latency', parent_id: 'main-p-latency', seq: 1, agent: 'claude-code', origin: 'coordinator', commits_ahead: 0, files_changed: 0, input_tokens: 0, output_tokens: 0,
    turns: 0, uncommitted: false, cwd: `/Users/me/.tingly-box/projects/p-latency/threads/${o.id}`,
    repo_path: '/Users/me/code/payments-api', base_branch: 'main', branch: `projects/${o.id}`,
    model: 'claude-sonnet-4-5', reported_model: 'claude-sonnet-4-5', created_at: ago(90), updated_at: ago(5), session_id: `${(o.id as string).length.toString(16).padStart(2, '0')}c9d5e7a-4b1e-4f7a-9d0e-5a2b7c3d8e9f`,
    cache_read_tokens: 0, cache_write_tokens: 0, context_tokens: 0, context_window: 0, last_turn_ms: 0, ...o,
})
const threadWithUsage = (o: Parameters<typeof thread>[0]) => { const t = thread(o); return { ...t, ...usageOf(t), ...o } }

const threads = [
    threadWithUsage({ id: 't-pool', title: 'Tune the DB connection pool', status: 'waiting', task: 'Profile /charge and size the connection pool for peak load.', wait_reason: 'Allow `go test ./... -run Bench` to run against staging?', turns: 9, input_tokens: 182_000, output_tokens: 9_400, commits_ahead: 1, files_changed: 2, updated_at: ago(4) }),
    threadWithUsage({ id: 't-cache', title: 'Cache idempotency keys in Redis', status: 'ready', task: 'Move idempotency-key lookups from Postgres to Redis with a 24h TTL.', last_report: 'Lookups moved to Redis behind `IDEMPOTENCY_REDIS`. p99 for /charge drops from 410 ms to 260 ms in the load test. Tests pass.', turns: 14, input_tokens: 240_000, output_tokens: 15_100, commits_ahead: 3, files_changed: 6, updated_at: ago(18) }),
    threadWithUsage({ id: 't-sdk', title: 'Retry budget in the SDK', status: 'working', task: 'Cap client retries with a token bucket so retries cannot amplify an outage.', repo_path: '/Users/me/code/payments-sdk', turns: 5, input_tokens: 64_000, output_tokens: 3_100, files_changed: 1, uncommitted: true, updated_at: ago(1) }),
    // Over the project's limit: its turn waits for a free slot.
    threadWithUsage({ id: 't-queued', title: 'Audit logging for refunds', status: 'working', queued: true, task: 'Add audit log entries for every refund path.', updated_at: ago(0) }),
    threadWithUsage({ id: 't-backoff', title: 'Pick a retry backoff', status: 'waiting', task: 'Choose how the SDK backs off between retries and wire it in.', repo_path: '/Users/me/code/payments-sdk', wait_reason: 'Which backoff should retries use?', turns: 3, input_tokens: 38_000, output_tokens: 1_900, updated_at: ago(2) }),
    threadWithUsage({ id: 't-trace', title: 'Add tracing spans to /charge', status: 'resolved', task: 'Add OpenTelemetry spans around each downstream call in /charge.', turns: 7, input_tokens: 90_000, output_tokens: 6_200, commits_ahead: 2, files_changed: 4, merged_at: ago(60 * 20), resolved_at: ago(60 * 20), updated_at: ago(60 * 20) }),
]

// A thread's transcript: the coordinator's brief, then what the thread did.
// t-pool waits on an approval, t-backoff on a question, t-sdk is running
// its tests; t-cache finished with a report.
const threadEvents = (th: (typeof threads)[number]) => {
    let n = 0
    const ev = (kind: string, content: string, min: number, extra: Record<string, unknown> = {}) =>
        ({ id: `${th.id}-e${++n}`, thread_id: th.id, kind, content, created_at: ago(min), ...extra })
    const call = (tool: string, input: unknown, out: string, min: number) => [
        ev('tool_use', tool, min, { payload: JSON.stringify(input) }),
        ev('tool_result', out, min, { payload: '{"is_error":false}' }),
    ]
    const brief = ev('user', th.task, 30, { source: 'thread', source_ref: 'main-p-latency' })
    if (th.id === 't-pool') {
        return [
            brief,
            ev('assistant', 'Profiling /charge first, then I will size the pool from the numbers.', 12),
            ...call('Read', { file_path: 'internal/db/pool.go' }, 'MaxOpenConns: 10, MaxIdleConns: 2', 11),
            ...call('Grep', { pattern: 'SetMaxOpenConns' }, 'internal/db/pool.go:42', 11),
            ...call('Edit', { file_path: 'internal/db/pool.go' }, 'Updated pool sizing', 8),
            ev('assistant', 'Raised the pool to 40 open / 10 idle. To confirm under load I need to run the benchmark against staging.', 5),
            ev('tool_use', 'Bash', 4, { payload: JSON.stringify({ command: 'go test ./... -run Bench -staging' }) }),
            ev('approval', 'Run go test ./... -run Bench -staging', 4, { payload: JSON.stringify({ request_id: 'req-pool', tool: 'Bash', input: { command: 'go test ./... -run Bench -staging' }, state: 'pending' }) }),
        ]
    }
    if (th.id === 't-backoff') {
        const input = { questions: [
            { question: 'Which backoff should retries use?', header: 'Backoff', multiSelect: false, options: [
                { label: 'Exponential with jitter (Recommended)', description: 'Spreads retries out so clients do not retry in lockstep after an outage.' },
                { label: 'Fixed 200 ms', description: 'Simplest; predictable, but retries arrive together.' },
                { label: 'Linear', description: 'Grows slowly; a middle ground for short outages.' },
            ] },
        ] }
        return [
            brief,
            ...call('Read', { file_path: 'client/retry.go' }, 'for attempt := 0; attempt < max; attempt++ {', 6),
            ev('assistant', 'The client retries immediately today. Before I add a backoff I need you to pick one.', 3),
            ev('ask', 'Which backoff should retries use?', 2, { payload: JSON.stringify({ request_id: 'req-backoff', tool: 'AskUserQuestion', input, state: 'pending', auto_at: new Date(Date.now() + 8 * 60_000).toISOString() }) }),
        ]
    }
    if (th.id === 't-sdk') {
        return [
            ev('user', th.task, 3, { source: 'thread', source_ref: 'main-p-latency' }),
            ...call('TaskCreate', { subject: 'Inspect the retry loop' }, 'Task #1 created successfully: Inspect the retry loop', 3),
            ...call('TaskUpdate', { taskId: '1', status: 'completed' }, 'Updated task #1', 3),
            ...call('TaskCreate', { subject: 'Add retry budget', activeForm: 'Adding retry budget' }, 'Task #2 created successfully: Add retry budget', 3),
            ...call('TaskCreate', { subject: 'Run client tests' }, 'Task #3 created successfully: Run client tests', 3),
            ...call('TaskCreate', { subject: 'Document the rollout' }, 'Task #4 created successfully: Document the rollout', 3),
            ...call('TaskUpdate', { taskId: '2', status: 'in_progress' }, 'Updated task #2', 3),
            ...call('Read', { file_path: 'client/retry.go' }, 'for attempt := 0; attempt < max; attempt++ {', 3),
            ev('assistant', 'Adding a token bucket in front of the retry loop; refills at 10% of the request rate.', 2),
            ...call('Edit', { file_path: 'client/budget.go' }, 'Created budget.go', 2),
            ev('tool_use', 'Bash', 1, { payload: JSON.stringify({ command: 'go test ./client/...' }) }),
            // Sent while the turn runs: Claude reads it at its next step. The next one is held for after the turn.
            ev('user', 'Also log each dropped retry at debug level.', 1, { payload: JSON.stringify({ injected: true }) }),
            ev('queued', 'When it passes, write the rollout notes into docs/retries.md.', 1, { payload: JSON.stringify({ state: 'held' }) }),
        ]
    }
    if (th.id === 't-cache') {
        const report = 'Lookups moved to Redis behind `IDEMPOTENCY_REDIS`. p99 for /charge drops from 410 ms to 260 ms in the load test.\n\n```go\nkey := "idem:" + req.Key\nok, err := rdb.SetNX(ctx, key, 1, 24*time.Hour).Result()\n```'
        return [
            brief,
            ...call('Grep', { pattern: 'idempotency_keys' }, 'store/idempotency.go:18', 40),
            ...call('Edit', { file_path: 'store/idempotency.go' }, 'ok', 35),
            ev('tool_use', 'Bash', 30, { payload: JSON.stringify({ command: 'go test ./store/...' }) }),
            ev('approval', 'Run go test ./store/...', 30, { payload: JSON.stringify({ request_id: 'req-cache', tool: 'Bash', input: { command: 'go test ./store/...' }, state: 'allowed', scope: 'thread' }) }),
            ev('tool_result', 'ok  store 1.8s', 29, { payload: '{"is_error":false}' }),
            ev('user', 'Also keep the Postgres path as a fallback.', 25),
            ev('assistant', report, 18),
            ev('report', report, 18, { payload: '{"commits_ahead":3,"files_changed":6}' }),
        ]
    }
    return [brief]
}

const counts = (pid: string) => {
    const list: { status: string }[] = pid === 'p-latency' ? threads : pid.startsWith('f-') ? sessions.filter((x) => x.project_id === pid) : []
    const n = (s: string) => list.filter((t) => t.status === s).length
    if (pid === 'p-docs') return { waiting: 0, ready: 1, working: 1, idle: 0, resolved: 4 }
    if (pid === 'p-migrate') return { waiting: 0, ready: 0, working: 0, idle: 2, resolved: 1 }
    return { waiting: n('waiting'), ready: n('ready'), working: n('working'), idle: n('idle'), resolved: n('resolved') }
}

// The project conversation is its main thread's events: kinds user,
// assistant, thread_started, thread_update (no role).
const msg = (id: string, kind: string, content: string, min: number, payload?: unknown, extra: Record<string, unknown> = {}) => ({
    id, thread_id: 'main-p-latency', kind, content, created_at: ago(min),
    ...(payload ? { payload: JSON.stringify(payload) } : {}), ...extra,
})

const messages = [
    msg('m1', 'user', '/charge p99 is at 410 ms in prod. Find out where the time goes and fix what you can — the pool, the idempotency lookup, anything obvious.', 95),
    msg('m2', 'assistant', 'Traces point at two hot spots: idempotency-key lookups hit Postgres on every request, and the connection pool saturates at peak. I started a thread for each, plus one to add the tracing spans we need to confirm the fix.', 94),
    msg('m3', 'thread_started', '', 94, { thread_ids: ['t-cache', 't-pool', 't-trace'] }),
    msg('m4', 'thread_update', '', 60 * 20, { thread_id: 't-trace', title: 'Add tracing spans to /charge', kind: 'finished' }),
    // Written from the bound Telegram chat (TEP #2143): the conversation shows where it came from.
    msg('m5', 'user', 'Also make sure SDK retries cannot pile onto an outage.', 30, undefined, { source: 'im', source_platform: 'telegram', source_chat_id: 'telegram:123456789' }),
    msg('m5a', 'assistant', 'I will add a retry budget in the SDK so an outage cannot amplify traffic. The pool benchmark still needs your approval; that thread remains available below.', 29.5),
    msg('m6', 'thread_started', '', 29, { thread_ids: ['t-sdk'] }),
    msg('m7', 'thread_update', '', 18, { thread_id: 't-cache', title: 'Cache idempotency keys in Redis', kind: 'finished' }),
    msg('m7a', 'user', 'Cross-thread message: Redis validation finished; load-test details are in the thread report.', 17, undefined, { source: 'thread', source_ref: 't-cache' }),
    msg('m8', 'thread_update', '', 4, { thread_id: 't-pool', title: 'Tune the DB connection pool', kind: 'needs_input' }),
    msg('m9', 'assistant', 'The Redis change is ready for review, and the tracing work is merged. Next, approve or deny the staging benchmark in the pool thread; I will keep the retry-budget work separate until its tests finish.', 3),
]

const library = [
    { path: 'load-test-2026-10-14.md', origin: 'thread', size: 4_812, updated_at: ago(18) },
    { path: 'charge-flamegraph.svg', origin: 'user', size: 182_330, updated_at: ago(60 * 22) },
]

// The Library tree: path → one level of entries (folders first; the real
// server hides dot entries such as the threads' .tmp).
const entry = (path: string, kind: 'dir' | 'file', size = 0, min = 60) => ({ name: path.split('/').pop()!, path, kind, size, updated_at: ago(min), origin: path.startsWith('outputs') ? 'output' : 'upload' })
const libraryTree: Record<string, ReturnType<typeof entry>[]> = {
    '': [entry('outputs', 'dir', 0, 18), entry('uploads', 'dir', 0, 60 * 22), entry('baseline-p99.csv', 'file', 31_204, 60 * 5), entry('charge-flamegraph.svg', 'file', 182_330, 60 * 22)],
    outputs: [entry('outputs/load-test-2026-10-14', 'dir', 0, 18), entry('outputs/redis-keys', 'dir', 0, 60 * 3), entry('outputs/summary.md', 'file', 2_410, 40)],
    'outputs/load-test-2026-10-14': [entry('outputs/load-test-2026-10-14/charts', 'dir', 0, 18), entry('outputs/load-test-2026-10-14/report.md', 'file', 4_812, 18), entry('outputs/load-test-2026-10-14/results.json', 'file', 96_020, 19)],
    'outputs/load-test-2026-10-14/charts': [entry('outputs/load-test-2026-10-14/charts/p99.png', 'file', 54_300, 18), entry('outputs/load-test-2026-10-14/charts/throughput.png', 'file', 61_870, 18)],
    'outputs/redis-keys': [entry('outputs/redis-keys/design.md', 'file', 3_120, 60 * 3)],
    uploads: [entry('uploads/incident-2026-10-02.pdf', 'file', 842_000, 60 * 22), entry('uploads/slo-targets.xlsx', 'file', 18_950, 60 * 30)],
}

// Library file bodies for the preview: a chart PNG, text for the rest.
const chartPng = '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'
const libraryText = (path: string) => path.endsWith('.csv') ? 'minute,p99_ms\n0,212\n1,208\n2,231\n3,219\n' : `# ${path.split('/').pop()}\n\nP99 charge latency dropped from 212 ms to 148 ms after the pool change.\n\n- Run: 2026-10-14, staging\n- Load: 400 rps for 20 min\n`

// Folder threads: Claude Code in plain folders (projects of type folder). One
// waits on an approval, one is working, one finished, one is archived.
const folderId = (cwd: string) => projects.find((p) => p.type === 'folder' && p.path === cwd)?.id ?? ''

const session = (o: Partial<Record<string, unknown>> & { id: string; title: string; status: string; task: string; cwd: string }) => ({
    project_id: folderId(o.cwd), seq: 1, agent: 'claude-code', origin: 'user', permission_mode: '' as string | undefined, profile: '' as string | undefined, commits_ahead: 0, files_changed: 0, input_tokens: 0, output_tokens: 0, turns: 1,
    uncommitted: false, model: '' as string | undefined, repo_path: '', base_branch: '', branch: '', session_id: o.id,
    created_at: ago(120), updated_at: ago(8),
    cache_read_tokens: 0, cache_write_tokens: 0, context_tokens: 0, context_window: 0, last_turn_ms: 0, ...o,
})
const sessionWithUsage = (o: Parameters<typeof session>[0]) => { const t = session(o); return { ...t, ...usageOf(t), ...o } }

const sessions = [
    sessionWithUsage({ id: 's-dark', title: 'Add a dark mode toggle', status: 'waiting', cwd: '/Users/me/code/notes-app', head_branch: 'main', task: 'Add a dark mode toggle to the settings page and remember it.', wait_reason: 'Approve: Run `npm test`', input_tokens: 48_000, output_tokens: 2_900, updated_at: ago(2) }),
    sessionWithUsage({ id: 's-links', title: 'Fix the broken links check', status: 'working', cwd: '/Users/me/code/website', head_branch: 'fix/link-anchors', task: 'The link checker fails on anchors. Fix it.', updated_at: ago(1) }),
    sessionWithUsage({ id: 's-readme', title: 'Rewrite the README intro', status: 'idle', cwd: '/Users/me/code/website', head_branch: 'main', task: 'Rewrite the README intro in plain language.', permission_mode: 'plan', input_tokens: 21_000, output_tokens: 1_500, updated_at: ago(50) }),
    sessionWithUsage({ id: 's-old', title: 'Rename the config keys', status: 'resolved', cwd: '/Users/me/scratch', task: 'Rename the config keys to snake_case.', resolved_at: ago(60 * 30), updated_at: ago(60 * 30) }),
]

// The project's main thread, with the coordinator session's own usage.
const coordinatorOf = (p: Proj) => {
    const turns = p.id === 'p-latency' ? 23 : 6
    const input = p.id === 'p-latency' ? 1_240_000 : 210_000
    const updated_at = ago(p.id === 'p-latency' ? 3 : 20)
    return {
        ...thread({ id: p.main_thread_id!, title: p.name, status: p.id === 'p-docs' ? 'working' : 'idle', task: '', turns, input_tokens: input, output_tokens: Math.round(input / 28), updated_at }),
        ...usageOf({ turns, input_tokens: input, updated_at }), agent: 'llm', project_id: p.id, parent_id: '', branch: '', repo_path: '', cwd: `/Users/me/.tingly-project/projects/${p.id}`,
        reported_model: 'claude-sonnet-4-5', session_id: '7d1f3a90-2c4e-4b7a-8e61-0a9b5c2d4f13',
        context_tokens: p.id === 'p-latency' ? 112_000 : 41_000,
    }
}

const stoppedTasks = new Set<string>()

const sessionEvents = (s: (typeof sessions)[number]) => {
    let n = 0
    const ev = (kind: string, content: string, min: number, extra: Record<string, unknown> = {}) =>
        ({ id: `${s.id}-e${++n}`, thread_id: s.id, kind, content, created_at: ago(min), ...extra })
    const call = (tool: string, input: unknown, out: string, min: number, id: string, extra: Record<string, unknown> = {}) => [
        ev('tool_use', tool, min, { request_id: id, payload: JSON.stringify(input), ...extra }),
        ev('tool_result', out, min, { request_id: id, payload: '{"is_error":false}', ...extra }),
    ]
    const first = ev('user', s.task, 30)
    if (s.id === 's-dark') {
        return [
            first,
            ...call('TodoWrite', { todos: [{ content: 'Review theme preferences', status: 'pending' }] }, 'ok', 15, 'todo-1'),
            ...call('TodoWrite', { todos: [{ content: 'Review theme preferences', status: 'completed' }] }, 'ok', 14, 'todo-2'),
            ...call('TodoWrite', { todos: [{ content: 'Add the theme toggle', status: 'completed' }, { content: 'Verify settings persist', activeForm: 'Verifying settings persist', status: 'in_progress' }, { content: 'Run the test suite', status: 'pending' }] }, 'ok', 13, 'todo-3'),
            ev('assistant', 'I will look at how the settings page stores preferences first.', 12),
            ...call('Read', { file_path: 'src/settings/Settings.tsx' }, 'export function Settings() { ... }', 11, 'c1'),
            ev('tool_use', 'Agent', 10, { request_id: 'agent-1', payload: JSON.stringify({ description: 'Find where the theme is applied', subagent_type: 'Explore' }) }),
            ev('task', '', 10, { request_id: 'agent-1', payload: JSON.stringify({ event: 'task_started', task_type: 'local_agent', description: 'Find where the theme is applied', subagent_type: 'Explore' }) }),
            ...call('Grep', { pattern: 'prefers-color-scheme' }, 'src/theme/index.ts:14', 10, 'c2', { parent: 'agent-1' }),
            ev('tool_result', 'The theme is applied in src/theme/index.ts and read from localStorage.', 9, { request_id: 'agent-1', payload: '{"is_error":false}' }),
            ...call('Edit', { file_path: 'src/settings/Settings.tsx' }, 'Updated', 6, 'c3'),
            ev('usage', '', 6, { payload: JSON.stringify({ model: 'cc', input_tokens: 3200, output_tokens: 1450, cache_read_tokens: 48200, cache_write_tokens: 6100, context_tokens: 57500, context_window: 200000 }) }),
            ev('tool_use', 'Bash', 5, { request_id: 'c-dev', payload: JSON.stringify({ command: 'npm run dev', description: 'Start the dev server', run_in_background: true }) }),
            ev('task', '', 5, { request_id: 'c-dev', payload: JSON.stringify({ event: 'task_started', task_id: 'bash-1', task_type: 'local_bash', background: true, description: 'Start the dev server' }) }),
            ev('task', '', 5, { request_id: 'c-dev', payload: JSON.stringify({ event: 'output_file', task_id: 'bash-1', output_file: '/tmp/bash-1.out' }) }),
            ev('tool_result', 'Command running in background with ID: bash-1', 5, { request_id: 'c-dev', payload: '{"is_error":false}' }),
            ev('assistant', 'Added the toggle and persisted it. Running the tests to check.', 3),
            ev('tool_use', 'Bash', 2, { request_id: 'c4', payload: JSON.stringify({ command: 'npm test' }) }),
            ev('approval', 'Run npm test', 2, { payload: JSON.stringify({ request_id: 'req-test', tool: 'Bash', input: { command: 'npm test' }, state: 'pending' }) }),
        ]
    }
    if (s.id === 's-links') {
        return [first, ev('assistant', 'Reading the checker to see how it parses anchors.', 1), ev('tool_use', 'Read', 1, { request_id: 'c1', payload: JSON.stringify({ file_path: 'scripts/check-links.mjs' }) })]
    }
    return [first, ev('assistant', 'Done. The intro now opens with what the tool does and who it is for, in three short paragraphs.', 49)]
}

// IM chats bound to threads (TEP #2143): a Telegram private chat on the
// latency project's main thread and a Telegram group on Docs'. The chat ids match the Remote
// mocks (handlers.ts: bots/:bot/chats and /groups). Held in memory, so binding
// and unbinding work within a mock session.
type Binding = { id: string; platform: string; chat_id: string; bot_uuid?: string; chat_title: string; project_id: string; thread_id: string; created_at: string; updated_at: string }
const bindings: Binding[] = [
    { id: 'cb-1', platform: 'telegram', chat_id: 'telegram:123456789', chat_title: '123456789', project_id: 'p-latency', thread_id: 'main-p-latency', created_at: ago(60 * 30), updated_at: ago(60 * 30) },
    { id: 'cb-2', platform: 'telegram', chat_id: 'telegram:-10024681012', chat_title: 'Release Crew', project_id: 'p-docs', thread_id: 'main-p-docs', created_at: ago(60 * 48), updated_at: ago(60 * 48) },
]
// What the Remote mocks call a chat, to title a new binding.
const mockChatTitles: Record<string, string> = {
    'telegram:123456789': '123456789', 'telegram:987654321': '987654321', 'telegram:-10024681012': 'Release Crew',
    'feishu:oc_mock_chat_0001': 'Platform team',
}

const findThread = (id: string) => [...threads, ...sessions].find((x) => x.id === id)
const summary = (p: Proj) => ({ ...p, threads: counts(p.id) })
const find = (id: string) => projects.find((p) => p.id === id)
const notFound = () => HttpResponse.json({ error: 'project not found' }, { status: 404 })

export const projectsHandlers = [
    // IM channel bindings. Before '/api/v1/projects/:id' so "channel-bindings" is not read as an id.
    http.get('/api/v1/projects/channel-bindings', () => HttpResponse.json({ bindings })),
    http.put('/api/v1/projects/channel-bindings', async ({ request }) => {
        const body = (await request.json()) as { platform: string; chat_id: string; bot_uuid?: string; project_id?: string; thread_id?: string }
        // A project resolves to its main thread; a folder has none, so it is bound by one of its threads.
        let thread_id = body.thread_id ?? ''
        let project_id = ''
        if (body.project_id) {
            const p = find(body.project_id)
            if (!p) return notFound()
            if (!p.main_thread_id) return HttpResponse.json({ error: 'a folder has no main thread: bind one of its threads' }, { status: 400 })
            thread_id = p.main_thread_id
            project_id = p.id
        } else {
            const th = findThread(thread_id)
            const main = projects.find((p) => p.main_thread_id === thread_id)
            if (!th && !main) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
            project_id = th ? th.project_id : main!.id
        }
        // One chat, one place: binding again moves it.
        const now = ago(0)
        const old = bindings.find((b) => b.platform === body.platform && b.chat_id === body.chat_id)
        if (old) {
            old.project_id = project_id
            old.thread_id = thread_id
            old.bot_uuid = body.bot_uuid || old.bot_uuid
            old.updated_at = now
            return HttpResponse.json(old)
        }
        const b: Binding = { id: `cb-${Date.now()}`, platform: body.platform, chat_id: body.chat_id, bot_uuid: body.bot_uuid, chat_title: mockChatTitles[body.chat_id] ?? body.chat_id, project_id, thread_id, created_at: now, updated_at: now }
        bindings.push(b)
        return HttpResponse.json(b)
    }),
    http.delete('/api/v1/projects/channel-bindings/:id', ({ params }) => {
        const i = bindings.findIndex((b) => b.id === params.id)
        if (i < 0) return HttpResponse.json({ error: 'binding not found' }, { status: 404 })
        bindings.splice(i, 1)
        return new HttpResponse(null, { status: 204 })
    }),
    // Every project's events (the sidebar's stream): an empty one that ends at once.
    http.get('/api/v1/projects/events', () => new HttpResponse('', { headers: { 'Content-Type': 'text/event-stream' } })),
    http.get('/api/v1/threads/permission-modes', () => HttpResponse.json({ modes: ['default', 'acceptEdits', 'auto', 'plan', 'dontAsk', 'bypassPermissions'] })),
    http.get('/api/v1/threads/:tid', ({ params }) => {
        const th = findThread(params.tid as string)
        if (!th) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        const background_tasks = th.id === 's-dark' && !stoppedTasks.has('bash-1') ? [{ task_id: 'bash-1', task_type: 'local_bash', description: 'Start the dev server' }] : []
        // A thread waiting on an approval is mid-turn.
        return HttpResponse.json({ thread: th, running: th.status === 'working' || th.status === 'waiting', background_tasks })
    }),
    http.get('/api/v1/threads/:tid/events', ({ params }) => {
        const id = params.tid as string
        const main = projects.find((p) => p.main_thread_id === id)
        if (main) return HttpResponse.json({ events: main.id === 'p-latency' ? messages : [] })
        const th = findThread(id)
        if (!th) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        return HttpResponse.json({ events: th.id.startsWith('t-') ? threadEvents(th as (typeof threads)[number]) : sessionEvents(th as (typeof sessions)[number]) })
    }),
    http.post('/api/v1/threads/:tid/messages', ({ params }) => {
        const th = findThread(params.tid as string)
        if (th) return HttpResponse.json(th)
        const main = projects.find((p) => p.main_thread_id === params.tid)
        return main ? HttpResponse.json({ ...thread({ id: main.main_thread_id!, title: main.name, status: 'working', task: '' }), agent: 'llm' }) : HttpResponse.json({ error: 'thread not found' }, { status: 404 })
    }),
    http.post('/api/v1/threads/:tid/events/:eid/start', () => HttpResponse.json(messages[2])),
    http.put('/api/v1/threads/:tid/profile', async ({ params, request }) => {
        const th = findThread(params.tid as string) as Record<string, unknown> | undefined
        if (!th) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        const body = (await request.json()) as { profile: string; inherit?: boolean }
        if (th.parent_id) { th.profile_override = body.inherit ? null : body.profile; th.model_override = '' }
        else th.profile = body.profile
        return HttpResponse.json(th)
    }),
    http.put('/api/v1/threads/:tid/model', async ({ params, request }) => {
        const th = findThread(params.tid as string) as Record<string, unknown> | undefined
        if (!th) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        const model = ((await request.json()) as { model: string }).model
        if (th.parent_id) th.model_override = model
        else th.model = model
        return HttpResponse.json(th)
    }),
    http.put('/api/v1/threads/:tid/effort', async ({ params, request }) => {
        const th = findThread(params.tid as string) as Record<string, unknown> | undefined
        if (!th) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        const effort = ((await request.json()) as { effort: string }).effort
        if (!['', 'low', 'medium', 'high', 'max'].includes(effort)) return HttpResponse.json({ error: 'invalid effort level' }, { status: 400 })
        th.effort_override = effort
        return HttpResponse.json(th)
    }),
    http.put('/api/v1/threads/:tid/title', async ({ params, request }) => {
        const th = findThread(params.tid as string) as Record<string, unknown> | undefined
        if (!th) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        th.title = ((await request.json()) as { title: string }).title
        return HttpResponse.json(th)
    }),
    http.post('/api/v1/threads/:tid/handoff', ({ params }) => {
        const s = sessions.find((x) => x.id === params.tid)
        if (!s) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        return HttpResponse.json({ command: `cd '${s.cwd}' && '/usr/local/bin/tingly-box' cc --resume '${s.session_id}'` })
    }),
    http.get('/api/v1/threads/:tid/tasks/:task_id/output', () =>
        HttpResponse.json({ content: '> notes-app@1.0.0 dev\n> vite\n\n  VITE v5.4.0  ready in 312 ms\n\n  ➜  Local:   http://localhost:5173/\n', truncated: false, size: 96 })),
    http.post('/api/v1/threads/:tid/tasks/:task_id/stop', ({ params }) => {
        stoppedTasks.add(params.task_id as string)
        return HttpResponse.json({ ok: true })
    }),
    http.put('/api/v1/threads/:tid/auto-answer', async ({ params, request }) => {
        const th = findThread(params.tid as string) as Record<string, unknown> | undefined
        if (!th) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        th.auto_answer = ((await request.json()) as { auto_answer: string }).auto_answer
        return HttpResponse.json(th)
    }),
    http.put('/api/v1/threads/:tid/permission-mode', async ({ params, request }) => {
        const th = findThread(params.tid as string) as Record<string, unknown> | undefined
        if (!th) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        th.permission_mode = ((await request.json()) as { permission_mode: string }).permission_mode
        return HttpResponse.json(th)
    }),
    http.get('/api/v1/projects', ({ request }) => {
        const url = new URL(request.url)
        const q = (url.searchParams.get('q') ?? '').toLowerCase()
        const archived = url.searchParams.get('archived') === 'true'
        // A newcomer (?mockData=newcomer) has no projects or folders yet.
        if (resolveMockDataProfile() === 'newcomer') return HttpResponse.json({ projects: [] })
        // Projects and folders in one list, each with its type.
        const list = projects
            .filter((p) => !!p.archived_at === archived)
            .filter((p) => !q || p.name.toLowerCase().includes(q) || p.goal.toLowerCase().includes(q))
        return HttpResponse.json({ projects: list.map(summary) })
    }),
    http.post('/api/v1/projects', async ({ request }) => {
        const body = (await request.json()) as Partial<Proj> & { path?: string }
        if (body.type === 'folder') {
            // Opening a folder returns its project when there is one.
            if (!body.path?.startsWith('/')) return HttpResponse.json({ error: `the path is not a folder: "${body.path}"` }, { status: 400 })
            const known = projects.find((p) => p.type === 'folder' && p.path === body.path)
            if (known) {
                delete known.archived_at
                return HttpResponse.json(known)
            }
            const f = folderProject(`f-${Date.now()}`, body.path, 0)
            projects.push(f)
            return HttpResponse.json(f, { status: 201 })
        }
        const id = `p-${Date.now()}`
        const p: Proj = { ...projects[1], id, main_thread_id: `main-${id}`, name: body.name ?? 'New project', goal: body.goal ?? '', repos: body.repos ?? [], starred: false, created_at: ago(0), updated_at: ago(0) }
        projects.unshift(p)
        return HttpResponse.json(p, { status: 201 })
    }),
    // An alias tier with its route, like the gateway lists them, so the
    // pickers show what the alias resolves to.
    http.get('/api/v1/projects/models', () => HttpResponse.json({ models: [
        { id: 'sonnet', display_name: 'Claude Sonnet 4.5', provider_model: 'claude-sonnet-4-5', provider_name: 'anthropic-primary' },
        { id: 'claude-haiku-4-5', display_name: 'Claude Haiku 4.5' },
    ] })),
    http.get('/api/v1/projects/runtime', () => HttpResponse.json({ cli_path: '/usr/local/bin/claude', base_url: 'http://localhost:12580/tingly/claude_code', max_running_threads_limit: 100 })),
    http.get('/api/v1/projects/:id', ({ params }) => {
        const p = find(params.id as string)
        if (!p) return notFound()
        return HttpResponse.json({ project: p, threads: p.id === 'p-latency' ? threads : p.type === 'folder' ? sessions.filter((x) => x.project_id === p.id) : [], counts: counts(p.id), coordinator_busy: p.id === 'p-docs', coordinator_queued: p.id === 'p-docs' ? 1 : 0, coordinator: p.type === 'folder' ? undefined : coordinatorOf(p) })
    }),
    http.patch('/api/v1/projects/:id', async ({ params, request }) => {
        const p = find(params.id as string)
        if (!p) return notFound()
        const { archived, ...rest } = (await request.json()) as Partial<Proj> & { archived?: boolean }
        Object.assign(p, rest)
        if (archived !== undefined) {
            if (archived) p.archived_at = ago(0)
            else delete p.archived_at
        }
        return HttpResponse.json(p)
    }),
    // Removing a project or a folder is archiving it: there is no hard delete.
    http.delete('/api/v1/projects/:id', ({ params }) => {
        const p = find(params.id as string)
        if (!p) return notFound()
        p.archived_at = ago(0)
        return HttpResponse.json({ ok: true })
    }),
    // A project's threads (not its main thread); a folder's, plain threads.
    http.get('/api/v1/projects/:id/threads', ({ params }) => {
        const id = params.id as string
        if (!find(id)) return notFound()
        return HttpResponse.json({ threads: id === 'p-latency' ? threads : sessions.filter((x) => x.project_id === id) })
    }),
    http.post('/api/v1/projects/:id/threads', async ({ params, request }) => {
        const f = find(params.id as string)
        if (!f || f.type !== 'folder') return HttpResponse.json({ error: 'only a folder starts threads directly' }, { status: 400 })
        const body = (await request.json()) as { content: string; effort?: string; permission_mode?: string; profile?: string; model?: string }
        const s = session({ id: `s-${Date.now()}`, title: body.content.slice(0, 60), status: 'working', cwd: f.path!, task: body.content, permission_mode: body.permission_mode, effort_override: body.effort, profile: body.profile, model: body.model, turns: 0, updated_at: ago(0) })
        sessions.unshift({ ...s, ...usageOf(s), last_turn_at: undefined } as (typeof sessions)[number])
        return HttpResponse.json(s, { status: 201 })
    }),
    // The live event stream: an empty stream that ends at once (the page
    // reconnects later); mock data does not change on its own.
    http.get('/api/v1/projects/:id/events', () =>
        new HttpResponse('', { headers: { 'Content-Type': 'text/event-stream' } })),
    http.get('/api/v1/projects/:id/library', ({ params }) =>
        HttpResponse.json({ dir: `/Users/me/.tingly-box/projects/${params.id}/library`, files: params.id === 'p-latency' ? library : [] })),
    http.get('/api/v1/projects/:id/library/tree', ({ params, request }) => {
        const path = new URL(request.url).searchParams.get('path') ?? ''
        const entries = params.id === 'p-latency' ? libraryTree[path] : path === '' ? [] : undefined
        return entries
            ? HttpResponse.json({ dir: `/Users/me/.tingly-box/projects/${params.id}/library`, path, entries })
            : HttpResponse.json({ error: 'folder not found' }, { status: 404 })
    }),
    http.get('/api/v1/projects/:id/library/file', ({ request }) => {
        const path = new URL(request.url).searchParams.get('path') ?? ''
        if (path.endsWith('.png')) return new HttpResponse(Uint8Array.from(atob(chartPng), (c) => c.charCodeAt(0)), { headers: { 'Content-Type': 'image/png' } })
        return new HttpResponse(libraryText(path), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } })
    }),
    http.get('/api/v1/projects/:id/memory', ({ params }) => HttpResponse.json({ dir: `/Users/me/.tingly-box/projects/${params.id}/memory`, files: [] })),
    http.get('/api/v1/projects/:id/usage', () => HttpResponse.json({ by_model: [], by_thread: [] })),
]
