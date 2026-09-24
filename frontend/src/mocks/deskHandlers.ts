import { http, HttpResponse } from 'msw'

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
    created_at: string
    last_activity: string
}

const TB = '/Users/me/code/tingly-box'
const SITE = '/Users/me/code/website'
const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString()

const sessions: Sess[] = [
    { id: 'desk-1', project: TB, status: 'running', request: 'Fix the flaky persistent session test in internal/desk', response: '', permission_mode: '', created_at: ago(12), last_activity: ago(1) },
    { id: 'desk-2', project: TB, status: 'completed', request: 'Add a dark mode toggle to the settings page', response: '', permission_mode: 'acceptEdits', created_at: ago(90), last_activity: ago(40) },
    { id: 'desk-3', project: SITE, status: 'failed', request: 'Update the pricing page copy', response: '', error: 'agent CLI not available', permission_mode: '', created_at: ago(200), last_activity: ago(199) },
    { id: 'desk-4', project: SITE, status: 'closed', request: 'Draft release notes for v1.2', response: '', permission_mode: '', created_at: ago(3000), last_activity: ago(2900) },
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
    ],
    'desk-3': [
        { role: 'user', content: 'Update the pricing page copy', timestamp: ago(200) },
        { kind: 'error', content: 'agent CLI not available', timestamp: ago(199) },
    ],
    'desk-4': [
        { role: 'user', content: 'Draft release notes for v1.2', timestamp: ago(3000) },
        { role: 'assistant', content: 'Drafted RELEASE_NOTES.md with the highlights from the last 40 commits.', timestamp: ago(2950) },
    ],
}

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
        push(s.id, { role: 'assistant', content: reply })
        touch(s, 'completed')
    }, 2500)
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
        const body = (await request.json()) as { path: string; prompt: string; permission_mode?: string }
        const now = new Date().toISOString()
        const s: Sess = { id: `desk-${Date.now()}`, project: body.path, status: 'pending', request: body.prompt, response: '', permission_mode: body.permission_mode ?? '', created_at: now, last_activity: now }
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
        setTimeout(() => {
            push(s.id, { role: 'assistant', content: approved ? 'All green across 5 race-checked runs.' : 'Skipped running the tests.' })
            touch(s, 'completed')
        }, 1500)
        return new HttpResponse(null, { status: 204 })
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
        touch(s, 'completed')
        return new HttpResponse(null, { status: 204 })
    }),
    http.post('/api/v1/desk/sessions/:id/archive', ({ params }) => {
        const s = find(params.id as string)
        if (!s) return HttpResponse.json({ error: { message: 'session not found' } }, { status: 404 })
        touch(s, 'closed')
        return HttpResponse.json(s)
    }),
]
