import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { downloadName, Library } from './Library';

const m = vi.hoisted(() => ({ fetch: vi.fn(), del: vi.fn(), notify: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
vi.mock('../../hooks/useNotify', () => ({ useNotify: () => m.notify }));
vi.mock('../../client', () => ({
    apiFetch: m.fetch,
    api: { DELETE: m.del },
    unwrap: (p: Promise<unknown>) => p,
}));

const e = (path: string, kind: 'dir' | 'file', size = 0) => ({ name: path.split('/').pop(), path, kind, size, updated_at: '2026-10-22T00:00:00Z', origin: path.startsWith('outputs') ? 'output' : 'upload' });
const tree: Record<string, unknown[]> = {
    '': [e('outputs', 'dir'), e('notes.md', 'file', 2048)],
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
    const msg = 'this folder has more than 100 files (a zip download is limited to 100 files and 10 MB)';
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

it('saves an upload under the name it was given, without the stored id', () => {
    expect(downloadName('uploads/0123456789ab-report final.pdf')).toBe('report final.pdf');
    expect(downloadName('uploads/0123456789ab-a-b.txt')).toBe('a-b.txt');
    expect(downloadName('uploads/notes.md')).toBe('notes.md');
    expect(downloadName('outputs/0123456789ab-x.txt')).toBe('0123456789ab-x.txt');
    expect(downloadName('notes.md')).toBe('notes.md');
});
