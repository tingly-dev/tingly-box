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
    UsageMetricValueCells,
    computeUsageSummary,
    useRosterAxis,
    RosterBreakdownTable,
} from '@/components/dashboard';
import type { AggregatedStat, MetricRow, SortField, SortDirection, UsageMetricLabels } from '@/components/dashboard';
import api from '@/services/api';

type TimeRange = 'today' | '7d' | '30d' | '90d';
type ViewMode = 'account' | 'model' | 'provider';

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

const accountKey = (row: UserUsageRow) => row.user_id;
const accountName = (row: UserUsageRow) => row.display_name;
const accountSearchText = (row: UserUsageRow) => [row.display_name, row.user_id];
const modelName = (row: AggregatedStat) => row.model || row.key;
const modelSearchText = (row: AggregatedStat) => [row.model || row.key, row.provider_name || ''];
const providerName = (row: AggregatedStat) => row.provider_name || row.key;
const providerSearchText = (row: AggregatedStat) => [row.provider_name || row.key];

// The three "detail" loaders below are stable module-level functions (not
// closures), so useRosterAxis's effect dependency on `loadDetail` never
// changes identity across renders — no useCallback needed at the call site.
async function fetchModelsForAccount(selected: UserUsageRow | undefined, range: TimeRange): Promise<AggregatedStat[]> {
    if (!selected) return [];
    const result = await api.getUsageStats({
        ...buildTimeParams(range),
        user_id: selected.user_id,
        group_by: 'model',
        sort_by: 'total_tokens',
        sort_order: 'desc',
        limit: 1000,
    });
    return result?.data || [];
}

// Which accounts used this model — scoped by provider + model together, not
// model name alone, since the same model name can exist under more than one
// provider.
async function fetchAccountsForModel(selected: AggregatedStat | undefined, range: TimeRange): Promise<AggregatedStat[]> {
    if (!selected) return [];
    const result = await api.getUsageStats({
        ...buildTimeParams(range),
        model: selected.model,
        provider: selected.provider_uuid,
        group_by: 'user',
        sort_by: 'total_tokens',
        sort_order: 'desc',
        limit: 500,
    });
    return result?.data || [];
}

// Which accounts used this provider (across all its models).
async function fetchAccountsForProvider(selected: AggregatedStat | undefined, range: TimeRange): Promise<AggregatedStat[]> {
    if (!selected) return [];
    const result = await api.getUsageStats({
        ...buildTimeParams(range),
        provider: selected.provider_uuid,
        group_by: 'user',
        sort_by: 'total_tokens',
        sort_order: 'desc',
        limit: 500,
    });
    return result?.data || [];
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

// Identity title/subtitle/status-chip block for the detail panel. Isolated
// into its own component because the three axes' identity shapes genuinely
// differ (account: name + user_id + enabled/primary + last-used/joined;
// model: name + provider name; provider: name only) — inlining it kept the
// page body dominated by one large three-way ternary.
function RosterDetailHeader({
    viewMode,
    selectedUser,
    selectedModel,
    selectedProvider,
    accountsCount,
    t,
}: {
    viewMode: ViewMode;
    selectedUser?: UserUsageRow;
    selectedModel?: AggregatedStat;
    selectedProvider?: AggregatedStat;
    accountsCount: number;
    t: (key: string, options?: Record<string, unknown>) => string;
}) {
    return (
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
                            count: accountsCount,
                            defaultValue: `${accountsCount} accounts`,
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
    const [rowsPerPage, setRowsPerPage] = useState(10);
    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [error, setError] = useState('');
    const requestSeq = useRef(0);
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

    // Each axis owns its own search/sort/pagination/selection/detail-load
    // lifecycle via the shared hook — see components/dashboard/RosterAxis.tsx.
    // All three run unconditionally (fixed hook-call count), so all three
    // rosters' detail queries are ready before the user ever toggles to them.
    const accountAxis = useRosterAxis<UserUsageRow, AggregatedStat, TimeRange>({
        roster: rows,
        getKey: accountKey,
        nameOf: accountName,
        searchText: accountSearchText,
        rowsPerPage,
        range,
        loadDetail: fetchModelsForAccount,
    });
    const modelAxis = useRosterAxis<AggregatedStat, AggregatedStat, TimeRange>({
        roster: modelRoster,
        getKey: getModelKey,
        nameOf: modelName,
        searchText: modelSearchText,
        rowsPerPage,
        range,
        loadDetail: fetchAccountsForModel,
    });
    const providerAxis = useRosterAxis<AggregatedStat, AggregatedStat, TimeRange>({
        roster: providerRoster,
        getKey: getProviderKey,
        nameOf: providerName,
        searchText: providerSearchText,
        rowsPerPage,
        range,
        loadDetail: fetchAccountsForProvider,
    });
    // Fields common to every axis (search/sort/page/handleSort/detailLoading)
    // are safe to read off this union without a three-way ternary at every
    // use site; axis-specific fields (selected/pagedRows/detail) are read
    // off the specific axis in the branch that renders them.
    const activeAxis = viewMode === 'account' ? accountAxis : viewMode === 'model' ? modelAxis : providerAxis;

    const selectedUser = accountAxis.selected;
    const selectedModel = modelAxis.selected;
    const selectedProvider = providerAxis.selected;
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
    // A column either sorts the roster ('sort') or is a plain, non-sortable
    // identity header ('label' — e.g. the Provider column on the model
    // roster, where only Model is sortable). Keeping non-sortable columns in
    // this same array means the header row and its colSpan never need a
    // separate special case for "the model axis has one extra column".
    type PrimaryColumn =
        | { kind: 'sort'; field: SortField; label: string; align?: 'right'; defaultDir: SortDirection }
        | { kind: 'label'; label: string };
    const metricColumns = (showCacheWrite: boolean): PrimaryColumn[] => getUsageMetricColumns({
        showTotal: true,
        showCacheWrite,
    }, usageMetricLabels).map((column) => ({
        kind: 'sort' as const,
        field: column.key as SortField,
        label: column.label,
        align: 'right' as const,
        defaultDir: 'desc' as const,
    }));
    const accountColumns: PrimaryColumn[] = [
        { kind: 'sort', field: 'name', label: t('dashboard.userUsage.user', { defaultValue: 'User' }), defaultDir: 'asc' },
        ...metricColumns(showAccountCacheWrite),
    ];
    const modelColumns: PrimaryColumn[] = [
        { kind: 'label', label: t('dashboard.userUsage.provider', { defaultValue: 'Provider' }) },
        { kind: 'sort', field: 'name', label: t('dashboard.userUsage.model', { defaultValue: 'Model' }), defaultDir: 'asc' },
        ...metricColumns(showModelRosterCacheWrite),
    ];
    const providerColumns: PrimaryColumn[] = [
        { kind: 'sort', field: 'name', label: t('dashboard.userUsage.provider', { defaultValue: 'Provider' }), defaultDir: 'asc' },
        ...metricColumns(showProviderRosterCacheWrite),
    ];
    const primaryColumns = viewMode === 'account' ? accountColumns : viewMode === 'model' ? modelColumns : providerColumns;

    const rosterRowSx = (selected: boolean) => ({
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
    });
    const selectionArrowSx = (selected: boolean) => ({ fontSize: 18, opacity: selected ? 1 : 0.22 });

    const scrollDetailIntoView = () => {
        requestAnimationFrame(() => {
            detailPanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
        });
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
                                    label={activeAxis.visibleRows.length}
                                    sx={{ height: 22 }}
                                />
                            </Stack>
                            <SearchField
                                value={activeAxis.search}
                                onChange={(event) => activeAxis.setSearch(event.target.value)}
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
                                        {primaryColumns.map((col, index) => (
                                            <TableCell
                                                key={col.kind === 'sort' ? col.field : `label-${index}`}
                                                align={col.kind === 'sort' ? col.align : undefined}
                                                sortDirection={col.kind === 'sort' && activeAxis.sortField === col.field ? activeAxis.sortDirection : false}
                                            >
                                                {col.kind === 'sort' ? (
                                                    <TableSortLabel
                                                        active={activeAxis.sortField === col.field}
                                                        direction={activeAxis.sortField === col.field ? activeAxis.sortDirection : col.defaultDir}
                                                        onClick={() => activeAxis.handleSort(col.field)}
                                                    >
                                                        {col.label}
                                                    </TableSortLabel>
                                                ) : col.label}
                                            </TableCell>
                                        ))}
                                        <TableCell padding="checkbox" />
                                    </TableRow>
                                </TableHead>
                                <TableBody>
                                    {viewMode === 'account' && accountAxis.pagedRows.map((row) => {
                                        const selected = row.user_id === accountAxis.selectedKey;
                                        return (
                                            <TableRow
                                                key={row.token_id}
                                                hover
                                                selected={selected}
                                                onClick={() => { accountAxis.setSelectedKey(row.user_id); scrollDetailIntoView(); }}
                                                sx={rosterRowSx(selected)}
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
                                                <UsageMetricValueCells usage={row} showTotal showCacheWrite={showAccountCacheWrite} />
                                                <TableCell padding="checkbox">
                                                    <ArrowForward sx={selectionArrowSx(selected)} color={selected ? 'primary' : 'inherit'} />
                                                </TableCell>
                                            </TableRow>
                                        );
                                    })}
                                    {viewMode === 'model' && modelAxis.pagedRows.map((row) => {
                                        const key = getModelKey(row);
                                        const selected = key === modelAxis.selectedKey;
                                        return (
                                            <TableRow
                                                key={key}
                                                hover
                                                selected={selected}
                                                onClick={() => { modelAxis.setSelectedKey(key); scrollDetailIntoView(); }}
                                                sx={rosterRowSx(selected)}
                                            >
                                                <TableCell>{row.provider_name || '—'}</TableCell>
                                                <TableCell sx={{ fontWeight: 600 }}>{row.model || row.key}</TableCell>
                                                <UsageMetricValueCells usage={row} showTotal showCacheWrite={showModelRosterCacheWrite} />
                                                <TableCell padding="checkbox">
                                                    <ArrowForward sx={selectionArrowSx(selected)} color={selected ? 'primary' : 'inherit'} />
                                                </TableCell>
                                            </TableRow>
                                        );
                                    })}
                                    {viewMode === 'provider' && providerAxis.pagedRows.map((row) => {
                                        const key = getProviderKey(row);
                                        const selected = key === providerAxis.selectedKey;
                                        return (
                                            <TableRow
                                                key={key}
                                                hover
                                                selected={selected}
                                                onClick={() => { providerAxis.setSelectedKey(key); scrollDetailIntoView(); }}
                                                sx={rosterRowSx(selected)}
                                            >
                                                <TableCell sx={{ fontWeight: 600 }}>{row.provider_name || row.key}</TableCell>
                                                <UsageMetricValueCells usage={row} showTotal showCacheWrite={showProviderRosterCacheWrite} />
                                                <TableCell padding="checkbox">
                                                    <ArrowForward sx={selectionArrowSx(selected)} color={selected ? 'primary' : 'inherit'} />
                                                </TableCell>
                                            </TableRow>
                                        );
                                    })}
                                    {activeAxis.visibleRows.length === 0 && (
                                        <TableRow>
                                            <TableCell
                                                colSpan={primaryColumns.length + 1}
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
                        {activeAxis.visibleRows.length > 0 && (
                            <TablePagination
                                component="div"
                                count={activeAxis.visibleRows.length}
                                page={activeAxis.page}
                                onPageChange={(_, newPage) => activeAxis.setPage(newPage)}
                                rowsPerPage={rowsPerPage}
                                onRowsPerPageChange={(event) => {
                                    setRowsPerPage(parseInt(event.target.value, 10));
                                    accountAxis.setPage(0);
                                    modelAxis.setPage(0);
                                    providerAxis.setPage(0);
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
                                <RosterDetailHeader
                                    viewMode={viewMode}
                                    selectedUser={selectedUser}
                                    selectedModel={selectedModel}
                                    selectedProvider={selectedProvider}
                                    accountsCount={viewMode === 'model' ? modelAxis.detail.length : providerAxis.detail.length}
                                    t={t}
                                />

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
                                            <Chip size="small" label={activeAxis.detail.length} sx={{ height: 22 }} />
                                        </Stack>
                                        <Typography variant="body2">
                                            {formatNumber(getTotalTokens(detailSubject))} {t('dashboard.userUsage.tokens', { defaultValue: 'tokens' }).toLocaleLowerCase()}
                                        </Typography>
                                    </Stack>
                                    {activeAxis.detailLoading ? (
                                        <Stack spacing={1.5} sx={{ overflow: 'hidden' }}>
                                            {Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} variant="rounded" height={44} />)}
                                        </Stack>
                                    ) : viewMode === 'account' ? (
                                        <RosterBreakdownTable
                                            items={accountAxis.detail}
                                            rowKey={(model) => `${model.provider_uuid}-${model.model || model.key}`}
                                            identityColumns={[
                                                {
                                                    key: 'provider',
                                                    label: t('dashboard.userUsage.provider', { defaultValue: 'Provider' }),
                                                    render: (model) => model.provider_name || '—',
                                                },
                                                {
                                                    key: 'model',
                                                    label: t('dashboard.userUsage.model', { defaultValue: 'Model' }),
                                                    render: (model) => (
                                                        <Typography variant="body2" sx={{ fontWeight: 600 }}>{model.model || model.key}</Typography>
                                                    ),
                                                },
                                            ]}
                                            ariaLabel={t('dashboard.userUsage.allModels', { defaultValue: 'All models' })}
                                            noUsageLabel={t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                                            noUsageHint={t('dashboard.userUsage.noUsageHint', { defaultValue: 'The user remains listed because their access is registered.' })}
                                            usageMetricLabels={usageMetricLabels}
                                        />
                                    ) : (
                                        <RosterBreakdownTable
                                            items={activeAxis.detail}
                                            rowKey={(account) => account.user_id || account.key}
                                            identityColumns={[
                                                {
                                                    key: 'user',
                                                    label: t('dashboard.userUsage.user', { defaultValue: 'User' }),
                                                    render: (account) => (
                                                        <>
                                                            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                                {accountDisplayName(account.user_id || account.key)}
                                                            </Typography>
                                                            <Typography variant="caption" sx={{ color: 'text.secondary', fontFamily: 'monospace' }}>
                                                                {account.user_id || account.key}
                                                            </Typography>
                                                        </>
                                                    ),
                                                },
                                            ]}
                                            ariaLabel={viewMode === 'model'
                                                ? t('dashboard.userUsage.accountsUsingModelTitle', { defaultValue: 'Accounts using this model' })
                                                : t('dashboard.userUsage.accountsUsingProviderTitle', { defaultValue: 'Accounts using this provider' })}
                                            noUsageLabel={t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
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
