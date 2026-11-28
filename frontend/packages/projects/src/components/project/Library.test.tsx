import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Library } from './Library';
import { MAX_UPLOAD_BYTES } from '../../utils/uploadLimit';

const m = vi.hoisted(() => ({ fetch: vi.fn(), del: vi.fn(), notify: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
vi.mock('../../hooks/useNotify', () => ({ useNotify: () => m.notify }));
vi.mock('../../client', () => ({
    apiFetch: m.fetch,
    api: { DELETE: m.del },
    unwrap: (p: Promise<unknown>) => p,
}));

const e = (path: string, kind: 'dir' | 'file', size = 0) => ({ name: path.split('/').pop(), path, kind, size, updated_at: '2026-10-22T00:00:00Z', origin: path.startsWith('outputs') ? 'output' : path.startsWith('.tmp') ? 'scratch' : 'upload' });
const tree: Record<string, unknown[]> = {
    '': [e('.tmp', 'dir'), e('outputs', 'dir'), e('notes.md', 'file', 2048)],
    '.tmp': [{ name: 'Fix the thing', path: '.tmp/t1', kind: 'dir', size: 0, updated_at: '2026-10-22T00:00:00Z', origin: 'scratch' }],
    '.tmp/t1': [e('.tmp/t1/scratch.md', 'file', 3)],
    outputs: [e('outputs/run', 'dir'), e('outputs/a.txt', 'file', 10)],
    'outputs/run': [e('outputs/run/report.md', 'file', 5)],
};
const json = (body: unknown) => new Response(JSON.stringify(body));
const requestedPaths = () => m.fetch.mock.calls.map((c) => new URL(String(c[0]), 'http://x').searchParams.get('path') ?? '').filter((_, i) => String(m.fetch.mock.calls[i][0]).includes('/library/tree'));

beforeEach(() => {
    m.fetch.mockImplementation((url: string) => {
        const u = new URL(url, 'http://x');
        if (u.pathname.endsWith('/library/tree')) {
            const path = u.searchParams.get('path') ?? '';
            return Promise.resolve(path in tree ? json({ dir: '/lib', path, entries: tree[path] }) : new Response('{"error":"folder not found"}', { status: 404 }));
        }
        return Promise.resolve(new Response('data'));
    });
});
afterEach(() => { cleanup(); localStorage.clear(); vi.resetAllMocks(); });

it('shows the library as a tree that opens folders on demand and remembers them per project', async () => {
    const view = render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('notes.md');
    expect(requestedPaths()).toEqual(['']);
    expect(screen.queryByText('a.txt')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('treeitem', { name: 'outputs' }));
    await screen.findByText('a.txt');
    fireEvent.click(screen.getByRole('treeitem', { name: 'run' }));
    await screen.findByText('report.md');
    expect(requestedPaths()).toEqual(['', 'outputs', 'outputs/run']);
    expect(JSON.parse(localStorage.getItem('tingly-project.library.expanded.p1')!)).toEqual(['outputs', 'outputs/run']);

    // Another visit opens the same folders; another project starts closed.
    view.unmount();
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('report.md');
    cleanup();
    render(<Library projectId="p2" refreshKey={0} />);
    await screen.findByText('notes.md');
    expect(screen.queryByText('a.txt')).not.toBeInTheDocument();
});

it('keeps downloading and deleting a file, and offers a folder only the zip download', async () => {
    const click = vi.fn();
    const URLspy = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:x');
    const anchor = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(click);
    m.del.mockResolvedValue({});
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('notes.md');
    const folder = within(screen.getByRole('treeitem', { name: 'outputs' }));
    expect(folder.getAllByRole('button').map((b) => b.getAttribute('aria-label'))).toEqual(['library.downloadZip']);

    fireEvent.click(within(screen.getByRole('treeitem', { name: 'notes.md' })).getByRole('button', { name: 'library.download' }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(String(m.fetch.mock.calls.at(-1)![0])).toContain('/library/file?path=notes.md');

    fireEvent.click(within(screen.getByRole('treeitem', { name: 'notes.md' })).getByRole('button', { name: 'common.delete' }));
    await waitFor(() => expect(m.del).toHaveBeenCalledWith('/api/v1/projects/{id}/library/file', { params: { path: { id: 'p1' }, query: { path: 'notes.md' } } }));
    // The tree is reloaded after a delete.
    await waitFor(() => expect(requestedPaths().length).toBeGreaterThan(1));
    URLspy.mockRestore();
    anchor.mockRestore();
});

it('says the library is empty, and shows the folder path with a copy button', async () => {
    m.fetch.mockResolvedValue(json({ dir: '/lib/p', path: '', entries: [] }));
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('library.empty');
    expect(screen.getByText('/lib/p')).toBeInTheDocument();
});

it('downloads a folder as a zip named after it', async () => {
    const click = vi.fn();
    let download = '';
    const URLspy = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:x');
    const anchor = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { download = this.download; click(); });
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('notes.md');
    fireEvent.click(screen.getByRole('treeitem', { name: 'outputs' }));
    await screen.findByText('run');

    fireEvent.click(within(screen.getByRole('treeitem', { name: 'run' })).getByRole('button', { name: 'library.downloadZip' }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(String(m.fetch.mock.calls.at(-1)![0])).toBe('/api/v1/projects/p1/library/archive?path=outputs%2Frun');
    expect(download).toBe('run.zip');
    expect(m.notify).not.toHaveBeenCalled();
    URLspy.mockRestore();
    anchor.mockRestore();
});

it("shows the server's message as a toast when the folder is over the limit, and downloads nothing", async () => {
    const click = vi.fn();
    const anchor = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(click);
    const msg = 'this folder has more than 200 files (a zip download is limited to 200 files and 40 MB)';
    m.fetch.mockImplementation((url: string) => {
        const u = new URL(url, 'http://x');
        if (u.pathname.endsWith('/library/archive')) return Promise.resolve(new Response(JSON.stringify({ error: msg }), { status: 413 }));
        return Promise.resolve(json({ dir: '/lib', path: '', entries: tree[u.searchParams.get('path') ?? ''] ?? [] }));
    });
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('notes.md');
    fireEvent.click(within(screen.getByRole('treeitem', { name: 'outputs' })).getByRole('button', { name: 'library.downloadZip' }));
    await waitFor(() => expect(m.notify).toHaveBeenCalledWith(msg, 'error'));
    expect(click).not.toHaveBeenCalled();
    anchor.mockRestore();
});

it('lists the threads scratch under .tmp, labelled as scratch', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('.tmp');
    // The .tmp folder row itself says what it is.
    expect(screen.getByText('library.scratch')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('treeitem', { name: '.tmp' }));
    // The server already names the folder after the thread; it opens like any other.
    await screen.findByText('Fix the thing');
    fireEvent.click(screen.getByRole('treeitem', { name: 'Fix the thing' }));
    await screen.findByText('scratch.md');
    expect(requestedPaths()).toEqual(['', '.tmp', '.tmp/t1']);
});

const rowNames = () => screen.getAllByRole('treeitem').map((r) => r.getAttribute('aria-label'));
const treeCalls = () => m.fetch.mock.calls.filter((c) => String(c[0]).includes('/library/tree'));

it('orders files in the browser, asks for no sort parameter, and remembers the choice', async () => {
    const file = (name: string, at: string) => ({ name, path: name, kind: 'file' as const, size: 1, updated_at: at, origin: 'upload' });
    m.fetch.mockImplementation((url: string) => {
        const u = new URL(url, 'http://x');
        if (u.pathname.endsWith('/library/tree')) return Promise.resolve(json({
            dir: '/lib', path: '', entries: [
                file('b.txt', '2026-01-01T00:00:03Z'),
                file('a.txt', '2026-01-01T00:00:01Z'),
                file('c.txt', '2026-01-01T00:00:02Z'),
            ],
        }));
        return Promise.resolve(new Response('data'));
    });
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('a.txt');
    expect(rowNames()).toEqual(['b.txt', 'c.txt', 'a.txt']);
    const before = treeCalls().length;

    fireEvent.mouseDown(screen.getByRole('combobox'));
    fireEvent.click(await screen.findByRole('option', { name: 'library.sortNameAsc' }));
    await waitFor(() => expect(rowNames()).toEqual(['a.txt', 'b.txt', 'c.txt']));
    // Sorting is local: nothing is fetched again.
    expect(treeCalls().length).toBe(before);
    expect(localStorage.getItem('tingly-project.library.sort')).toBe('name-asc');
    // The tree endpoint is asked for its level only, never with sort/order.
    expect(treeCalls().every((c) => {
        const u = new URL(String(c[0]), 'http://x');
        return !u.searchParams.has('sort') && !u.searchParams.has('order');
    })).toBe(true);
});

it('shows an upload without its stored id and downloads it under the readable name', async () => {
    const click = vi.fn();
    let download = '';
    const URLspy = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:x');
    const anchor = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { download = this.download; click(); });
    // The server lists the stored basename; the browser derives the readable name.
    const entry = { name: '0123456789ab-report final.md', path: 'uploads/0123456789ab-report final.md', kind: 'file' as const, size: 12, updated_at: '2026-10-22T00:00:00Z', origin: 'upload' };
    m.fetch.mockImplementation((url: string) => {
        const u = new URL(url, 'http://x');
        if (u.pathname.endsWith('/library/tree')) return Promise.resolve(json({ dir: '/lib', path: '', entries: [entry] }));
        return Promise.resolve(new Response('data'));
    });
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('report final.md');
    expect(screen.queryByText('0123456789ab-report final.md')).not.toBeInTheDocument();
    fireEvent.click(within(screen.getByRole('treeitem', { name: 'report final.md' })).getByRole('button', { name: 'library.download' }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(download).toBe('report final.md');
    URLspy.mockRestore();
    anchor.mockRestore();
});

it('filters by name (case-insensitive, CJK) and shows a no-results state', async () => {
    m.fetch.mockImplementation((url: string) => {
        const u = new URL(url, 'http://x');
        if (u.pathname.endsWith('/library/tree')) return Promise.resolve(json({
            dir: '/lib', path: '', entries: [e('Report.txt', 'file'), e('季度报告.md', 'file'), e('notes.md', 'file'), e('outputs', 'dir')],
        }));
        return Promise.resolve(new Response('data'));
    });
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('notes.md');
    const search = screen.getByPlaceholderText('library.search');

    fireEvent.change(search, { target: { value: 'REPORT' } });
    await waitFor(() => expect(rowNames()).toEqual(['Report.txt']));
    fireEvent.change(search, { target: { value: '报告' } });
    await waitFor(() => expect(rowNames()).toEqual(['季度报告.md']));
    fireEvent.change(search, { target: { value: 'nothing-matches' } });
    await screen.findByText('library.noResults');
    fireEvent.change(search, { target: { value: '' } });
    await screen.findByText('notes.md');
});

it('keeps expanded folders while filtering and reveals their matches', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('notes.md');
    fireEvent.click(screen.getByRole('treeitem', { name: 'outputs' }));
    await screen.findByText('a.txt');

    fireEvent.change(screen.getByPlaceholderText('library.search'), { target: { value: 'a.txt' } });
    await waitFor(() => expect(rowNames()).toEqual(['outputs', 'a.txt']));

    fireEvent.change(screen.getByPlaceholderText('library.search'), { target: { value: '' } });
    await screen.findByText('notes.md');
    // The folder is still open, so its files come back with it.
    expect(screen.getByText('a.txt')).toBeInTheDocument();
});

// Only name/size are read, so a real 200 MB File is not needed.
const tooBig = (name: string, over: number) => ({ name, size: MAX_UPLOAD_BYTES + over, type: 'application/octet-stream' }) as unknown as File;
const uploadCalls = () => m.fetch.mock.calls.filter((c) => (c[1] as RequestInit | undefined)?.method === 'POST');

it('refuses an oversized file before uploading it, naming the limit', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('notes.md');
    fireEvent.change(screen.getByTestId('library-file-input'), { target: { files: [tooBig('huge.bin', 1)] } });

    await waitFor(() => expect(m.notify).toHaveBeenCalled());
    expect(m.notify.mock.calls[0][1]).toBe('error');
    expect(uploadCalls()).toEqual([]);
});

it('uploads the acceptable files and drops only the oversized one', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('notes.md');
    const ok = new File(['abc'], 'ok.txt', { type: 'text/plain' });
    fireEvent.change(screen.getByTestId('library-file-input'), { target: { files: [ok, tooBig('huge.bin', 1)] } });

    await waitFor(() => expect(uploadCalls()).toHaveLength(1));
    const body = uploadCalls()[0][1]!.body as FormData;
    expect(body.getAll('files').map((f) => (f as File).name)).toEqual(['ok.txt']);
    expect(m.notify).toHaveBeenCalled();
});
