import { act, fireEvent, render, renderHook, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SharingKey } from '@/components/SharingKeysTable';
import type { Team } from '@/types/team';
import TeamKeysOverview, { useTeamKeysOverviewExpanded } from './TeamKeysOverview';

// t() echoes the key, plus the count so the folded line's number is checkable.
vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, opts?: { count?: number }) => (opts?.count !== undefined ? `${key}:${opts.count}` : key),
    }),
}));

const team: Team = {
    id: 'team-1', name: 'Default', slug: 'default', enabled: true, is_default: true,
    created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z',
};

const makeKey = (i: number, enabled = true): SharingKey => ({
    token_id: `tb-share-${String(i).padStart(4, '0')}abcdefghijklmnopqrstuvwxyz`,
    user_id: `user-${i}`,
    team_id: 'team-1',
    display_name: `Person ${i}`,
    enabled,
    created_at: '2026-10-01T00:00:00Z',
});

const makeActions = () => ({
    visibleTokens: {} as Record<string, boolean>,
    onToggleVisibility: vi.fn(),
    onCopy: vi.fn(),
    onToggleEnabled: vi.fn(),
    onDelete: vi.fn(),
    onMove: vi.fn(),
});

const renderOverview = (keys: SharingKey[], expanded: boolean, extra: Partial<React.ComponentProps<typeof TeamKeysOverview>> = {}) =>
    render(
        <TeamKeysOverview
            team={team}
            keys={keys}
            loading={false}
            expanded={expanded}
            onToggle={vi.fn()}
            actions={makeActions()}
            onCreate={vi.fn()}
            {...extra}
        />,
    );

describe('TeamKeysOverview', () => {
    it('folded: one line with the key count, no keys or actions', () => {
        const onToggle = vi.fn();
        renderOverview([makeKey(1), makeKey(2), makeKey(3, false), makeKey(4), makeKey(5)], false, { onToggle });
        expect(screen.getByTestId('team-keys-count')).toHaveTextContent('sharingKeys.overview.count:5');
        expect(screen.queryByText('Person 1')).toBeNull();
        expect(screen.queryByText('sharingKeys.createToken')).toBeNull();
        fireEvent.click(screen.getByRole('button', { expanded: false }));
        expect(onToggle).toHaveBeenCalledOnce();
    });

    it('open: lists every key by person, masked', () => {
        const keys = Array.from({ length: 12 }, (_, i) => makeKey(i + 1));
        renderOverview(keys, true);
        const list = screen.getByRole('list', { name: 'sharingKeys.title' });
        const rows = within(list).getAllByRole('listitem');
        expect(rows).toHaveLength(12);
        keys.forEach((key) => expect(within(list).getByText(key.display_name)).toBeInTheDocument());
        // masked like the Team Keys table: prefix … last four, never the full key
        expect(within(rows[0]).getByText('tb-share-000...wxyz')).toBeInTheDocument();
        expect(screen.queryByText(keys[0].token_id)).toBeNull();
    });

    it('open: the chevron folds it, Create creates', () => {
        const onToggle = vi.fn();
        const onCreate = vi.fn();
        renderOverview([makeKey(1)], true, { onToggle, onCreate });
        fireEvent.click(screen.getByRole('button', { name: 'sharingKeys.createToken' }));
        expect(onCreate).toHaveBeenCalledOnce();
        expect(onToggle).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { expanded: true }));
        expect(onToggle).toHaveBeenCalledOnce();
    });

    it('wires each row to the key actions', () => {
        const key = makeKey(1);
        const actions = makeActions();
        renderOverview([key], true, { actions });
        const row = screen.getByRole('listitem');
        fireEvent.click(within(row).getByLabelText('Person 1 sharingKeys.table.status'));
        expect(actions.onToggleEnabled).toHaveBeenCalledWith(key);
        fireEvent.click(within(row).getByRole('button', { name: 'sharingKeys.table.moveTooltip' }));
        expect(actions.onMove).toHaveBeenCalledWith(key);
        fireEvent.click(within(row).getByRole('button', { name: 'sharingKeys.table.deleteTooltip' }));
        expect(actions.onDelete).toHaveBeenCalledWith(key);
    });

    it('reveals a key the user asked to see', () => {
        const key = makeKey(1);
        renderOverview([key], true, { actions: { ...makeActions(), visibleTokens: { [key.token_id]: true } } });
        expect(screen.getByText(key.token_id)).toBeInTheDocument();
    });

    it('shows a quiet empty state for a Team without keys', () => {
        renderOverview([], true);
        expect(screen.getByTestId('team-keys-count')).toHaveTextContent('sharingKeys.overview.count:0');
        expect(screen.getByText('sharingKeys.overview.empty')).toBeInTheDocument();
        expect(screen.queryByRole('list')).toBeNull();
    });
});

describe('useTeamKeysOverviewExpanded', () => {
    beforeEach(() => localStorage.clear());

    it('starts folded and remembers the user\'s choice across reloads', () => {
        const { result } = renderHook(() => useTeamKeysOverviewExpanded());
        expect(result.current.expanded).toBe(false);
        act(() => result.current.toggle());
        expect(result.current.expanded).toBe(true);
        expect(localStorage.getItem('team-keys-overview-expanded')).toBe('true');
        // a fresh mount (page reload) starts from the saved choice
        const again = renderHook(() => useTeamKeysOverviewExpanded());
        expect(again.result.current.expanded).toBe(true);
        act(() => again.result.current.toggle());
        expect(renderHook(() => useTeamKeysOverviewExpanded()).result.current.expanded).toBe(false);
    });

    it('stays folded without storage', () => {
        const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
        const { result } = renderHook(() => useTeamKeysOverviewExpanded());
        expect(result.current.expanded).toBe(false);
        spy.mockRestore();
    });
});
