import { describe, expect, it } from 'vitest';
import type { TranscriptEntry } from '../../transcript';
import { backgroundTasks } from './backgroundTasks';

const at = (s: number) => new Date(Date.UTC(2026, 9, 16, 10, 0, s)).toISOString();

describe('backgroundTasks', () => {
    const entries: TranscriptEntry[] = [
        { kind: 'tool_use', content: 'Bash', request_id: 'c1', payload: { command: 'npm run dev', run_in_background: true }, timestamp: at(0) },
        { kind: 'task', content: '', request_id: 'c1', payload: { event: 'task_started', task_id: 't1', task_type: 'local_bash', background: true, description: 'dev server' }, timestamp: at(1) },
        { kind: 'tool_use', content: 'Agent', request_id: 'c2', payload: { prompt: 'look around', subagent_type: 'Explore' }, timestamp: at(2) },
        { kind: 'task', content: '', request_id: 'c2', payload: { event: 'task_started', task_id: 't2', task_type: 'local_agent', background: true }, timestamp: at(3) },
        { kind: 'tool_use', content: 'Read', request_id: 'c3', parent: 'c2', payload: { file_path: 'a.ts' }, timestamp: at(4) },
        { role: 'assistant', content: 'Found it.', parent: 'c2', timestamp: at(5) },
        { kind: 'task', content: '', request_id: 'c2', payload: { event: 'task_notification', task_id: 't2', status: 'completed', summary: 'done' }, timestamp: at(6) },
        // A foreground subagent is not a background task.
        { kind: 'task', content: '', request_id: 'c4', payload: { event: 'task_started', task_id: 't4', background: false }, timestamp: at(7) },
    ];

    it('lists running work first, with what each call asked and did', () => {
        const rows = backgroundTasks(entries, [{ task_id: 't1', task_type: 'local_bash', description: 'dev server' }]);
        expect(rows.map((r) => r.taskId)).toEqual(['t1', 't2']);
        expect(rows[0]).toMatchObject({ status: 'running', ended: false, input: { command: 'npm run dev' }, startedAt: at(0) });
        expect(rows[1]).toMatchObject({ status: 'completed', finishedAt: at(6), reply: 'Found it.', recent: [{ name: 'Read', summary: 'a.ts' }] });
    });

    it('settles a task whose process went away, and adds one known only live', () => {
        const rows = backgroundTasks(entries, [{ task_id: 't9', task_type: 'local_bash', description: 'watch' }]);
        expect(rows.find((r) => r.taskId === 't1')?.ended).toBe(true);
        expect(rows[0]).toMatchObject({ taskId: 't9', status: 'running', callId: '' });
    });
});
