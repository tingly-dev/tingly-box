import { http, HttpResponse } from 'msw'

// Mock-mode data for Projects (/api/v1/projects, @tingly/projects): a few
// projects in different states, one of them with a conversation, threads in
// every status (one waiting on the user, so the rail shows its dot) and a
// small Library. Mutations answer with plausible objects; nothing persists.

const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString()

const repo = (path: string) => ({ path, base_branch: 'main' })

type Proj = {
    id: string; name: string; goal: string; instructions: string; paused: boolean; starred: boolean
    repos: { path: string; base_branch: string }[]; coordinator_model: string; thread_model: string
    thread_permission_mode: string; created_at: string; updated_at: string; archived_at?: string
}

const projects: Proj[] = [
    {
        id: 'p-latency', name: 'Payments API latency', goal: 'Get p99 of /charge under 300 ms without dropping retries.',
        instructions: 'Keep changes behind the existing feature flags.', paused: false, starred: true,
        repos: [repo('/Users/me/code/payments-api'), repo('/Users/me/code/payments-sdk')],
        coordinator_model: 'claude-sonnet-4-5', thread_model: 'claude-sonnet-4-5', thread_permission_mode: 'acceptEdits',
        created_at: ago(60 * 26), updated_at: ago(4),
    },
    {
        id: 'p-docs', name: 'Docs refresh', goal: 'Every public endpoint documented with a runnable example.',
        instructions: '', paused: false, starred: false, repos: [repo('/Users/me/code/website')],
        coordinator_model: 'claude-sonnet-4-5', thread_model: 'claude-haiku-4-5', thread_permission_mode: 'default',
        created_at: ago(60 * 24 * 5), updated_at: ago(60 * 3),
    },
    {
        id: 'p-migrate', name: 'Postgres 17 migration', goal: 'Move every service off Postgres 13 with zero downtime.',
        instructions: '', paused: true, starred: false, repos: [repo('/Users/me/code/infra')],
        coordinator_model: 'claude-sonnet-4-5', thread_model: 'claude-sonnet-4-5', thread_permission_mode: 'default',
        created_at: ago(60 * 24 * 12), updated_at: ago(60 * 24 * 2),
    },
]

const thread = (o: Partial<Record<string, unknown>> & { id: string; title: string; status: string; task: string }) => ({
    project_id: 'p-latency', origin: 'coordinator', commits_ahead: 0, files_changed: 0, input_tokens: 0, output_tokens: 0,
    turns: 0, uncommitted: false, worktree: `/Users/me/.tingly-box/projects/worktrees/${o.id}`,
    repo_path: '/Users/me/code/payments-api', base_branch: 'main', branch: `projects/${o.id}`,
    model: 'claude-sonnet-4-5', created_at: ago(90), updated_at: ago(5), ...o,
})

const threads = [
    thread({ id: 't-pool', title: 'Tune the DB connection pool', status: 'waiting', task: 'Profile /charge and size the connection pool for peak load.', wait_reason: 'Allow `go test ./... -run Bench` to run against staging?', turns: 9, input_tokens: 182_000, output_tokens: 9_400, commits_ahead: 1, files_changed: 2, updated_at: ago(4) }),
    thread({ id: 't-cache', title: 'Cache idempotency keys in Redis', status: 'ready', task: 'Move idempotency-key lookups from Postgres to Redis with a 24h TTL.', last_report: 'Lookups moved to Redis behind `IDEMPOTENCY_REDIS`. p99 for /charge drops from 410 ms to 260 ms in the load test. Tests pass.', turns: 14, input_tokens: 240_000, output_tokens: 15_100, commits_ahead: 3, files_changed: 6, updated_at: ago(18) }),
    thread({ id: 't-sdk', title: 'Retry budget in the SDK', status: 'working', task: 'Cap client retries with a token bucket so retries cannot amplify an outage.', repo_path: '/Users/me/code/payments-sdk', turns: 5, input_tokens: 64_000, output_tokens: 3_100, files_changed: 1, uncommitted: true, updated_at: ago(1) }),
    thread({ id: 't-trace', title: 'Add tracing spans to /charge', status: 'resolved', task: 'Add OpenTelemetry spans around each downstream call in /charge.', turns: 7, input_tokens: 90_000, output_tokens: 6_200, commits_ahead: 2, files_changed: 4, merged_at: ago(60 * 20), resolved_at: ago(60 * 20), updated_at: ago(60 * 20) }),
]

const counts = (pid: string) => {
    const list = pid === 'p-latency' ? threads : []
    const n = (s: string) => list.filter((t) => t.status === s).length
    if (pid === 'p-docs') return { waiting: 0, ready: 1, working: 1, idle: 0, resolved: 4 }
    if (pid === 'p-migrate') return { waiting: 0, ready: 0, working: 0, idle: 2, resolved: 1 }
    return { waiting: n('waiting'), ready: n('ready'), working: n('working'), idle: n('idle'), resolved: n('resolved') }
}

const msg = (id: string, role: string, kind: string, content: string, min: number, payload?: unknown) => ({
    id, project_id: 'p-latency', role, kind, content, created_at: ago(min),
    ...(payload ? { payload: JSON.stringify(payload) } : {}),
})

const messages = [
    msg('m1', 'user', 'text', '/charge p99 is at 410 ms in prod. Find out where the time goes and fix what you can — the pool, the idempotency lookup, anything obvious.', 95),
    msg('m2', 'assistant', 'text', 'Traces point at two hot spots: idempotency-key lookups hit Postgres on every request, and the connection pool saturates at peak. I started a thread for each, plus one to add the tracing spans we need to confirm the fix.', 94),
    msg('m3', 'assistant', 'thread_started', '', 94, { thread_ids: ['t-cache', 't-pool', 't-trace'] }),
    msg('m4', 'assistant', 'thread_update', '', 60 * 20, { thread_id: 't-trace', title: 'Add tracing spans to /charge', kind: 'finished' }),
    msg('m5', 'user', 'text', 'Also make sure SDK retries cannot pile onto an outage.', 30),
    msg('m6', 'assistant', 'thread_started', '', 29, { thread_ids: ['t-sdk'] }),
    msg('m7', 'assistant', 'thread_update', '', 18, { thread_id: 't-cache', title: 'Cache idempotency keys in Redis', kind: 'finished' }),
    msg('m8', 'assistant', 'thread_update', '', 4, { thread_id: 't-pool', title: 'Tune the DB connection pool', kind: 'needs_input' }),
]

const library = [
    { path: 'load-test-2026-10-14.md', origin: 'thread', size: 4_812, updated_at: ago(18) },
    { path: 'charge-flamegraph.svg', origin: 'user', size: 182_330, updated_at: ago(60 * 22) },
]

const summary = (p: Proj) => ({ ...p, threads: counts(p.id) })
const find = (id: string) => projects.find((p) => p.id === id)
const notFound = () => HttpResponse.json({ error: 'project not found' }, { status: 404 })

export const projectsHandlers = [
    http.get('/api/v1/projects', ({ request }) => {
        const url = new URL(request.url)
        const q = (url.searchParams.get('q') ?? '').toLowerCase()
        const archived = url.searchParams.get('archived') === 'true'
        const list = projects
            .filter((p) => !!p.archived_at === archived)
            .filter((p) => !q || p.name.toLowerCase().includes(q) || p.goal.toLowerCase().includes(q))
        return HttpResponse.json({ projects: list.map(summary) })
    }),
    http.post('/api/v1/projects', async ({ request }) => {
        const body = (await request.json()) as Partial<Proj>
        const p: Proj = { ...projects[1], id: `p-${Date.now()}`, name: body.name ?? 'New project', goal: body.goal ?? '', repos: body.repos ?? [], starred: false, created_at: ago(0), updated_at: ago(0) }
        projects.unshift(p)
        return HttpResponse.json(p)
    }),
    http.get('/api/v1/projects/models', () => HttpResponse.json({ models: [{ id: 'claude-sonnet-4-5', display_name: 'Claude Sonnet 4.5' }, { id: 'claude-haiku-4-5', display_name: 'Claude Haiku 4.5' }] })),
    http.get('/api/v1/projects/runtime', () => HttpResponse.json({ cli_path: '/usr/local/bin/claude', base_url: 'http://localhost:12580/tingly/claude_code' })),
    http.get('/api/v1/projects/:id', ({ params }) => {
        const p = find(params.id as string)
        if (!p) return notFound()
        return HttpResponse.json({ project: p, threads: p.id === 'p-latency' ? threads : [], counts: counts(p.id), coordinator_busy: false })
    }),
    http.patch('/api/v1/projects/:id', async ({ params, request }) => {
        const p = find(params.id as string)
        if (!p) return notFound()
        Object.assign(p, (await request.json()) as Partial<Proj>)
        return HttpResponse.json(p)
    }),
    http.get('/api/v1/projects/:id/messages', ({ params }) =>
        HttpResponse.json({ messages: params.id === 'p-latency' ? messages : [] })),
    // The live event stream: an empty stream that ends at once (the page
    // reconnects later); mock data does not change on its own.
    http.get('/api/v1/projects/:id/events', () =>
        new HttpResponse('', { headers: { 'Content-Type': 'text/event-stream' } })),
    http.get('/api/v1/projects/:id/library', ({ params }) =>
        HttpResponse.json({ dir: `/Users/me/.tingly-box/projects/${params.id}/library`, files: params.id === 'p-latency' ? library : [] })),
    http.get('/api/v1/projects/:id/memory', ({ params }) => HttpResponse.json({ dir: `/Users/me/.tingly-box/projects/${params.id}/memory`, files: [] })),
    http.get('/api/v1/projects/:id/usage', () => HttpResponse.json({ by_model: [], by_thread: [] })),
    http.get('/api/v1/projects/:id/threads/:tid', ({ params }) => {
        const th = threads.find((t) => t.id === params.tid)
        if (!th) return HttpResponse.json({ error: 'thread not found' }, { status: 404 })
        return HttpResponse.json({
            thread: th,
            running: th.status === 'working',
            events: [{ id: 'e1', thread_id: th.id, kind: 'task', from: 'coordinator', content: th.task, created_at: th.created_at }],
        })
    }),
]
