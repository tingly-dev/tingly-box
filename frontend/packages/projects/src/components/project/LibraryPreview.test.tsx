import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Library } from './Library';
import { previewKind, sniffImage, PREVIEW_LIMIT } from './LibraryPreview';

const m = vi.hoisted(() => ({ fetch: vi.fn(), notify: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
vi.mock('../../hooks/useNotify', () => ({ useNotify: () => m.notify }));
vi.mock('../../client', () => ({ apiFetch: m.fetch, api: { DELETE: vi.fn() }, unwrap: (p: Promise<unknown>) => p }));

const f = (path: string, size = 10) => ({ name: path, path, kind: 'file', size, updated_at: '2026-10-22T00:00:00Z', origin: 'upload' });
const root = [f('pic.png'), f('logo.svg'), f('huge.png', PREVIEW_LIMIT.image + 1), f('notes.md', 20), f('long.txt', PREVIEW_LIMIT.text + 1), f('data.zip'), f('broken.png'), f('fake.png'), f('binary.txt'), f('page.html'), f('notes.markdown')];
const PNG = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0]);
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
        if (path === 'notes.md') return Promise.resolve(new Response('# Title\n<script>x</script>\n[link](https://x.test)'));
        if (path === 'fake.png') return Promise.resolve(new Response('<html><script>alert(1)</script></html>', { headers: { 'Content-Type': 'text/html' } }));
        if (path === 'binary.txt') return Promise.resolve(new Response(new Uint8Array([0x61, 0, 0x62])));
        return Promise.resolve(new Response(PNG));
    });
});
afterEach(() => { cleanup(); localStorage.clear(); vi.resetAllMocks(); });

const open = async (name: string) => {
    await screen.findByText(name);
    fireEvent.click(screen.getByRole('treeitem', { name }));
};

it('previews only an allowlist of raster images and plain-text formats, by extension, case-insensitively', () => {
    for (const n of ['a.png', 'a.JPG', 'a.jpeg', 'a.gif', 'a.webp']) expect(previewKind(n)).toBe('image');
    for (const n of ['a.md', 'a.MARKDOWN', 'a.txt', 'a.json', 'a.log', 'a.csv', 'a.yaml', 'a.yml', 'a.toml', 'a.ini']) expect(previewKind(n)).toBe('text');
    // SVG and markup carry script or external references; the rest are not known to be safe.
    for (const n of ['a.svg', 'a.svgz', 'a.html', 'a.htm', 'a.xml', 'a.xhtml', 'a.js', 'a.css', 'a.sh', 'a.pdf', 'a.zip', 'a.png.exe', 'noext', 'png', '.png']) expect(previewKind(n)).toBeNull();
});

it('accepts an image only when its first bytes are the format its extension claims', () => {
    const png = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
    const jpg = new Uint8Array([0xff, 0xd8, 0xff, 0xe0]);
    const gif = new Uint8Array([...'GIF89a'].map((c) => c.charCodeAt(0)));
    const webp = new Uint8Array([...'RIFF\0\0\0\0WEBP'].map((c) => c.charCodeAt(0)));
    const html = new Uint8Array([...'<svg onload=alert(1)>'].map((c) => c.charCodeAt(0)));
    expect([sniffImage(png, 'png'), sniffImage(jpg, 'jpg'), sniffImage(jpg, 'jpeg'), sniffImage(gif, 'gif'), sniffImage(webp, 'webp')]).toEqual([true, true, true, true, true]);
    expect([sniffImage(png, 'jpg'), sniffImage(jpg, 'png'), sniffImage(html, 'png'), sniffImage(html, 'gif'), sniffImage(png, 'svg'), sniffImage(new Uint8Array(), 'png')]).toEqual([false, false, false, false, false, false]);
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

it('keeps SVG download-only: no dialog, no request', async () => {
    m.fetch.mockImplementation((url: string) => String(url).includes('/library/tree')
        ? Promise.resolve(json({ dir: '/lib', path: '', entries: [f('logo.svg'), f('page.html')] }))
        : Promise.resolve(new Response('<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>')));
    render(<Library projectId="p1" refreshKey={0} />);
    await open('logo.svg');
    await open('page.html');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(fileCalls()).toEqual([]);
});

it('hands the browser a blob typed by the extension, not by the server', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('pic.png');
    await screen.findByRole('img', { name: 'pic.png' });
    expect(blobs[0].type).toBe('image/png');
});

it('refuses a file that only has an image name, such as HTML named .png', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('fake.png');
    const dialog = await screen.findByRole('dialog');
    expect(await within(dialog).findByText('library.notPreviewable')).toBeInTheDocument();
    expect(within(dialog).queryByRole('img')).toBeNull();
    expect(URL.createObjectURL).not.toHaveBeenCalled();
    expect(dialog.querySelector('script')).toBeNull();
});

it('drops the image if the browser cannot decode it', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('pic.png');
    fireEvent.error(await screen.findByRole('img', { name: 'pic.png' }));
    expect(await screen.findByText('library.notPreviewable')).toBeInTheDocument();
    expect(screen.queryByRole('img')).toBeNull();
});

it('refuses a text file that is really binary', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('binary.txt');
    expect(await screen.findByText('library.notPreviewable')).toBeInTheDocument();
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
    release(new Response(PNG));
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
    expect(text.textContent).toBe('# Title\n<script>x</script>\n[link](https://x.test)');
    expect(dialog.querySelector('script, h1, a')).toBeNull();
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
