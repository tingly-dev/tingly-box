import { useState } from 'react';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { LazyTree, type TreeNode } from './LazyTree';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(() => cleanup());

const dir = (path: string): TreeNode => ({ path, name: path.split('/').pop()!, kind: 'dir' });
const file = (path: string): TreeNode => ({ path, name: path.split('/').pop()!, kind: 'file' });
const fs: Record<string, TreeNode[]> = {
    '': [dir('outputs'), dir('empty'), file('notes.md')],
    outputs: [dir('outputs/run'), file('outputs/a.txt')],
    'outputs/run': [file('outputs/run/report.md')],
    empty: [],
};

function Harness({ load, initial = [], onActivate, onExpanded, refreshKey = 0 }: { load: (p: string) => Promise<TreeNode[]>; initial?: string[]; onActivate?: (n: TreeNode) => void; onExpanded?: (p: string[]) => void; refreshKey?: number }) {
    const [expanded, setExpanded] = useState(initial);
    return <LazyTree ariaLabel="files" load={load} expanded={expanded} refreshKey={refreshKey} onActivate={onActivate}
        onExpandedChange={(p) => { setExpanded(p); onExpanded?.(p); }}
        renderMeta={(n) => <span>{`meta:${n.name}`}</span>}
        renderActions={(n, { tabIndex }) => <button tabIndex={tabIndex} aria-label={`act ${n.name}`}>x</button>} />;
}
const loader = () => vi.fn((p: string) => (p in fs ? Promise.resolve(fs[p]) : Promise.reject(new Error('missing'))));
const item = (name: string) => screen.getByRole('treeitem', { name: new RegExp(`^${name}`) });

it('loads only the root until a folder is expanded, then one level at a time', async () => {
    const load = loader();
    render(<Harness load={load} />);
    await screen.findByText('outputs');
    expect(load.mock.calls.map((c) => c[0])).toEqual(['']);
    expect(item('outputs')).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('a.txt')).not.toBeInTheDocument();

    fireEvent.click(item('outputs'));
    await screen.findByText('a.txt');
    expect(load.mock.calls.map((c) => c[0])).toEqual(['', 'outputs']);
    expect(item('outputs')).toHaveAttribute('aria-expanded', 'true');
    expect(item('a.txt')).toHaveAttribute('aria-level', '2');
    expect(screen.queryByText('report.md')).not.toBeInTheDocument();

    // Collapsing hides the children; expanding again reuses what was loaded.
    fireEvent.click(item('outputs'));
    expect(screen.queryByText('a.txt')).not.toBeInTheDocument();
    fireEvent.click(item('outputs'));
    await screen.findByText('a.txt');
    expect(load).toHaveBeenCalledTimes(2);
});

it('restores expanded folders from the controlled state, loading each level', async () => {
    const load = loader();
    render(<Harness load={load} initial={['outputs', 'outputs/run']} />);
    await screen.findByText('report.md');
    expect(load.mock.calls.map((c) => c[0]).sort()).toEqual(['', 'outputs', 'outputs/run']);
});

it('forgets an expanded folder that no longer exists', async () => {
    const load = loader();
    const changed = vi.fn();
    render(<Harness load={load} initial={['gone', 'outputs']} onExpanded={changed} />);
    await screen.findByText('a.txt');
    await waitFor(() => expect(changed).toHaveBeenLastCalledWith(['outputs']));
});

it('shows an empty folder, and a failed load with a retry', async () => {
    let fail = true;
    const load = vi.fn((p: string) => p === 'outputs' && fail ? Promise.reject(new Error('boom')) : Promise.resolve(fs[p] ?? []));
    render(<Harness load={load} />);
    fireEvent.click(await screen.findByText('empty'));
    await screen.findByText('tree.emptyFolder');
    fireEvent.click(item('outputs'));
    await screen.findByText('boom');
    fail = false;
    fireEvent.click(screen.getByRole('button', { name: 'tree.retry' }));
    await screen.findByText('a.txt');
});

it('supports the keyboard: arrows, expand/collapse, parent, home/end, one tab stop', async () => {
    const onActivate = vi.fn();
    render(<Harness load={loader()} onActivate={onActivate} />);
    await screen.findByText('outputs');
    const rows = () => screen.getAllByRole('treeitem');
    expect(rows().filter((r) => r.tabIndex === 0)).toHaveLength(1);
    rows()[0].focus();
    const key = (k: string) => fireEvent.keyDown(document.activeElement!, { key: k });

    key('ArrowRight');
    await screen.findByText('a.txt');
    key('ArrowRight'); // already open: first child
    expect(document.activeElement).toBe(item('run'));
    key('ArrowDown');
    expect(document.activeElement).toBe(item('a.txt'));
    key('ArrowLeft'); // a file goes to its parent folder
    expect(document.activeElement).toBe(item('outputs'));
    key('ArrowLeft'); // an open folder collapses
    expect(item('outputs')).toHaveAttribute('aria-expanded', 'false');
    key('End');
    expect(document.activeElement).toBe(item('notes.md'));
    key('Home');
    expect(document.activeElement).toBe(item('outputs'));
    key('ArrowDown');
    expect(document.activeElement).toBe(item('empty'));
    key('ArrowUp');
    expect(document.activeElement).toBe(item('outputs'));
    key('Enter'); // toggles a folder
    expect(item('outputs')).toHaveAttribute('aria-expanded', 'true');
    key('End');
    key('Enter'); // activates a file
    expect(onActivate).toHaveBeenCalledWith(expect.objectContaining({ path: 'notes.md' }));
    // Only the focused row's actions are in the tab order.
    expect(screen.getByRole('button', { name: 'act notes.md' }).tabIndex).toBe(0);
    expect(screen.getByRole('button', { name: 'act outputs' }).tabIndex).toBe(-1);
});

it('reloads every loaded folder when refreshed, keeping the rows meanwhile', async () => {
    const load = loader();
    const { rerender } = render(<Harness load={load} initial={['outputs']} refreshKey={0} />);
    await screen.findByText('a.txt');
    fs.outputs = [dir('outputs/run'), file('outputs/a.txt'), file('outputs/b.txt')];
    rerender(<Harness load={load} initial={['outputs']} refreshKey={1} />);
    await screen.findByText('b.txt');
    expect(screen.getByText('a.txt')).toBeInTheDocument();
    fs.outputs = fs.outputs.slice(0, 2);
});

it('activates a file on click, and a folder only toggles', async () => {
    const onActivate = vi.fn();
    render(<Harness load={loader()} onActivate={onActivate} />);
    fireEvent.click(await screen.findByText('outputs'));
    await screen.findByText('a.txt');
    expect(onActivate).not.toHaveBeenCalled();
    fireEvent.click(item('a.txt'));
    expect(onActivate).toHaveBeenCalledWith(expect.objectContaining({ path: 'outputs/a.txt' }));
});
