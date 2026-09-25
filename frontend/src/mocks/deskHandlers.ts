import { http, HttpResponse } from 'msw'
import { mockClaudeCodeModels } from './claudeCodeModels'

// Mock-mode data for the Desk page (/api/v1/desk). Covers every state the
// page renders: a turn waiting on an approval, a finished multi-turn session,
// a failed launch, and an archived session. Mutations simulate the turn
// finishing a moment later.

type Msg = {
    role?: string
    content: string
    kind?: string
    request_id?: string
    payload?: unknown
    parent?: string
    timestamp: string
}
type Sess = {
    id: string
    project: string
    status: string
    request: string
    response: string
    error?: string
    permission_mode: string
    profile: string
    model: string
    awaiting_input: boolean
    background_tasks: { task_id: string; task_type: string; description: string }[]
    created_at: string
    last_activity: string
}

const TB = '/Users/me/code/tingly-box'
const SITE = '/Users/me/code/website'
const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString()

const sessions: Sess[] = [
    { id: 'desk-1', project: TB, status: 'running', request: 'Fix the flaky persistent session test in internal/desk', response: '', permission_mode: '', profile: '', model: '', awaiting_input: true, background_tasks: [], created_at: ago(12), last_activity: ago(1) },
    { id: 'desk-2', project: TB, status: 'completed', request: 'Add a dark mode toggle to the settings page', response: '', permission_mode: 'acceptEdits', profile: 'p1', model: 'opus', awaiting_input: false, background_tasks: [], created_at: ago(90), last_activity: ago(40) },
    { id: 'desk-3', project: SITE, status: 'failed', request: 'Update the pricing page copy', response: '', error: 'agent CLI not available', permission_mode: '', profile: '', model: '', awaiting_input: false, background_tasks: [], created_at: ago(200), last_activity: ago(199) },
    { id: 'desk-5', project: TB, status: 'completed', request: 'Audit how the auth middleware handles expired tokens', response: '', permission_mode: 'acceptEdits', profile: '', model: '', awaiting_input: false, background_tasks: [{ task_id: 'ar1', task_type: 'local_agent', description: 'Review refresh path' }, { task_id: 'bd1', task_type: 'local_bash', description: 'Start the dev server' }], created_at: ago(30), last_activity: ago(2) },
    { id: 'desk-4', project: SITE, status: 'closed', request: 'Draft release notes for v1.2', response: '', permission_mode: '', profile: '', model: '', awaiting_input: false, background_tasks: [], created_at: ago(3000), last_activity: ago(2900) },
]

const messages: Record<string, Msg[]> = {
    'desk-1': [
        { role: 'user', content: 'Fix the flaky persistent session test in internal/desk', timestamp: ago(12) },
        { kind: 'thinking', content: 'The test waits on StatusFailed right after SendMessage, but the status is already Failed from turn 1.', timestamp: ago(11) },
        { kind: 'tool_use', content: 'Grep', request_id: 't1', payload: { pattern: 'waitStatus', path: 'internal/desk' }, timestamp: ago(11) },
        { kind: 'tool_result', content: 'internal/desk/service_test.go:365:func waitStatus(...)\ninternal/desk/service_test.go:718:\twaitStatus(t, svc, sess.ID, session.StatusFailed, time.Second)', request_id: 't1', payload: { is_error: false }, timestamp: ago(11) },
        { kind: 'tool_use', content: 'Read', request_id: 't2', payload: { file_path: `${TB}/internal/desk/service_test.go` }, timestamp: ago(10) },
        { kind: 'tool_result', content: 'func TestPersistentTurn_CrashMidTurnDropsFromPoolAndFails(t *testing.T) {\n\t…', request_id: 't2', payload: { is_error: false }, timestamp: ago(10) },
        { kind: 'tool_use', content: 'Bash', request_id: 't3', payload: { command: 'go test ./internal/desk/ -run Crash -count=20' }, timestamp: ago(9) },
        { kind: 'tool_result', content: '--- FAIL: TestPersistentTurn_CrashMidTurnDropsFromPoolAndFails (0.01s)\n    service_test.go:727: Open called 1 times, want 2', request_id: 't3', payload: { is_error: true }, timestamp: ago(9) },
        { kind: 'usage', content: '', payload: { model: 'tingly/cc', input_tokens: 3200, output_tokens: 1450, cache_read_tokens: 48200, cache_write_tokens: 6100, context_tokens: 57500, context_window: 200000 }, timestamp: ago(8) },
        { role: 'assistant', content: 'Reproduced it: `waitStatus` returns immediately because the session is **already Failed** from the first turn, so the assertion runs before the second turn even starts.\n\nI\'ll wait on the second `Open` call instead of the status:\n\n```go\nfor fa.openCalls.Load() < 2 {\n\tif time.Now().After(deadline) {\n\t\tt.Fatalf("Open called %d times, want 2", fa.openCalls.Load())\n\t}\n\ttime.Sleep(5 * time.Millisecond)\n}\n```', timestamp: ago(8) },
        { kind: 'tool_use', content: 'Edit', request_id: 't4', payload: { file_path: `${TB}/internal/desk/service_test.go` }, timestamp: ago(7) },
        { kind: 'tool_result', content: 'Applied 1 edit.', request_id: 't4', payload: { is_error: false }, timestamp: ago(7) },
        { kind: 'approval_request', content: 'Bash', request_id: 'r1', payload: { command: 'go test ./internal/desk/... -race -count=5' }, timestamp: ago(1) },
    ],
    'desk-2': [
        { role: 'user', content: 'Add a dark mode toggle to the settings page', timestamp: ago(90) },
        { kind: 'tool_use', content: 'Glob', request_id: 'a1', payload: { pattern: 'frontend/src/pages/settings/**' }, timestamp: ago(89) },
        { kind: 'tool_result', content: 'frontend/src/pages/settings/SettingsPage.tsx', request_id: 'a1', payload: { is_error: false }, timestamp: ago(89) },
        { kind: 'tool_use', content: 'Edit', request_id: 'a2', payload: { file_path: `${TB}/frontend/src/pages/settings/SettingsPage.tsx` }, timestamp: ago(88) },
        { kind: 'tool_result', content: 'Applied 2 edits.', request_id: 'a2', payload: { is_error: false }, timestamp: ago(88) },
        { kind: 'usage', content: '', payload: { model: 'tingly/cc', input_tokens: 2100, output_tokens: 900, cache_read_tokens: 20500, cache_write_tokens: 4000, context_tokens: 26600, context_window: 128000 }, timestamp: ago(87) },
        { role: 'assistant', content: 'Added a "Theme" row with Light / Dark / System to the settings page, backed by the existing ThemeContext.', timestamp: ago(87) },
        { role: 'user', content: 'Default it to System please', timestamp: ago(45) },
        { kind: 'tool_use', content: 'Edit', request_id: 'a3', payload: { file_path: `${TB}/frontend/src/contexts/ThemeContext.tsx` }, timestamp: ago(44) },
        { kind: 'tool_result', content: 'Applied 1 edit.', request_id: 'a3', payload: { is_error: false }, timestamp: ago(44) },
        { role: 'assistant', content: [
            'Done — new installs now follow the **OS theme** until a choice is made.',
            '',
            'What changed:',
            '',
            '1. `ThemeContext` reads `prefers-color-scheme` when nothing is stored',
            '2. The settings row gains a **System** option',
            '3. The choice persists under `tingly-theme-mode`',
            '',
            '```tsx',
            "const initial = stored ?? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');",
            '```',
            '',
            '```bash',
            'pnpm test -- ThemeContext && git diff --stat',
            '```',
            '',
            '```diff',
            "- const initial = stored ?? 'light';",
            "+ const initial = stored ?? systemPreference();",
            '```',
            '',
            '| Stored value | Result |',
            '| --- | --- |',
            '| none | follows the OS |',
            '| `dark` / `light` | fixed |',
            '',
            '> Existing users keep their saved choice.',
            '',
            'Raw HTML in a reply is shown, not rendered: <b>not bold</b> <script>alert(1)</script>',
            '',
            'See [the MUI docs](https://mui.com/material-ui/customization/dark-mode/) for the approach.',
        ].join('\n'), timestamp: ago(40) },
        { kind: 'usage', content: '', payload: { model: 'tingly/cc', input_tokens: 800, output_tokens: 1200, cache_read_tokens: 30100, cache_write_tokens: 1500, context_tokens: 32400, context_window: 128000 }, timestamp: ago(40) },
    ],
    'desk-3': [
        { role: 'user', content: 'Update the pricing page copy', timestamp: ago(200) },
        { kind: 'error', content: 'agent CLI not available', timestamp: ago(199) },
    ],
    // Subagents: a foreground Explore run that finished, and a background
    // review still running after its turn ended, next to a background test run.
    'desk-5': [
        { role: 'user', content: 'Audit how the auth middleware handles expired tokens', timestamp: ago(30) },
        { role: 'assistant', content: 'I\'ll map the middleware first, then have a reviewer check the refresh path while the tests run.', timestamp: ago(29) },
        { kind: 'tool_use', content: 'Agent', request_id: 'a-explore', payload: { description: 'Map auth middleware', subagent_type: 'Explore', prompt: 'Find where HTTP requests are authenticated and how expired tokens are detected. Report file paths and the decision points.', run_in_background: false }, timestamp: ago(29) },
        { kind: 'task', content: '', request_id: 'a-explore', payload: { event: 'task_started', task_id: 'ae1', task_type: 'local_agent', subagent_type: 'Explore', background: false, description: 'Map auth middleware' }, timestamp: ago(29) },
        { kind: 'tool_use', content: 'Grep', request_id: 'x1', parent: 'a-explore', payload: { pattern: 'ExpiresAt|isExpired', path: 'internal/server' }, timestamp: ago(28) },
        { kind: 'tool_result', content: 'internal/server/middleware/auth.go:88\ninternal/server/middleware/auth.go:131', request_id: 'x1', parent: 'a-explore', payload: { is_error: false }, timestamp: ago(28) },
        { kind: 'tool_use', content: 'Read', request_id: 'x2', parent: 'a-explore', payload: { file_path: 'internal/server/middleware/auth.go' }, timestamp: ago(28) },
        { kind: 'tool_result', content: 'func (m *Auth) Check(c *gin.Context) {…', request_id: 'x2', parent: 'a-explore', payload: { is_error: false }, timestamp: ago(28) },
        { kind: 'task', content: '', request_id: 'a-explore', payload: { event: 'task_progress', description: 'Reading auth.go', last_tool: 'Read', usage: { total_tokens: 18200, tool_uses: 2, duration_ms: 21000 } }, timestamp: ago(28) },
        { role: 'assistant', parent: 'a-explore', content: 'Tokens are checked in `internal/server/middleware/auth.go`:\n\n- `Check` (line 88) rejects a token whose `ExpiresAt` is past, with **401**.\n- `refresh` (line 131) retries once with the refresh token, but does not clear the cached session on failure.', timestamp: ago(27) },
        { kind: 'task', content: '', request_id: 'a-explore', payload: { event: 'task_notification', status: 'completed', summary: 'Mapped the auth middleware', usage: { total_tokens: 24100, tool_uses: 2, duration_ms: 38000 } }, timestamp: ago(27) },
        { kind: 'tool_result', content: '[Subagent hand-back] Tokens are checked in internal/server/middleware/auth.go…', request_id: 'a-explore', payload: { is_error: false }, timestamp: ago(27) },
        { kind: 'tool_use', content: 'Agent', request_id: 'a-review', payload: { description: 'Review refresh path', subagent_type: 'general-purpose', prompt: 'Review refresh() in internal/server/middleware/auth.go for races when two requests refresh the same expired token.' }, timestamp: ago(26) },
        { kind: 'task', content: '', request_id: 'a-review', payload: { event: 'task_started', task_id: 'ar1', task_type: 'local_agent', subagent_type: 'general-purpose', background: true, description: 'Review refresh path' }, timestamp: ago(26) },
        { kind: 'tool_result', content: 'Async agent launched successfully.', request_id: 'a-review', payload: { is_error: false }, timestamp: ago(26) },
        { kind: 'tool_use', content: 'Bash', request_id: 'b-test', payload: { command: 'go test ./internal/server/middleware/... -race', description: 'Run middleware tests', run_in_background: true }, timestamp: ago(26) },
        { kind: 'task', content: '', request_id: 'b-test', payload: { event: 'task_started', task_id: 'bt1', task_type: 'local_bash', background: true, description: 'Run middleware tests' }, timestamp: ago(26) },
        { kind: 'tool_result', content: 'Command running in background with ID: bt1.', request_id: 'b-test', payload: { is_error: false }, timestamp: ago(26) },
        { kind: 'task', content: '', request_id: 'b-test', payload: { event: 'output_file', task_id: 'bt1', output_file: '/tmp/claude-501/-Users-me-code-tingly-box/desk-5/tasks/bt1.output' }, timestamp: ago(26) },
        { kind: 'tool_use', content: 'Bash', request_id: 'b-dev', payload: { command: 'pnpm --dir frontend dev --port 5173', description: 'Start the dev server', run_in_background: true }, timestamp: ago(25) },
        { kind: 'task', content: '', request_id: 'b-dev', payload: { event: 'task_started', task_id: 'bd1', task_type: 'local_bash', background: true, description: 'Start the dev server' }, timestamp: ago(25) },
        { kind: 'tool_result', content: 'Command running in background with ID: bd1.', request_id: 'b-dev', payload: { is_error: false }, timestamp: ago(25) },
        { kind: 'task', content: '', request_id: 'b-dev', payload: { event: 'output_file', task_id: 'bd1', output_file: '/tmp/claude-501/-Users-me-code-tingly-box/desk-5/tasks/bd1.output' }, timestamp: ago(25) },
        { kind: 'tool_use', content: 'Read', request_id: 'r1', parent: 'a-review', payload: { file_path: 'internal/server/middleware/auth.go' }, timestamp: ago(20) },
        { kind: 'tool_result', content: 'func (m *Auth) refresh(…', request_id: 'r1', parent: 'a-review', payload: { is_error: false }, timestamp: ago(20) },
        { kind: 'task', content: '', request_id: 'a-review', payload: { event: 'task_progress', description: 'Tracing concurrent refresh calls', last_tool: 'Grep', usage: { total_tokens: 41300, tool_uses: 6, duration_ms: 312000 } }, timestamp: ago(3) },
        { role: 'assistant', content: 'The middleware map is above. A reviewer is checking the refresh path for races and the middleware tests are running; I\'ll pick up both when they report back.', timestamp: ago(25) },
        { kind: 'task', content: '', request_id: 'b-test', payload: { event: 'task_notification', status: 'completed', summary: 'Background command "Run middleware tests" completed (exit code 0)' }, timestamp: ago(2) },
    ],
    'desk-4': [
        { role: 'user', content: 'Draft release notes for v1.2', timestamp: ago(3000) },
        { role: 'assistant', content: 'Drafted RELEASE_NOTES.md with the highlights from the last 40 commits.', timestamp: ago(2950) },
    ],
}

let devReads = 0
const find = (id: string) => sessions.find((s) => s.id === id)
const touch = (s: Sess, status?: string) => {
    if (status) s.status = status
    s.last_activity = new Date().toISOString()
}
const push = (id: string, m: Omit<Msg, 'timestamp'>) => {
    (messages[id] ??= []).push({ ...m, timestamp: new Date().toISOString() })
}
// Simulates a turn: a tool call now, the reply and completion a moment later.
const runTurn = (s: Sess, reply: string) => {
    touch(s, 'running')
    const tid = `t-${Date.now()}`
    push(s.id, { kind: 'tool_use', content: 'Read', request_id: tid, payload: { file_path: `${s.project}/README.md` } })
    setTimeout(() => {
        push(s.id, { kind: 'tool_result', content: '# README\n…', request_id: tid, payload: { is_error: false } })
        push(s.id, { kind: 'usage', content: '', payload: { model: 'tingly/cc', input_tokens: 600, output_tokens: 240, cache_read_tokens: 12000, cache_write_tokens: 900, context_tokens: 13500, context_window: 200000 } })
        push(s.id, { role: 'assistant', content: reply })
        touch(s, 'completed')
    }, 2500)
}
// Where each profile routes, with the provider's quota: the default routing
// is close to its 5-hour window to show the warning state.
const inHours = (h: number) => new Date(Date.now() + h * 3_600_000).toISOString()
const routes: Record<string, object> = {
    '': {
        provider_name: 'Anthropic (team)', provider_model: 'claude-sonnet-4-5',
        quota: [
            { type: 'session', balance: false, text: '14% left', used_percent: 86, resets_at: inHours(2.2), limit_reached: false },
            { type: 'weekly', balance: false, text: '81% left', used_percent: 19, resets_at: inHours(80), limit_reached: false },
        ],
    },
    p1: {
        provider_name: 'DeepSeek', provider_model: 'deepseek-chat',
        quota: [{ type: 'balance', balance: true, text: '$12.40', used_percent: 0, limit_reached: false }],
    },
}
const tiers = mockClaudeCodeModels
const tierOf = (s: Sess) => {
    const t = tiers[s.profile] ?? tiers['']
    return t.tiers.find((x) => x.alias === s.model) ?? t.tiers[0]
}
const sorted = () => [...sessions].sort((a, b) => b.last_activity.localeCompare(a.last_activity))

export const deskHandlers = [
    http.get('/api/v1/desk/folders/recent', () => {
        const seen = new Map<string, { path: string; name: string; last_used_at: string }>()
        for (const s of sorted()) {
            if (!seen.has(s.project)) seen.set(s.project, { path: s.project, name: s.project.split('/').pop() ?? s.project, last_used_at: s.last_activity })
        }
        return HttpResponse.json({ folders: [...seen.values()] })
    }),
    http.get('/api/v1/desk/permission-modes', () =>
        HttpResponse.json({ modes: ['default', 'acceptEdits', 'auto', 'plan', 'dontAsk', 'bypassPermissions'] })),
    http.get('/api/v1/desk/sessions', () => HttpResponse.json({ sessions: sorted() })),
    http.post('/api/v1/desk/sessions', async ({ request }) => {
        const body = (await request.json()) as { path: string; prompt: string; permission_mode?: string; profile?: string; model?: string }
        const now = new Date().toISOString()
        const s: Sess = { id: `desk-${Date.now()}`, project: body.path, status: 'pending', request: body.prompt, response: '', permission_mode: body.permission_mode ?? '', profile: body.profile ?? '', model: body.model ?? '', awaiting_input: false, background_tasks: [], created_at: now, last_activity: now }
        sessions.push(s)
        push(s.id, { role: 'user', content: body.prompt })
        runTurn(s, 'Looked around the project — here is what I found and what I would change next.')
        return HttpResponse.json(s, { status: 201 })
    }),
    http.get('/api/v1/desk/sessions/:id', ({ params }) => {
        const s = find(params.id as string)
        return s ? HttpResponse.json(s) : HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
    }),
    http.get('/api/v1/desk/sessions/:id/messages', ({ params }) =>
        HttpResponse.json({ messages: messages[params.id as string] ?? [] })),
    http.post('/api/v1/desk/sessions/:id/messages', async ({ params, request }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        if (s.status === 'running' || s.status === 'pending') return HttpResponse.json({ error: { message: 'a turn is already in progress' } }, { status: 409 })
        const { text } = (await request.json()) as { text: string }
        push(s.id, { role: 'user', content: text })
        runTurn(s, `On it — "${text}" is done.`)
        return new HttpResponse(null, { status: 202 })
    }),
    http.post('/api/v1/desk/sessions/:id/respond', async ({ params, request }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        const { request_id, approved } = (await request.json()) as { request_id: string; approved: boolean }
        push(s.id, { kind: 'approval_response', content: approved ? 'approved' : 'denied', request_id })
        s.awaiting_input = false
        setTimeout(() => {
            push(s.id, { role: 'assistant', content: approved ? 'All green across 5 race-checked runs.' : 'Skipped running the tests.' })
            touch(s, 'completed')
        }, 1500)
        return new HttpResponse(null, { status: 204 })
    }),
    http.get('/api/v1/desk/sessions/:id/status', ({ params }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        const scenario = s.profile ? `claude_code:${s.profile}` : 'claude_code'
        const tier = tierOf(s)
        const quota = (routes[s.profile] ?? routes['']) as { quota: unknown[] }
        return HttpResponse.json({ scenario, requested_model: tier.model, provider_name: tier.provider_name, provider_model: tier.provider_model, quota: quota.quota })
    }),
    http.put('/api/v1/desk/sessions/:id/model', async ({ params, request }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        const model = ((await request.json()) as { model: string }).model
        if (model && (tiers[s.profile] ?? tiers['']).unified) {
            return HttpResponse.json({ error: { message: 'this profile routes every tier to one model; edit its rules to change it' } }, { status: 400 })
        }
        s.model = model
        push(s.id, { kind: 'system', content: `model: ${model || 'default'}` })
        return HttpResponse.json(s)
    }),
    http.put('/api/v1/desk/sessions/:id/profile', async ({ params, request }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        s.profile = ((await request.json()) as { profile: string }).profile
        if ((tiers[s.profile] ?? tiers['']).unified) s.model = ''
        push(s.id, { kind: 'system', content: `profile: ${s.profile || 'default'}` })
        return HttpResponse.json(s)
    }),
    http.put('/api/v1/desk/sessions/:id/permission-mode', async ({ params, request }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        s.permission_mode = ((await request.json()) as { mode: string }).mode
        return HttpResponse.json(s)
    }),
    http.post('/api/v1/desk/sessions/:id/interrupt', ({ params }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        push(s.id, { kind: 'system', content: 'interrupted; send a message to resume' })
        s.awaiting_input = false
        touch(s, 'completed')
        return new HttpResponse(null, { status: 204 })
    }),
    http.post('/api/v1/desk/sessions/:id/handoff', ({ params }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        if (s.status === 'running' || s.status === 'pending') return HttpResponse.json({ error: { message: 'a turn is in progress; stop it or wait for it to finish' } }, { status: 409 })
        const tb = "'/usr/local/bin/tingly-box'"
        const launch = s.profile ? `${tb} profile '${s.profile}'` : `${tb} cc`
        return HttpResponse.json({ command: `cd '${s.project}' && ${launch} --resume '${s.id}'` })
    }),
    http.post('/api/v1/desk/sessions/:id/tasks/:taskId/stop', ({ params }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        const task = s.background_tasks.find((t) => t.task_id === params.taskId)
        if (!task) return HttpResponse.json({ error: { message: `task ${params.taskId} is not running` } }, { status: 409 })
        s.background_tasks = s.background_tasks.filter((t) => t !== task)
        const call = (messages[s.id] ?? []).find((m) => m.kind === 'task' && (m.payload as { task_id?: string }).task_id === task.task_id)
        push(s.id, { kind: 'task', content: task.description, request_id: call?.request_id, payload: { event: 'task_notification', task_id: task.task_id, status: 'stopped', summary: task.description } })
        return new HttpResponse(null, { status: 202 })
    }),
    http.get('/api/v1/desk/sessions/:id/tasks/:taskId/output', ({ params }) => {
        if (params.taskId === 'bd1') {
            // A running server: each read shows a few more request lines.
            devReads++
            const lines = ['> frontend@0.1.0 dev', '> vite --port 5173', '', '  VITE v6.3.5  ready in 412 ms', '', '  ➜  Local:   http://localhost:5173/']
            for (let i = 0; i < devReads * 2; i++) lines.push(`${new Date(Date.now() - (devReads * 2 - i) * 1000).toLocaleTimeString()} [vite] hmr update /src/pages/desk/DeskPage.tsx`)
            const content = lines.join('\n') + '\n'
            return HttpResponse.json({ content, truncated: false, size: content.length })
        }
        if (params.taskId !== 'bt1') return HttpResponse.json({ error: { message: 'output not found' } }, { status: 404 })
        const content = '=== RUN   TestAuth_Check\n--- PASS: TestAuth_Check (0.01s)\n=== RUN   TestAuth_RefreshConcurrent\n--- PASS: TestAuth_RefreshConcurrent (0.23s)\nPASS\nok  \tgithub.com/tingly-dev/tingly-box/internal/server/middleware\t1.412s\n\n[exited with code 0]\n'
        return HttpResponse.json({ content, truncated: false, size: content.length })
    }),
    http.post('/api/v1/desk/sessions/:id/archive', ({ params }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        s.background_tasks = []
        touch(s, 'closed')
        return HttpResponse.json(s)
    }),
]
