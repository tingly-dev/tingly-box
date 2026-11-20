import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { PhoneShellContext } from '@tingly/ui/hooks/usePhoneShell';
import SharingKeysTable, { type SharingKey } from './SharingKeysTable';

vi.mock('react-i18next', async (importOriginal) => ({
    ...await importOriginal<typeof import('react-i18next')>(),
    useTranslation: () => ({ t: (key: string, o?: Record<string, string>) => (o?.name ? `${key}:${o.name}` : key) }),
}));

const keys: SharingKey[] = [
    { token_id: 'tb-share-design-0001', user_id: 'user-design', team_id: 't', display_name: 'Design team', enabled: true, created_at: '2026-01-01T00:00:00Z' },
    { token_id: 'tb-share-ci-0002', user_id: 'user-ci', team_id: 't', display_name: 'CI', enabled: false, created_at: '2026-01-01T00:00:00Z' },
];

const actions = () => ({
    visibleTokens: {} as Record<string, boolean>,
    onToggleVisibility: vi.fn(),
    onCopy: vi.fn(),
    onToggleEnabled: vi.fn(),
    onDelete: vi.fn(),
    onMove: vi.fn(),
    onLimits: vi.fn(),
});

describe('Team keys in the phone shell', () => {
    it('lists the keys as rows instead of a table, one person per row', () => {
        render(<PhoneShellContext.Provider value><SharingKeysTable tokens={keys} {...actions()} /></PhoneShellContext.Provider>);
        expect(screen.queryByRole('table')).not.toBeInTheDocument();
        const list = screen.getByRole('list', { name: 'sharingKeys.title' });
        expect(within(list).getAllByRole('listitem')).toHaveLength(2);
        expect(within(list).getByText('Design team')).toBeInTheDocument();
        expect(within(list).getByText('user-design')).toBeInTheDocument();
        // Masked until revealed.
        expect(within(list).queryByText('tb-share-design-0001')).not.toBeInTheDocument();
    });

    it('copies, reveals and toggles a key in place; limits, move and delete are in its menu', () => {
        const a = actions();
        render(<PhoneShellContext.Provider value><SharingKeysTable tokens={keys} {...a} /></PhoneShellContext.Provider>);
        const row = screen.getByText('Design team').closest('li') as HTMLElement;

        fireEvent.click(within(row).getByRole('button', { name: 'sharingKeys.table.copy' }));
        expect(a.onCopy).toHaveBeenCalledWith('tb-share-design-0001');
        fireEvent.click(within(row).getByRole('button', { name: 'sharingKeys.table.show' }));
        expect(a.onToggleVisibility).toHaveBeenCalledWith('tb-share-design-0001');
        fireEvent.click(within(row).getByRole('switch'));
        expect(a.onToggleEnabled).toHaveBeenCalledWith(keys[0]);

        fireEvent.click(within(row).getByRole('button', { name: 'sharingKeys.phone.keyActions:Design team' }));
        fireEvent.click(screen.getByRole('menuitem', { name: 'sharingKeys.table.deleteTooltip' }));
        // The delete goes through the page's confirm dialog (useSharingKeyActions).
        expect(a.onDelete).toHaveBeenCalledWith(keys[0]);
    });

    it('keeps the table on the desktop', () => {
        render(<SharingKeysTable tokens={keys} {...actions()} />);
        expect(screen.getByRole('table')).toBeInTheDocument();
    });
});
