import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AttachmentChips } from './attachments';
import { PREVIEW_LIMIT } from './project/LibraryPreview';

const m = vi.hoisted(() => ({ fetch: vi.fn(), notify: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../client', () => ({ apiFetch: m.fetch }));
vi.mock('../hooks/useNotify', () => ({ useNotify: () => m.notify }));

const PNG = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0]);
const file = (name: string, type: string, size = 12): { name: string; path: string; type: string; size: number } => ({ name, path: name, type, size });

let blobs: Blob[];
beforeEach(() => {
    blobs = [];
    URL.createObjectURL = vi.fn((b: Blob) => { blobs.push(b); return `blob:${blobs.length}`; });
    URL.revokeObjectURL = vi.fn();
    // A fresh Response per call: a body can be read once.
    m.fetch.mockImplementation(() => Promise.resolve(new Response(PNG)));
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const calls = () => m.fetch.mock.calls.map((c) => String(c[0]));

describe('conversation attachments', () => {
    it('opens an image in a dialog and closes it without opening a page', async () => {
        const open = vi.spyOn(window, 'open');
        render(<AttachmentChips owner={{ threadId: 'thread' }} items={[file('diagram.png', 'image/png', 5)]} />);
        const button = await screen.findByRole('button', { name: 'diagram.png' });
        await waitFor(() => expect(within(button).getByRole('img')).toHaveAttribute('src', 'blob:1'));
        fireEvent.click(button);
        const dialog = screen.getByRole('dialog', { name: 'diagram.png' });
        expect(await within(dialog).findByRole('img', { name: 'diagram.png' })).toHaveAttribute('src', 'blob:2');
        expect(open).not.toHaveBeenCalled();
        fireEvent.click(within(dialog).getByRole('button', { name: 'common.close' }));
        await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    });

    it('previews a .md attachment like the library does, instead of handing it to the browser', async () => {
        const open = vi.spyOn(window, 'open');
        m.fetch.mockImplementation((url: string) => String(url).includes('notes.md')
            ? Promise.resolve(new Response('# Plan\n\nSome **bold** text.\n\n<script>alert(1)</script>'))
            : Promise.resolve(new Response('gone', { status: 404 })));
        render(<AttachmentChips owner={{ threadId: 't1' }} items={[file('notes.md', 'text/markdown')]} />);
        fireEvent.click(await screen.findByRole('button', { name: /notes\.md/ }));
        const dialog = await screen.findByRole('dialog', { name: 'notes.md' });
        expect(await within(dialog).findByRole('heading', { name: 'Plan' })).toBeInTheDocument();
        expect(within(dialog).getByText('bold').tagName).toBe('STRONG');
        expect(dialog.querySelector('script')).toBeNull();
        // The thread attachment endpoint served it, and nothing was opened in a tab.
        expect(calls()).toEqual(['/api/v1/threads/t1/attachments/file?path=notes.md']);
        expect(open).not.toHaveBeenCalled();
    });

    it('previews a plain-text attachment as text, not rendered', async () => {
        m.fetch.mockImplementation((url: string) => String(url).includes('data.csv')
            ? Promise.resolve(new Response('a,b\n<script>x</script>'))
            : Promise.resolve(new Response('gone', { status: 404 })));
        render(<AttachmentChips owner={{ projectId: 'p1' }} items={[file('data.csv', 'text/csv')]} />);
        fireEvent.click(await screen.findByRole('button', { name: /data\.csv/ }));
        const dialog = await screen.findByRole('dialog');
        const text = await within(dialog).findByText(/a,b/);
        expect(text.tagName).toBe('PRE');
        expect(dialog.querySelector('script')).toBeNull();
        expect(calls()).toEqual(['/api/v1/projects/p1/library/file?path=data.csv']);
    });

    it('steps through the message\'s images, skipping the files between them', async () => {
        render(<AttachmentChips owner={{ threadId: 't1' }} items={[
            file('a.png', 'image/png'), file('notes.md', 'text/markdown'), file('b.png', 'image/png'),
        ]} />);
        const thumb = await screen.findByRole('button', { name: 'a.png' });
        await waitFor(() => expect(within(thumb).getByRole('img')).toBeInTheDocument());
        fireEvent.click(thumb);
        const dialog = await screen.findByRole('dialog');
        await within(dialog).findByRole('img', { name: 'a.png' });
        expect(within(dialog).getByText('1 / 2')).toBeInTheDocument();
        fireEvent.click(within(dialog).getByRole('button', { name: 'library.nextImage' }));
        await within(dialog).findByRole('img', { name: 'b.png' });
        expect(within(dialog).getByText('2 / 2')).toBeInTheDocument();
    });

    it('still hands a non-previewable attachment to the browser instead of a preview', async () => {
        const open = vi.spyOn(window, 'open').mockReturnValue(null);
        render(<AttachmentChips owner={{ threadId: 't1' }} items={[file('spec.pdf', 'application/pdf')]} />);
        fireEvent.click(await screen.findByRole('button', { name: /spec\.pdf/ }));
        await waitFor(() => expect(open).toHaveBeenCalledWith('blob:1', '_blank', 'noopener'));
        expect(screen.queryByRole('dialog')).toBeNull();
    });

    it('offers the download for an attachment over the preview size cap', async () => {
        const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
        render(<AttachmentChips owner={{ projectId: 'p1' }} items={[file('big.txt', 'text/plain', PREVIEW_LIMIT.text + 1)]} />);
        fireEvent.click(await screen.findByRole('button', { name: /big\.txt/ }));
        const dialog = await screen.findByRole('dialog');
        expect(within(dialog).getByText('library.tooLarge')).toBeInTheDocument();
        expect(calls()).toEqual([]);
        fireEvent.click(within(dialog).getAllByRole('button', { name: 'library.download' })[0]);
        await waitFor(() => expect(click).toHaveBeenCalled());
        expect(m.fetch).toHaveBeenCalledWith('/api/v1/projects/p1/library/file?path=big.txt');
        click.mockRestore();
    });

    it('disables the chip of an image that is gone', async () => {
        m.fetch.mockRejectedValue(new Error('gone'));
        render(<AttachmentChips owner={{ threadId: 't1' }} items={[file('pic.png', 'image/png')]} />);
        const chip = await screen.findByRole('button', { name: /attachments\.removed/ });
        expect(chip).toBeDisabled();
        fireEvent.click(chip);
        expect(screen.queryByRole('dialog')).toBeNull();
    });
});
