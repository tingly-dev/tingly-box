import { cleanup, fireEvent, render as renderWithHost, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Thread, ThreadEvent } from '../../client';
import { ThreadPanel } from './ThreadPanel';
import { ProjectsHostProvider } from '../../host/ProjectsHost';
import { PhoneShellContext } from '@tingly/ui/hooks/usePhoneShell';

const host = { paths: { list: '/', project: () => '/', folder: () => '/' } };
const render = (ui: React.ReactElement) => renderWithHost(ui, {
    wrapper: ({ children }) => (
        <ProjectsHostProvider host={host}>
            <PhoneShellContext.Provider value>{children}</PhoneShellContext.Provider>
        </ProjectsHostProvider>
    ),
});

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? key, i18n: { language: 'en' } }) }));
vi.mock('./threadOps', () => ({ threadOps: () => ({ respond: vi.fn() }) }));
vi.mock('../ThreadComposer', () => ({ ThreadComposer: () => <div>Composer</div> }));
vi.mock('./DiffDialog', () => ({ DiffDialog: () => null }));
vi.mock('./ComposerStrip', () => ({ ComposerStrip: () => null }));
vi.mock('../Markdown', () => ({ Markdown: ({ text }: { text: string }) => <div>{text}</div>, CodeBlock: () => null }));

beforeEach(() => localStorage.clear());
afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
});

const thread = { id: 't', title: 'Work', status: 'waiting', task: '', branch: '', cwd: '/work/app', input_tokens: 0, output_tokens: 0, turns: 0 } as Thread;
const user = { id: 'e1', kind: 'user', content: 'Hello there', thread_id: 't', created_at: '2026-10-25T00:00:00Z', updated_at: '' } as ThreadEvent;
const approval = {
    id: 'e2', kind: 'approval', content: 'Run tests', thread_id: 't', created_at: '2026-10-25T00:01:00Z', updated_at: '',
    payload: JSON.stringify({ request_id: 'r1', tool: 'Bash', input: { command: 'go test ./...' }, state: 'pending' }),
} as ThreadEvent;
const props = { projectId: 'p', threadId: 't', thread, events: [user, approval], running: true, disabled: false, onBack: () => {} };

describe('the thread page on a phone', () => {
    it('uses the compact bar and moves the thread details into the ⋯ menu', () => {
        render(<ThreadPanel {...props} />);
        expect(screen.getByText('Work')).toBeInTheDocument();
        // The wide-screen Details icon is gone; the same cards open from the menu.
        expect(screen.queryByRole('button', { name: 'rail.title' })).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'common.more' }));
        fireEvent.click(screen.getByRole('menuitem', { name: 'rail.title' }));
        expect(screen.getByRole('dialog')).toBeInTheDocument();
        expect(screen.getByRole('region', { name: 'rail.status' })).toBeInTheDocument();
    });

    it('docks the pending request above the composer and keeps it out of the transcript', () => {
        render(<ThreadPanel {...props} />);
        expect(screen.getAllByRole('button', { name: 'Allow' })).toHaveLength(1);
        expect(screen.getByRole('button', { name: 'thread.allowThread' })).toBeInTheDocument();
        expect(screen.getByText('Composer')).toBeInTheDocument();
    });
});
