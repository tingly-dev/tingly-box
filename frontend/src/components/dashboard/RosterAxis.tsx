import { useEffect, useMemo, useRef, useState } from 'react';
import {
    Box,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
} from '@mui/material';
import { getTotalTokens, getCacheHitRate, hasCacheWrites } from './chartStyles';
import { UsageMetricHeaderCells, UsageMetricValueCells } from './UsageMetricCells';
import type { UsageMetricKey, UsageMetricLabels } from './usageMetricColumns';

export type SortField = 'name' | UsageMetricKey;
export type SortDirection = 'asc' | 'desc';

// Common shape every roster-axis row satisfies, so summary/sort/detail logic
// can run once against any axis (account, model, provider, ...) instead of
// once per axis.
export interface MetricRow {
    request_count: number;
    total_input_tokens: number;
    total_output_tokens: number;
    cache_read_tokens?: number;
    cache_write_tokens?: number;
    error_count?: number;
    error_rate?: number;
}

// Sum + derive the same aggregates (display total, cache hit rate, error
// rate) over any axis' row shape.
export const computeUsageSummary = (items: MetricRow[]) => {
    const totals = items.reduce(
        (acc, row) => {
            acc.tokens += getTotalTokens(row);
            acc.inputTokens += row.total_input_tokens;
            acc.outputTokens += row.total_output_tokens;
            acc.cacheTokens += row.cache_read_tokens || 0;
            acc.cacheWriteTokens += row.cache_write_tokens || 0;
            acc.requests += row.request_count;
            acc.errors += row.error_count || 0;
            return acc;
        },
        { tokens: 0, inputTokens: 0, outputTokens: 0, cacheTokens: 0, cacheWriteTokens: 0, requests: 0, errors: 0 },
    );
    return {
        ...totals,
        cacheHitRate: getCacheHitRate(totals.cacheTokens, totals.inputTokens),
        errorRate: totals.requests > 0 ? (totals.errors / totals.requests) * 100 : 0,
    };
};

// Shared filter + sort for every roster axis.
export function filterAndSort<T extends MetricRow>(
    items: T[],
    query: string,
    searchText: (item: T) => string[],
    sortField: SortField,
    sortDirection: SortDirection,
    nameOf: (item: T) => string,
): T[] {
    const q = query.trim().toLocaleLowerCase();
    const direction = sortDirection === 'asc' ? 1 : -1;
    return items
        .filter((item) => !q || searchText(item).some((text) => text.toLocaleLowerCase().includes(q)))
        .sort((a, b) => {
            if (sortField === 'name') return direction * nameOf(a).localeCompare(nameOf(b));
            if (sortField === 'requests') return direction * (a.request_count - b.request_count);
            if (sortField === 'cacheRead') return direction * ((a.cache_read_tokens || 0) - (b.cache_read_tokens || 0));
            if (sortField === 'cacheWrite') return direction * ((a.cache_write_tokens || 0) - (b.cache_write_tokens || 0));
            if (sortField === 'cacheHit') {
                return direction * (
                    getCacheHitRate(a.cache_read_tokens || 0, a.total_input_tokens)
                    - getCacheHitRate(b.cache_read_tokens || 0, b.total_input_tokens)
                );
            }
            if (sortField === 'input') return direction * (a.total_input_tokens - b.total_input_tokens);
            if (sortField === 'output') return direction * (a.total_output_tokens - b.total_output_tokens);
            if (sortField === 'errorRate') return direction * ((a.error_rate || 0) - (b.error_rate || 0));
            return direction * (getTotalTokens(a) - getTotalTokens(b));
        });
}

export interface RosterAxis<T extends MetricRow, D> {
    search: string;
    setSearch: (value: string) => void;
    sortField: SortField;
    sortDirection: SortDirection;
    page: number;
    setPage: (page: number) => void;
    handleSort: (field: SortField) => void;
    visibleRows: T[];
    pagedRows: T[];
    selectedKey: string;
    setSelectedKey: (key: string) => void;
    selected: T | undefined;
    detail: D[];
    detailLoading: boolean;
}

// Drives one roster axis (account / model / provider / ...) end to end:
// client-side search + sort + pagination over an already-fetched roster,
// selection that survives filtering (falls back to the first visible row),
// and a race-guarded load of the "detail" data for whichever row is
// selected. The roster fetch itself stays with the caller, since how a
// roster is fetched genuinely differs per axis (e.g. the account roster is
// left-joined against registered tokens; a model/provider roster is not).
export function useRosterAxis<T extends MetricRow, D, R>({
    roster,
    getKey,
    nameOf,
    searchText,
    rowsPerPage,
    range,
    loadDetail,
}: {
    roster: T[];
    getKey: (row: T) => string;
    nameOf: (row: T) => string;
    searchText: (row: T) => string[];
    rowsPerPage: number;
    range: R;
    loadDetail: (selected: T | undefined, range: R) => Promise<D[]>;
}): RosterAxis<T, D> {
    const [search, setSearch] = useState('');
    const [sortField, setSortField] = useState<SortField>('total');
    const [sortDirection, setSortDirection] = useState<SortDirection>('desc');
    const [page, setPage] = useState(0);
    const [selectedKey, setSelectedKey] = useState('');
    const [detail, setDetail] = useState<D[]>([]);
    const [detailLoading, setDetailLoading] = useState(false);
    const detailSeq = useRef(0);

    const visibleRows = useMemo(
        () => filterAndSort(roster, search, searchText, sortField, sortDirection, nameOf),
        [roster, search, sortField, sortDirection, searchText, nameOf],
    );
    const pagedRows = useMemo(
        () => visibleRows.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage),
        [visibleRows, page, rowsPerPage],
    );

    // Filtering/sorting/range can shrink the result set below the current page.
    useEffect(() => {
        setPage(0);
    }, [search, sortField, sortDirection, range]);

    useEffect(() => {
        if (visibleRows.length === 0) {
            setSelectedKey('');
            return;
        }
        if (!visibleRows.some((row) => getKey(row) === selectedKey)) {
            setSelectedKey(getKey(visibleRows[0]));
        }
    }, [roster, selectedKey, visibleRows, getKey]);

    const selected = useMemo(
        () => roster.find((row) => getKey(row) === selectedKey),
        [roster, selectedKey, getKey],
    );

    useEffect(() => {
        const seq = ++detailSeq.current;
        setDetailLoading(true);
        loadDetail(selected, range)
            .then((result) => { if (seq === detailSeq.current) setDetail(result); })
            .catch(() => { if (seq === detailSeq.current) setDetail([]); })
            .finally(() => { if (seq === detailSeq.current) setDetailLoading(false); });
    }, [selected, range, loadDetail]);

    const handleSort = (field: SortField) => {
        if (field === sortField) {
            setSortDirection((direction) => (direction === 'asc' ? 'desc' : 'asc'));
        } else {
            setSortField(field);
            setSortDirection(field === 'name' ? 'asc' : 'desc');
        }
    };

    return {
        search, setSearch,
        sortField, sortDirection, page, setPage, handleSort,
        visibleRows, pagedRows,
        selectedKey, setSelectedKey, selected,
        detail, detailLoading,
    };
}

// One shared "breakdown" table shape, used for every roster-axis detail
// direction (e.g. models used by an account, accounts behind a model,
// accounts behind a provider) — only the identity column(s) and row key
// differ; the metric columns always come from the same shared definition.
export function RosterBreakdownTable<D extends MetricRow>({
    items,
    rowKey,
    identityColumns,
    ariaLabel,
    noUsageLabel,
    noUsageHint,
    usageMetricLabels,
}: {
    items: D[];
    rowKey: (item: D) => string;
    identityColumns: Array<{ key: string; label: string; render: (item: D) => React.ReactNode }>;
    ariaLabel: string;
    noUsageLabel: string;
    noUsageHint?: string;
    usageMetricLabels: UsageMetricLabels;
}) {
    const showCacheWrite = hasCacheWrites(items);
    if (items.length === 0) {
        return (
            <Box sx={{ py: 4, textAlign: 'center', bgcolor: 'action.hover', borderRadius: 1.5 }}>
                <Typography variant="body1">{noUsageLabel}</Typography>
                {noUsageHint && <Typography variant="body2">{noUsageHint}</Typography>}
            </Box>
        );
    }
    return (
        <TableContainer
            sx={{
                maxHeight: 520,
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1.5,
                overscrollBehavior: 'contain',
            }}
            role="region"
            aria-label={ariaLabel}
        >
            <Table stickyHeader sx={{ minWidth: showCacheWrite ? 1060 : 960 }}>
                <TableHead>
                    <TableRow
                        sx={{
                            '& .MuiTableCell-root': {
                                fontWeight: 600,
                                fontSize: '0.75rem',
                                textTransform: 'uppercase',
                                letterSpacing: '0.05em',
                                color: 'text.secondary',
                                py: 1.25,
                            },
                        }}
                    >
                        {identityColumns.map((col) => <TableCell key={col.key}>{col.label}</TableCell>)}
                        <UsageMetricHeaderCells
                            labels={usageMetricLabels}
                            showTotal
                            showCacheWrite={showCacheWrite}
                        />
                    </TableRow>
                </TableHead>
                <TableBody>
                    {items.map((item) => (
                        <TableRow
                            key={rowKey(item)}
                            hover
                            sx={{
                                '& .MuiTableCell-root': {
                                    py: 1.25,
                                    borderBottom: '1px solid',
                                    borderColor: 'divider',
                                },
                            }}
                        >
                            {identityColumns.map((col) => <TableCell key={col.key}>{col.render(item)}</TableCell>)}
                            <UsageMetricValueCells
                                usage={item}
                                showTotal
                                showCacheWrite={showCacheWrite}
                            />
                        </TableRow>
                    ))}
                </TableBody>
            </Table>
        </TableContainer>
    );
}
