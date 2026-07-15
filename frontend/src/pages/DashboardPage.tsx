import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
    Box,
    Grid,
    IconButton,
    Tooltip,
    Typography,
    Switch,
    FormControlLabel,
    CircularProgress,
    Skeleton,
    FormControl,
    InputLabel,
    Select,
    MenuItem,
    ListSubheader,
    Paper,
    Divider,
    useTheme,
} from '@mui/material';
import { Refresh as RefreshIcon, Outbound as CallMadeIcon, ErrorOutline as ErrorOutlineIcon, Token as PaidIcon, Stream as StreamIcon, Autorenew as CachedIcon, FilterOff, ChevronLeft, ChevronRight } from '@/components/icons';
import { StatCard, DailyTokenHistoryChart, HourlyTokenHistoryChart, ServiceStatsTable, AgentQuickNav, RequestsView, DashboardHeatmapSection, formatNumber } from '@/components/dashboard';
import type { TimeSeriesData, AggregatedStat, UsageRecord } from '@/components/dashboard';
import { ToggleButtonGroup, ToggleButton } from '@mui/material';
import PageHeader from '@/components/PageHeader';
import { switchControlLabelStyle } from '@/styles/toggleStyles';
import api from '../services/api';

interface Provider {
    uuid: string;
    name: string;
    auth_type?: string;
}

interface APIToken {
    user_id?: string;
    display_name?: string;
    enabled?: boolean;
}

interface UsageIdentity {
    userId: string;
    label: string;
    type: 'owner' | 'sharing_key';
    enabled: boolean;
}

const MAIN_ACCOUNT_USER_ID = 'admin';

const shortenUserId = (userId: string): string => {
    if (userId.length <= 12) return userId;
    return `${userId.slice(0, 4)}…${userId.slice(-4)}`;
};

type TimeRange = 'today' | 'yesterday' | '3d' | '7d' | '30d' | '90d';

const TIME_RANGE_CONFIG: Record<TimeRange, { label: string; days: number; interval: string }> = {
    today: { label: 'Today', days: 1, interval: 'minute' },
    yesterday: { label: 'Yesterday', days: 1, interval: 'minute' },
    '3d': { label: '3 Days', days: 3, interval: 'day' },
    '7d': { label: '7 Days', days: 7, interval: 'day' },
    '30d': { label: '30 Days', days: 30, interval: 'day' },
    '90d': { label: '90 Days', days: 90, interval: 'day' },
};

// Format date to local ISO string (with timezone offset)
// Backend stores local time, so we send local time with timezone offset
const toLocalISOString = (date: Date): string => {
    const tzOffset = -date.getTimezoneOffset();
    const sign = tzOffset >= 0 ? '+' : '-';
    const pad = (n: number) => String(Math.floor(Math.abs(n))).padStart(2, '0');
    return date.getFullYear() +
        '-' + pad(date.getMonth() + 1) +
        '-' + pad(date.getDate()) +
        'T' + pad(date.getHours()) +
        ':' + pad(date.getMinutes()) +
        ':' + pad(date.getSeconds()) +
        sign + pad(tzOffset / 60) + ':' + pad(tzOffset % 60);
};

// Create a Date at local midnight (00:00:00 local time)
const getLocalMidnight = (date: Date): Date => {
    const d = new Date(date.getFullYear(), date.getMonth(), date.getDate());
    return d;
};

const DashboardSkeleton = () => (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
        <Box sx={{ pb: 2.5, borderBottom: '1px solid', borderColor: 'divider' }}>
            <Skeleton variant="text" width={220} height={34} />
            <Skeleton variant="text" width={140} height={24} />
        </Box>
        <Grid container spacing={{ xs: 1.5, sm: 2 }}>
            {Array.from({ length: 5 }).map((_, index) => (
                <Grid key={index} size={{ xs: 6, sm: 4, md: 2.4 }}>
                    <Skeleton variant="rounded" height={118} sx={{ borderRadius: 2 }} />
                </Grid>
            ))}
        </Grid>
        <Grid container spacing={2}>
            <Grid size={{ xs: 12, lg: 8 }}>
                <Skeleton variant="rounded" height={320} sx={{ borderRadius: 2 }} />
            </Grid>
            <Grid size={{ xs: 12, lg: 4 }}>
                <Skeleton variant="rounded" height={320} sx={{ borderRadius: 2 }} />
            </Grid>
        </Grid>
        <Skeleton variant="rounded" height={360} sx={{ borderRadius: 2 }} />
    </Box>
);

export default function DashboardPage() {
    const theme = useTheme();
    const { timeRange: urlTimeRange } = useParams<{ timeRange: TimeRange }>();
    const navigate = useNavigate();

    // Validate and set time range from URL
    const validTimeRanges: TimeRange[] = ['today', 'yesterday', '3d', '7d', '30d', '90d'];
    const timeRange: TimeRange = validTimeRanges.includes(urlTimeRange as TimeRange)
        ? (urlTimeRange as TimeRange)
        : 'today';

    const isHourlyRange = timeRange === 'today' || timeRange === 'yesterday';

    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(false);
    const [stats, setStats] = useState<AggregatedStat[]>([]);
    const [timeSeries, setTimeSeries] = useState<TimeSeriesData[]>([]);
    const [providers, setProviders] = useState<Provider[]>([]);
    const [usageIdentities, setUsageIdentities] = useState<UsageIdentity[]>([
        { userId: MAIN_ACCOUNT_USER_ID, label: 'Main account', type: 'owner', enabled: true },
    ]);
    const [selectedProvider, setSelectedProvider] = useState<string>('all');
    const [selectedModel, setSelectedModel] = useState<string>('all');
    const [selectedUser, setSelectedUser] = useState<string>('all');
    const [modelsPage, setModelsPage] = useState(0);
    const [modelsPerPage] = useState(10);
    // Bumped on manual refresh so the fixed-window activity heatmap refetches too.
    const [heatmapRefresh, setHeatmapRefresh] = useState(0);

    // Chart view mode: the trend chart ('summary'), the per-request list
    // ('requests', hourly ranges only), or the fixed 180-day activity heatmap
    // ('activity').
    const [viewMode, setViewMode] = useState<'summary' | 'requests' | 'activity'>('summary');
    // "By Request" only exists for hourly ranges; fall back to the trend if a
    // stale 'requests' selection carries into a daily range.
    const effectiveViewMode = viewMode === 'requests' && !isHourlyRange ? 'summary' : viewMode;
    const [records, setRecords] = useState<UsageRecord[]>([]);
    const [recordsLoading, setRecordsLoading] = useState(false);
    const [recordsTimeParams, setRecordsTimeParams] = useState<{ start_time: string; end_time: string } | null>(null);

    const buildTimeParams = useCallback((provider: string, model: string, user: string, range: TimeRange) => {
        const now = new Date();
        const config = TIME_RANGE_CONFIG[range];
        const todayStart = getLocalMidnight(now);
        const startTime = new Date(todayStart);
        let endTime: Date;

        if (range === 'today') {
            endTime = now;
        } else if (range === 'yesterday') {
            startTime.setDate(startTime.getDate() - 1);
            endTime = new Date(todayStart);
        } else {
            startTime.setDate(startTime.getDate() - (config.days - 1));
            endTime = new Date(todayStart);
            endTime.setDate(endTime.getDate() + 1);
        }

        const params: Record<string, string> = {
            start_time: toLocalISOString(startTime),
            end_time: toLocalISOString(endTime),
        };
        if (provider && provider !== 'all') {
            params.provider = provider;
        }
        if (model && model !== 'all') {
            params.model = model;
        }
        if (user && user !== 'all') {
            params.user_id = user;
        }
        return params;
    }, []);

    // Monotonic sequence used to drop out-of-order responses when filters
    // change faster than requests complete.
    const requestSeq = useRef(0);

    // Providers and API tokens are filter metadata that doesn't depend on the
    // selected time range or filters — fetch them once (and on manual
    // refresh) instead of on every filter change / auto-refresh tick.
    const loadFilterOptions = useCallback(async () => {
        try {
            const [providersResult, tokensResult] = await Promise.all([
                api.getProviders(),
                api.listAPITokens({ limit: 500 }),
            ]);

            if (providersResult?.success && providersResult?.data) {
                setProviders(providersResult.data);
            }
            if (tokensResult?.success && tokensResult?.data) {
                const tokens: APIToken[] = Array.isArray(tokensResult.data) ? tokensResult.data : tokensResult.data.tokens || [];
                const sharingKeysByUserId = new Map<string, UsageIdentity>();
                tokens.forEach((token) => {
                    if (!token.user_id) return;
                    sharingKeysByUserId.set(token.user_id, {
                        userId: token.user_id,
                        label: token.display_name?.trim() || 'Unnamed Sharing Key',
                        type: 'sharing_key',
                        enabled: token.enabled !== false,
                    });
                });
                const sharingKeys = Array.from(sharingKeysByUserId.values())
                    .sort((a, b) => a.label.localeCompare(b.label));
                setUsageIdentities([
                    { userId: MAIN_ACCOUNT_USER_ID, label: 'Main account', type: 'owner', enabled: true },
                    ...sharingKeys,
                ]);
            }
        } catch (error) {
            console.error('Failed to load dashboard filter options:', error);
        }
    }, []);

    const loadData = useCallback(async (provider: string, model: string, user: string, range: TimeRange) => {
        const seq = ++requestSeq.current;
        try {
            const config = TIME_RANGE_CONFIG[range];
            const params = buildTimeParams(provider, model, user, range);

            const [statsResult, timeSeriesResult] = await Promise.all([
                api.getUsageStats({ ...params, group_by: 'model', limit: 100 }),
                api.getUsageTimeSeries({ ...params, interval: config.interval }),
            ]);

            // A newer request was issued while this one was in flight —
            // discard the stale response instead of overwriting fresh data.
            if (seq !== requestSeq.current) {
                return;
            }

            if (statsResult?.data) {
                setStats(statsResult.data);
            }
            if (timeSeriesResult?.data) {
                setTimeSeries(timeSeriesResult.data);
            }

            // Store time params for records loading
            setRecordsTimeParams({ start_time: params.start_time, end_time: params.end_time });
        } catch (error) {
            console.error('Failed to load dashboard data:', error);
        } finally {
            if (seq === requestSeq.current) {
                setLoading(false);
                setRefreshing(false);
            }
        }
    }, [buildTimeParams]);

    const loadRecords = useCallback(async (
        timeParams: { start_time: string; end_time: string } | null,
        provider: string,
        model: string,
        user: string,
    ) => {
        if (!timeParams) return;
        setRecordsLoading(true);
        try {
            const filters: Record<string, any> = {
                ...timeParams,
                limit: 500,
                offset: 0,
            };
            if (provider !== 'all') {
                filters.provider = provider;
            }
            if (model !== 'all') {
                filters.model = model;
            }
            if (user !== 'all') {
                filters.user_id = user;
            }
            const result = await api.getUsageRecords(filters);
            if (result?.data) {
                setRecords(result.data);
            }
        } catch (error) {
            console.error('Failed to load records:', error);
        } finally {
            setRecordsLoading(false);
        }
    }, []);

    useEffect(() => {
        loadFilterOptions();
    }, [loadFilterOptions]);

    useEffect(() => {
        loadData(selectedProvider, selectedModel, selectedUser, timeRange);
    }, [loadData, selectedProvider, selectedModel, selectedUser, timeRange]);

    // Reset view mode when switching away from hourly ranges
    useEffect(() => {
        if (!isHourlyRange) {
            setViewMode('summary');
        }
    }, [isHourlyRange]);

    // Load records when entering requests view or time/provider/model changes
    useEffect(() => {
        if (viewMode === 'requests') {
            loadRecords(recordsTimeParams, selectedProvider, selectedModel, selectedUser);
        }
    }, [viewMode, recordsTimeParams, selectedProvider, selectedModel, selectedUser, loadRecords]);

    // Reset model pagination when filters or data change
    useEffect(() => {
        setModelsPage(0);
    }, [stats, selectedProvider, selectedModel, selectedUser]);

    // Reset filters if selected provider/model no longer exists in current data
    useEffect(() => {
        if (selectedProvider !== 'all') {
            const providerExists = stats.some(s => s.provider_uuid === selectedProvider);
            if (!providerExists) {
                setSelectedProvider('all');
            }
        }
        if (selectedModel !== 'all') {
            const modelExists = stats.some(s => (s.model || s.key) === selectedModel);
            if (!modelExists) {
                setSelectedModel('all');
            }
        }
        if (selectedUser !== 'all' && !usageIdentities.some((identity) => identity.userId === selectedUser)) {
            setSelectedUser('all');
        }
    }, [stats, usageIdentities, selectedProvider, selectedModel, selectedUser]);

    useEffect(() => {
        if (autoRefresh) {
            const interval = setInterval(() => {
                loadData(selectedProvider, selectedModel, selectedUser, timeRange);
                if (viewMode === 'requests') {
                    loadRecords(recordsTimeParams, selectedProvider, selectedModel, selectedUser);
                }
            }, 60000);
            return () => clearInterval(interval);
        }
    }, [autoRefresh, loadData, selectedProvider, selectedModel, selectedUser, timeRange, viewMode, loadRecords, recordsTimeParams]);

    const handleRefresh = () => {
        setRefreshing(true);
        loadFilterOptions();
        loadData(selectedProvider, selectedModel, selectedUser, timeRange);
        setHeatmapRefresh((n) => n + 1);
    };

    // Calculate totals from stats
    const totalRequests = stats.reduce((sum, s) => sum + (s.request_count || 0), 0);
    const totalInputTokens = stats.reduce((sum, s) => sum + (s.total_input_tokens || 0), 0);
    const totalOutputTokens = stats.reduce((sum, s) => sum + (s.total_output_tokens || 0), 0);
    const totalCacheTokens = stats.reduce((sum, s) => sum + (s.cache_input_tokens || 0), 0);
    const totalTokens = totalInputTokens + totalOutputTokens + totalCacheTokens;

    // Calculate average latency (weighted by request count)
    const totalLatencyWeight = stats.reduce((sum, s) => sum + (s.avg_latency_ms || 0) * (s.request_count || 0), 0);
    const avgLatency = totalRequests > 0 ? totalLatencyWeight / totalRequests : 0;

    // Calculate error rate
    const totalErrors = stats.reduce((sum, s) => sum + (s.error_count || 0), 0);
    const errorRate = totalRequests > 0 ? (totalErrors / totalRequests) * 100 : 0;

    // Calculate streamed rate
    const totalStreamed = stats.reduce((sum, s) => sum + (s.streamed_count || 0), 0);
    const streamedRate = totalRequests > 0 ? (totalStreamed / totalRequests) * 100 : 0;

    // Calculate cache hit rate: cache / (cache + input)
    const cacheHitRate = (totalCacheTokens + totalInputTokens) > 0
        ? (totalCacheTokens / (totalCacheTokens + totalInputTokens)) * 100
        : 0;

    // Build provider name → uuid lookup for top-model click filtering
    const providerNameToUuid = useMemo(() => {
        const map: Record<string, string> = {};
        providers.forEach((p) => { map[p.name] = p.uuid; });
        return map;
    }, [providers]);

    // Prepare chart data - include provider name to distinguish same model from different providers
    // Sort by total tokens first
    const sortedStats = [...stats].sort((a, b) => {
        const totalA = (a.total_input_tokens || 0) + (a.total_output_tokens || 0) + (a.cache_input_tokens || 0);
        const totalB = (b.total_input_tokens || 0) + (b.total_output_tokens || 0) + (b.cache_input_tokens || 0);
        return totalB - totalA;
    });

    const tokenChartData = sortedStats.slice(0, 10).map((stat) => {
        const provider = stat.provider_name || 'Unknown';
        const model = stat.model || stat.key || 'Unknown';
        const label = `${provider} - ${model}`;
        return {
            name: label,
            provider: provider,
            providerUuid: stat.provider_uuid || providerNameToUuid[provider] || '',
            model: model,
            inputTokens: stat.total_input_tokens || 0,
            outputTokens: stat.total_output_tokens || 0,
            cacheTokens: stat.cache_input_tokens || 0,
        };
    });

    // Group providers by auth_type for the dropdown
    const authTypeLabel = (authType: string): string => {
        switch (authType) {
            case 'oauth': return 'OAuth';
            case 'api_key': return 'API Key';
            case 'bearer_token': return 'Bearer Token';
            case 'basic_auth': return 'Basic Auth';
            case 'vmodel': return 'Virtual Model';
            default: return authType || 'Other';
        }
    };

    const AUTH_TYPE_ORDER = ['oauth', 'api_key', 'bearer_token', 'basic_auth', 'vmodel'];

    const groupedProviderOptions = useMemo(() => {
        // Extract provider UUIDs that exist in current stats data
        const providerUuidsInData = new Set(
            stats
                .map(s => s.provider_uuid)
                .filter((uuid): uuid is string => uuid != null && uuid !== '')
        );

        const groups: Record<string, Provider[]> = {};
        providers
            .filter(p => providerUuidsInData.has(p.uuid))  // Only include providers in current data
            .forEach((p) => {
                const authType = p.auth_type || 'api_key';
                if (!groups[authType]) groups[authType] = [];
                groups[authType].push(p);
            });
        // Sort providers within each group by name
        Object.values(groups).forEach((list) => list.sort((a, b) => a.name.localeCompare(b.name)));

        // Return in predefined order, skip empty groups
        return AUTH_TYPE_ORDER
            .filter((t) => groups[t]?.length)
            .map((authType) => ({
                authType,
                label: authTypeLabel(authType),
                providers: groups[authType],
            }));
    }, [providers, stats]);

    // Extract unique models from stats, sorted by usage
    const modelOptions = useMemo(() => {
        const modelMap = new Map<string, { model: string; providerName: string; totalTokens: number }>();
        stats.forEach((stat) => {
            const model = stat.model || stat.key || 'Unknown';
            const totalTokens = (stat.total_input_tokens || 0) + (stat.total_output_tokens || 0) + (stat.cache_input_tokens || 0);
            const existing = modelMap.get(model);
            if (!existing || totalTokens > existing.totalTokens) {
                modelMap.set(model, {
                    model,
                    providerName: stat.provider_name || 'Unknown',
                    totalTokens,
                });
            }
        });
        return Array.from(modelMap.values())
            .sort((a, b) => b.totalTokens - a.totalTokens)
            .map((m) => m.model);
    }, [stats]);

    const hasActiveFilters = selectedProvider !== 'all' || selectedModel !== 'all' || selectedUser !== 'all';

    const selectedIdentityLabel = selectedUser === 'all'
        ? 'All identities'
        : usageIdentities.find((identity) => identity.userId === selectedUser)?.label || shortenUserId(selectedUser);

    const handleClearFilters = () => {
        setSelectedProvider('all');
        setSelectedModel('all');
        setSelectedUser('all');
    };

    if (loading) {
        return <DashboardSkeleton />;
    }

    const headerActions = (
        <>
            <FormControl size="small" sx={{ minWidth: { xs: 140, sm: 160 } }}>
                <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>Provider</InputLabel>
                <Select
                    value={selectedProvider}
                    label="Provider"
                    onChange={(e) => setSelectedProvider(e.target.value)}
                    sx={{
                        borderRadius: 2,
                        '& .MuiOutlinedInput-input': { py: 1 },
                    }}
                >
                    <MenuItem value="all">All Providers</MenuItem>
                    {groupedProviderOptions.map((group) => [
                        <ListSubheader
                            key={`header-${group.authType}`}
                            sx={{
                                fontWeight: 600,
                                fontSize: '0.7rem',
                                textTransform: 'uppercase',
                                letterSpacing: '0.05em',
                                lineHeight: '28px',
                                pt: 1,
                                pl: 1.5,
                                borderLeft: '3px solid',
                                borderLeftColor: 'primary.main',
                                backgroundColor: 'action.hover',
                            }}
                        >
                            {group.label}
                        </ListSubheader>,
                        ...group.providers.map((p) => (
                            <MenuItem key={p.uuid} value={p.uuid}>
                                {p.name}
                            </MenuItem>
                        )),
                    ])}
                </Select>
            </FormControl>

            <FormControl size="small" sx={{ minWidth: { xs: 140, sm: 160 } }}>
                <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>Model</InputLabel>
                <Select
                    value={selectedModel}
                    label="Model"
                    onChange={(e) => setSelectedModel(e.target.value)}
                    disabled={!stats.length}
                    sx={{
                        borderRadius: 2,
                        '& .MuiOutlinedInput-input': { py: 1 },
                    }}
                >
                    <MenuItem value="all">All Models</MenuItem>
                    {modelOptions.map((model) => (
                        <MenuItem key={model} value={model}>
                            {model}
                        </MenuItem>
                    ))}
                </Select>
            </FormControl>

            <FormControl size="small" sx={{ minWidth: { xs: 160, sm: 200 } }}>
                <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>Identity</InputLabel>
                <Select
                    value={selectedUser}
                    label="Identity"
                    onChange={(e) => setSelectedUser(e.target.value)}
                    renderValue={() => selectedIdentityLabel}
                    sx={{
                        borderRadius: 2,
                        '& .MuiOutlinedInput-input': { py: 1 },
                    }}
                >
                    <MenuItem value="all">All identities</MenuItem>
                    <ListSubheader>Account</ListSubheader>
                    {usageIdentities.filter((identity) => identity.type === 'owner').map((identity) => (
                        <MenuItem key={identity.userId} value={identity.userId}>
                            {identity.label}
                        </MenuItem>
                    ))}
                    {usageIdentities.some((identity) => identity.type === 'sharing_key') && (
                        <ListSubheader>Sharing Keys</ListSubheader>
                    )}
                    {usageIdentities.filter((identity) => identity.type === 'sharing_key').map((identity) => (
                        <MenuItem key={identity.userId} value={identity.userId}>
                            <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 2, width: '100%' }}>
                                <Typography variant="body2" noWrap>
                                    {identity.label}{!identity.enabled ? ' (disabled)' : ''}
                                </Typography>
                                <Tooltip title={identity.userId} placement="right">
                                    <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace', flexShrink: 0 }}>
                                        {shortenUserId(identity.userId)}
                                    </Typography>
                                </Tooltip>
                            </Box>
                        </MenuItem>
                    ))}
                </Select>
            </FormControl>

            {hasActiveFilters && (
                <>
                    <Divider orientation="vertical" flexItem sx={{ mx: 0.5, display: { xs: 'none', sm: 'block' } }} />
                    <Tooltip title="Clear all filters">
                        <IconButton
                            size="small"
                            onClick={handleClearFilters}
                            sx={{
                                backgroundColor: 'action.hover',
                                '&:hover': { backgroundColor: 'action.selected' },
                            }}
                        >
                            <FilterOff />
                        </IconButton>
                    </Tooltip>
                </>
            )}

            <Divider orientation="vertical" flexItem sx={{ mx: 0.5, display: { xs: 'none', sm: 'block' } }} />

            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <FormControlLabel
                    control={
                        <Switch
                            size="small"
                            checked={autoRefresh}
                            onChange={(e) => setAutoRefresh(e.target.checked)}
                            color="primary"
                        />
                    }
                    label={<Typography variant="body2">Auto</Typography>}
                    sx={switchControlLabelStyle}
                />
                <Tooltip title="Refresh data">
                    <IconButton
                        size="small"
                        onClick={handleRefresh}
                        disabled={refreshing}
                        sx={{
                            backgroundColor: 'action.hover',
                            '&:hover': { backgroundColor: 'action.selected' },
                            '&:disabled': { backgroundColor: 'transparent' },
                        }}
                    >
                        {refreshing ? <CircularProgress size={18} /> : <RefreshIcon />}
                    </IconButton>
                </Tooltip>
            </Box>
        </>
    );

    return (
        <Box
            sx={{
                display: 'flex',
                flexDirection: 'column',
                gap: 3,
                minHeight: '100vh',
            }}
        >
            <PageHeader
                title="Usage Dashboard"
                subtitle={TIME_RANGE_CONFIG[timeRange].label}
                actions={headerActions}
            />

            {/* Main Content: Three Column Layout */}
            <Box
                sx={{
                    display: 'flex',
                    gap: 2,
                    flexDirection: { xs: 'column', md: 'row' },
                }}
            >
                {/* Middle Column (68%) */}
                <Box sx={{ flex: { xs: 1, md: 7, lg: 6.8 }, display: 'flex', flexDirection: 'column', gap: 2 }}>
                    {/* Stat Cards Row - 5 cards */}
                    <Grid container spacing={{ xs: 1.5, sm: 2 }}>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title="Total Requests"
                                value={totalRequests.toLocaleString()}
                                subtitle={TIME_RANGE_CONFIG[timeRange].label}
                                icon={<CallMadeIcon />}
                                // Volume metric — no health judgment, so keep it neutral.
                                color="secondary"
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title="Total Tokens"
                                value={formatNumber(totalTokens)}
                                subtitle={`Input: ${formatNumber(totalInputTokens)} + Cache: ${formatNumber(totalCacheTokens)}\nOutput: ${formatNumber(totalOutputTokens)}`}
                                icon={<PaidIcon />}
                                // Volume metric — no health judgment, so keep it neutral.
                                color="secondary"
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title="Cache Hit Rate"
                                value={`${cacheHitRate.toFixed(1)}%`}
                                subtitle={`${formatNumber(totalCacheTokens)} cached`}
                                icon={<CachedIcon />}
                                // Health gauge, inverted from Error Rate: higher is better,
                                // so a too-low cache-hit rate is a problem.
                                // green healthy (>=50%) -> amber low (>=20%) -> red too low.
                                color={cacheHitRate >= 50 ? 'success' : cacheHitRate >= 20 ? 'warning' : 'error'}
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title="Error Rate"
                                value={`${errorRate.toFixed(2)}%`}
                                subtitle={`${totalErrors} errors`}
                                icon={<ErrorOutlineIcon />}
                                // Health gauge: green healthy → amber elevated → red high.
                                color={errorRate > 5 ? 'error' : errorRate > 1 ? 'warning' : 'success'}
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title="Streamed Rate"
                                value={`${streamedRate.toFixed(1)}%`}
                                subtitle={`${totalStreamed} streamed`}
                                icon={<StreamIcon />}
                                color="secondary"
                            />
                        </Grid>
                    </Grid>

                    {/* Chart view toggle: Summary trend, By Request (hourly only),
                        or the 12-month Activity heatmap. flex: 1 so the active
                        view can use the full pane height (the Activity grid
                        centers itself vertically in it). */}
                    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                        <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                            <ToggleButtonGroup
                                value={effectiveViewMode}
                                exclusive
                                onChange={(_, v) => v && setViewMode(v)}
                                size="small"
                                sx={{
                                    '& .MuiToggleButton-root': {
                                        px: 1.75,
                                        py: 0.375,
                                        fontSize: '0.78rem',
                                        textTransform: 'none',
                                    },
                                }}
                            >
                                <ToggleButton value="summary">Summary</ToggleButton>
                                {isHourlyRange && <ToggleButton value="requests">By Request</ToggleButton>}
                                <ToggleButton value="activity">Activity</ToggleButton>
                            </ToggleButtonGroup>
                        </Box>

                        {effectiveViewMode === 'activity' ? (
                            <DashboardHeatmapSection provider={selectedProvider} refreshKey={heatmapRefresh} />
                        ) : effectiveViewMode === 'summary' ? (
                            timeRange === 'today' || timeRange === 'yesterday' ? (
                                <HourlyTokenHistoryChart data={timeSeries} />
                            ) : (
                                <DailyTokenHistoryChart data={timeSeries} />
                            )
                        ) : (
                            <RequestsView records={records} loading={recordsLoading} />
                        )}
                    </Box>
                </Box>

                {/* Right Column (20%) - Token Usage List */}
                <Box sx={{ flex: { xs: 1, md: 3, lg: 2 } }}>
                    <Paper
                        elevation={0}
                        sx={{
                            p: 2.5,
                            borderRadius: 2,
                            border: '1px solid',
                            borderColor: 'divider',
                            backgroundColor: 'background.paper',
                            boxShadow: 'none',
                            height: '100%',
                            display: 'flex',
                            flexDirection: 'column',
                        }}
                    >
                        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                            <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                                Models by Token Usage
                            </Typography>
                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                {tokenChartData.length} total
                            </Typography>
                        </Box>

                        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 1, overflow: 'hidden' }}>
                            {tokenChartData.length === 0 ? (
                                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1 }}>
                                    <Typography variant="body2" color="text.secondary">No models found</Typography>
                                </Box>
                            ) : (
                                <>
                                    {tokenChartData
                                        .slice(modelsPage * modelsPerPage, (modelsPage + 1) * modelsPerPage)
                                        .map((item, index) => {
                                            const globalIndex = modelsPage * modelsPerPage + index;
                                            const totalTokens = item.inputTokens + item.outputTokens + (item.cacheTokens || 0);
                                            const maxTokens = Math.max(...tokenChartData.map(d => d.inputTokens + d.outputTokens + (d.cacheTokens || 0)));
                                            const percentage = maxTokens > 0 ? (totalTokens / maxTokens) * 100 : 0;

                                return (
                                    <Tooltip
                                        key={index}
                                        componentsProps={{
                                            tooltip: {
                                                sx: {
                                                    backgroundColor: theme.palette.mode === 'dark' ? '#1e293b' : '#ffffff',
                                                    color: theme.palette.mode === 'dark' ? '#f1f5f9' : '#1a1a1a',
                                                    fontSize: '0.75rem',
                                                    p: 1.5,
                                                    borderRadius: 1.5,
                                                    border: '1px solid',
                                                    borderColor: theme.palette.mode === 'dark' ? '#334155' : '#e2e8f0',
                                                    '& .MuiTooltip-arrow': {
                                                        color: theme.palette.mode === 'dark' ? '#1e293b' : '#ffffff',
                                                    },
                                                },
                                            },
                                        }}
                                        title={
                                            <Box>
                                                <Typography sx={{ fontWeight: 600, fontSize: '0.8rem', mb: 0.5 }}>{item.model}</Typography>
                                                <Typography sx={{ color: theme.palette.mode === 'dark' ? '#94a3b8' : '#a0a0a0', fontSize: '0.75rem' }}>{item.provider}</Typography>
                                                <Typography sx={{ color: theme.palette.mode === 'dark' ? '#94a3b8' : '#a0a0a0', fontSize: '0.7rem', mt: 0.75 }}>
                                                    Total: {formatNumber(totalTokens)} | Input: {formatNumber(item.inputTokens)} | Output: {formatNumber(item.outputTokens)}
                                                </Typography>
                                            </Box>
                                        }
                                        arrow
                                        placement="left"
                                    >
                                        <Box
                                            onClick={() => {
                                                // When clicking a model, filter by that model
                                                if (item.model) {
                                                    setSelectedModel(item.model);
                                                }
                                            }}
                                            sx={{
                                                display: 'flex',
                                                alignItems: 'center',
                                                gap: 1,
                                                py: 1,
                                                px: 1,
                                                borderRadius: 1,
                                                transition: 'background-color 0.15s ease',
                                                cursor: item.model ? 'pointer' : 'default',
                                                '&:hover': {
                                                    backgroundColor: 'action.hover',
                                                },
                                            }}
                                        >
                                            {/* Rank Badge */}
                                            <Box
                                                sx={{
                                                    minWidth: 18,
                                                    height: 18,
                                                    borderRadius: 1,
                                                    backgroundColor: 'action.selected',
                                                    color: 'text.secondary',
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    fontSize: '0.65rem',
                                                    fontWeight: 600,
                                                    flexShrink: 0,
                                                }}
                                            >
                                                {globalIndex + 1}
                                            </Box>

                                            {/* Content */}
                                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                                {/* Model Name */}
                                                <Box
                                                    component="span"
                                                    sx={{
                                                        display: 'block',
                                                        fontWeight: 500,
                                                        fontSize: '0.7rem',
                                                        overflow: 'hidden',
                                                        textOverflow: 'ellipsis',
                                                        whiteSpace: 'nowrap',
                                                        mb: 0.5,
                                                    }}
                                                >
                                                    {item.model}
                                                </Box>

                                                {/* Progress Bar + Value */}
                                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                                    <Box
                                                        sx={{
                                                            flex: 1,
                                                            height: 4,
                                                            borderRadius: 2,
                                                            backgroundColor: 'action.hover',
                                                            overflow: 'hidden',
                                                        }}
                                                    >
                                                        <Box
                                                            sx={{
                                                                height: '100%',
                                                                width: `${percentage}%`,
                                                                borderRadius: 2,
                                                                backgroundColor: 'primary.main',
                                                                transition: 'width 0.3s ease',
                                                            }}
                                                        />
                                                    </Box>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            fontSize: '0.65rem',
                                                            color: 'text.secondary',
                                                            minWidth: 40,
                                                            flexShrink: 0,
                                                            textAlign: 'right',
                                                        }}
                                                    >
                                                        {formatNumber(totalTokens)}
                                                    </Typography>
                                                </Box>
                                            </Box>
                                        </Box>
                                    </Tooltip>
                                );
                            })}

                            {/* Pagination Controls */}
                            {tokenChartData.length > modelsPerPage && (
                                <Box
                                    sx={{
                                        display: 'flex',
                                        justifyContent: 'center',
                                        alignItems: 'center',
                                        gap: 0.5,
                                        pt: 1.5,
                                        borderTop: '1px solid',
                                        borderColor: 'divider',
                                        mt: 'auto',
                                    }}
                                >
                                    <IconButton
                                        size="small"
                                        onClick={() => setModelsPage(p => Math.max(0, p - 1))}
                                        disabled={modelsPage === 0}
                                        sx={{ borderRadius: 1 }}
                                    >
                                        <ChevronLeft sx={{ fontSize: '1.1rem' }} />
                                    </IconButton>
                                    <Typography
                                        variant="caption"
                                        sx={{ minWidth: 60, textAlign: 'center', color: 'text.secondary', fontSize: '0.75rem' }}
                                    >
                                        {modelsPage + 1} / {Math.ceil(tokenChartData.length / modelsPerPage)}
                                    </Typography>
                                    <IconButton
                                        size="small"
                                        onClick={() => setModelsPage(p => Math.min(Math.ceil(tokenChartData.length / modelsPerPage) - 1, p + 1))}
                                        disabled={modelsPage >= Math.ceil(tokenChartData.length / modelsPerPage) - 1}
                                        sx={{ borderRadius: 1 }}
                                    >
                                        <ChevronRight sx={{ fontSize: '1.1rem' }} />
                                    </IconButton>
                                </Box>
                            )}
                        </>
                        )}
                        </Box>
                    </Paper>
                </Box>
            </Box>

            {/* Stats Table */}
            <ServiceStatsTable stats={stats} />
        </Box>
    );
}
