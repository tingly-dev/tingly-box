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

    it('folds to the fact that matters now plus a count, and unfolds to one line per fact', () => {
        const onAutoAnswer = vi.fn();
        const th = thread({ branch: 'tb/fix-login', base_branch: 'main' });
        const { rerender } = render(<ComposerStrip thread={th} onAutoAnswer={onAutoAnswer} />);
        // Folded: the branch leads, the folder and the setting are counted.
        expect(screen.getByText('tb/fix-login')).toBeInTheDocument();
        expect(screen.getByText('+2')).toBeInTheDocument();
        expect(screen.queryByText('app')).toBeNull();
        // A question about to answer itself takes the line.
        rerender(<ComposerStrip thread={th} onAutoAnswer={onAutoAnswer} autoAt={new Date().toISOString()} />);
        expect(screen.getByText('strip.autoAt')).toBeInTheDocument();
        expect(screen.queryByText('tb/fix-login')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'strip.expand' }));
        expect(screen.getByText('/work/app')).toBeInTheDocument();
        expect(screen.getByText('tb/fix-login')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'strip.autoAnswer.change' }));
        fireEvent.click(screen.getByText('strip.autoAnswer.now'));
        expect(onAutoAnswer).toHaveBeenCalledWith('now');
    });

    it('names a folder session’s branch, and drops auto-answer once resolved', () => {
        render(<ComposerStrip thread={thread({ head_branch: 'main', status: 'resolved' })} folder onAutoAnswer={vi.fn()} />);
        expect(screen.getByText('main')).toBeInTheDocument();
        expect(screen.queryByText('strip.autoAnswer.off')).toBeNull();
    });

    describe('linked issues and PRs', () => {
        const at = (m: number) => new Date(Date.now() - m * 60_000).toISOString();
        const link = (o: Partial<ThreadLink>): ThreadLink => ({
            kind: 'pr', repo: 'acme/w', number: 12, url: 'https://github.com/acme/w/pull/12', title: 'Add the tool',
            role: 'opened', state: 'open', refreshed_at: at(1), attempted_at: at(1), created_at: at(60), ...o,
        } as ThreadLink);

        it('leads the folded line with the PR and what is wrong with it, ahead of the branch', () => {
            const th = thread({ branch: 'tb/fix', base_branch: 'main', links: [link({ ci: 'failing', merge_state: 'conflicts' })] });
            render(<ComposerStrip thread={th} onAutoAnswer={vi.fn()} />);
            const strip = screen.getByTestId('composer-strip');
            expect(within(strip).getByText('#12')).toBeInTheDocument();
            expect(within(strip).getByText('links.checksFailing')).toBeInTheDocument();
            expect(within(strip).getByText('links.conflicts')).toBeInTheDocument();
            expect(within(strip).queryByText('tb/fix')).not.toBeInTheDocument();
            // The PR, the branch, the folder and the setting: three more behind the lead.
            expect(within(strip).getByText('+3')).toBeInTheDocument();
        });

        it('lists each link in the open strip with its title, state and the refresh and details buttons', () => {
            const th = thread({ branch: 'tb/fix', links: [link({ ci: 'passing', merge_state: 'ready' })] });
            render(<ComposerStrip thread={th} onAutoAnswer={vi.fn()} />);
            fireEvent.click(screen.getByRole('button', { name: 'strip.expand' }));
            const strip = screen.getByTestId('composer-strip');
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
            fireEvent.click(screen.getByRole('button', { name: 'strip.expand' }));
            expect(screen.getAllByRole('button', { name: /links\.filesOf/ })).toHaveLength(1);
        });

        it('puts an open PR first, then an open issue, then what finished, and keeps the strip short', () => {
            const th = thread({ links: [
                link({ number: 1, state: 'merged', title: 'Old', created_at: at(10) }),
                link({ number: 2, kind: 'issue', title: 'Bug', url: 'https://github.com/acme/w/issues/2', created_at: at(30) }),
                link({ number: 3, title: 'New PR', created_at: at(90) }),
                link({ number: 4, title: 'Older PR', created_at: at(120) }),
            ] });
            render(<ComposerStrip thread={th} />);
            fireEvent.click(screen.getByRole('button', { name: 'strip.expand' }));
            const strip = screen.getByTestId('composer-strip');
            expect(within(strip).getAllByRole('link').map((a) => a.textContent)).toEqual(['#3 New PR', '#4 Older PR', '#2 Bug']);
            expect(within(strip).getByText('strip.moreLinks')).toBeInTheDocument();
        });

        it('is there for a thread that has only links', () => {
            render(<ComposerStrip thread={thread({ cwd: '', links: [link({})] })} />);
            expect(screen.getByTestId('composer-strip')).toBeInTheDocument();
        });

        it('leaves the strip as it was for a thread with none', () => {
            render(<ComposerStrip thread={thread({ branch: 'tb/fix' })} onAutoAnswer={vi.fn()} />);
            expect(screen.getByText('tb/fix')).toBeInTheDocument();
            expect(screen.queryByRole('button', { name: 'links.refresh' })).not.toBeInTheDocument();
        });
    });
});
