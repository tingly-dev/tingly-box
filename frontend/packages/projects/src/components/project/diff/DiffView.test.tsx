import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Diff, DiffFile } from '../../../client';

const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('../../../client', async (importOriginal) => ({ ...(await importOriginal<typeof import('../../../client')>()), api: { GET: get } }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o && 'count' in o ? `${key}:${o.count}` : key), i18n: { language: 'en' } }) }));

import { DiffView } from './DiffView';

const file = (path: string, extra: Partial<DiffFile> = {}): DiffFile => ({ path, status: 'modified', additions: 2, deletions: 1, ...extra });
const diffOf = (files: DiffFile[]): Diff => ({ base: 'main', branch: 'tp/x', files, additions: 0, deletions: 0, uncommitted: '' });
const patchOf = (n: number) => ['@@ -1,2 +1,2 @@', ...Array.from({ length: n }, (_, i) => `+line ${i}`)].join('\n');

const reply = (patch: string, extra: Record<string, unknown> = {}) => Promise.resolve({ data: { path: 'x', patch, ...extra }, response: { ok: true } });
const queryOf = (call: unknown[]) => (call[1] as { params: { query: { path: string } } }).params.query;

beforeEach(() => {
    get.mockReset();
    get.mockImplementation(() => reply(patchOf(5)));
});
afterEach(() => {
    cleanup();
    localStorage.clear();
});

describe('DiffView', () => {
    it('shows a branch with no changes whose file list came back null, instead of crashing', () => {
        // A diff of an unchanged branch used to be sent as `"files": null`.
        const { container } = render(<DiffView threadId="t1" diff={{ ...diffOf([]), files: null } as unknown as Diff} />);
        expect(container.textContent).toContain('thread.noChanges');
    });

    it('windows a 5000-file list and fetches only the file it opens on, skipping generated and binary ones first', async () => {
        const files = [file('assets/a.png', { binary: true }), file('package-lock.json', { generated: true }), ...Array.from({ length: 5000 }, (_, i) => file(`src/f${String(i).padStart(4, '0')}.ts`))];
        render(<DiffView threadId="t1" diff={diffOf(files)} />);
        expect(screen.getAllByRole('treeitem').length).toBeLessThan(60);
        await waitFor(() => expect(screen.getByTestId('diff-patch')).toBeInTheDocument());
        const paths = get.mock.calls.map((c) => queryOf(c).path);
        expect(paths).toContain('src/f0000.ts');
        expect(paths).not.toContain('package-lock.json');
        expect(paths).not.toContain('assets/a.png');
        expect(paths.length).toBeLessThanOrEqual(2);
    });

    it('folds a generated file until asked for it', async () => {
        const files = [file('a.ts'), file('pnpm-lock.yaml', { generated: true })];
        render(<DiffView threadId="t1" diff={diffOf(files)} />);
        await waitFor(() => expect(screen.getByTestId('diff-patch')).toBeInTheDocument());
        fireEvent.click(screen.getByTitle('pnpm-lock.yaml'));
        expect(screen.getByText('thread.diff.generatedTitle')).toBeInTheDocument();
        expect(get.mock.calls.some((c) => queryOf(c).path === 'pnpm-lock.yaml')).toBe(false);
        fireEvent.click(screen.getByText('thread.diff.showDiff'));
        await waitFor(() => expect(screen.getByTestId('diff-patch')).toBeInTheDocument());
    });

    it('does not show, fetch or offer to load a huge or a binary file', async () => {
        const files = [file('a.ts'), file('huge.log', { additions: 30000, deletions: 1 }), file('pic.png', { binary: true })];
        render(<DiffView threadId="t1" diff={diffOf(files)} />);
        await waitFor(() => expect(screen.getByTestId('diff-patch')).toBeInTheDocument());
        get.mockClear();

        fireEvent.click(screen.getByTitle('huge.log'));
        expect(screen.getByText('thread.diff.tooLargeTitle')).toBeInTheDocument();
        fireEvent.click(screen.getByTitle('pic.png'));
        expect(screen.getByText('thread.diff.binaryTitle')).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: /load|show/i })).not.toBeInTheDocument();
        expect(get).not.toHaveBeenCalled();
    });

    it('says so when the server refuses a patch for its size, and gives no way around it', async () => {
        get.mockImplementation(() => reply('', { too_large: true }));
        render(<DiffView threadId="t1" diff={diffOf([file('a.ts')])} />);
        await waitFor(() => expect(screen.getByText('thread.diff.tooLargeTitle')).toBeInTheDocument());
        expect(screen.queryByText('thread.diff.showDiff')).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: /load/i })).not.toBeInTheDocument();
    });

    it('renders only the visible rows of a 100k-line patch', async () => {
        get.mockImplementation(() => reply(patchOf(100000)));
        render(<DiffView threadId="t1" diff={diffOf([file('a.ts')])} />);
        await waitFor(() => expect(screen.getByTestId('diff-patch')).toBeInTheDocument());
        await act(async () => undefined);
        expect(screen.getByTestId('diff-patch').querySelectorAll('.r').length).toBeLessThan(100);
    });

    it('moves between files with j and k, and v marks a file viewed and moves on, remembered for the thread', async () => {
        const files = [file('a.ts'), file('b.ts'), file('c.ts')];
        const { unmount } = render(<DiffView threadId="t9" diff={diffOf(files)} />);
        await waitFor(() => expect(screen.getByTestId('diff-patch')).toBeInTheDocument());
        const selected = () => screen.getAllByRole('treeitem').find((o) => o.getAttribute('aria-selected') === 'true')?.getAttribute('title');
        expect(selected()).toBe('a.ts');
        fireEvent.keyDown(window, { key: 'j' });
        expect(selected()).toBe('b.ts');
        fireEvent.keyDown(window, { key: 'k' });
        expect(selected()).toBe('a.ts');
        fireEvent.keyDown(window, { key: 'v' });
        expect(selected()).toBe('b.ts');
        unmount();
        render(<DiffView threadId="t9" diff={diffOf(files)} />);
        expect((screen.getAllByLabelText('thread.diff.viewed')[0] as HTMLInputElement).checked).toBe(true);
    });

    it('forgets a viewed mark once the file’s change has moved on', () => {
        localStorage.setItem('tingly.projects.diffViewed.t2', JSON.stringify({ 'a.ts': 'modified:2:1:' }));
        render(<DiffView threadId="t2" diff={diffOf([file('a.ts', { additions: 5 })])} />);
        expect((screen.getAllByLabelText('thread.diff.viewed')[0] as HTMLInputElement).checked).toBe(false);
    });

    it('filters by name and says when nothing matches', () => {
        render(<DiffView threadId="t1" diff={diffOf([file('src/a.ts'), file('docs/b.md')])} />);
        fireEvent.change(screen.getByLabelText('thread.diff.filterPlaceholder'), { target: { value: 'docs' } });
        // The match, plus the folder kept above it.
        expect(screen.getAllByRole('treeitem')).toHaveLength(2);
        fireEvent.change(screen.getByLabelText('thread.diff.filterPlaceholder'), { target: { value: 'zzz' } });
        expect(screen.getByText('thread.diff.noMatch')).toBeInTheDocument();
    });

    it('says how many files the server left out of the list, and which long lines were shortened', async () => {
        get.mockImplementation(() => reply(patchOf(3), { lines_cut: 2 }));
        render(<DiffView threadId="t1" diff={{ ...diffOf([file('a.ts')]), files_omitted: 30002 }} />);
        expect(screen.getByText('thread.diff.omitted:30002')).toBeInTheDocument();
        await waitFor(() => expect(screen.getByText('thread.diff.linesCut:2')).toBeInTheDocument());
    });

    it('keeps only the last few patches in memory: an old file is fetched again when revisited', async () => {
        const files = Array.from({ length: 10 }, (_, i) => file(`f${i}.ts`));
        render(<DiffView threadId="t1" diff={diffOf(files)} />);
        const flush = () => act(async () => undefined);
        await flush();
        for (let i = 0; i < 9; i++) {
            fireEvent.keyDown(window, { key: 'j' });
            await flush();
        }
        const fetched = (name: string) => get.mock.calls.filter((c) => queryOf(c).path === name).length;
        expect(fetched('f0.ts')).toBe(1);
        fireEvent.click(screen.getByTitle('f0.ts'));
        await flush();
        await waitFor(() => expect(fetched('f0.ts')).toBe(2));
    });
});
