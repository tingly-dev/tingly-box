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
import { formatNumber, StatCard } from '@/components/dashboard';
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
    const [sortMode, setSortMode] = useState<SortMode>('tokens');
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
            hint: `${totalRequests ? ((totalErrors / totalRequests) * 100).toFixed(1) : '0.0'}%`,
            icon: <ErrorOutline />,
            color: totalErrors > 0 ? 'warning' as const : 'success' as const,
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
                sx={{
                    '& .MuiTypography-body2': {
                        typography: 'body1',
                    },
                }}
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
                    <Grid key={item.label} size={{ xs: 6, lg: 3 }}>
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

            <Paper
                variant="outlined"
                sx={{
                    borderRadius: 2,
                    overflow: 'hidden',
                    height: { xs: 'auto', lg: 640 },
                }}
            >
                <Grid container sx={{ alignItems: 'stretch', height: '100%' }}>
                    <Grid
                        size={{ xs: 12, lg: 7, xl: 5 }}
                        sx={{
                            display: 'flex',
                            flexDirection: 'column',
                            minHeight: 0,
                            borderRight: { lg: '1px solid' },
                            borderBottom: { xs: '1px solid', lg: 0 },
                            borderColor: 'divider',
                        }}
                    >
                        <Box
                            sx={{
                                p: 2,
                                display: 'grid',
                                gridTemplateColumns: { xs: '1fr', sm: 'minmax(180px, 1fr) minmax(180px, 210px) 138px' },
                                gap: 1.25,
                                alignItems: 'center',
                                borderBottom: '1px solid',
                                borderColor: 'divider',
                                bgcolor:
                                    theme.palette.mode === 'dark'
                                        ? 'rgba(255, 255, 255, 0.025)'
                                        : alpha(theme.palette.action.hover, 0.45),
                            }}
                        >
                            <Box sx={{ flex: '1 1 240px' }}>
                                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                    <Typography variant="h6">
                                    {t('dashboard.userUsage.allUsers', { defaultValue: 'All registered users' })}
                                    </Typography>
                                    <Chip size="small" label={visibleRows.length} sx={{ height: 22 }} />
                                </Stack>
                                <Typography variant="body2">
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
                                sx={{ width: '100%', bgcolor: 'background.paper' }}
                            />
                            <Select
                                size="small"
                                value={sortMode}
                                onChange={(event) => setSortMode(event.target.value as SortMode)}
                                aria-label={t('dashboard.userUsage.sortBy', { defaultValue: 'Sort users' })}
                                sx={{ minWidth: 138, bgcolor: 'background.paper' }}
                            >
                                <MenuItem value="tokens">{t('dashboard.userUsage.sortTokens', { defaultValue: 'Most tokens' })}</MenuItem>
                                <MenuItem value="requests">{t('dashboard.userUsage.sortRequests', { defaultValue: 'Most requests' })}</MenuItem>
                                <MenuItem value="errors">{t('dashboard.userUsage.sortErrors', { defaultValue: 'Highest errors' })}</MenuItem>
                                <MenuItem value="name">{t('dashboard.userUsage.sortName', { defaultValue: 'Name' })}</MenuItem>
                            </Select>
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
                                            '& th': {
                                                typography: 'subtitle2',
                                                textTransform: 'uppercase',
                                                letterSpacing: '0.035em',
                                                py: 1.4,
                                            },
                                        }}
                                    >
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
                                                onClick={() => handleSelectUser(row.user_id)}
                                                sx={{
                                                    cursor: 'pointer',
                                                    position: 'relative',
                                                    '& .MuiTableCell-root': { py: 1.3 },
                                                    '&.Mui-selected': {
                                                        bgcolor: alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.13 : 0.045),
                                                        boxShadow: `inset 3px 0 0 ${theme.palette.primary.main}`,
                                                        '&:hover': { bgcolor: alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.17 : 0.07) },
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
                                                            color: row.error_rate >= 0.05 ? 'error.main' : 'text.primary',
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
                                                <Typography variant="body1">
                                                    {t('dashboard.userUsage.noUsers', { defaultValue: 'No users match your search.' })}
                                                </Typography>
                                            </TableCell>
                                        </TableRow>
                                    )}
                                </TableBody>
                            </Table>
                        </TableContainer>
                    </Grid>

                    <Grid
                        ref={detailPanelRef}
                        size={{ xs: 12, lg: 5, xl: 7 }}
                        sx={{
                            display: 'flex',
                            height: '100%',
                            bgcolor: alpha(theme.palette.background.paper, 0.6),
                            scrollMarginTop: { xs: 72, lg: 0 },
                            minHeight: 0,
                            overflow: { lg: 'hidden' },
                        }}
                    >
                        <Box sx={{ p: { xs: 2, sm: 2.5 }, width: '100%', height: '100%', minHeight: { xs: 420, lg: 0 } }}>
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
                                        bgcolor:
                                            theme.palette.mode === 'dark'
                                                ? 'rgba(255, 255, 255, 0.025)'
                                                : alpha(theme.palette.action.hover, 0.5),
                                    }}
                                >
                                    {[
                                        [t('dashboard.userUsage.input', { defaultValue: 'Input' }), selectedUser.total_input_tokens],
                                        [t('dashboard.userUsage.output', { defaultValue: 'Output' }), selectedUser.total_output_tokens],
                                        [t('dashboard.userUsage.cache', { defaultValue: 'Cache' }), selectedUser.cache_input_tokens],
                                    ].map(([label, value]) => (
                                        <Grid
                                            key={String(label)}
                                            size={{ xs: 4 }}
                                            sx={{ '&:not(:last-of-type)': { borderRight: '1px solid', borderColor: 'divider' } }}
                                        >
                                            <Box sx={{ px: 1.5, py: 1.25 }}>
                                                <Typography variant="subtitle2">{label}</Typography>
                                                <Typography variant="h4" sx={{ fontVariantNumeric: 'tabular-nums' }}>
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
                                                const value = model.total_tokens || 0;
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
                                                            sx={{
                                                                mt: 0.65,
                                                                height: 4,
                                                                borderRadius: 2,
                                                                bgcolor: alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.16 : 0.1),
                                                            }}
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
                    </Grid>
                </Grid>
            </Paper>
        </Box>
    );
}
