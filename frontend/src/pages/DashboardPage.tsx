import { useCallback, useEffect, useMemo, useState } from 'react';
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
import { Refresh as RefreshIcon, Outbound as CallMadeIcon, ErrorOutline as ErrorOutlineIcon } from '@/components/icons';
import { tablerMui } from '@/components/icons';
import { IconCoin, IconActivity, IconReload } from '@tabler/icons-react';

// Predefined ones come from the central module; these three have no MUI-named
// equivalent there, so build them ad-hoc via the generic factory.
const PaidIcon = tablerMui(IconCoin);
const StreamIcon = tablerMui(IconActivity);
const CachedIcon = tablerMui(IconReload);
import { StatCard, DailyTokenHistoryChart, HourlyTokenHistoryChart, ServiceStatsTable, AgentQuickNav, RequestsView } from '@/components/dashboard';
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

type TimeRange = 'today' | 'yesterday' | '3d' | '7d' | '30d' | '90d';

const TIME_RANGE_CONFIG: Record<TimeRange, { label: string; days: number; interval: string }> = {
    today: { label: 'Today', days: 1, interval: 'hour' },
    yesterday: { label: 'Yesterday', days: 1, interval: 'hour' },
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
    const [selectedProvider, setSelectedProvider] = useState<string>('all');

    // By-request view state
    const [viewMode, setViewMode] = useState<'summary' | 'requests'>('summary');
    const [records, setRecords] = useState<UsageRecord[]>([]);
    const [recordsLoading, setRecordsLoading] = useState(false);
    const [recordsTimeParams, setRecordsTimeParams] = useState<{ start_time: string; end_time: string } | null>(null);

    const buildTimeParams = useCallback((provider: string, range: TimeRange) => {
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
        return params;
    }, []);

    const loadData = useCallback(async (provider: string, range: TimeRange) => {
        try {
            const config = TIME_RANGE_CONFIG[range];
            const params = buildTimeParams(provider, range);

            const [statsResult, timeSeriesResult, providersResult] = await Promise.all([
                api.getUsageStats({ ...params, group_by: 'model', limit: 100 }),
                api.getUsageTimeSeries({ ...params, interval: config.interval }),
                api.getProviders(),
            ]);

            if (statsResult?.data) {
                setStats(statsResult.data);
            }
            if (timeSeriesResult?.data) {
                setTimeSeries(timeSeriesResult.data);
            }
            if (providersResult?.success && providersResult?.data) {
                setProviders(providersResult.data);
            }

            // Store time params for records loading
            setRecordsTimeParams({ start_time: params.start_time, end_time: params.end_time });
        } catch (error) {
            console.error('Failed to load dashboard data:', error);
        } finally {
            setLoading(false);
            setRefreshing(false);
        }
    }, [buildTimeParams]);

    const loadRecords = useCallback(async (
        timeParams: { start_time: string; end_time: string } | null,
        provider: string,
    ) => {
        if (!timeParams) return;
        setRecordsLoading(true);
        try {
            const result = await api.getUsageRecords({
                ...timeParams,
                ...(provider !== 'all' ? { provider } : {}),
                limit: 500,
                offset: 0,
            });
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
        loadData(selectedProvider, timeRange);
    }, [loadData, selectedProvider, timeRange]);

    // Reset view mode when switching away from hourly ranges
    useEffect(() => {
        if (!isHourlyRange) {
            setViewMode('summary');
        }
    }, [isHourlyRange]);

    // Load records when entering requests view or time/provider changes
    useEffect(() => {
        if (viewMode === 'requests') {
            loadRecords(recordsTimeParams, selectedProvider);
        }
    }, [viewMode, recordsTimeParams, selectedProvider, loadRecords]);

    useEffect(() => {
        if (autoRefresh) {
            const interval = setInterval(() => {
                loadData(selectedProvider, timeRange);
                if (viewMode === 'requests') {
                    loadRecords(recordsTimeParams, selectedProvider);
                }
            }, 60000);
            return () => clearInterval(interval);
        }
    }, [autoRefresh, loadData, selectedProvider, timeRange, viewMode, loadRecords, recordsTimeParams]);

    const handleRefresh = () => {
        setRefreshing(true);
        loadData(selectedProvider, timeRange);
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

    // Format large numbers
    const formatNumber = (num: number): string => {
        if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
        if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
        return num.toLocaleString();
    };

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
        const groups: Record<string, Provider[]> = {};
        providers.forEach((p) => {
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
    }, [providers]);

    if (loading) {
        return <DashboardSkeleton />;
    }

    const headerActions = (
        <>
            <FormControl size="small" sx={{ minWidth: { xs: 180, sm: 150 } }}>
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
                                // colored left accent bar
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
                                color="primary"
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title="Total Tokens"
                                value={formatNumber(totalTokens)}
                                subtitle={`Input: ${formatNumber(totalInputTokens)}\nOutput: ${formatNumber(totalOutputTokens)}`}
                                icon={<PaidIcon />}
                                color="success"
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title="Cache Hit Rate"
                                value={`${cacheHitRate.toFixed(1)}%`}
                                subtitle={`${formatNumber(totalCacheTokens)} cached`}
                                icon={<CachedIcon />}
                                color={cacheHitRate >= 50 ? 'success' : cacheHitRate >= 20 ? 'info' : 'warning'}
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title="Error Rate"
                                value={`${errorRate.toFixed(2)}%`}
                                subtitle={`${totalErrors} errors`}
                                icon={<ErrorOutlineIcon />}
                                color={errorRate > 5 ? 'error' : errorRate > 1 ? 'warning' : 'info'}
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

                    {/* Chart / Requests toggle */}
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                        {isHourlyRange && (
                            <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                                <ToggleButtonGroup
                                    value={viewMode}
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
                                    <ToggleButton value="requests">By Request</ToggleButton>
                                </ToggleButtonGroup>
                            </Box>
                        )}

                        {viewMode === 'summary' ? (
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
                        <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem', mb: 2 }}>
                            Top Models by Token Usage
                        </Typography>
                        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 1 }}>
                            {tokenChartData.slice(0, 6).map((item, index) => {
                                const totalTokens = item.inputTokens + item.outputTokens + (item.cacheTokens || 0);
                                const maxTokens = Math.max(...tokenChartData.slice(0, 6).map(d => d.inputTokens + d.outputTokens + (d.cacheTokens || 0)));
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
                                                if (item.providerUuid) {
                                                    setSelectedProvider(item.providerUuid);
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
                                                cursor: item.providerUuid ? 'pointer' : 'default',
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
                                                {index + 1}
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
                        </Box>
                    </Paper>
                </Box>
            </Box>

            {/* Stats Table */}
            <ServiceStatsTable stats={stats} />
        </Box>
    );
}
