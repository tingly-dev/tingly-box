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
    MenuItem,
    Paper,
    Select,
    Skeleton,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
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
import { StatCard, formatNumber } from '@/components/dashboard';
import type { AggregatedStat } from '@/components/dashboard';
import api from '@/services/api';

type TimeRange = 'today' | '7d' | '30d' | '90d';
type SortMode = 'tokens' | 'requests' | 'errors' | 'name';

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

const UserUsageSkeleton = () => (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
        <Skeleton variant="rounded" height={72} />
        <Grid container spacing={2}>
            {Array.from({ length: 4 }).map((_, index) => (
                <Grid key={index} size={{ xs: 6, lg: 3 }}>
                    <Skeleton variant="rounded" height={118} />
                </Grid>
            ))}
        </Grid>
        <Grid container spacing={2}>
            <Grid size={{ xs: 12, lg: 7 }}><Skeleton variant="rounded" height={520} /></Grid>
            <Grid size={{ xs: 12, lg: 5 }}><Skeleton variant="rounded" height={520} /></Grid>
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
    const [sortMode, setSortMode] = useState<SortMode>('tokens');
    const [loading, setLoading] = useState(true);
    const [detailLoading, setDetailLoading] = useState(false);
    const [refreshing, setRefreshing] = useState(false);
    const [error, setError] = useState('');
    const requestSeq = useRef(0);
    const detailSeq = useRef(0);

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
                setError(loadError instanceof Error ? loadError.message : 'Unable to load user usage');
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
                limit: 20,
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
                total_tokens: stat?.total_tokens || 0,
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
        return rows
            .filter((row) => !query
                || row.display_name.toLocaleLowerCase().includes(query)
                || row.user_id.toLocaleLowerCase().includes(query))
            .sort((a, b) => {
                if (sortMode === 'name') return a.display_name.localeCompare(b.display_name);
                if (sortMode === 'requests') return b.request_count - a.request_count;
                if (sortMode === 'errors') return b.error_rate - a.error_rate;
                return b.total_tokens - a.total_tokens;
            });
    }, [rows, search, sortMode]);

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

    const selectedUser = rows.find((row) => row.user_id === selectedUserID);
    const totalTokens = rows.reduce((sum, row) => sum + row.total_tokens, 0);
    const totalRequests = rows.reduce((sum, row) => sum + row.request_count, 0);
    const totalErrors = rows.reduce((sum, row) => sum + row.error_count, 0);
    const activeUsers = rows.filter((row) => row.request_count > 0).length;
    const maxTokens = Math.max(...visibleRows.map((row) => row.total_tokens), 1);

    if (loading) return <UserUsageSkeleton />;

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
            <PageHeader
                title={t('dashboard.userUsage.title', { defaultValue: 'User usage' })}
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
                <Grid size={{ xs: 6, lg: 3 }}>
                    <StatCard
                        title={t('dashboard.userUsage.registeredUsers', { defaultValue: 'Registered users' })}
                        value={rows.length}
                        subtitle={t('dashboard.userUsage.activeUsers', {
                            count: activeUsers,
                            defaultValue: `${activeUsers} active in this period`,
                        })}
                        icon={<Users />}
                    />
                </Grid>
                <Grid size={{ xs: 6, lg: 3 }}>
                    <StatCard
                        title={t('dashboard.userUsage.totalTokens', { defaultValue: 'Total tokens' })}
                        value={formatNumber(totalTokens)}
                        subtitle={t('dashboard.userUsage.acrossUsers', {
                            count: activeUsers,
                            defaultValue: `Across ${activeUsers} active users`,
                        })}
                        icon={<Token />}
                        color="info"
                    />
                </Grid>
                <Grid size={{ xs: 6, lg: 3 }}>
                    <StatCard
                        title={t('dashboard.userUsage.requests', { defaultValue: 'Requests' })}
                        value={formatNumber(totalRequests)}
                        subtitle={t('dashboard.userUsage.averagePerUser', {
                            value: activeUsers ? formatNumber(Math.round(totalRequests / activeUsers)) : '0',
                            defaultValue: `${activeUsers ? formatNumber(Math.round(totalRequests / activeUsers)) : '0'} per active user`,
                        })}
                        icon={<BarChart />}
                        color="success"
                    />
                </Grid>
                <Grid size={{ xs: 6, lg: 3 }}>
                    <StatCard
                        title={t('dashboard.userUsage.errors', { defaultValue: 'Errors' })}
                        value={formatNumber(totalErrors)}
                        subtitle={`${totalRequests ? ((totalErrors / totalRequests) * 100).toFixed(1) : '0.0'}%`}
                        icon={<ErrorOutline />}
                        color={totalErrors > 0 ? 'warning' : 'secondary'}
                    />
                </Grid>
            </Grid>

            <Grid container spacing={2} alignItems="stretch">
                <Grid size={{ xs: 12, lg: 7 }}>
                    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden', height: '100%' }}>
                        <Box sx={{ p: 2, display: 'flex', gap: 1.5, flexWrap: 'wrap', alignItems: 'center' }}>
                            <Box sx={{ flex: '1 1 240px' }}>
                                <Typography variant="subtitle1" fontWeight={600}>
                                    {t('dashboard.userUsage.allUsers', { defaultValue: 'All registered users' })}
                                </Typography>
                                <Typography variant="caption" color="text.secondary">
                                    {t('dashboard.userUsage.rowHint', { defaultValue: 'Select a user to inspect their usage mix.' })}
                                </Typography>
                            </Box>
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
                                sx={{ width: { xs: '100%', sm: 210 } }}
                            />
                            <Select
                                size="small"
                                value={sortMode}
                                onChange={(event) => setSortMode(event.target.value as SortMode)}
                                aria-label={t('dashboard.userUsage.sortBy', { defaultValue: 'Sort users' })}
                                sx={{ minWidth: 138 }}
                            >
                                <MenuItem value="tokens">{t('dashboard.userUsage.sortTokens', { defaultValue: 'Most tokens' })}</MenuItem>
                                <MenuItem value="requests">{t('dashboard.userUsage.sortRequests', { defaultValue: 'Most requests' })}</MenuItem>
                                <MenuItem value="errors">{t('dashboard.userUsage.sortErrors', { defaultValue: 'Highest errors' })}</MenuItem>
                                <MenuItem value="name">{t('dashboard.userUsage.sortName', { defaultValue: 'Name' })}</MenuItem>
                            </Select>
                        </Box>
                        <TableContainer>
                            <Table>
                                <TableHead>
                                    <TableRow sx={{ '& th': { color: 'text.secondary', fontSize: '0.72rem', textTransform: 'uppercase' } }}>
                                        <TableCell>{t('dashboard.userUsage.user', { defaultValue: 'User' })}</TableCell>
                                        <TableCell align="right" sx={{ display: { xs: 'none', sm: 'table-cell' } }}>
                                            {t('dashboard.userUsage.requests', { defaultValue: 'Requests' })}
                                        </TableCell>
                                        <TableCell align="right">{t('dashboard.userUsage.tokens', { defaultValue: 'Tokens' })}</TableCell>
                                        <TableCell align="right" sx={{ display: { xs: 'none', md: 'table-cell' } }}>
                                            {t('dashboard.userUsage.errorRate', { defaultValue: 'Error rate' })}
                                        </TableCell>
                                        <TableCell padding="checkbox" />
                                    </TableRow>
                                </TableHead>
                                <TableBody>
                                    {visibleRows.map((row) => {
                                        const selected = row.user_id === selectedUserID;
                                        return (
                                            <TableRow
                                                key={row.token_id}
                                                hover
                                                selected={selected}
                                                onClick={() => setSelectedUserID(row.user_id)}
                                                sx={{
                                                    cursor: 'pointer',
                                                    '&.Mui-selected': {
                                                        bgcolor: alpha(theme.palette.primary.main, 0.08),
                                                        '&:hover': { bgcolor: alpha(theme.palette.primary.main, 0.12) },
                                                    },
                                                }}
                                            >
                                                <TableCell>
                                                    <Stack direction="row" spacing={1.25} alignItems="center">
                                                        <Avatar sx={{ width: 34, height: 34, fontSize: 14, bgcolor: alpha(theme.palette.primary.main, 0.12), color: 'primary.main' }}>
                                                            {row.display_name.slice(0, 1).toUpperCase()}
                                                        </Avatar>
                                                        <Box sx={{ minWidth: 0 }}>
                                                            <Stack direction="row" spacing={0.75} alignItems="center">
                                                                <Typography variant="body2" fontWeight={600} noWrap>{row.display_name}</Typography>
                                                                {row.account_type === 'primary' && (
                                                                    <Chip
                                                                        size="small"
                                                                        color="primary"
                                                                        variant="outlined"
                                                                        label={t('dashboard.userUsage.primary', { defaultValue: 'Primary' })}
                                                                        sx={{ height: 19, fontSize: '0.65rem' }}
                                                                    />
                                                                )}
                                                                {!row.enabled && (
                                                                    <Chip
                                                                        size="small"
                                                                        label={t('dashboard.userUsage.disabled', { defaultValue: 'Disabled' })}
                                                                        sx={{ height: 19, fontSize: '0.65rem' }}
                                                                    />
                                                                )}
                                                            </Stack>
                                                            <Typography variant="caption" color="text.secondary">
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
                                                    {formatNumber(row.request_count)}
                                                </TableCell>
                                                <TableCell align="right" sx={{ minWidth: { xs: 88, sm: 140 } }}>
                                                    <Typography variant="body2" fontWeight={600}>{formatNumber(row.total_tokens)}</Typography>
                                                    <LinearProgress
                                                        variant="determinate"
                                                        value={(row.total_tokens / maxTokens) * 100}
                                                        sx={{ mt: 0.6, height: 3, borderRadius: 2 }}
                                                    />
                                                </TableCell>
                                                <TableCell align="right" sx={{ display: { xs: 'none', md: 'table-cell' } }}>
                                                    <Typography
                                                        variant="body2"
                                                        color={row.error_rate >= 0.05 ? 'error.main' : 'text.secondary'}
                                                    >
                                                        {(row.error_rate * 100).toFixed(1)}%
                                                    </Typography>
                                                </TableCell>
                                                <TableCell padding="checkbox"><ArrowForward fontSize="small" color={selected ? 'primary' : 'disabled'} /></TableCell>
                                            </TableRow>
                                        );
                                    })}
                                    {visibleRows.length === 0 && (
                                        <TableRow>
                                            <TableCell colSpan={5} align="center" sx={{ py: 8 }}>
                                                <Typography color="text.secondary">
                                                    {t('dashboard.userUsage.noUsers', { defaultValue: 'No users match your search.' })}
                                                </Typography>
                                            </TableCell>
                                        </TableRow>
                                    )}
                                </TableBody>
                            </Table>
                        </TableContainer>
                    </Paper>
                </Grid>

                <Grid size={{ xs: 12, lg: 5 }}>
                    <Paper variant="outlined" sx={{ borderRadius: 2, p: { xs: 2, sm: 2.5 }, height: '100%', minHeight: 420 }}>
                        {selectedUser ? (
                            <Stack spacing={2.5}>
                                <Box>
                                    <Stack direction="row" justifyContent="space-between" alignItems="flex-start" gap={2}>
                                        <Box sx={{ minWidth: 0 }}>
                                            <Typography variant="h6" fontWeight={650} noWrap>{selectedUser.display_name}</Typography>
                                            <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
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
                                        <Stack direction="row" spacing={0.6} alignItems="center">
                                            <AccessTime sx={{ fontSize: 15, color: 'text.disabled' }} />
                                            <Typography variant="caption" color="text.secondary">
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
                                            <Typography variant="caption" color="text.secondary">
                                                {t('dashboard.userUsage.joined', {
                                                    value: formatDateTime(selectedUser.created_at),
                                                    defaultValue: `Added ${formatDateTime(selectedUser.created_at)}`,
                                                })}
                                            </Typography>
                                        )}
                                    </Stack>
                                </Box>

                                <Grid container spacing={1}>
                                    {[
                                        [t('dashboard.userUsage.input', { defaultValue: 'Input' }), selectedUser.total_input_tokens],
                                        [t('dashboard.userUsage.output', { defaultValue: 'Output' }), selectedUser.total_output_tokens],
                                        [t('dashboard.userUsage.cache', { defaultValue: 'Cache' }), selectedUser.cache_input_tokens],
                                    ].map(([label, value]) => (
                                        <Grid key={String(label)} size={{ xs: 4 }}>
                                            <Box sx={{ p: 1.25, borderRadius: 1.5, bgcolor: 'action.hover' }}>
                                                <Typography variant="caption" color="text.secondary">{label}</Typography>
                                                <Typography variant="body1" fontWeight={650}>{formatNumber(Number(value))}</Typography>
                                            </Box>
                                        </Grid>
                                    ))}
                                </Grid>

                                <Box>
                                    <Stack direction="row" justifyContent="space-between" alignItems="baseline" sx={{ mb: 1.25 }}>
                                        <Typography variant="subtitle2" fontWeight={650}>
                                            {t('dashboard.userUsage.modelMix', { defaultValue: 'Where their tokens went' })}
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            {formatNumber(selectedUser.total_tokens)} {t('dashboard.userUsage.tokens', { defaultValue: 'tokens' }).toLocaleLowerCase()}
                                        </Typography>
                                    </Stack>
                                    {detailLoading ? (
                                        <Stack spacing={1.5}>
                                            {Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} variant="rounded" height={44} />)}
                                        </Stack>
                                    ) : modelStats.length > 0 ? (
                                        <Stack spacing={1.5}>
                                            {modelStats.slice(0, 6).map((model) => {
                                                const value = model.total_tokens || 0;
                                                const share = selectedUser.total_tokens ? (value / selectedUser.total_tokens) * 100 : 0;
                                                return (
                                                    <Box key={`${model.provider_uuid}-${model.model || model.key}`}>
                                                        <Stack direction="row" justifyContent="space-between" gap={2}>
                                                            <Box sx={{ minWidth: 0 }}>
                                                                <Typography variant="body2" fontWeight={550} noWrap>{model.model || model.key}</Typography>
                                                                <Typography variant="caption" color="text.secondary">{model.provider_name || '—'}</Typography>
                                                            </Box>
                                                            <Box sx={{ textAlign: 'right', flexShrink: 0 }}>
                                                                <Typography variant="body2" fontWeight={600}>{formatNumber(value)}</Typography>
                                                                <Typography variant="caption" color="text.secondary">{share.toFixed(1)}%</Typography>
                                                            </Box>
                                                        </Stack>
                                                        <LinearProgress variant="determinate" value={Math.min(share, 100)} sx={{ mt: 0.65, height: 4, borderRadius: 2 }} />
                                                    </Box>
                                                );
                                            })}
                                        </Stack>
                                    ) : (
                                        <Box sx={{ py: 4, textAlign: 'center', bgcolor: 'action.hover', borderRadius: 1.5 }}>
                                            <Typography variant="body2" color="text.secondary">
                                                {t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                                            </Typography>
                                            <Typography variant="caption" color="text.disabled">
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
                                    <Typography color="text.secondary">
                                        {t('dashboard.userUsage.selectUser', { defaultValue: 'Select a user to see details.' })}
                                    </Typography>
                                </Box>
                            </Box>
                        )}
                    </Paper>
                </Grid>
            </Grid>
        </Box>
    );
}
