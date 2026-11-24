import { useState } from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import RosterTable from './RosterTable';
import { accountKey } from './userUsageModel';
import type { AggregatedStat, RosterAxisState, SortDirection, SortField } from '@/components/dashboard';
import type { UserUsageRow } from './userUsageModel';

// t() echoes the key so assertions read against the source, not a locale.
// The rest of react-i18next stays real: the dashboard barrel pulls in
// TokenHeatmap → @/i18n, whose init needs the real initReactI18next.
vi.mock('react-i18next', async (importOriginal) => ({
    ...(await importOriginal<Record<string, unknown>>()),
    useTranslation: () => ({ t: (key: string) => key }),
}));

// token_id and user_id deliberately differ: the account axis keys by user_id,
// so a table that emits token_id on click puts the selection outside the
// axis' key space (the regression this file pins — the selection used to snap
// straight back to the first row).
const makeRow = (i: number): UserUsageRow => ({
    token_id: `tb-share-000${i}abcdefghijklmnopqrstuvwxyz`,
    user_id: `user-${i}`,
    display_name: `Person ${i}`,
    enabled: true,
    account_type: 'sharing',
    last_used_at: '2026-11-09T10:00:00Z',
    request_count: i,
    total_tokens: i * 100,
    total_input_tokens: i * 70,
    total_output_tokens: i * 30,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    error_count: 0,
    error_rate: 0,
});

const makeStat = (provider: string, model: string, i: number): AggregatedStat => ({
    key: `${provider}::${model}`,
    provider_uuid: provider,
    provider_name: provider,
    model,
    request_count: i,
    total_input_tokens: i * 70,
    total_output_tokens: i * 30,
});

// A stand-in for the page's useRosterAxis result: same shape, same wiring —
// onSelectRow feeds setSelectedKey exactly as UserUsagePage does — but without
// the hook's own bounce guard, so the component's click payload is what the
// assertions see.
const useFakeAxis = <T,>(rows: T[], getKey: (row: T) => string) => {
    const [selectedKey, setSelectedKey] = useState('');
    const axis = {
        search: '',
        setSearch: vi.fn(),
        sortField: 'total' as SortField,
        sortDirection: 'desc' as SortDirection,
        page: 0,
        setPage: vi.fn(),
        handleSort: vi.fn(),
        visibleRows: rows,
        pagedRows: rows,
        selectedKey,
        setSelectedKey,
        selected: rows.find((row) => getKey(row) === selectedKey),
        detail: [],
        detailLoading: false,
    } as unknown as RosterAxisState<UserUsageRow, AggregatedStat>;
    return { axis };
};

const columns = [
    { kind: 'sort' as const, field: 'name' as const, label: 'User', defaultDir: 'asc' as const },
    { kind: 'sort' as const, field: 'total' as const, label: 'Total', align: 'right' as const, defaultDir: 'desc' as const },
];

const AccountHarness = ({ rows, onSelectRow }: { rows: UserUsageRow[]; onSelectRow?: (key: string) => void }) => {
    const { axis } = useFakeAxis(rows, accountKey);
    return (
        <RosterTable
            viewMode="account"
            onViewModeChange={vi.fn()}
            activeAxis={axis}
            primaryColumns={columns}
            showCacheWrite={false}
            rowsPerPage={10}
            onRowsPerPageChange={vi.fn()}
            onSelectRow={(key) => { onSelectRow?.(key); axis.setSelectedKey(key); }}
        />
    );
};

const rowByName = (name: string) => screen.getByRole('row', { name: new RegExp(name) });

describe('RosterTable selection', () => {
    beforeEach(() => {
        vi.clearAllMocks();
    });

    it('account rows: clicking selects by user_id and the selection sticks', () => {
        const onSelectRow = vi.fn();
        render(<AccountHarness rows={[makeRow(1), makeRow(2), makeRow(3)]} onSelectRow={onSelectRow} />);

        // Click a non-first row — the reported bug was that it snapped back.
        fireEvent.click(within(rowByName('Person 2')).getByText('Person 2'));

        // The click payload must be the axis' key space (user_id), not token_id.
        expect(onSelectRow).toHaveBeenCalledWith('user-2');

        // The clicked row is the selected one, and the previously implied
        // first row is not — selection actually moved and holds.
        expect(rowByName('Person 2')).toHaveClass('Mui-selected');
        expect(rowByName('Person 1')).not.toHaveClass('Mui-selected');
        expect(rowByName('Person 3')).not.toHaveClass('Mui-selected');
    });

    it('account rows: selection follows later clicks (no bounce to the first row)', () => {
        render(<AccountHarness rows={[makeRow(1), makeRow(2), makeRow(3)]} />);

        fireEvent.click(within(rowByName('Person 2')).getByText('Person 2'));
        expect(rowByName('Person 2')).toHaveClass('Mui-selected');

        fireEvent.click(within(rowByName('Person 3')).getByText('Person 3'));
        expect(rowByName('Person 3')).toHaveClass('Mui-selected');
        expect(rowByName('Person 2')).not.toHaveClass('Mui-selected');
        expect(rowByName('Person 1')).not.toHaveClass('Mui-selected');
    });
});

const ModelHarness = ({ rows, onSelectRow }: { rows: AggregatedStat[]; onSelectRow?: (key: string) => void }) => {
    const { axis } = useFakeAxis(rows, (row) => `${row.provider_uuid}::${row.model}`);
    return (
        <RosterTable
            viewMode="model"
            onViewModeChange={vi.fn()}
            activeAxis={axis}
            primaryColumns={columns}
            showCacheWrite={false}
            rowsPerPage={10}
            onRowsPerPageChange={vi.fn()}
            onSelectRow={(key) => { onSelectRow?.(key); axis.setSelectedKey(key); }}
        />
    );
};

describe('RosterTable model axis (control: already worked)', () => {
    it('clicking a model row selects it by its provider+model key', () => {
        const onSelectRow = vi.fn();
        render(<ModelHarness rows={[makeStat('p1', 'm1', 1), makeStat('p2', 'm2', 2)]} onSelectRow={onSelectRow} />);

        fireEvent.click(within(rowByName('m2')).getByText('m2'));

        expect(onSelectRow).toHaveBeenCalledWith('p2::m2');
        expect(rowByName('m2')).toHaveClass('Mui-selected');
        expect(rowByName('m1')).not.toHaveClass('Mui-selected');
    });
});
