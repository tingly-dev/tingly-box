import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AttachmentChips } from './attachments';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../client', () => ({ apiFetch: vi.fn().mockResolvedValue({ ok: true, blob: async () => new Blob(['image'], { type: 'image/png' }) }) }));

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

describe('conversation attachments', () => {
    it('opens an image in a dialog and closes it without opening a page', async () => {
        vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() }));
        const open = vi.spyOn(window, 'open');
        render(<AttachmentChips owner={{ threadId: 'thread' }} items={[{ name: 'diagram.png', path: 'diagram.png', type: 'image/png', size: 5 }]} />);
        const button = await screen.findByRole('button', { name: 'diagram.png' });
        await waitFor(() => expect(within(button).getByRole('img')).toHaveAttribute('src', 'blob:preview'));
        fireEvent.click(button);
        const dialog = screen.getByRole('dialog', { name: 'diagram.png' });
        expect(within(dialog).getByRole('img', { name: 'diagram.png' })).toHaveAttribute('src', 'blob:preview');
        expect(open).not.toHaveBeenCalled();
        fireEvent.click(within(dialog).getByRole('button', { name: 'common.close' }));
        await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    });

    it('never renders an SVG as an image, whatever the chip says', async () => {
        const { apiFetch } = await import('../client');
        vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:svg'), revokeObjectURL: vi.fn() }));
        render(<AttachmentChips owner={{ threadId: 'thread' }} items={[{ name: 'logo.svg', path: 'logo.svg', type: 'image/svg+xml', size: 5 }]} />);
        const chip = await screen.findByRole('button');
        expect(within(chip).queryByRole('img')).toBeNull();
        expect(chip).toHaveTextContent('logo.svg');
        expect(apiFetch).not.toHaveBeenCalled();
    });

    it('previews a GIF like any other raster image', async () => {
        vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:gif'), revokeObjectURL: vi.fn() }));
        render(<AttachmentChips owner={{ threadId: 'thread' }} items={[{ name: 'loop.gif', path: 'loop.gif', type: 'image/gif', size: 5 }]} />);
        const button = await screen.findByRole('button', { name: 'loop.gif' });
        await waitFor(() => expect(within(button).getByRole('img')).toHaveAttribute('src', 'blob:gif'));
    });
});
