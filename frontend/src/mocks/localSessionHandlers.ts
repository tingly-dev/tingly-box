import { http, HttpResponse } from 'msw';
import type { SessionEvent } from '@/services/localSessionsApi';

const entries = [
    { id: 'demo-latest', name: 'a1b2c3-session.jsonl', title: 'Improve the session preview without loading the entire file', size: 1088364544, modified: '2026-10-31T03:40:00Z' },
    { id: 'demo-earlier', name: 'd4e5f6-session.jsonl', title: 'Review the local API access boundary', size: 2846241, modified: '2026-10-30T13:12:00Z' },
];
const events: SessionEvent[] = [
    { id: 'm1', type: 'user', role: 'user', timestamp: '2026-10-31T03:37:00Z', blocks: [{ kind: 'text', text: 'The session file is over 1 GiB. Can we preview the latest conversation without loading everything?' }] },
    { id: 'm2', type: 'assistant', role: 'assistant', timestamp: '2026-10-31T03:37:04Z', blocks: [{ kind: 'text', text: 'I’ll read a bounded window from the end of the transcript, then load earlier messages only when requested.' }, { kind: 'thinking', text: 'Keep disk reads, response bytes and rendered messages bounded independently.' }] },
    { id: 'm3', type: 'assistant', role: 'assistant', timestamp: '2026-10-31T03:38:00Z', blocks: [{ kind: 'tool_use', label: 'Read', text: '{"file_path":"internal/localsessions/preview.go"}' }] },
    { id: 'm4', type: 'user', role: 'user', timestamp: '2026-10-31T03:38:01Z', blocks: [{ kind: 'tool_result', text: 'Read a bounded tail window; oversized records are skipped across successive cursors.' }] },
    { id: 'm5', type: 'assistant', role: 'assistant', timestamp: '2026-10-31T03:39:00Z', blocks: [{ kind: 'text', text: 'The preview now reads at most 2 MiB per request and returns up to 50 events. Tools stay collapsed, attachments are omitted, and changed files require a fresh preview.' }] },
];
const json = (data: unknown) => HttpResponse.json({ success: true, data }, { headers: { 'Cache-Control': 'no-store' } });
export const localSessionHandlers = [
    http.get('/api/v2/local-sessions/status', () => json({ local_access: true, local_binding: true, enabled: true })),
    http.get('/api/v2/local-sessions/projects', () => json({ entries: [{ id: 'demo-project', name: '-Users-demo-work-tingly-box', title: '/Users/demo/work/tingly-box', size: 0, modified: entries[0].modified }], total: 1 })),
    http.get('/api/v2/local-sessions/projects/:project/sessions', () => json({ entries, total: entries.length })),
    http.get('/api/v2/local-sessions/sessions/:session/events', ({ request }) => {
        const query = new URL(request.url).searchParams;
        const earlier = query.get('cursor') === 'demo-earlier-page';
        const beginning = query.get('direction') === 'forward';
        return json({ events: earlier || beginning ? [{ ...events[0], id: 'old-1', blocks: [{ kind: 'text', text: 'Start with a read-only session browser on this computer.' }] }] : events,
            next_cursor: earlier || beginning ? undefined : 'demo-earlier-page', direction: beginning ? 'forward' : 'backward', file_size: entries[0].size, scanned_bytes: 2097152, skipped_records: 2 });
    }),
];
