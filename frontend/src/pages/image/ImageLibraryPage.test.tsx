import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PhoneShellContext } from '@tingly/ui/hooks/usePhoneShell';

const { removeSnippet } = vi.hoisted(() => ({ removeSnippet: vi.fn() }));
vi.mock('react-i18next', async (importOriginal) => ({
    ...await importOriginal<typeof import('react-i18next')>(),
    useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock('@/services/api', () => ({ api: { getImageGenInfo: async () => ({ success: true, output_dir: '/data/image' }) } }));
vi.mock('./library/assetStore', () => ({
    useAssets: () => [
        { id: 'a1', name: 'rain.png', src: 'blob:a1', createdAt: 1 },
        { id: 'a2', name: 'cafe.png', src: 'blob:a2', createdAt: 2 },
    ],
    useSnippets: () => [{ id: 's1', name: 'Film look', text: '35mm, soft grain' }],
    useLibraryReady: () => true,
    removeAsset: vi.fn(),
    removeSnippet,
    renameAsset: vi.fn(),
    saveSnippet: vi.fn(),
}));

import ImageLibraryPage from './ImageLibraryPage';

const at = (tab: string, phone: boolean) => {
    const page: ReactNode = <MemoryRouter initialEntries={[`/image/library${tab}`]}><ImageLibraryPage /></MemoryRouter>;
    return render(phone ? <PhoneShellContext.Provider value>{page}</PhoneShellContext.Provider> : page);
};

afterEach(() => { cleanup(); removeSnippet.mockClear(); });

describe('Image Library in the phone shell', () => {
    it('shows the images two to a row', () => {
        at('', true);
        const grid = screen.getByText('rain.png').closest('button')!.parentElement as HTMLElement;
        expect(grid).toHaveStyle({ gridTemplateColumns: 'repeat(2, minmax(0, 1fr))' });
    });

    it('asks before deleting a snippet', () => {
        at('?tab=snippets', true);
        fireEvent.click(screen.getByRole('button', { name: 'imageLibrary.deleteSnippet' }));
        expect(removeSnippet).not.toHaveBeenCalled();
        fireEvent.click(screen.getAllByRole('button', { name: 'imageLibrary.deleteSnippet' }).at(-1)!);
        expect(removeSnippet).toHaveBeenCalledWith('s1');
    });

    it('keeps the desktop grid and one-tap delete', () => {
        at('?tab=snippets', false);
        fireEvent.click(screen.getByRole('button', { name: 'imageLibrary.deleteSnippet' }));
        expect(removeSnippet).toHaveBeenCalledWith('s1');
        cleanup();
        at('', false);
        const grid = screen.getByText('rain.png').closest('button')!.parentElement as HTMLElement;
        expect(grid).toHaveStyle({ gridTemplateColumns: 'repeat(auto-fill, minmax(168px, 1fr))' });
    });
});
