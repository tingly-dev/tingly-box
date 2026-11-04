import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Library } from './Library';
import { previewKind, PREVIEW_LIMIT } from './LibraryPreview';

const m = vi.hoisted(() => ({ fetch: vi.fn(), notify: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
vi.mock('../../hooks/useNotify', () => ({ useNotify: () => m.notify }));
vi.mock('../../client', () => ({ apiFetch: m.fetch, api: { DELETE: vi.fn() }, unwrap: (p: Promise<unknown>) => p }));

const f = (path: string, size = 10) => ({ name: path, path, kind: 'file', size, updated_at: '2026-10-22T00:00:00Z', origin: 'upload' });
const root = [f('pic.png'), f('logo.svg'), f('huge.png', PREVIEW_LIMIT.image + 1), f('notes.md', 20), f('long.txt', PREVIEW_LIMIT.text + 1), f('data.zip'), f('broken.png')];
const json = (body: unknown) => new Response(JSON.stringify(body));
const fileCalls = () => m.fetch.mock.calls.map((c) => String(c[0])).filter((u) => u.includes('/library/file'));

let blobs: Blob[];
beforeEach(() => {
    blobs = [];
    URL.createObjectURL = vi.fn((b: Blob) => { blobs.push(b); return `blob:${blobs.length}`; });
    URL.revokeObjectURL = vi.fn();
    m.fetch.mockImplementation((url: string) => {
        const u = new URL(url, 'http://x');
        if (u.pathname.endsWith('/library/tree')) return Promise.resolve(json({ dir: '/lib', path: '', entries: root }));
        const path = u.searchParams.get('path');
        if (path === 'broken.png') return Promise.resolve(new Response('{"error":"file not found"}', { status: 404 }));
        if (path === 'notes.md') return Promise.resolve(new Response('# Title\n<script>x</script>'));
        return Promise.resolve(new Response('bytes'));
    });
});
afterEach(() => { cleanup(); localStorage.clear(); vi.resetAllMocks(); });

const open = async (name: string) => {
    await screen.findByText(name);
    fireEvent.click(screen.getByRole('treeitem', { name }));
};

it('picks what to preview by extension, case-insensitively', () => {
    for (const n of ['a.png', 'a.JPG', 'a.jpeg', 'a.gif', 'a.webp', 'a.svg']) expect(previewKind(n)).toBe('image');
    for (const n of ['a.md', 'a.txt', 'a.json', 'a.log', 'a.csv', 'a.go']) expect(previewKind(n)).toBe('text');
    for (const n of ['a.zip', 'a.pdf', 'noext', 'png', '.png.bak']) expect(previewKind(n)).toBeNull();
});

it('opens an image in a dialog from the same file endpoint, as an <img>', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('pic.png');
    const dialog = await screen.findByRole('dialog');
    const img = await within(dialog).findByRole('img', { name: 'pic.png' });
    expect(img.tagName).toBe('IMG');
    expect(img).toHaveAttribute('src', 'blob:1');
    expect(fileCalls()).toEqual(['/api/v1/projects/p1/library/file?path=pic.png']);
    expect(within(dialog).getByText('pic.png')).toBeInTheDocument();
});

it('shows an SVG through <img> with an image type, never as markup', async () => {
    m.fetch.mockImplementation((url: string) => String(url).includes('/library/tree')
        ? Promise.resolve(json({ dir: '/lib', path: '', entries: root }))
        : Promise.resolve(new Response('<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>')));
    render(<Library projectId="p1" refreshKey={0} />);
    await open('logo.svg');
    const dialog = await screen.findByRole('dialog');
    const img = await within(dialog).findByRole('img', { name: 'logo.svg' });
    expect(img.tagName).toBe('IMG');
    expect(dialog.querySelector('svg script, script')).toBeNull();
    expect(blobs[0].type).toBe('image/svg+xml');
});

it('shows a loading state, then closes with Escape and the close button, and frees the image', async () => {
    let release!: (r: Response) => void;
    m.fetch.mockImplementation((url: string) => String(url).includes('/library/tree')
        ? Promise.resolve(json({ dir: '/lib', path: '', entries: root }))
        : new Promise<Response>((r) => { release = r; }));
    render(<Library projectId="p1" refreshKey={0} />);
    await open('pic.png');
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByRole('progressbar')).toBeInTheDocument();
    release(new Response('bytes'));
    await within(dialog).findByRole('img', { name: 'pic.png' });

    fireEvent.keyDown(dialog, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:1');

    fireEvent.click(screen.getByRole('treeitem', { name: 'pic.png' }));
    fireEvent.click(await screen.findByRole('button', { name: 'common.close' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
});

it('opens from the keyboard with Enter, moves focus into the dialog and back to the row on close', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('pic.png');
    const row = screen.getByRole('treeitem', { name: 'pic.png' });
    row.focus();
    fireEvent.keyDown(row, { key: 'Enter' });
    const dialog = await screen.findByRole('dialog');
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
    fireEvent.keyDown(dialog, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(row));
});

it('downloads from the dialog', async () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    render(<Library projectId="p1" refreshKey={0} />);
    await open('pic.png');
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByRole('img', { name: 'pic.png' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'library.download' }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    click.mockRestore();
});

it('shows a text or markdown file as plain text, not rendered', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('notes.md');
    const dialog = await screen.findByRole('dialog');
    const text = await within(dialog).findByText(/# Title/);
    expect(text.tagName).toBe('PRE');
    expect(text.textContent).toBe('# Title\n<script>x</script>');
    expect(dialog.querySelector('script, h1')).toBeNull();
});

it('does not fetch a file over the size cap, and offers the download instead', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('huge.png');
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('library.tooLarge')).toBeInTheDocument();
    expect(within(dialog).getAllByRole('button', { name: 'library.download' })).toHaveLength(2);
    expect(fileCalls()).toEqual([]);

    fireEvent.keyDown(dialog, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await open('long.txt');
    expect(await screen.findByText('library.tooLarge')).toBeInTheDocument();
    expect(fileCalls()).toEqual([]);
});

it('reports a failed load in the dialog', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('broken.png');
    expect(await screen.findByText('file not found')).toBeInTheDocument();
});

it('keeps other types download-only', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('data.zip');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(fileCalls()).toEqual([]);
});

it('does not open a preview when the row actions are used', async () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    render(<Library projectId="p1" refreshKey={0} />);
    await screen.findByText('pic.png');
    fireEvent.click(within(screen.getByRole('treeitem', { name: 'pic.png' })).getByRole('button', { name: 'library.download' }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(screen.queryByRole('dialog')).toBeNull();
    click.mockRestore();
});
