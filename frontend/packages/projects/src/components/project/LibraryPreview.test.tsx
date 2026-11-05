import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Library } from './Library';
import { Markdown } from '../Markdown';
import { previewKind, sniffImage, PREVIEW_LIMIT } from './LibraryPreview';

const m = vi.hoisted(() => ({ fetch: vi.fn(), notify: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
vi.mock('../../hooks/useNotify', () => ({ useNotify: () => m.notify }));
vi.mock('../../client', () => ({ apiFetch: m.fetch, api: { DELETE: vi.fn() }, unwrap: (p: Promise<unknown>) => p }));

const f = (path: string, size = 10) => ({ name: path, path, kind: 'file', size, updated_at: '2026-10-22T00:00:00Z', origin: 'upload' });
const dir = (path: string) => ({ name: path, path, kind: 'dir', size: 0, updated_at: '2026-10-22T00:00:00Z', origin: 'upload' });
const g = (folder: string, name: string, size = 10) => ({ name, path: `${folder}/${name}`, kind: 'file', size, updated_at: '2026-10-22T00:00:00Z', origin: 'upload' });
// A folder with images mixed with other files, in the order the server lists them.
const pics = [g('pics', 'a.png'), g('pics', 'b.jpg'), g('pics', 'notes.md', 20), g('pics', 'c.gif'), g('pics', 'd.svg'), g('pics', 'e.webp')];
const solo = [g('solo', 'only.png'), g('solo', 'readme.txt')];
const root = [f('pic.png'), f('logo.svg'), f('huge.png', PREVIEW_LIMIT.image + 1), f('notes.md', 20), f('long.txt', PREVIEW_LIMIT.text + 1), f('data.zip'), f('broken.png'), f('fake.png'), f('binary.txt'), f('page.html'), f('notes.markdown'), f('huge.md', PREVIEW_LIMIT.text + 1), dir('pics'), dir('solo')];
const PNG = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0]);
const json = (body: unknown) => new Response(JSON.stringify(body));
const fileCalls = () => m.fetch.mock.calls.map((c) => String(c[0])).filter((u) => u.includes('/library/file'));

let blobs: Blob[];
let contents: Record<string, string>;
beforeEach(() => {
    blobs = [];
    URL.createObjectURL = vi.fn((b: Blob) => { blobs.push(b); return `blob:${blobs.length}`; });
    URL.revokeObjectURL = vi.fn();
    contents = {};
    m.fetch.mockImplementation((url: string) => {
        const u = new URL(url, 'http://x');
        const path = u.searchParams.get('path') ?? '';
        if (u.pathname.endsWith('/library/tree')) return Promise.resolve(json({ dir: '/lib', path, entries: path === 'pics' ? pics : path === 'solo' ? solo : root }));
        if (path in contents) return Promise.resolve(new Response(contents[path]));
        if (path === 'broken.png') return Promise.resolve(new Response('{"error":"file not found"}', { status: 404 }));
        if (path === 'notes.md') return Promise.resolve(new Response('# Title\n<script>x</script>\n[link](https://x.test)'));
        if (path === 'fake.png') return Promise.resolve(new Response('<html><script>alert(1)</script></html>', { headers: { 'Content-Type': 'text/html' } }));
        if (path === 'binary.txt') return Promise.resolve(new Response(new Uint8Array([0x61, 0, 0x62])));
        if (path.endsWith('.jpg')) return Promise.resolve(new Response(new Uint8Array([0xff, 0xd8, 0xff, 0xe0, 0, 0])));
        if (path.endsWith('.gif')) return Promise.resolve(new Response(new Uint8Array([...'GIF89a'].map((c) => c.charCodeAt(0)))));
        if (path.endsWith('.webp')) return Promise.resolve(new Response(new Uint8Array([...'RIFF\0\0\0\0WEBP'].map((c) => c.charCodeAt(0)))));
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

it('shows a plain-text type as text, not rendered', async () => {
    contents['readme.txt'] = '# Title\n<script>x</script>\n[link](https://x.test)';
    m.fetch.mockImplementationOnce(() => Promise.resolve(json({ dir: '/lib', path: '', entries: [f('readme.txt')] })));
    render(<Library projectId="p1" refreshKey={0} />);
    await open('readme.txt');
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

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await open('huge.md');
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


const preview = async (name: string, markdown: string) => {
    contents[name] = markdown;
    m.fetch.mockImplementationOnce(() => Promise.resolve(json({ dir: '/lib', path: '', entries: [f(name)] })));
    render(<Library projectId="p1" refreshKey={0} />);
    await open(name);
    return screen.findByRole('dialog');
};

it.each(['md', 'markdown'])('renders .%s as Markdown, with the conversation look', async (ext) => {
    const dialog = await preview(`doc.${ext}`, '# Title\n\nSome **bold** text with `code`.\n\n- one\n- two\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n~~gone~~');
    const heading = await within(dialog).findByRole('heading', { name: 'Title' });
    expect(within(dialog).getByText('bold').tagName).toBe('STRONG');
    expect(within(dialog).getAllByRole('listitem')).toHaveLength(2);
    expect(within(dialog).getByRole('table')).toBeInTheDocument();
    expect(dialog.querySelector('del')).toHaveTextContent('gone');
    expect(dialog.querySelector('pre')).toBeNull();
    // Same typography as a reply in the project conversation.
    const reply = render(<Markdown conversation text="# Title" />);
    expect(getComputedStyle(heading).fontSize).toBe(getComputedStyle(within(reply.container).getByRole('heading')).fontSize);
    expect(getComputedStyle(heading).fontSize).toBe('20px');
});

it('highlights fenced code in Markdown with the same copyable block as the conversation', async () => {
    const dialog = await preview('doc.md', '```js\nconst value = 1;\n```');
    expect(await within(dialog).findByRole('button', { name: 'common.copy' })).toBeInTheDocument();
    expect(within(dialog).getByText('js')).toBeInTheDocument();
});

it('never renders raw HTML in Markdown: it is shown as text or dropped', async () => {
    const html = [
        '<script>alert(1)</script>',
        '<img src="https://evil.test/p.gif" onerror="alert(2)">',
        '<iframe src="https://evil.test"></iframe>',
        '<a href="javascript:alert(3)" onclick="alert(4)">raw link</a>',
        '<svg onload="alert(5)"></svg>',
        '<style>body{display:none}</style>',
        '<form action="https://evil.test"><input name="x"></form>',
        '<div style="position:fixed" onclick="alert(6)">div</div>',
        '<details open ontoggle="alert(7)"><summary>s</summary></details>',
        '',
        'after',
    ].join('\n');
    const dialog = await preview('doc.md', html);
    await within(dialog).findByText(/after/);
    expect(dialog.querySelector('script, img, iframe, svg[onload], style, form, input, details, div[style], [onclick], [onerror], [onload], [ontoggle]')).toBeNull();
    expect(within(dialog).queryByRole('link')).toBeNull();
});

it.each([
    ['javascript:', '[x](javascript:alert(1))'],
    ['mixed-case javascript:', '[x](JaVaScRiPt:alert(1))'],
    ['whitespace-obfuscated javascript:', '[x](<java\tscript:alert(1)>)'],
    ['data:', '[x](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)'],
    ['vbscript:', '[x](vbscript:msgbox(1))'],
    ['file:', '[x](file:///etc/passwd)'],
    ['blob:', '[x](blob:https://x.test/1)'],
    ['relative path', '[x](../secret.md)'],
    ['protocol-relative', '[x](//evil.test/a)'],
    ['in-page anchor', '[x](#top)'],
    ['reference definition', '[x][1]\n\n[1]: javascript:alert(1)'],
    ['autolink', '<javascript:alert(1)>'],
])('drops the target of a %s link and keeps the text', async (_name, md) => {
    const dialog = await preview('doc.md', `${md}\n\nafter`);
    await within(dialog).findByText('after');
    expect(dialog.querySelector('[href]')).toBeNull();
    expect(dialog.querySelector('a')).toBeNull();
});

it('opens only http, https and mailto links, in a new tab without opener or referrer', async () => {
    const dialog = await preview('doc.md', '[web](https://example.com/a?b=1) [plain](http://example.com) [mail](mailto:me@example.com) https://auto.example.com');
    const links = await within(dialog).findAllByRole('link');
    expect(links.map((a) => a.getAttribute('href'))).toEqual(['https://example.com/a?b=1', 'http://example.com', 'mailto:me@example.com', 'https://auto.example.com']);
    for (const a of links) {
        expect(a).toHaveAttribute('target', '_blank');
        expect(a.getAttribute('rel')!.split(' ').sort()).toEqual(['noopener', 'noreferrer']);
    }
});

it('does not load remote images: they become a link or a placeholder', async () => {
    const dialog = await preview('doc.md', [
        '![remote pic](https://evil.test/track.png)',
        '![inline](data:image/png;base64,iVBORw0KGgo=)',
        '![local](other.png)',
        '[![badge](https://evil.test/badge.svg)](https://example.com)',
        '![ref][r]',
        '',
        '[r]: http://evil.test/x.gif',
    ].join('\n\n'));
    await within(dialog).findByText(/remote pic/);
    expect(dialog.querySelector('img, picture, source, video, audio, object, embed')).toBeNull();
    // The alt text stands in for the image; the address is only a tooltip, never a link or a request.
    expect(within(dialog).getByText('remote pic')).toHaveAttribute('title', 'https://evil.test/track.png');
    expect(within(dialog).getByText('inline')).not.toHaveAttribute('title');
    expect(within(dialog).getByText('local')).not.toHaveAttribute('title');
    expect(within(dialog).queryByRole('link', { name: /remote pic|inline|local/ })).toBeNull();
    // Nothing but the file itself was requested.
    expect(m.fetch.mock.calls.map((c) => String(c[0])).filter((u) => !u.includes('/library/'))).toEqual([]);
    expect(blobs).toEqual([]);
});

it('does not turn a nested badge link into an image', async () => {
    const dialog = await preview('doc.md', '[![badge](https://evil.test/badge.svg)](https://example.com)');
    await within(dialog).findByText(/badge/);
    expect(dialog.querySelector('img')).toBeNull();
    expect(within(dialog).getAllByRole('link').map((a) => a.getAttribute('href'))).toEqual(['https://example.com']);
    expect(dialog.querySelector('a a')).toBeNull();
});

it('renders a large Markdown file that is under the cap in full', async () => {
    const dialog = await preview('doc.md', `${'word '.repeat(20000)}\n\n# End`);
    expect(await within(dialog).findByRole('heading', { name: 'End' })).toBeInTheDocument();
});

it('keeps the conversation renderer unchanged for links and images', () => {
    const view = render(<Markdown conversation text="[Docs](https://example.com)" />);
    expect(view.getByRole('link', { name: 'Docs' })).toHaveAttribute('rel', 'noreferrer');
});

// Image navigation.

const openPics = async (name: string) => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('pics');
    await open(name);
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByRole('img', { name });
    return dialog;
};
const key = (dialog: HTMLElement, k: string, init: KeyboardEventInit = {}) => fireEvent.keyDown(dialog, { key: k, ...init });
const shown = async (dialog: HTMLElement, name: string) => expect(await within(dialog).findByRole('img', { name })).toBeInTheDocument();
const position = (dialog: HTMLElement) => within(dialog).getByText(/^\d+ \/ \d+$/).textContent;

it('switches between the images of the same folder with Left/Right and Up/Down, skipping other files', async () => {
    const dialog = await openPics('a.png');
    expect(position(dialog)).toBe('1 / 4');

    key(dialog, 'ArrowRight');
    await shown(dialog, 'b.jpg');
    expect(position(dialog)).toBe('2 / 4');
    key(dialog, 'ArrowDown');
    await shown(dialog, 'c.gif'); // notes.md is not an image
    key(dialog, 'ArrowDown');
    await shown(dialog, 'e.webp'); // d.svg is not previewable
    expect(position(dialog)).toBe('4 / 4');
    key(dialog, 'ArrowUp');
    await shown(dialog, 'c.gif');
    key(dialog, 'ArrowLeft');
    await shown(dialog, 'b.jpg');
    expect(fileCalls().map((u) => decodeURIComponent(u.split('path=')[1]))).toEqual(['pics/a.png', 'pics/b.jpg', 'pics/c.gif', 'pics/e.webp', 'pics/c.gif', 'pics/b.jpg']);
});

it('stops at the first and last image instead of wrapping', async () => {
    const dialog = await openPics('a.png');
    key(dialog, 'ArrowLeft');
    key(dialog, 'ArrowUp');
    expect(position(dialog)).toBe('1 / 4');
    expect(within(dialog).getByRole('button', { name: 'library.previousImage' })).toBeDisabled();
    expect(within(dialog).getByRole('button', { name: 'library.nextImage' })).toBeEnabled();
    for (const name of ['b.jpg', 'c.gif', 'e.webp']) { key(dialog, 'ArrowRight'); await shown(dialog, name); }
    key(dialog, 'ArrowRight');
    key(dialog, 'ArrowDown');
    expect(position(dialog)).toBe('4 / 4');
    expect(within(dialog).getByRole('img', { name: 'e.webp' })).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'library.nextImage' })).toBeDisabled();
    expect(fileCalls()).toHaveLength(4);
});

it('has previous and next buttons and an n / m indicator, and keeps focus in the dialog', async () => {
    const dialog = await openPics('b.jpg');
    expect(position(dialog)).toBe('2 / 4');
    expect(within(dialog).getByText('2 / 4')).toHaveAttribute('aria-live', 'polite');
    fireEvent.click(within(dialog).getByRole('button', { name: 'library.nextImage' }));
    await shown(dialog, 'c.gif');
    fireEvent.click(within(dialog).getByRole('button', { name: 'library.nextImage' }));
    await shown(dialog, 'e.webp');
    // The button that was just used is now disabled; focus must not fall out of the dialog.
    await waitFor(() => expect(dialog.contains(document.activeElement) && !(document.activeElement as HTMLButtonElement).disabled).toBe(true));
    key(document.activeElement as HTMLElement, 'ArrowLeft');
    await shown(dialog, 'c.gif');
    fireEvent.click(within(dialog).getByRole('button', { name: 'library.previousImage' }));
    await shown(dialog, 'b.jpg');
});

it('only offers navigation among images of the opened file\'s own folder', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('solo');
    await open('only.png');
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByRole('img', { name: 'only.png' });
    expect(within(dialog).queryByRole('button', { name: 'library.nextImage' })).toBeNull();
    expect(within(dialog).queryByText(/^\d+ \/ \d+$/)).toBeNull();
    key(dialog, 'ArrowRight');
    expect(fileCalls()).toHaveLength(1);

    // The root's pic.png does not count as a neighbour of anything in a subfolder, and vice versa.
    key(dialog, 'Escape');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await open('pic.png');
    const rootDialog = await screen.findByRole('dialog');
    await within(rootDialog).findByRole('img', { name: 'pic.png' });
    expect(position(rootDialog)).toMatch(/^1 \/ \d+$/);
    key(rootDialog, 'ArrowRight');
    await waitFor(() => expect(within(rootDialog).queryByRole('img', { name: 'pic.png' })).toBeNull());
    expect(fileCalls().some((u) => u.includes('only.png') && u.includes('pic.png'))).toBe(false);
});

it('ignores arrow keys with a modifier and on non-image previews, where they scroll', async () => {
    const dialog = await openPics('a.png');
    key(dialog, 'ArrowRight', { altKey: true }); // browser history
    key(dialog, 'ArrowRight', { metaKey: true });
    key(dialog, 'ArrowRight', { ctrlKey: true });
    key(dialog, 'ArrowRight', { shiftKey: true });
    expect(position(dialog)).toBe('1 / 4');
    key(dialog, 'ArrowRight');
    await shown(dialog, 'b.jpg');
    key(dialog, 'Escape');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());

    contents['pics/notes.md'] = '# Title';
    fireEvent.click(screen.getAllByRole('treeitem', { name: 'notes.md' }).at(-1)!);
    const md = await screen.findByRole('dialog');
    await within(md).findByRole('heading', { name: 'Title' });
    const calls = fileCalls().length;
    key(md, 'ArrowDown');
    key(md, 'ArrowRight');
    expect(fileCalls()).toHaveLength(calls);
    expect(within(md).queryByRole('button', { name: 'library.nextImage' })).toBeNull();
});

it('shows only the newest image when switching quickly, and frees the ones it leaves', async () => {
    const dialog = await openPics('a.png');
    key(dialog, 'ArrowRight');
    key(dialog, 'ArrowRight');
    await shown(dialog, 'c.gif');
    expect(within(dialog).getAllByRole('img')).toHaveLength(1);
    expect(position(dialog)).toBe('3 / 4');
    await waitFor(() => expect((URL.revokeObjectURL as ReturnType<typeof vi.fn>).mock.calls.length).toBe(blobs.length - 1));
});

it('closes with Escape from an image reached by navigation, returning focus to the row that opened it', async () => {
    render(<Library projectId="p1" refreshKey={0} />);
    await open('pics');
    await screen.findByText('a.png');
    screen.getByRole('treeitem', { name: 'a.png' }).focus();
    fireEvent.keyDown(screen.getByRole('treeitem', { name: 'a.png' }), { key: 'Enter' });
    const dialog = await screen.findByRole('dialog');
    await shown(dialog, 'a.png');
    key(dialog, 'ArrowRight');
    await shown(dialog, 'b.jpg');
    key(dialog, 'Escape');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await waitFor(() => expect(screen.getByRole('treeitem', { name: 'a.png' })).toHaveFocus());
});

it('navigates on the images of the folder as listed after a refresh', async () => {
    const dialog = await openPics('e.webp');
    expect(position(dialog)).toBe('4 / 4');
    key(dialog, 'ArrowLeft');
    await shown(dialog, 'c.gif');
});
