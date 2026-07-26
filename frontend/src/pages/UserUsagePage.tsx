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
    InputAdornment,
    LinearProgress,
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
    TextField,
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
    ErrorOutline,
    Refresh,
    Search,
    Token,
    Users,
} from '@/components/icons';
import PageHeader from '@/components/PageHeader';
import {
    formatNumber,
    StatCard,
    TOKEN_COLORS,
    getTotalTokens,
    getCacheHitRate,
    getCacheHitRateColor,
    getErrorRateColor,
} from '@/components/dashboard';
import type { AggregatedStat } from '@/components/dashboard';
import api from '@/services/api';

type TimeRange = 'today' | '7d' | '30d' | '90d';
type SortField = 'name' | 'requests' | 'tokens' | 'errors';
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
    cache_input_tokens: number;
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

// Shared base style for the master-list and detail-panel cards: separate
// elevation-0 Paper cards (matching StatCard/ServiceStatsTable's convention)
// rather than one Paper split by an internal divider line.
const masterDetailCardSx = {
    width: '100%',
    borderRadius: 2,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    boxShadow: 'none',
    overflow: 'hidden',
    height: { xs: 'auto', lg: 640 },
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

export default function UserUsagePage() {
    const { t } = useTranslation();
    const theme = useTheme();
    const [range, setRange] = useState<TimeRange>('7d');
    const [tokens, setTokens] = useState<APITokenInfo[]>([]);
    const [userStats, setUserStats] = useState<AggregatedStat[]>([]);
    const [modelStats, setModelStats] = useState<AggregatedStat[]>([]);
    const [selectedUserID, setSelectedUserID] = useState('');
    const [search, setSearch] = useState('');
    const [sortField, setSortField] = useState<SortField>('tokens');
    const [sortDirection, setSortDirection] = useState<SortDirection>('desc');
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(10);
    const [loading, setLoading] = useState(true);
    const [detailLoading, setDetailLoading] = useState(false);
    const [refreshing, setRefreshing] = useState(false);
    const [error, setError] = useState('');
    const requestSeq = useRef(0);
    const detailSeq = useRef(0);
    const detailPanelRef = useRef<HTMLDivElement>(null);

    const loadUsers = useCallback(async (selectedRange: TimeRange, manual = false) => {
        const seq = ++requestSeq.current;
        if (manual) setRefreshing(true);
        setError('');
        try {
            const timeParams = buildTimeParams(selectedRange);
            const [tokensResult, statsResult] = await Promise.all([
                api.listAPITokens({ limit: 500 }),
                api.getUsageStats({
                    ...timeParams,
                    group_by: 'user',
                    sort_by: 'total_tokens',
                    sort_order: 'desc',
                    limit: 500,
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
            setUserStats(statsResult?.data || []);
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

    const loadUserDetail = useCallback(async (userID: string, selectedRange: TimeRange) => {
        if (!userID) {
            setModelStats([]);
            return;
        }
        const seq = ++detailSeq.current;
        setDetailLoading(true);
        try {
            const result = await api.getUsageStats({
                ...buildTimeParams(selectedRange),
                user_id: userID,
                group_by: 'model',
                sort_by: 'total_tokens',
                sort_order: 'desc',
                limit: 1000,
            });
            if (seq === detailSeq.current) setModelStats(result?.data || []);
        } catch {
            if (seq === detailSeq.current) setModelStats([]);
        } finally {
            if (seq === detailSeq.current) setDetailLoading(false);
        }
    }, []);

    useEffect(() => {
        loadUsers(range);
    }, [loadUsers, range]);

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
                cache_input_tokens: stat?.cache_input_tokens || 0,
                error_count: stat?.error_count || 0,
                error_rate: stat?.error_rate || 0,
            };
        });
    }, [t, tokens, userStats]);

    const visibleRows = useMemo(() => {
        const query = search.trim().toLocaleLowerCase();
        const direction = sortDirection === 'asc' ? 1 : -1;
        return rows
            .filter((row) => !query
                || row.display_name.toLocaleLowerCase().includes(query)
                || row.user_id.toLocaleLowerCase().includes(query))
            .sort((a, b) => {
                if (sortField === 'name') return direction * a.display_name.localeCompare(b.display_name);
                if (sortField === 'requests') return direction * (a.request_count - b.request_count);
                if (sortField === 'errors') return direction * (a.error_rate - b.error_rate);
                return direction * (a.total_tokens - b.total_tokens);
            });
    }, [rows, search, sortField, sortDirection]);

    const pagedRows = useMemo(
        () => visibleRows.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage),
        [visibleRows, page, rowsPerPage],
    );

    const handleSort = (field: SortField) => {
        if (field === sortField) {
            setSortDirection((direction) => (direction === 'asc' ? 'desc' : 'asc'));
        } else {
            setSortField(field);
            setSortDirection(field === 'name' ? 'asc' : 'desc');
        }
    };

    // Filtering/sorting can shrink the result set below the current page.
    useEffect(() => {
        setPage(0);
    }, [search, sortField, sortDirection, range]);

    useEffect(() => {
        if (visibleRows.length === 0) {
            setSelectedUserID('');
            return;
        }
        if (!visibleRows.some((row) => row.user_id === selectedUserID)) {
            setSelectedUserID(visibleRows[0].user_id);
        }
    }, [rows, selectedUserID, visibleRows]);

    useEffect(() => {
        loadUserDetail(selectedUserID, range);
    }, [loadUserDetail, range, selectedUserID]);

    const selectedUser = useMemo(
        () => rows.find((row) => row.user_id === selectedUserID),
        [rows, selectedUserID],
    );

    // Single pass over rows for every summary aggregate, instead of one
    // reduce/filter per metric — recomputed only when rows actually change.
    const summary = useMemo(() => {
        const totals = rows.reduce(
            (acc, row) => {
                acc.tokens += row.total_tokens;
                acc.inputTokens += row.total_input_tokens;
                acc.cacheTokens += row.cache_input_tokens;
                acc.requests += row.request_count;
                acc.errors += row.error_count;
                if (row.request_count > 0) acc.activeUsers += 1;
                return acc;
            },
            { tokens: 0, inputTokens: 0, cacheTokens: 0, requests: 0, errors: 0, activeUsers: 0 },
        );
        return {
            ...totals,
            cacheHitRate: getCacheHitRate(totals.cacheTokens, totals.inputTokens),
            errorRate: totals.requests > 0 ? (totals.errors / totals.requests) * 100 : 0,
        };
    }, [rows]);
    const {
        tokens: totalTokens,
        cacheTokens: totalCacheTokens,
        requests: totalRequests,
        errors: totalErrors,
        activeUsers,
        cacheHitRate,
        errorRate,
    } = summary;
    const maxTokens = useMemo(
        () => visibleRows.reduce((max, row) => Math.max(max, row.total_tokens), 1),
        [visibleRows],
    );
    const summaryItems = [
        {
            label: t('dashboard.userUsage.registeredUsers', { defaultValue: 'Registered users' }),
            value: String(rows.length),
            hint: t('dashboard.userUsage.activeUsers', {
                count: activeUsers,
                defaultValue: `${activeUsers} active in this period`,
            }),
            icon: <Users />,
            color: 'primary' as const,
        },
        {
            label: t('dashboard.userUsage.totalTokens', { defaultValue: 'Total tokens' }),
            value: formatNumber(totalTokens),
            hint: t('dashboard.userUsage.acrossUsers', {
                count: activeUsers,
                defaultValue: `Across ${activeUsers} active users`,
            }),
            icon: <Token />,
            color: 'secondary' as const,
        },
        {
            label: t('dashboard.userUsage.cacheHitRate', { defaultValue: 'Cache hit rate' }),
            value: `${cacheHitRate.toFixed(1)}%`,
            hint: t('dashboard.userUsage.cached', {
                value: formatNumber(totalCacheTokens),
                defaultValue: `${formatNumber(totalCacheTokens)} cached`,
            }),
            icon: <CachedIcon />,
            color: getCacheHitRateColor(cacheHitRate),
        },
        {
            label: t('dashboard.userUsage.requests', { defaultValue: 'Requests' }),
            value: formatNumber(totalRequests),
            hint: t('dashboard.userUsage.averagePerUser', {
                value: activeUsers ? formatNumber(Math.round(totalRequests / activeUsers)) : '0',
                defaultValue: `${activeUsers ? formatNumber(Math.round(totalRequests / activeUsers)) : '0'} per active user`,
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

    const userColumns: Array<{
        field: SortField;
        label: string;
        align?: 'right';
        defaultDir: SortDirection;
        sx?: object;
    }> = [
        { field: 'name', label: t('dashboard.userUsage.user', { defaultValue: 'User' }), defaultDir: 'asc' },
        {
            field: 'requests',
            label: t('dashboard.userUsage.requests', { defaultValue: 'Requests' }),
            align: 'right',
            defaultDir: 'desc',
            sx: { display: { xs: 'none', sm: 'table-cell' } },
        },
        { field: 'tokens', label: t('dashboard.userUsage.tokens', { defaultValue: 'Tokens' }), align: 'right', defaultDir: 'desc' },
        {
            field: 'errors',
            label: t('dashboard.userUsage.errorRate', { defaultValue: 'Error rate' }),
            align: 'right',
            defaultDir: 'desc',
            sx: { display: { xs: 'none', md: 'table-cell' } },
        },
    ];

    const handleSelectUser = (userID: string) => {
        setSelectedUserID(userID);
        if (window.matchMedia('(max-width: 1199.95px)').matches) {
            requestAnimationFrame(() => {
                detailPanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
            });
        }
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
                                    onClick={() => loadUsers(range, true)}
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
                <Grid size={{ xs: 12, lg: 7, xl: 5 }} sx={{ display: 'flex' }}>
                    <Paper
                        elevation={0}
                        sx={{ ...masterDetailCardSx, display: 'flex', flexDirection: 'column' }}
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
                                    {t('dashboard.userUsage.allUsers', { defaultValue: 'All registered users' })}
                                </Typography>
                                <Chip size="small" label={visibleRows.length} sx={{ height: 22 }} />
                            </Stack>
                            <TextField
                                size="small"
                                value={search}
                                onChange={(event) => setSearch(event.target.value)}
                                placeholder={t('dashboard.userUsage.search', { defaultValue: 'Search users' })}
                                slotProps={{
                                    input: {
                                        startAdornment: (
                                            <InputAdornment position="start"><Search fontSize="small" /></InputAdornment>
                                        ),
                                    },
                                }}
                                sx={{ width: { xs: '100%', sm: 220 } }}
                            />
                        </Box>
                        <TableContainer
                            sx={{
                                maxHeight: { xs: 420, lg: 'none' },
                                flex: { lg: 1 },
                                minHeight: 0,
                                overscrollBehavior: 'contain',
                            }}
                        >
                            <Table stickyHeader>
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
                                        {userColumns.map((col) => (
                                            <TableCell
                                                key={col.field}
                                                align={col.align}
                                                sortDirection={sortField === col.field ? sortDirection : false}
                                                sx={col.sx}
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
                                    {pagedRows.map((row) => {
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
                                                <TableCell align="right" sx={{ display: { xs: 'none', sm: 'table-cell' } }}>
                                                    <Typography variant="body1" sx={{ color: 'text.primary', fontWeight: 550 }}>
                                                        {formatNumber(row.request_count)}
                                                    </Typography>
                                                </TableCell>
                                                <TableCell align="right" sx={{ minWidth: { xs: 88, sm: 140 } }}>
                                                    <Typography variant="body1" sx={{ color: 'text.primary', fontWeight: 600 }}>
                                                        {formatNumber(row.total_tokens)}
                                                    </Typography>
                                                    <LinearProgress
                                                        variant="determinate"
                                                        value={(row.total_tokens / maxTokens) * 100}
                                                        sx={{ mt: 0.6, height: 3, borderRadius: 2 }}
                                                    />
                                                </TableCell>
                                                <TableCell align="right" sx={{ display: { xs: 'none', md: 'table-cell' } }}>
                                                    <Typography
                                                        variant="body1"
                                                        sx={{
                                                            // Same threshold/color rule as ServiceStatsTable's per-model Error Rate column.
                                                            color: row.error_rate > 0.05 ? 'error.main' : 'text.secondary',
                                                            fontWeight: 550,
                                                        }}
                                                    >
                                                        {(row.error_rate * 100).toFixed(1)}%
                                                    </Typography>
                                                </TableCell>
                                                <TableCell padding="checkbox">
                                                    <ArrowForward sx={{ fontSize: 18, opacity: selected ? 1 : 0.22 }} color={selected ? 'primary' : 'inherit'} />
                                                </TableCell>
                                            </TableRow>
                                        );
                                    })}
                                    {visibleRows.length === 0 && (
                                        <TableRow>
                                            <TableCell colSpan={5} align="center" sx={{ py: 8 }}>
                                                <Typography variant="body1" sx={{ color: 'text.secondary' }}>
                                                    {t('dashboard.userUsage.noUsers', { defaultValue: 'No users match your search.' })}
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
                        {visibleRows.length > 0 && (
                            <TablePagination
                                component="div"
                                count={visibleRows.length}
                                page={page}
                                onPageChange={(_, newPage) => setPage(newPage)}
                                rowsPerPage={rowsPerPage}
                                onRowsPerPageChange={(event) => {
                                    setRowsPerPage(parseInt(event.target.value, 10));
                                    setPage(0);
                                }}
                                rowsPerPageOptions={[5, 10, 25, 50]}
                                sx={{ borderTop: '1px solid', borderColor: 'divider', flexShrink: 0 }}
                            />
                        )}
                    </Paper>
                </Grid>

                <Grid
                    ref={detailPanelRef}
                    size={{ xs: 12, lg: 5, xl: 7 }}
                    sx={{ display: 'flex', scrollMarginTop: { xs: 72, lg: 0 } }}
                >
                    <Paper elevation={0} sx={{ ...masterDetailCardSx, display: 'flex' }}>
                        <Box sx={{
                            p: { xs: 2, sm: 2.5 },
                            width: '100%',
                            height: '100%',
                            minHeight: { xs: 420, lg: 0 },
                            display: 'flex',
                            flexDirection: 'column',
                            overflow: { lg: 'hidden' },
                        }}>
                        {selectedUser ? (
                            <Stack spacing={2.5} sx={{ height: '100%', minHeight: 0 }}>
                                <Box>
                                    <Stack direction="row" spacing={2} sx={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
                                        <Box sx={{ minWidth: 0 }}>
                                            <Typography variant="h6" noWrap sx={{ fontWeight: 650 }}>
                                                {selectedUser.display_name}
                                            </Typography>
                                            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                                                {selectedUser.user_id}
                                            </Typography>
                                        </Box>
                                        {selectedUser.account_type === 'primary' ? (
                                            <Chip
                                                size="small"
                                                color="primary"
                                                label={t('dashboard.userUsage.primaryAccount', { defaultValue: 'Primary account' })}
                                                variant="outlined"
                                            />
                                        ) : (
                                            <Chip
                                                size="small"
                                                icon={selectedUser.enabled ? <CheckCircle /> : <Block />}
                                                color={selectedUser.enabled ? 'success' : 'default'}
                                                label={selectedUser.enabled
                                                    ? t('dashboard.userUsage.enabled', { defaultValue: 'Enabled' })
                                                    : t('dashboard.userUsage.disabled', { defaultValue: 'Disabled' })}
                                                variant="outlined"
                                            />
                                        )}
                                    </Stack>
                                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={{ xs: 0.5, sm: 2 }} sx={{ mt: 1.25 }}>
                                        <Stack direction="row" spacing={0.6} sx={{ alignItems: 'center' }}>
                                            <AccessTime sx={{ fontSize: 17, color: 'text.secondary' }} />
                                            <Typography variant="body2">
                                                {selectedUser.account_type === 'primary'
                                                    ? t('dashboard.userUsage.globalTokenUsage', { defaultValue: 'Usage through the global model token' })
                                                    : selectedUser.last_used_at
                                                    ? t('dashboard.userUsage.lastUsed', {
                                                        value: formatDateTime(selectedUser.last_used_at),
                                                        defaultValue: `Last used ${formatDateTime(selectedUser.last_used_at)}`,
                                                    })
                                                    : t('dashboard.userUsage.neverUsed', { defaultValue: 'Never used' })}
                                            </Typography>
                                        </Stack>
                                        {selectedUser.created_at && (
                                            <Typography variant="body2">
                                                {t('dashboard.userUsage.joined', {
                                                    value: formatDateTime(selectedUser.created_at),
                                                    defaultValue: `Added ${formatDateTime(selectedUser.created_at)}`,
                                                })}
                                            </Typography>
                                        )}
                                    </Stack>
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
                                        { label: t('dashboard.userUsage.input', { defaultValue: 'Input' }), value: selectedUser.total_input_tokens, color: TOKEN_COLORS.input.main },
                                        { label: t('dashboard.userUsage.output', { defaultValue: 'Output' }), value: selectedUser.total_output_tokens, color: TOKEN_COLORS.output.main },
                                        { label: t('dashboard.userUsage.cache', { defaultValue: 'Cache' }), value: selectedUser.cache_input_tokens, color: TOKEN_COLORS.cache.main },
                                    ].map(({ label, value, color }) => (
                                        <Grid
                                            key={label}
                                            size={{ xs: 4 }}
                                            sx={{ '&:not(:last-of-type)': { borderRight: '1px solid', borderColor: 'divider' } }}
                                        >
                                            <Box sx={{ px: 1.5, py: 1.25 }}>
                                                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                                    <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: color, flexShrink: 0 }} />
                                                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>{label}</Typography>
                                                </Stack>
                                                <Typography variant="h4" sx={{ fontVariantNumeric: 'tabular-nums', mt: 0.5 }}>
                                                    {formatNumber(Number(value))}
                                                </Typography>
                                            </Box>
                                        </Grid>
                                    ))}
                                </Grid>

                                <Box sx={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
                                    <Stack direction="row" sx={{ mb: 1.25, justifyContent: 'space-between', alignItems: 'baseline' }}>
                                        <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                            <Typography variant="subtitle2" sx={{ fontWeight: 650 }}>
                                                {t('dashboard.userUsage.allModels', { defaultValue: 'All models' })}
                                            </Typography>
                                            <Chip size="small" label={modelStats.length} sx={{ height: 22 }} />
                                        </Stack>
                                        <Typography variant="body2">
                                            {formatNumber(selectedUser.total_tokens)} {t('dashboard.userUsage.tokens', { defaultValue: 'tokens' }).toLocaleLowerCase()}
                                        </Typography>
                                    </Stack>
                                    {detailLoading ? (
                                        <Stack spacing={1.5} sx={{ overflow: 'hidden' }}>
                                            {Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} variant="rounded" height={44} />)}
                                        </Stack>
                                    ) : modelStats.length > 0 ? (
                                        <Stack
                                            spacing={1.5}
                                            role="region"
                                            aria-label={t('dashboard.userUsage.allModels', { defaultValue: 'All models' })}
                                            sx={{
                                                flex: { xs: 'none', lg: 1 },
                                                minHeight: 0,
                                                maxHeight: { xs: 'none', lg: 360 },
                                                overflowY: 'auto',
                                                overscrollBehavior: 'contain',
                                                pr: 0.75,
                                            }}
                                        >
                                            {modelStats.map((model) => {
                                                const value = getTotalTokens(model);
                                                const share = selectedUser.total_tokens ? (value / selectedUser.total_tokens) * 100 : 0;
                                                return (
                                                    <Box key={`${model.provider_uuid}-${model.model || model.key}`}>
                                                        <Stack direction="row" spacing={2} sx={{ justifyContent: 'space-between' }}>
                                                            <Box sx={{ minWidth: 0 }}>
                                                                <Typography
                                                                    variant="body1"
                                                                    noWrap
                                                                    sx={{ color: 'text.primary', fontWeight: 650 }}
                                                                >
                                                                    {model.model || model.key}
                                                                </Typography>
                                                                <Typography variant="body2">{model.provider_name || '—'}</Typography>
                                                            </Box>
                                                            <Box sx={{ textAlign: 'right', flexShrink: 0 }}>
                                                                <Typography variant="body1" sx={{ color: 'text.primary', fontWeight: 650 }}>
                                                                    {formatNumber(value)}
                                                                </Typography>
                                                                <Typography variant="body2">{share.toFixed(1)}%</Typography>
                                                            </Box>
                                                        </Stack>
                                                        <LinearProgress
                                                            variant="determinate"
                                                            value={Math.min(share, 100)}
                                                            sx={{ mt: 0.65, height: 4, borderRadius: 2 }}
                                                        />
                                                    </Box>
                                                );
                                            })}
                                        </Stack>
                                    ) : (
                                        <Box sx={{ py: 4, textAlign: 'center', bgcolor: 'action.hover', borderRadius: 1.5 }}>
                                            <Typography variant="body1">
                                                {t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                                            </Typography>
                                            <Typography variant="body2">
                                                {t('dashboard.userUsage.noUsageHint', { defaultValue: 'The user remains listed because their access is registered.' })}
                                            </Typography>
                                        </Box>
                                    )}
                                </Box>
                            </Stack>
                        ) : (
                            <Box sx={{ height: '100%', display: 'grid', placeItems: 'center', textAlign: 'center' }}>
                                <Box>
                                    <Users sx={{ fontSize: 42, color: 'text.disabled', mb: 1 }} />
                                    <Typography variant="body1">
                                        {t('dashboard.userUsage.selectUser', { defaultValue: 'Select a user to see details.' })}
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
