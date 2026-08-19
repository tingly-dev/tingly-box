import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Alert,
    Avatar,
    Box,
    Chip,
    CircularProgress,
    Grid,
    IconButton,
    Paper,
    Skeleton,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TablePagination,
    TableRow,
    TableSortLabel,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
    alpha,
    useTheme,
} from '@mui/material';
import {
    AccessTime,
    ArrowForward,
    Autorenew as CachedIcon,
    BarChart,
    Block,
    CheckCircle,
    Cloud,
    ErrorOutline,
    Refresh,
    Server,
    Token,
    Users,
} from '@/components/icons';
import PageHeader from '@/components/PageHeader';
import SearchField from '@/components/SearchField';
import {
    formatNumber,
    StatCard,
    TOKEN_COLORS,
    getTotalTokens,
    getCacheHitRate,
    getCacheHitRateColor,
    formatCacheBreakdown,
    hasCacheWrites,
    getErrorRateColor,
    getUsageMetricColumns,
    UsageMetricHeaderCells,
    UsageMetricValueCells,
} from '@/components/dashboard';
import type { AggregatedStat, UsageMetricKey, UsageMetricLabels } from '@/components/dashboard';
import api from '@/services/api';

type TimeRange = 'today' | '7d' | '30d' | '90d';
type ViewMode = 'account' | 'model' | 'provider';
type SortField = 'name' | UsageMetricKey;
type SortDirection = 'asc' | 'desc';

interface APITokenInfo {
    token_id: string;
    user_id: string;
    display_name: string;
    enabled: boolean;
    last_used_at?: string;
    created_at?: string;
    account_type?: 'primary' | 'sharing';
}

interface UserUsageRow extends APITokenInfo {
    request_count: number;
    total_tokens: number;
    total_input_tokens: number;
    total_output_tokens: number;
    cache_read_tokens: number;
    cache_write_tokens: number;
    error_count: number;
    error_rate: number;
}

// Common shape UserUsageRow and AggregatedStat both satisfy, so summary/sort
// logic can run once against any of the three roster axes.
interface MetricRow {
    request_count: number;
    total_input_tokens: number;
    total_output_tokens: number;
    cache_read_tokens?: number;
    cache_write_tokens?: number;
    error_count?: number;
    error_rate?: number;
}

const RANGE_DAYS: Record<TimeRange, number> = {
    today: 1,
    '7d': 7,
    '30d': 30,
    '90d': 90,
};

const toLocalISOString = (date: Date): string => {
    const offset = -date.getTimezoneOffset();
    const sign = offset >= 0 ? '+' : '-';
    const pad = (value: number) => String(Math.floor(Math.abs(value))).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
        `T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}` +
        `${sign}${pad(offset / 60)}:${pad(offset % 60)}`;
};

const buildTimeParams = (range: TimeRange) => {
    const now = new Date();
    const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    if (range !== 'today') {
        start.setDate(start.getDate() - (RANGE_DAYS[range] - 1));
    }
    return {
        start_time: toLocalISOString(start),
        end_time: toLocalISOString(now),
    };
};

const formatDateTime = (value?: string) => {
    if (!value) return '—';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '—';
    return new Intl.DateTimeFormat(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
    }).format(date);
};

// Model name alone can collide across providers, so the model axis is keyed
// by provider + model together (mirrors the provider filter always paired
// with the model filter when querying the backend for one exact model).
const getModelKey = (stat: Pick<AggregatedStat, 'provider_uuid' | 'model' | 'key'>) =>
    `${stat.provider_uuid || ''}::${stat.model || stat.key}`;

// provider_uuid is already the whole axis, so no collision-avoidance pairing
// is needed the way model+provider needs one.
const getProviderKey = (stat: Pick<AggregatedStat, 'provider_uuid' | 'provider_name' | 'key'>) =>
    stat.provider_uuid || stat.provider_name || stat.key;

// Sum + derive the same aggregates (display total, cache hit rate, error
// rate) over any axis' row shape.
const computeUsageSummary = (items: MetricRow[]) => {
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
function filterAndSort<T extends MetricRow>(
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

const usageTableCardSx = {
    width: '100%',
    borderRadius: 2,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    boxShadow: 'none',
    overflow: 'hidden',
} as const;

const UserUsageSkeleton = () => (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
        <Skeleton variant="rounded" height={72} />
        <Grid container spacing={2}>
            {Array.from({ length: 5 }).map((_, index) => (
                <Grid key={index} size={{ xs: 6, sm: 4, md: 2.4 }}>
                    <Skeleton variant="rounded" height={118} />
                </Grid>
            ))}
        </Grid>
        <Grid container spacing={2}>
            <Grid size={{ xs: 12, lg: 7, xl: 5 }}><Skeleton variant="rounded" height={520} /></Grid>
            <Grid size={{ xs: 12, lg: 5, xl: 7 }}><Skeleton variant="rounded" height={520} /></Grid>
        </Grid>
    </Box>
);

// Shared shape of the "which accounts used this X" breakdown, used by both
// the model detail panel and the provider detail panel (only the data
// source, aria label, and identity column heading differ).
function AccountsBreakdownTable({
    accounts,
    ariaLabel,
    identityLabel,
    noUsageLabel,
    accountDisplayName,
    usageMetricLabels,
}: {
    accounts: AggregatedStat[];
    ariaLabel: string;
    identityLabel: string;
    noUsageLabel: string;
    accountDisplayName: (userID: string) => string;
    usageMetricLabels: UsageMetricLabels;
}) {
    const showCacheWrite = hasCacheWrites(accounts);
    if (accounts.length === 0) {
        return (
            <Box sx={{ py: 4, textAlign: 'center', bgcolor: 'action.hover', borderRadius: 1.5 }}>
                <Typography variant="body1">{noUsageLabel}</Typography>
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
                        <TableCell>{identityLabel}</TableCell>
                        <UsageMetricHeaderCells
                            labels={usageMetricLabels}
                            showTotal
                            showCacheWrite={showCacheWrite}
                        />
                    </TableRow>
                </TableHead>
                <TableBody>
                    {accounts.map((account) => (
                        <TableRow
                            key={account.user_id || account.key}
                            hover
                            sx={{
                                '& .MuiTableCell-root': {
                                    py: 1.25,
                                    borderBottom: '1px solid',
                                    borderColor: 'divider',
                                },
                            }}
                        >
                            <TableCell>
                                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                    {accountDisplayName(account.user_id || account.key)}
                                </Typography>
                                <Typography variant="caption" sx={{ color: 'text.secondary', fontFamily: 'monospace' }}>
                                    {account.user_id || account.key}
                                </Typography>
                            </TableCell>
                            <UsageMetricValueCells
                                usage={account}
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

export default function UserUsagePage() {
    const { t } = useTranslation();
    const theme = useTheme();
    const [range, setRange] = useState<TimeRange>('7d');
    const [viewMode, setViewMode] = useState<ViewMode>('account');
    const [tokens, setTokens] = useState<APITokenInfo[]>([]);
    const [userStats, setUserStats] = useState<AggregatedStat[]>([]);
    const [modelRoster, setModelRoster] = useState<AggregatedStat[]>([]);
    const [providerRoster, setProviderRoster] = useState<AggregatedStat[]>([]);
    const [modelStats, setModelStats] = useState<AggregatedStat[]>([]);
    const [accountsByModel, setAccountsByModel] = useState<AggregatedStat[]>([]);
    const [accountsByProvider, setAccountsByProvider] = useState<AggregatedStat[]>([]);
    const [selectedUserID, setSelectedUserID] = useState('');
    const [selectedModelKey, setSelectedModelKey] = useState('');
    const [selectedProviderKey, setSelectedProviderKey] = useState('');
    const [accountSearch, setAccountSearch] = useState('');
    const [modelSearch, setModelSearch] = useState('');
    const [providerSearch, setProviderSearch] = useState('');
    const [accountSortField, setAccountSortField] = useState<SortField>('total');
    const [accountSortDirection, setAccountSortDirection] = useState<SortDirection>('desc');
    const [modelSortField, setModelSortField] = useState<SortField>('total');
    const [modelSortDirection, setModelSortDirection] = useState<SortDirection>('desc');
    const [providerSortField, setProviderSortField] = useState<SortField>('total');
    const [providerSortDirection, setProviderSortDirection] = useState<SortDirection>('desc');
    const [accountPage, setAccountPage] = useState(0);
    const [modelPage, setModelPage] = useState(0);
    const [providerPage, setProviderPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(10);
    const [loading, setLoading] = useState(true);
    const [modelDetailLoading, setModelDetailLoading] = useState(false);
    const [accountsByModelLoading, setAccountsByModelLoading] = useState(false);
    const [accountsByProviderLoading, setAccountsByProviderLoading] = useState(false);
    const [refreshing, setRefreshing] = useState(false);
    const [error, setError] = useState('');
    const requestSeq = useRef(0);
    const modelDetailSeq = useRef(0);
    const accountsByModelSeq = useRef(0);
    const accountsByProviderSeq = useRef(0);
    const detailPanelRef = useRef<HTMLDivElement>(null);

    const loadRosters = useCallback(async (selectedRange: TimeRange, manual = false) => {
        const seq = ++requestSeq.current;
        if (manual) setRefreshing(true);
        setError('');
        try {
            const timeParams = buildTimeParams(selectedRange);
            const [tokensResult, userStatsResult, modelStatsResult, providerStatsResult] = await Promise.all([
                api.listAPITokens({ limit: 500 }),
                api.getUsageStats({
                    ...timeParams,
                    group_by: 'user',
                    sort_by: 'total_tokens',
                    sort_order: 'desc',
                    limit: 500,
                }),
                api.getUsageStats({
                    ...timeParams,
                    group_by: 'model',
                    sort_by: 'total_tokens',
                    sort_order: 'desc',
                    limit: 1000,
                }),
                api.getUsageStats({
                    ...timeParams,
                    group_by: 'provider',
                    sort_by: 'total_tokens',
                    sort_order: 'desc',
                    limit: 200,
                }),
            ]);
            if (seq !== requestSeq.current) return;
            if (!tokensResult?.success) throw new Error(tokensResult?.error || 'Unable to load registered users');
            const tokenData = Array.isArray(tokensResult.data)
                ? tokensResult.data
                : tokensResult.data?.tokens || [];
            const sharingUsers: APITokenInfo[] = tokenData
                .filter((token: APITokenInfo) => token.user_id !== 'admin')
                .map((token: APITokenInfo) => ({ ...token, account_type: 'sharing' }));
            setTokens([
                {
                    token_id: 'primary-account',
                    user_id: 'admin',
                    display_name: '',
                    enabled: true,
                    account_type: 'primary',
                },
                ...sharingUsers,
            ]);
            setUserStats(userStatsResult?.data || []);
            setModelRoster(modelStatsResult?.data || []);
            setProviderRoster(providerStatsResult?.data || []);
        } catch (loadError) {
            if (seq === requestSeq.current) {
                setError(loadError instanceof Error ? loadError.message : 'Unable to load team usage');
            }
        } finally {
            if (seq === requestSeq.current) {
                setLoading(false);
                setRefreshing(false);
            }
        }
    }, []);

    const loadModelDetail = useCallback(async (userID: string, selectedRange: TimeRange) => {
        if (!userID) {
            setModelStats([]);
            return;
        }
        const seq = ++modelDetailSeq.current;
        setModelDetailLoading(true);
        try {
            const result = await api.getUsageStats({
                ...buildTimeParams(selectedRange),
                user_id: userID,
                group_by: 'model',
                sort_by: 'total_tokens',
                sort_order: 'desc',
                limit: 1000,
            });
            if (seq === modelDetailSeq.current) setModelStats(result?.data || []);
        } catch {
            if (seq === modelDetailSeq.current) setModelStats([]);
        } finally {
            if (seq === modelDetailSeq.current) setModelDetailLoading(false);
        }
    }, []);

    // Which accounts used this model — the mirror of loadModelDetail. Scoped
    // by provider + model together, not model name alone, since the same
    // model name can exist under more than one provider.
    const loadAccountsByModel = useCallback(async (model: AggregatedStat | undefined, selectedRange: TimeRange) => {
        if (!model) {
            setAccountsByModel([]);
            return;
        }
        const seq = ++accountsByModelSeq.current;
        setAccountsByModelLoading(true);
        try {
            const result = await api.getUsageStats({
                ...buildTimeParams(selectedRange),
                model: model.model,
                provider: model.provider_uuid,
                group_by: 'user',
                sort_by: 'total_tokens',
                sort_order: 'desc',
                limit: 500,
            });
            if (seq === accountsByModelSeq.current) setAccountsByModel(result?.data || []);
        } catch {
            if (seq === accountsByModelSeq.current) setAccountsByModel([]);
        } finally {
            if (seq === accountsByModelSeq.current) setAccountsByModelLoading(false);
        }
    }, []);

    // Which accounts used this provider (across all its models).
    const loadAccountsByProvider = useCallback(async (provider: AggregatedStat | undefined, selectedRange: TimeRange) => {
        if (!provider) {
            setAccountsByProvider([]);
            return;
        }
        const seq = ++accountsByProviderSeq.current;
        setAccountsByProviderLoading(true);
        try {
            const result = await api.getUsageStats({
                ...buildTimeParams(selectedRange),
                provider: provider.provider_uuid,
                group_by: 'user',
                sort_by: 'total_tokens',
                sort_order: 'desc',
                limit: 500,
            });
            if (seq === accountsByProviderSeq.current) setAccountsByProvider(result?.data || []);
        } catch {
            if (seq === accountsByProviderSeq.current) setAccountsByProvider([]);
        } finally {
            if (seq === accountsByProviderSeq.current) setAccountsByProviderLoading(false);
        }
    }, []);

    useEffect(() => {
        loadRosters(range);
    }, [loadRosters, range]);

    const rows = useMemo<UserUsageRow[]>(() => {
        const statsByUser = new Map(
            userStats.map((stat) => [stat.user_id || stat.key, stat]),
        );
        return tokens.map((token) => {
            const stat = statsByUser.get(token.user_id);
            return {
                ...token,
                display_name: token.account_type === 'primary'
                    ? t('dashboard.userUsage.primaryAccount', { defaultValue: 'Primary account' })
                    : token.display_name,
                request_count: stat?.request_count || 0,
                // total_tokens is derived via the shared helper, not read from
                // the API's total_tokens field (input+output only, excludes
                // cache — see .design/stream-usage-tracking.md).
                total_tokens: getTotalTokens(stat ?? {}),
                total_input_tokens: stat?.total_input_tokens || 0,
                total_output_tokens: stat?.total_output_tokens || 0,
                cache_read_tokens: stat?.cache_read_tokens || 0,
                cache_write_tokens: stat?.cache_write_tokens || 0,
                error_count: stat?.error_count || 0,
                error_rate: stat?.error_rate || 0,
            };
        });
    }, [t, tokens, userStats]);

    const tokenByUserID = useMemo(
        () => new Map(tokens.map((token) => [token.user_id, token])),
        [tokens],
    );
    const accountDisplayName = useCallback((userID: string) => {
        const token = tokenByUserID.get(userID);
        if (token?.account_type === 'primary') {
            return t('dashboard.userUsage.primaryAccount', { defaultValue: 'Primary account' });
        }
        return token?.display_name || userID;
    }, [t, tokenByUserID]);

    const visibleAccountRows = useMemo(() => filterAndSort(
        rows,
        accountSearch,
        (row) => [row.display_name, row.user_id],
        accountSortField,
        accountSortDirection,
        (row) => row.display_name,
    ), [rows, accountSearch, accountSortField, accountSortDirection]);

    const visibleModelRows = useMemo(() => filterAndSort(
        modelRoster,
        modelSearch,
        (row) => [row.model || row.key, row.provider_name || ''],
        modelSortField,
        modelSortDirection,
        (row) => row.model || row.key,
    ), [modelRoster, modelSearch, modelSortField, modelSortDirection]);

    const visibleProviderRows = useMemo(() => filterAndSort(
        providerRoster,
        providerSearch,
        (row) => [row.provider_name || row.key],
        providerSortField,
        providerSortDirection,
        (row) => row.provider_name || row.key,
    ), [providerRoster, providerSearch, providerSortField, providerSortDirection]);

    const pagedAccountRows = useMemo(
        () => visibleAccountRows.slice(accountPage * rowsPerPage, accountPage * rowsPerPage + rowsPerPage),
        [visibleAccountRows, accountPage, rowsPerPage],
    );
    const pagedModelRows = useMemo(
        () => visibleModelRows.slice(modelPage * rowsPerPage, modelPage * rowsPerPage + rowsPerPage),
        [visibleModelRows, modelPage, rowsPerPage],
    );
    const pagedProviderRows = useMemo(
        () => visibleProviderRows.slice(providerPage * rowsPerPage, providerPage * rowsPerPage + rowsPerPage),
        [visibleProviderRows, providerPage, rowsPerPage],
    );

    // Per-axis roster state, looked up by the active view mode so the render
    // below doesn't need a three-way ternary at every use site.
    const rosterByMode: Record<ViewMode, {
        visibleLength: number;
        page: number;
        setPage: (page: number) => void;
        sortField: SortField;
        sortDirection: SortDirection;
        setSortField: (field: SortField) => void;
        setSortDirection: (updater: (direction: SortDirection) => SortDirection) => void;
    }> = {
        account: {
            visibleLength: visibleAccountRows.length,
            page: accountPage,
            setPage: setAccountPage,
            sortField: accountSortField,
            sortDirection: accountSortDirection,
            setSortField: setAccountSortField,
            setSortDirection: setAccountSortDirection,
        },
        model: {
            visibleLength: visibleModelRows.length,
            page: modelPage,
            setPage: setModelPage,
            sortField: modelSortField,
            sortDirection: modelSortDirection,
            setSortField: setModelSortField,
            setSortDirection: setModelSortDirection,
        },
        provider: {
            visibleLength: visibleProviderRows.length,
            page: providerPage,
            setPage: setProviderPage,
            sortField: providerSortField,
            sortDirection: providerSortDirection,
            setSortField: setProviderSortField,
            setSortDirection: setProviderSortDirection,
        },
    };
    const activeRoster = rosterByMode[viewMode];

    const handleSort = (field: SortField) => {
        const { sortField: currentField, setSortField, setSortDirection } = activeRoster;
        if (field === currentField) {
            setSortDirection((direction) => (direction === 'asc' ? 'desc' : 'asc'));
        } else {
            setSortField(field);
            setSortDirection(() => (field === 'name' ? 'asc' : 'desc'));
        }
    };

    // Filtering/sorting can shrink the result set below the current page.
    useEffect(() => {
        setAccountPage(0);
    }, [accountSearch, accountSortField, accountSortDirection, range]);
    useEffect(() => {
        setModelPage(0);
    }, [modelSearch, modelSortField, modelSortDirection, range]);
    useEffect(() => {
        setProviderPage(0);
    }, [providerSearch, providerSortField, providerSortDirection, range]);

    useEffect(() => {
        if (visibleAccountRows.length === 0) {
            setSelectedUserID('');
            return;
        }
        if (!visibleAccountRows.some((row) => row.user_id === selectedUserID)) {
            setSelectedUserID(visibleAccountRows[0].user_id);
        }
    }, [rows, selectedUserID, visibleAccountRows]);

    useEffect(() => {
        if (visibleModelRows.length === 0) {
            setSelectedModelKey('');
            return;
        }
        if (!visibleModelRows.some((row) => getModelKey(row) === selectedModelKey)) {
            setSelectedModelKey(getModelKey(visibleModelRows[0]));
        }
    }, [modelRoster, selectedModelKey, visibleModelRows]);

    useEffect(() => {
        if (visibleProviderRows.length === 0) {
            setSelectedProviderKey('');
            return;
        }
        if (!visibleProviderRows.some((row) => getProviderKey(row) === selectedProviderKey)) {
            setSelectedProviderKey(getProviderKey(visibleProviderRows[0]));
        }
    }, [providerRoster, selectedProviderKey, visibleProviderRows]);

    useEffect(() => {
        loadModelDetail(selectedUserID, range);
    }, [loadModelDetail, range, selectedUserID]);

    useEffect(() => {
        const model = modelRoster.find((item) => getModelKey(item) === selectedModelKey);
        loadAccountsByModel(model, range);
    }, [loadAccountsByModel, modelRoster, range, selectedModelKey]);

    useEffect(() => {
        const provider = providerRoster.find((item) => getProviderKey(item) === selectedProviderKey);
        loadAccountsByProvider(provider, range);
    }, [loadAccountsByProvider, providerRoster, range, selectedProviderKey]);

    const selectedUser = useMemo(
        () => rows.find((row) => row.user_id === selectedUserID),
        [rows, selectedUserID],
    );
    const selectedModel = useMemo(
        () => modelRoster.find((row) => getModelKey(row) === selectedModelKey),
        [modelRoster, selectedModelKey],
    );
    const selectedProvider = useMemo(
        () => providerRoster.find((row) => getProviderKey(row) === selectedProviderKey),
        [providerRoster, selectedProviderKey],
    );
    const detailSubject: MetricRow | undefined = viewMode === 'account'
        ? selectedUser
        : viewMode === 'model'
        ? selectedModel
        : selectedProvider;

    // Single pass over the active axis' rows for every summary aggregate.
    const summary = useMemo(() => {
        const source = viewMode === 'account' ? rows : viewMode === 'model' ? modelRoster : providerRoster;
        return computeUsageSummary(source);
    }, [rows, modelRoster, providerRoster, viewMode]);
    const {
        tokens: totalTokens,
        inputTokens: totalInputTokens,
        outputTokens: totalOutputTokens,
        cacheTokens: totalCacheTokens,
        cacheWriteTokens: totalCacheWriteTokens,
        requests: totalRequests,
        errors: totalErrors,
        cacheHitRate,
        errorRate,
    } = summary;
    const activeAccounts = useMemo(() => rows.filter((row) => row.request_count > 0).length, [rows]);
    const providerCount = useMemo(
        () => new Set(modelRoster.map((row) => row.provider_uuid || row.provider_name || row.key)).size,
        [modelRoster],
    );
    const showAccountCacheWrite = hasCacheWrites(rows);
    const showModelRosterCacheWrite = hasCacheWrites(modelRoster);
    const showProviderRosterCacheWrite = hasCacheWrites(providerRoster);
    const showModelDetailCacheWrite = hasCacheWrites(modelStats);

    const primarySummaryItem = viewMode === 'account'
        ? {
            label: t('dashboard.userUsage.registeredUsers', { defaultValue: 'Registered users' }),
            value: String(rows.length),
            hint: t('dashboard.userUsage.activeUsers', {
                count: activeAccounts,
                defaultValue: `${activeAccounts} active in this period`,
            }),
            icon: <Users />,
            color: 'primary' as const,
        }
        : viewMode === 'model'
        ? {
            label: t('dashboard.userUsage.modelsUsed', { defaultValue: 'Models used' }),
            value: String(modelRoster.length),
            hint: t('dashboard.userUsage.acrossProviders', {
                count: providerCount,
                defaultValue: `Across ${providerCount} provider${providerCount === 1 ? '' : 's'}`,
            }),
            icon: <Server />,
            color: 'primary' as const,
        }
        : {
            label: t('dashboard.userUsage.providersUsed', { defaultValue: 'Providers used' }),
            value: String(providerRoster.length),
            hint: t('dashboard.userUsage.acrossModels', {
                count: modelRoster.length,
                defaultValue: `Across ${modelRoster.length} model${modelRoster.length === 1 ? '' : 's'}`,
            }),
            icon: <Cloud />,
            color: 'primary' as const,
        };
    const summaryItems = [
        primarySummaryItem,
        {
            label: t('dashboard.userUsage.totalTokens', { defaultValue: 'Total tokens' }),
            value: formatNumber(totalTokens),
            hint: t('dashboard.userUsage.tokenBreakdown', {
                cache: formatNumber(totalCacheTokens),
                input: formatNumber(totalInputTokens),
                output: formatNumber(totalOutputTokens),
                defaultValue: `Cache: ${formatNumber(totalCacheTokens)} · Input: ${formatNumber(totalInputTokens)} · Output: ${formatNumber(totalOutputTokens)}`,
            }),
            icon: <Token />,
            color: 'secondary' as const,
        },
        {
            label: t('dashboard.userUsage.cacheHitRate', { defaultValue: 'Cache hit rate' }),
            value: `${cacheHitRate.toFixed(1)}%`,
            // Already a fully composed string; wrapping it in t() would be an
            // identity call against a key that does not exist.
            hint: formatCacheBreakdown(totalCacheTokens, totalCacheWriteTokens, formatNumber),
            icon: <CachedIcon />,
            color: getCacheHitRateColor(cacheHitRate),
        },
        {
            label: t('dashboard.userUsage.requests', { defaultValue: 'Requests' }),
            value: formatNumber(totalRequests),
            hint: t('dashboard.userUsage.averagePerUser', {
                value: activeAccounts ? formatNumber(Math.round(totalRequests / activeAccounts)) : '0',
                defaultValue: `${activeAccounts ? formatNumber(Math.round(totalRequests / activeAccounts)) : '0'} per active user`,
            }),
            icon: <BarChart />,
            color: 'secondary' as const,
        },
        {
            label: t('dashboard.userUsage.errors', { defaultValue: 'Errors' }),
            value: formatNumber(totalErrors),
            hint: `${errorRate.toFixed(1)}%`,
            icon: <ErrorOutline />,
            color: getErrorRateColor(errorRate),
        },
    ];

    const usageMetricLabels: UsageMetricLabels = {
        requests: t('dashboard.userUsage.requests', { defaultValue: 'Requests' }),
        total: t('dashboard.userUsage.total', { defaultValue: 'Total' }),
        cacheRead: t('dashboard.userUsage.cacheRead', { defaultValue: 'Cache Read' }),
        cacheWrite: t('dashboard.userUsage.cacheWrite', { defaultValue: 'Cache Write' }),
        cacheHit: t('dashboard.userUsage.cacheHit', { defaultValue: 'Cache Hit' }),
        input: t('dashboard.userUsage.input', { defaultValue: 'Input' }),
        output: t('dashboard.userUsage.output', { defaultValue: 'Output' }),
        reasoning: t('dashboard.userUsage.reasoning', { defaultValue: 'Reasoning' }),
        errorRate: t('dashboard.userUsage.errorRate', { defaultValue: 'Error rate' }),
    };
    type PrimaryColumn = { field: SortField; label: string; align?: 'right'; defaultDir: SortDirection };
    const metricColumns = (showCacheWrite: boolean): PrimaryColumn[] => getUsageMetricColumns({
        showTotal: true,
        showCacheWrite,
    }, usageMetricLabels).map((column) => ({
        field: column.key as SortField,
        label: column.label,
        align: 'right' as const,
        defaultDir: 'desc' as const,
    }));
    const accountColumns: PrimaryColumn[] = [
        { field: 'name', label: t('dashboard.userUsage.user', { defaultValue: 'User' }), defaultDir: 'asc' },
        ...metricColumns(showAccountCacheWrite),
    ];
    const modelColumns: PrimaryColumn[] = [
        { field: 'name', label: t('dashboard.userUsage.model', { defaultValue: 'Model' }), defaultDir: 'asc' },
        ...metricColumns(showModelRosterCacheWrite),
    ];
    const providerColumns: PrimaryColumn[] = [
        { field: 'name', label: t('dashboard.userUsage.provider', { defaultValue: 'Provider' }), defaultDir: 'asc' },
        ...metricColumns(showProviderRosterCacheWrite),
    ];
    const primaryColumns = viewMode === 'account' ? accountColumns : viewMode === 'model' ? modelColumns : providerColumns;
    const { sortField, sortDirection } = activeRoster;

    const scrollDetailIntoView = () => {
        requestAnimationFrame(() => {
            detailPanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
        });
    };
    const handleSelectUser = (userID: string) => {
        setSelectedUserID(userID);
        scrollDetailIntoView();
    };
    const handleSelectModel = (modelKey: string) => {
        setSelectedModelKey(modelKey);
        scrollDetailIntoView();
    };
    const handleSelectProvider = (providerKey: string) => {
        setSelectedProviderKey(providerKey);
        scrollDetailIntoView();
    };

    if (loading) return <UserUsageSkeleton />;

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
            <PageHeader
                title={t('dashboard.userUsage.title', { defaultValue: 'Team usage' })}
                subtitle={t('dashboard.userUsage.subtitle', {
                    defaultValue: 'See how every registered user is consuming shared AI access.',
                })}
                actions={
                    <>
                        <ToggleButtonGroup
                            size="small"
                            exclusive
                            value={viewMode}
                            onChange={(_, value: ViewMode | null) => value && setViewMode(value)}
                            aria-label={t('dashboard.userUsage.viewMode', { defaultValue: 'View' })}
                        >
                            <ToggleButton value="account">
                                {t('dashboard.userUsage.byAccount', { defaultValue: 'By account' })}
                            </ToggleButton>
                            <ToggleButton value="model">
                                {t('dashboard.userUsage.byModel', { defaultValue: 'By model' })}
                            </ToggleButton>
                            <ToggleButton value="provider">
                                {t('dashboard.userUsage.byProvider', { defaultValue: 'By provider' })}
                            </ToggleButton>
                        </ToggleButtonGroup>
                        <ToggleButtonGroup
                            size="small"
                            exclusive
                            value={range}
                            onChange={(_, value: TimeRange | null) => value && setRange(value)}
                            aria-label={t('dashboard.userUsage.timeRange', { defaultValue: 'Time range' })}
                        >
                            <ToggleButton value="today">{t('layout.today')}</ToggleButton>
                            <ToggleButton value="7d">7D</ToggleButton>
                            <ToggleButton value="30d">30D</ToggleButton>
                            <ToggleButton value="90d">90D</ToggleButton>
                        </ToggleButtonGroup>
                        <Tooltip title={t('common.refresh', { defaultValue: 'Refresh' })}>
                            <span>
                                <IconButton
                                    onClick={() => loadRosters(range, true)}
                                    disabled={refreshing}
                                    aria-label={t('common.refresh', { defaultValue: 'Refresh' })}
                                >
                                    {refreshing ? <CircularProgress size={20} /> : <Refresh />}
                                </IconButton>
                            </span>
                        </Tooltip>
                    </>
                }
            />

            {error && <Alert severity="error">{error}</Alert>}

            <Grid container spacing={{ xs: 1.5, sm: 2 }}>
                {summaryItems.map((item) => (
                    <Grid key={item.label} size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={item.label}
                            value={item.value}
                            subtitle={item.hint}
                            icon={item.icon}
                            color={item.color}
                        />
                    </Grid>
                ))}
            </Grid>

            <Grid container spacing={2} sx={{ alignItems: 'stretch' }}>
                <Grid size={{ xs: 12 }} sx={{ display: 'flex' }}>
                    <Paper
                        elevation={0}
                        sx={{ ...usageTableCardSx, display: 'flex', flexDirection: 'column' }}
                    >
                        <Box
                            sx={{
                                p: 2.5,
                                display: 'flex',
                                flexWrap: 'wrap',
                                alignItems: 'center',
                                justifyContent: 'space-between',
                                gap: 1.5,
                                borderBottom: '1px solid',
                                borderColor: 'divider',
                            }}
                        >
                            <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                                    {viewMode === 'account'
                                        ? t('dashboard.userUsage.allUsers', { defaultValue: 'All registered users' })
                                        : viewMode === 'model'
                                        ? t('dashboard.userUsage.allModels', { defaultValue: 'All models' })
                                        : t('dashboard.userUsage.allProviders', { defaultValue: 'All providers' })}
                                </Typography>
                                <Chip
                                    size="small"
                                    label={activeRoster.visibleLength}
                                    sx={{ height: 22 }}
                                />
                            </Stack>
                            <SearchField
                                value={viewMode === 'account' ? accountSearch : viewMode === 'model' ? modelSearch : providerSearch}
                                onChange={(event) => {
                                    if (viewMode === 'account') setAccountSearch(event.target.value);
                                    else if (viewMode === 'model') setModelSearch(event.target.value);
                                    else setProviderSearch(event.target.value);
                                }}
                                placeholder={viewMode === 'account'
                                    ? t('dashboard.userUsage.search', { defaultValue: 'Search users' })
                                    : viewMode === 'model'
                                    ? t('dashboard.userUsage.searchModels', { defaultValue: 'Search models' })
                                    : t('dashboard.userUsage.searchProviders', { defaultValue: 'Search providers' })}
                                sx={{ width: { xs: '100%', sm: 220 } }}
                            />
                        </Box>
                        <TableContainer
                            sx={{
                                maxHeight: 520,
                                overscrollBehavior: 'contain',
                            }}
                        >
                            <Table stickyHeader sx={{ minWidth: (
                                viewMode === 'account' ? showAccountCacheWrite
                                    : viewMode === 'model' ? showModelRosterCacheWrite
                                    : showProviderRosterCacheWrite
                            ) ? 1080 : 980 }}
                            >
                                <TableHead>
                                    <TableRow
                                        sx={{
                                            backgroundColor: alpha(theme.palette.background.paper, 0.8),
                                            '& .MuiTableCell-root': {
                                                fontWeight: 600,
                                                fontSize: '0.75rem',
                                                textTransform: 'uppercase',
                                                letterSpacing: '0.05em',
                                                color: 'text.secondary',
                                                py: 1.25,
                                                borderBottom: '1px solid',
                                                borderColor: 'divider',
                                            },
                                        }}
                                    >
                                        {viewMode === 'model' && (
                                            <TableCell>{t('dashboard.userUsage.provider', { defaultValue: 'Provider' })}</TableCell>
                                        )}
                                        {primaryColumns.map((col) => (
                                            <TableCell
                                                key={col.field}
                                                align={col.align}
                                                sortDirection={sortField === col.field ? sortDirection : false}
                                            >
                                                <TableSortLabel
                                                    active={sortField === col.field}
                                                    direction={sortField === col.field ? sortDirection : col.defaultDir}
                                                    onClick={() => handleSort(col.field)}
                                                >
                                                    {col.label}
                                                </TableSortLabel>
                                            </TableCell>
                                        ))}
                                        <TableCell padding="checkbox" />
                                    </TableRow>
                                </TableHead>
                                <TableBody>
                                    {viewMode === 'account' && pagedAccountRows.map((row) => {
                                        const selected = row.user_id === selectedUserID;
                                        return (
                                            <TableRow
                                                key={row.token_id}
                                                hover
                                                selected={selected}
                                                onClick={() => handleSelectUser(row.user_id)}
                                                sx={{
                                                    cursor: 'pointer',
                                                    position: 'relative',
                                                    transition: 'background-color 0.15s ease',
                                                    '& .MuiTableCell-root': {
                                                        py: 1.25,
                                                        borderBottom: '1px solid',
                                                        borderColor: 'divider',
                                                    },
                                                    '&.Mui-selected': {
                                                        bgcolor: alpha(theme.palette.primary.main, 0.08),
                                                        boxShadow: `inset 3px 0 0 ${theme.palette.primary.main}`,
                                                        '&:hover': { bgcolor: alpha(theme.palette.primary.main, 0.12) },
                                                    },
                                                }}
                                            >
                                                <TableCell>
                                                    <Stack direction="row" spacing={1.25} sx={{ alignItems: 'center' }}>
                                                        <Avatar sx={{
                                                            width: 34,
                                                            height: 34,
                                                            fontSize: 14,
                                                            bgcolor: selected ? 'primary.main' : alpha(theme.palette.primary.main, 0.1),
                                                            color: selected ? 'primary.contrastText' : 'primary.main',
                                                        }}>
                                                            {row.display_name.slice(0, 1).toUpperCase()}
                                                        </Avatar>
                                                        <Box sx={{ minWidth: 0 }}>
                                                            <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                                                <Typography
                                                                    variant="body1"
                                                                    noWrap
                                                                    sx={{ color: 'text.primary', fontWeight: 600 }}
                                                                >
                                                                    {row.display_name}
                                                                </Typography>
                                                                {row.account_type === 'primary' && (
                                                                    <Chip
                                                                        size="small"
                                                                        color="primary"
                                                                        variant="outlined"
                                                                        label={t('dashboard.userUsage.primary', { defaultValue: 'Primary' })}
                                                                        sx={{ height: 22 }}
                                                                    />
                                                                )}
                                                                {!row.enabled && (
                                                                    <Chip
                                                                        size="small"
                                                                        label={t('dashboard.userUsage.disabled', { defaultValue: 'Disabled' })}
                                                                        sx={{ height: 22 }}
                                                                    />
                                                                )}
                                                            </Stack>
                                                            <Typography variant="body2">
                                                                {row.account_type === 'primary'
                                                                    ? t('dashboard.userUsage.globalToken', { defaultValue: 'Global model token' })
                                                                    : row.last_used_at
                                                                    ? t('dashboard.userUsage.lastUsed', {
                                                                        value: formatDateTime(row.last_used_at),
                                                                        defaultValue: `Last used ${formatDateTime(row.last_used_at)}`,
                                                                    })
                                                                    : t('dashboard.userUsage.neverUsed', { defaultValue: 'Never used' })}
                                                            </Typography>
                                                        </Box>
                                                    </Stack>
                                                </TableCell>
                                                <UsageMetricValueCells
                                                    usage={row}
                                                    showTotal
                                                    showCacheWrite={showAccountCacheWrite}
                                                />
                                                <TableCell padding="checkbox">
                                                    <ArrowForward sx={{ fontSize: 18, opacity: selected ? 1 : 0.22 }} color={selected ? 'primary' : 'inherit'} />
                                                </TableCell>
                                            </TableRow>
                                        );
                                    })}
                                    {viewMode === 'model' && pagedModelRows.map((row) => {
                                        const key = getModelKey(row);
                                        const selected = key === selectedModelKey;
                                        return (
                                            <TableRow
                                                key={key}
                                                hover
                                                selected={selected}
                                                onClick={() => handleSelectModel(key)}
                                                sx={{
                                                    cursor: 'pointer',
                                                    position: 'relative',
                                                    transition: 'background-color 0.15s ease',
                                                    '& .MuiTableCell-root': {
                                                        py: 1.25,
                                                        borderBottom: '1px solid',
                                                        borderColor: 'divider',
                                                    },
                                                    '&.Mui-selected': {
                                                        bgcolor: alpha(theme.palette.primary.main, 0.08),
                                                        boxShadow: `inset 3px 0 0 ${theme.palette.primary.main}`,
                                                        '&:hover': { bgcolor: alpha(theme.palette.primary.main, 0.12) },
                                                    },
                                                }}
                                            >
                                                <TableCell>{row.provider_name || '—'}</TableCell>
                                                <TableCell sx={{ fontWeight: 600 }}>{row.model || row.key}</TableCell>
                                                <UsageMetricValueCells
                                                    usage={row}
                                                    showTotal
                                                    showCacheWrite={showModelRosterCacheWrite}
                                                />
                                                <TableCell padding="checkbox">
                                                    <ArrowForward sx={{ fontSize: 18, opacity: selected ? 1 : 0.22 }} color={selected ? 'primary' : 'inherit'} />
                                                </TableCell>
                                            </TableRow>
                                        );
                                    })}
                                    {viewMode === 'provider' && pagedProviderRows.map((row) => {
                                        const key = getProviderKey(row);
                                        const selected = key === selectedProviderKey;
                                        return (
                                            <TableRow
                                                key={key}
                                                hover
                                                selected={selected}
                                                onClick={() => handleSelectProvider(key)}
                                                sx={{
                                                    cursor: 'pointer',
                                                    position: 'relative',
                                                    transition: 'background-color 0.15s ease',
                                                    '& .MuiTableCell-root': {
                                                        py: 1.25,
                                                        borderBottom: '1px solid',
                                                        borderColor: 'divider',
                                                    },
                                                    '&.Mui-selected': {
                                                        bgcolor: alpha(theme.palette.primary.main, 0.08),
                                                        boxShadow: `inset 3px 0 0 ${theme.palette.primary.main}`,
                                                        '&:hover': { bgcolor: alpha(theme.palette.primary.main, 0.12) },
                                                    },
                                                }}
                                            >
                                                <TableCell sx={{ fontWeight: 600 }}>{row.provider_name || row.key}</TableCell>
                                                <UsageMetricValueCells
                                                    usage={row}
                                                    showTotal
                                                    showCacheWrite={showProviderRosterCacheWrite}
                                                />
                                                <TableCell padding="checkbox">
                                                    <ArrowForward sx={{ fontSize: 18, opacity: selected ? 1 : 0.22 }} color={selected ? 'primary' : 'inherit'} />
                                                </TableCell>
                                            </TableRow>
                                        );
                                    })}
                                    {activeRoster.visibleLength === 0 && (
                                        <TableRow>
                                            <TableCell
                                                colSpan={primaryColumns.length + (viewMode === 'model' ? 2 : 1)}
                                                align="center"
                                                sx={{ py: 8 }}
                                            >
                                                <Typography variant="body1" sx={{ color: 'text.secondary' }}>
                                                    {viewMode === 'account'
                                                        ? t('dashboard.userUsage.noUsers', { defaultValue: 'No users match your search.' })
                                                        : viewMode === 'model'
                                                        ? t('dashboard.userUsage.noModels', { defaultValue: 'No models match your search.' })
                                                        : t('dashboard.userUsage.noProviders', { defaultValue: 'No providers match your search.' })}
                                                </Typography>
                                                <Typography variant="caption" sx={{ color: 'text.disabled', mt: 0.5, display: 'block' }}>
                                                    {t('dashboard.userUsage.noUsersHint', { defaultValue: 'Try a different search term or time range.' })}
                                                </Typography>
                                            </TableCell>
                                        </TableRow>
                                    )}
                                </TableBody>
                            </Table>
                        </TableContainer>
                        {activeRoster.visibleLength > 0 && (
                            <TablePagination
                                component="div"
                                count={activeRoster.visibleLength}
                                page={activeRoster.page}
                                onPageChange={(_, newPage) => activeRoster.setPage(newPage)}
                                rowsPerPage={rowsPerPage}
                                onRowsPerPageChange={(event) => {
                                    setRowsPerPage(parseInt(event.target.value, 10));
                                    setAccountPage(0);
                                    setModelPage(0);
                                    setProviderPage(0);
                                }}
                                rowsPerPageOptions={[5, 10, 25, 50]}
                                sx={{ borderTop: '1px solid', borderColor: 'divider', flexShrink: 0 }}
                            />
                        )}
                    </Paper>
                </Grid>

                <Grid
                    ref={detailPanelRef}
                    size={{ xs: 12 }}
                    sx={{ display: 'flex', scrollMarginTop: { xs: 72, lg: 0 } }}
                >
                    <Paper elevation={0} sx={{ ...usageTableCardSx, display: 'flex' }}>
                        <Box sx={{
                            p: { xs: 2, sm: 2.5 },
                            width: '100%',
                            display: 'flex',
                            flexDirection: 'column',
                        }}>
                        {detailSubject ? (
                            <Stack spacing={2.5}>
                                <Box>
                                    <Stack direction="row" spacing={2} sx={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
                                        <Box sx={{ minWidth: 0 }}>
                                            <Typography variant="h6" noWrap sx={{ fontWeight: 650 }}>
                                                {viewMode === 'account'
                                                    ? selectedUser!.display_name
                                                    : viewMode === 'model'
                                                    ? (selectedModel!.model || selectedModel!.key)
                                                    : (selectedProvider!.provider_name || selectedProvider!.key)}
                                            </Typography>
                                            {viewMode !== 'provider' && (
                                                <Typography variant="body2" sx={{ fontFamily: viewMode === 'account' ? 'monospace' : undefined }}>
                                                    {viewMode === 'account' ? selectedUser!.user_id : (selectedModel!.provider_name || '—')}
                                                </Typography>
                                            )}
                                        </Box>
                                        {viewMode === 'account' ? (
                                            selectedUser!.account_type === 'primary' ? (
                                                <Chip
                                                    size="small"
                                                    color="primary"
                                                    label={t('dashboard.userUsage.primaryAccount', { defaultValue: 'Primary account' })}
                                                    variant="outlined"
                                                />
                                            ) : (
                                                <Chip
                                                    size="small"
                                                    icon={selectedUser!.enabled ? <CheckCircle /> : <Block />}
                                                    color={selectedUser!.enabled ? 'success' : 'default'}
                                                    label={selectedUser!.enabled
                                                        ? t('dashboard.userUsage.enabled', { defaultValue: 'Enabled' })
                                                        : t('dashboard.userUsage.disabled', { defaultValue: 'Disabled' })}
                                                    variant="outlined"
                                                />
                                            )
                                        ) : (
                                            <Chip
                                                size="small"
                                                color="primary"
                                                variant="outlined"
                                                label={t('dashboard.userUsage.accountsUsingModel', {
                                                    count: viewMode === 'model' ? accountsByModel.length : accountsByProvider.length,
                                                    defaultValue: `${viewMode === 'model' ? accountsByModel.length : accountsByProvider.length} accounts`,
                                                })}
                                            />
                                        )}
                                    </Stack>
                                    {viewMode === 'account' && (
                                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={{ xs: 0.5, sm: 2 }} sx={{ mt: 1.25 }}>
                                            <Stack direction="row" spacing={0.6} sx={{ alignItems: 'center' }}>
                                                <AccessTime sx={{ fontSize: 17, color: 'text.secondary' }} />
                                                <Typography variant="body2">
                                                    {selectedUser!.account_type === 'primary'
                                                        ? t('dashboard.userUsage.globalTokenUsage', { defaultValue: 'Usage through the global model token' })
                                                        : selectedUser!.last_used_at
                                                        ? t('dashboard.userUsage.lastUsed', {
                                                            value: formatDateTime(selectedUser!.last_used_at),
                                                            defaultValue: `Last used ${formatDateTime(selectedUser!.last_used_at)}`,
                                                        })
                                                        : t('dashboard.userUsage.neverUsed', { defaultValue: 'Never used' })}
                                                </Typography>
                                            </Stack>
                                            {selectedUser!.created_at && (
                                                <Typography variant="body2">
                                                    {t('dashboard.userUsage.joined', {
                                                        value: formatDateTime(selectedUser!.created_at),
                                                        defaultValue: `Added ${formatDateTime(selectedUser!.created_at)}`,
                                                    })}
                                                </Typography>
                                            )}
                                        </Stack>
                                    )}
                                </Box>

                                <Grid
                                    container
                                    sx={{
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 1.5,
                                        overflow: 'hidden',
                                    }}
                                >
                                    {[
                                        {
                                            label: t('dashboard.userUsage.input', { defaultValue: 'Input' }),
                                            value: detailSubject.total_input_tokens,
                                            color: TOKEN_COLORS.input.main,
                                            detail: (detailSubject.cache_write_tokens || 0) > 0
                                                ? t('dashboard.userUsage.cacheWriteIncluded', {
                                                    value: formatNumber(detailSubject.cache_write_tokens || 0),
                                                    defaultValue: `incl. ${formatNumber(detailSubject.cache_write_tokens || 0)} written`,
                                                })
                                                : '',
                                        },
                                        {
                                            label: t('dashboard.userUsage.output', { defaultValue: 'Output' }),
                                            value: detailSubject.total_output_tokens,
                                            color: TOKEN_COLORS.output.main,
                                            detail: '',
                                        },
                                        {
                                            label: t('dashboard.userUsage.cacheRead', { defaultValue: 'Cache Read' }),
                                            value: detailSubject.cache_read_tokens || 0,
                                            color: TOKEN_COLORS.cache.main,
                                            detail: '',
                                        },
                                        {
                                            label: t('dashboard.userUsage.cacheHitRate', { defaultValue: 'Cache hit rate' }),
                                            value: `${getCacheHitRate(detailSubject.cache_read_tokens || 0, detailSubject.total_input_tokens).toFixed(1)}%`,
                                            color: theme.palette[getCacheHitRateColor(getCacheHitRate(detailSubject.cache_read_tokens || 0, detailSubject.total_input_tokens))].main,
                                            detail: '',
                                        },
                                    ].map(({ label, value, color, detail }, index) => (
                                        <Grid
                                            key={label}
                                            size={{ xs: 6, sm: 3 }}
                                            sx={{
                                                borderColor: 'divider',
                                                borderRight: {
                                                    xs: index % 2 === 0 ? '1px solid' : 0,
                                                    sm: index < 3 ? '1px solid' : 0,
                                                },
                                                borderBottom: {
                                                    xs: index < 2 ? '1px solid' : 0,
                                                    sm: 0,
                                                },
                                            }}
                                        >
                                            <Box sx={{ px: 1.5, py: 1.25 }}>
                                                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                                    <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: color, flexShrink: 0 }} />
                                                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>{label}</Typography>
                                                </Stack>
                                                <Typography variant="h4" sx={{ fontVariantNumeric: 'tabular-nums', mt: 0.5 }}>
                                                    {typeof value === 'number' ? formatNumber(value) : value}
                                                </Typography>
                                                {detail && (
                                                    <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mt: 0.25 }}>
                                                        {detail}
                                                    </Typography>
                                                )}
                                            </Box>
                                        </Grid>
                                    ))}
                                </Grid>

                                <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                                    <Stack direction="row" sx={{ mb: 1.25, justifyContent: 'space-between', alignItems: 'baseline' }}>
                                        <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                            <Typography variant="subtitle2" sx={{ fontWeight: 650 }}>
                                                {viewMode === 'account'
                                                    ? t('dashboard.userUsage.allModels', { defaultValue: 'All models' })
                                                    : viewMode === 'model'
                                                    ? t('dashboard.userUsage.accountsUsingModelTitle', { defaultValue: 'Accounts using this model' })
                                                    : t('dashboard.userUsage.accountsUsingProviderTitle', { defaultValue: 'Accounts using this provider' })}
                                            </Typography>
                                            <Chip
                                                size="small"
                                                label={viewMode === 'account' ? modelStats.length : viewMode === 'model' ? accountsByModel.length : accountsByProvider.length}
                                                sx={{ height: 22 }}
                                            />
                                        </Stack>
                                        <Typography variant="body2">
                                            {formatNumber(getTotalTokens(detailSubject))} {t('dashboard.userUsage.tokens', { defaultValue: 'tokens' }).toLocaleLowerCase()}
                                        </Typography>
                                    </Stack>
                                    {(viewMode === 'account' ? modelDetailLoading : viewMode === 'model' ? accountsByModelLoading : accountsByProviderLoading) ? (
                                        <Stack spacing={1.5} sx={{ overflow: 'hidden' }}>
                                            {Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} variant="rounded" height={44} />)}
                                        </Stack>
                                    ) : viewMode === 'account' ? (
                                        modelStats.length > 0 ? (
                                            <TableContainer
                                                sx={{
                                                    maxHeight: 520,
                                                    border: '1px solid',
                                                    borderColor: 'divider',
                                                    borderRadius: 1.5,
                                                    overscrollBehavior: 'contain',
                                                }}
                                                role="region"
                                                aria-label={t('dashboard.userUsage.allModels', { defaultValue: 'All models' })}
                                            >
                                                <Table stickyHeader sx={{ minWidth: showModelDetailCacheWrite ? 1060 : 960 }}>
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
                                                            <TableCell>{t('dashboard.userUsage.provider', { defaultValue: 'Provider' })}</TableCell>
                                                            <TableCell>{t('dashboard.userUsage.model', { defaultValue: 'Model' })}</TableCell>
                                                            <UsageMetricHeaderCells
                                                                labels={usageMetricLabels}
                                                                showTotal
                                                                showCacheWrite={showModelDetailCacheWrite}
                                                            />
                                                        </TableRow>
                                                    </TableHead>
                                                    <TableBody>
                                                        {modelStats.map((model) => (
                                                            <TableRow
                                                                key={`${model.provider_uuid}-${model.model || model.key}`}
                                                                hover
                                                                sx={{
                                                                    '& .MuiTableCell-root': {
                                                                        py: 1.25,
                                                                        borderBottom: '1px solid',
                                                                        borderColor: 'divider',
                                                                    },
                                                                }}
                                                            >
                                                                <TableCell>{model.provider_name || '—'}</TableCell>
                                                                <TableCell sx={{ fontWeight: 600 }}>{model.model || model.key}</TableCell>
                                                                <UsageMetricValueCells
                                                                    usage={model}
                                                                    showTotal
                                                                    showCacheWrite={showModelDetailCacheWrite}
                                                                />
                                                            </TableRow>
                                                        ))}
                                                    </TableBody>
                                                </Table>
                                            </TableContainer>
                                        ) : (
                                            <Box sx={{ py: 4, textAlign: 'center', bgcolor: 'action.hover', borderRadius: 1.5 }}>
                                                <Typography variant="body1">
                                                    {t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                                                </Typography>
                                                <Typography variant="body2">
                                                    {t('dashboard.userUsage.noUsageHint', { defaultValue: 'The user remains listed because their access is registered.' })}
                                                </Typography>
                                            </Box>
                                        )
                                    ) : viewMode === 'model' ? (
                                        <AccountsBreakdownTable
                                            accounts={accountsByModel}
                                            ariaLabel={t('dashboard.userUsage.accountsUsingModelTitle', { defaultValue: 'Accounts using this model' })}
                                            identityLabel={t('dashboard.userUsage.user', { defaultValue: 'User' })}
                                            noUsageLabel={t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                                            accountDisplayName={accountDisplayName}
                                            usageMetricLabels={usageMetricLabels}
                                        />
                                    ) : (
                                        <AccountsBreakdownTable
                                            accounts={accountsByProvider}
                                            ariaLabel={t('dashboard.userUsage.accountsUsingProviderTitle', { defaultValue: 'Accounts using this provider' })}
                                            identityLabel={t('dashboard.userUsage.user', { defaultValue: 'User' })}
                                            noUsageLabel={t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                                            accountDisplayName={accountDisplayName}
                                            usageMetricLabels={usageMetricLabels}
                                        />
                                    )}
                                </Box>
                            </Stack>
                        ) : (
                            <Box sx={{ height: '100%', display: 'grid', placeItems: 'center', textAlign: 'center' }}>
                                <Box>
                                    {viewMode === 'account'
                                        ? <Users sx={{ fontSize: 42, color: 'text.disabled', mb: 1 }} />
                                        : viewMode === 'model'
                                        ? <Server sx={{ fontSize: 42, color: 'text.disabled', mb: 1 }} />
                                        : <Cloud sx={{ fontSize: 42, color: 'text.disabled', mb: 1 }} />}
                                    <Typography variant="body1">
                                        {viewMode === 'account'
                                            ? t('dashboard.userUsage.selectUser', { defaultValue: 'Select a user to see details.' })
                                            : viewMode === 'model'
                                            ? t('dashboard.userUsage.selectModel', { defaultValue: 'Select a model to see details.' })
                                            : t('dashboard.userUsage.selectProvider', { defaultValue: 'Select a provider to see details.' })}
                                    </Typography>
                                </Box>
                            </Box>
                        )}
                        </Box>
                    </Paper>
                </Grid>
            </Grid>
        </Box>
    );
}
