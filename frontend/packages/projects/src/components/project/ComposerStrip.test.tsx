import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread, ThreadLink } from '../../client';
import { autoAnswerOf, ComposerStrip } from './ComposerStrip';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

afterEach(() => {
    cleanup();
    localStorage.clear();
});

const thread = (o: Partial<Thread>): Thread => ({
    id: 't', project_id: 'p', title: 'Fix login', task: '', status: 'working', origin: 'user', agent: 'claude-code', seq: 1, cwd: '/work/app',
    commits_ahead: 0, files_changed: 0, uncommitted: false, turns: 0, input_tokens: 0, output_tokens: 0, created_at: '', updated_at: '', ...o,
} as Thread);

describe('ComposerStrip', () => {
    it('defaults to waiting ten minutes in a project thread and to off in a folder session', () => {
        expect(autoAnswerOf(thread({}))).toBe('delay');
        expect(autoAnswerOf(thread({}), true)).toBe('off');
        expect(autoAnswerOf(thread({ auto_answer: 'now' }), true)).toBe('now');
    });

    it('leaves where the thread runs to the rail: no folder or branch line', () => {
        render(<ComposerStrip thread={thread({ branch: 'tb/fix-login', base_branch: 'main' })} onAutoAnswer={vi.fn()} />);
        expect(screen.queryByText('tb/fix-login')).toBeNull();
        expect(screen.queryByText('/work/app')).toBeNull();
        expect(screen.queryByText('strip.title')).toBeNull();
    });

    it('changes the auto-answer setting from its menu', () => {
        const onAutoAnswer = vi.fn();
        render(<ComposerStrip thread={thread({})} onAutoAnswer={onAutoAnswer} />);
        fireEvent.click(screen.getByRole('button', { name: 'strip.autoAnswer.change' }));
        fireEvent.click(screen.getByText('strip.autoAnswer.now'));
        expect(onAutoAnswer).toHaveBeenCalledWith('now');
    });

    it('drops auto-answer once resolved, and with it the whole strip when there is nothing else', () => {
        render(<ComposerStrip thread={thread({ head_branch: 'main', status: 'resolved' })} folder onAutoAnswer={vi.fn()} />);
        expect(screen.queryByTestId('composer-strip')).toBeNull();
    });

    describe('linked issues and PRs', () => {
        const at = (m: number) => new Date(Date.now() - m * 60_000).toISOString();
        const link = (o: Partial<ThreadLink>): ThreadLink => ({
            kind: 'pr', repo: 'acme/w', number: 12, url: 'https://github.com/acme/w/pull/12', title: 'Add the tool',
            role: 'opened', state: 'open', refreshed_at: at(1), attempted_at: at(1), created_at: at(60), ...o,
        } as ThreadLink);

        it('lists each link under the title with its title, state and the refresh and details buttons', () => {
            const th = thread({ branch: 'tb/fix', links: [link({ ci: 'passing', merge_state: 'ready' })] });
            render(<ComposerStrip thread={th} onAutoAnswer={vi.fn()} />);
            const strip = screen.getByTestId('composer-strip');
            expect(within(strip).getByText('strip.title')).toBeInTheDocument();
            expect(within(strip).getByText('strip.pr')).toBeInTheDocument();
            const a = within(strip).getByRole('link', { name: '#12 Add the tool' });
            expect(a).toHaveAttribute('href', 'https://github.com/acme/w/pull/12');
            expect(within(strip).getByText('links.checksPassing')).toBeInTheDocument();
            expect(within(strip).getByRole('button', { name: 'links.refresh' })).toBeInTheDocument();
            expect(within(strip).getByRole('button', { name: /links\.detailsOf/ })).toHaveAttribute('aria-haspopup', 'dialog');
        });

        it('offers an open PR\'s changes on its line, and nothing of the kind on an issue or a finished PR', () => {
            const th = thread({ branch: 'tb/fix', links: [
                link({ head: 'tb/fix', base: 'main' }),
                link({ number: 13, title: 'Done', state: 'merged', head: 'x', base: 'main' }),
                link({ kind: 'issue', number: 3, title: 'Flaky', url: 'https://github.com/acme/w/issues/3' }),
            ] });
            render(<ComposerStrip thread={th} onAutoAnswer={vi.fn()} />);
            expect(screen.getAllByRole('button', { name: /links\.filesOf/ })).toHaveLength(1);
        });

        it('puts an open PR first, then an open issue, then what finished, and lists every one', () => {
            const th = thread({ links: [
                link({ number: 1, state: 'merged', title: 'Old', created_at: at(10) }),
                link({ number: 2, kind: 'issue', title: 'Bug', url: 'https://github.com/acme/w/issues/2', created_at: at(30) }),
                link({ number: 3, title: 'New PR', created_at: at(90) }),
                link({ number: 4, title: 'Older PR', created_at: at(120) }),
            ] });
            render(<ComposerStrip thread={th} />);
            const strip = screen.getByTestId('composer-strip');
            expect(within(strip).getAllByRole('link').map((a) => a.textContent)).toEqual(['#3 New PR', '#4 Older PR', '#2 Bug', '#1 Old']);
        });

        it('is there for a thread that has only links', () => {
            render(<ComposerStrip thread={thread({ cwd: '', links: [link({})] })} />);
            expect(screen.getByTestId('composer-strip')).toBeInTheDocument();
        });

        it('has no title or link lines for a thread with none', () => {
            render(<ComposerStrip thread={thread({ branch: 'tb/fix' })} onAutoAnswer={vi.fn()} />);
            expect(screen.queryByText('strip.title')).toBeNull();
            expect(screen.queryByRole('button', { name: 'links.refresh' })).not.toBeInTheDocument();
        });
    });
});
