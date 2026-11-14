import { cleanup, fireEvent, render as renderWithHost, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { ThreadPanel } from './ThreadPanel';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

const host = { paths: { list: '/', project: () => '/', folder: () => '/' } };
const render = (ui: React.ReactElement) => renderWithHost(ui, {
    wrapper: ({ children }) => <ProjectsHostProvider host={host}>{children}</ProjectsHostProvider>,
});

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? key, i18n: { language: 'en' } }) }));
vi.mock('./threadOps', () => ({ threadOps: () => ({ respond: vi.fn() }) }));
vi.mock('../ThreadComposer', () => ({ ThreadComposer: () => <div>Composer</div> }));
vi.mock('./DiffDialog', () => ({ DiffDialog: () => null }));
vi.mock('./ComposerStrip', () => ({ ComposerStrip: () => null }));
vi.mock('../Markdown', () => ({ Markdown: ({ text }: { text: string }) => <div>{text}</div>, CodeBlock: () => null }));

/** A panel of the given width, as the ResizeObserver reports it (jsdom has none, so the rail never fits there). */
function panelWidth(width: number) {
    vi.stubGlobal('ResizeObserver', class {
        constructor(private cb: (entries: { contentRect: { width: number } }[]) => void) {}
        observe() { this.cb([{ contentRect: { width } }]); }
        disconnect() {}
    });
}

beforeEach(() => localStorage.clear());
afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
});

const thread = { id: 't', title: 'Work', status: 'idle', task: '', branch: '', cwd: '/work/app', input_tokens: 0, output_tokens: 0, turns: 0 } as Thread;
const props = { projectId: 'p', threadId: 't', thread, events: [], running: false, disabled: false, onBack: () => {} };
const rail = () => screen.queryByRole('complementary', { name: 'rail.title' });

describe('the Details button', () => {
    it.each([false, true])('opens the same cards under it where the rail does not fit (folder=%s)', (folder) => {
        render(<ThreadPanel {...props} folder={folder} />);
        expect(rail()).not.toBeInTheDocument();
        const button = screen.getByRole('button', { name: 'rail.title' });
        expect(button).toHaveAttribute('aria-haspopup', 'dialog');
        fireEvent.click(button);
        expect(rail()).toBeInTheDocument();
        expect(screen.getByText('/work/app')).toBeInTheDocument();
        expect(screen.getByRole('region', { name: 'rail.status' })).toBeInTheDocument();
    });

    it.each([false, true])('docks and folds the column where it fits, and remembers the choice (folder=%s)', (folder) => {
        panelWidth(1000);
        render(<ThreadPanel {...props} folder={folder} />);
        expect(rail()).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'rail.hide' }));
        expect(rail()).not.toBeInTheDocument();
        expect(localStorage.getItem('tingly-project.thread.rail')).toBe('0');
        fireEvent.click(screen.getByRole('button', { name: 'rail.show' }));
        expect(rail()).toBeInTheDocument();
    });

    it('is the same button for a folder session as for a project thread, whatever sits beside the page', () => {
        panelWidth(700);
        render(<ThreadPanel {...props} folder />);
        // The session list lives in the page's side pane, never in the rail: this width docks the rail like a thread's.
        expect(rail()).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'rail.title' })).toBeInTheDocument();
        expect(screen.queryByText('session.sessions')).not.toBeInTheDocument();
    });
});

describe('the Back button', () => {
    it('names where it goes, and is a bare arrow only when it is not told', () => {
        const onBack = vi.fn();
        const { rerender } = render(<ThreadPanel {...props} onBack={onBack} backLabel="Coordinator" />);
        const named = screen.getByRole('button', { name: 'thread.backTo' });
        expect(named).toHaveTextContent('Coordinator');
        fireEvent.click(named);
        expect(onBack).toHaveBeenCalledTimes(1);
        rerender(<ThreadPanel {...props} onBack={onBack} />);
        expect(screen.getByRole('button', { name: 'thread.back' })).not.toHaveTextContent('Coordinator');
    });
});
