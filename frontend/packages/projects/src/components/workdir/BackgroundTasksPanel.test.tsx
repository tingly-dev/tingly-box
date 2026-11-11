import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { BackgroundTaskRow } from './backgroundTasks';
import { BackgroundTasksPanel } from './BackgroundTasksPanel';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key), i18n: { language: 'en' } }) }));

afterEach(cleanup);

const task = (o: Partial<BackgroundTaskRow> = {}): BackgroundTaskRow => ({
    taskId: 'a', callId: '', status: 'running', background: true, taskType: 'local_agent', description: 'scout', ended: false,
    input: { prompt: 'look around' }, ...o,
} as BackgroundTaskRow);

const props = { sessionId: 's', onChanged: () => undefined, onReveal: () => undefined };

describe('BackgroundTasksPanel opening rows', () => {
    it('starts with the task a drill-in asked for expanded', () => {
        render(<BackgroundTasksPanel {...props} tasks={[task({}), task({ taskId: 'b', description: 'build' })]} initialOpenId="b" />);
        expect(screen.getByText('tasks.agentPrompt')).toBeTruthy();
        expect(screen.getByText('look around')).toBeTruthy();
    });

    it('still opens a lone task by itself', () => {
        render(<BackgroundTasksPanel {...props} tasks={[task()]} />);
        expect(screen.getByText('tasks.agentPrompt')).toBeTruthy();
    });

    it('keeps rows folded with several tasks and nothing asked, and folds out on click', () => {
        render(<BackgroundTasksPanel {...props} tasks={[task({}), task({ taskId: 'b', description: 'build' })]} />);
        expect(screen.queryByText('tasks.agentPrompt')).toBeNull();
        fireEvent.click(screen.getByText('build'));
        expect(screen.getByText('tasks.agentPrompt')).toBeTruthy();
    });
});
