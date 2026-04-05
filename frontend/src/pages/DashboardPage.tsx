import { useCallback, useEffect, useState } from 'react';
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
    FormControl,
    InputLabel,
    Select,
    MenuItem,
    Paper,
    Divider,
    useTheme,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import CallMadeIcon from '@mui/icons-material/CallMade';
import PaidIcon from '@mui/icons-material/Paid';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import StreamIcon from '@mui/icons-material/Stream';
import SpeedIcon from '@mui/icons-material/Speed';
import CachedIcon from '@mui/icons-material/Cached';
import { StatCard, TokenUsageChart, DailyTokenHistoryChart, HourlyTokenHistoryChart, ServiceStatsTable } from '@/components/dashboard';
import type { TimeSeriesData, AggregatedStat } from '@/components/dashboard';
import { switchControlLabelStyle } from '@/styles/toggleStyles';
import api from '../services/api';

interface Provider {
    uuid: string;
    name: string;
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

export default function DashboardPage() {
    const theme = useTheme();
    const { timeRange: urlTimeRange } = useParams<{ timeRange: TimeRange }>();
    const navigate = useNavigate();

    // Validate and set time range from URL
    const validTimeRanges: TimeRange[] = ['today', 'yesterday', '3d', '7d', '30d', '90d'];
    const timeRange: TimeRange = validTimeRanges.includes(urlTimeRange as TimeRange)
        ? (urlTimeRange as TimeRange)
        : '7d';

    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(false);
    const [stats, setStats] = useState<AggregatedStat[]>([]);
    const [timeSeries, setTimeSeries] = useState<TimeSeriesData[]>([]);
    const [providers, setProviders] = useState<Provider[]>([]);
    const [selectedProvider, setSelectedProvider] = useState<string>('all');

    const loadData = useCallback(async (provider: string, range: TimeRange) => {
        try {
            // Build query params based on time range
            const now = new Date();
            const config = TIME_RANGE_CONFIG[range];

            // Calculate start time based on today 00:00:00 LOCAL time
            // For multi-day mode, start from (config.days - 1) days ago at 00:00:00
            // For 'today' mode, start from today 00:00:00
            const todayStart = getLocalMidnight(now);

            const startTime = new Date(todayStart);
            let endTime: Date;

            if (range === 'today') {
                // For today: from today 00:00:00 to now
                endTime = now;
            } else if (range === 'yesterday') {
                // For yesterday: from yesterday 00:00:00 to today 00:00:00
                startTime.setDate(startTime.getDate() - 1);
                endTime = new Date(todayStart);
            } else {
                // For multi-day mode: from (N-1) days ago 00:00:00 to tomorrow 00:00:00
                // This ensures we get complete data for today
                startTime.setDate(startTime.getDate() - (config.days - 1));
                endTime = new Date(todayStart);
                endTime.setDate(endTime.getDate() + 1); // Next day at 00:00:00
            }

            const params: Record<string, string> = {
                start_time: toLocalISOString(startTime),
                end_time: toLocalISOString(endTime),
            };
            if (provider && provider !== 'all') {
                params.provider = provider;
            }

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
        } catch (error) {
            console.error('Failed to load dashboard data:', error);
        } finally {
            setLoading(false);
            setRefreshing(false);
        }
    }, []);

    useEffect(() => {
        loadData(selectedProvider, timeRange);
    }, [loadData, selectedProvider, timeRange]);

    useEffect(() => {
        if (autoRefresh) {
            const interval = setInterval(() => {
                loadData(selectedProvider, timeRange);
            }, 60000);
            return () => clearInterval(interval);
        }
    }, [autoRefresh, loadData, selectedProvider, timeRange]);

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

    if (loading) {
        return (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '50vh' }}>
                <CircularProgress />
            </Box>
        );
    }

    return (
        <Box
            sx={{
                display: 'flex',
                flexDirection: 'column',
                gap: 1,
                minHeight: '100vh',
            }}
        >
            {/* Header with Filters */}
            <Paper
                sx={{
                    p: 2,
                    mb: 3,
                    borderRadius: 2,
                    border: '1px solid',
                    borderColor: 'divider',
                    boxShadow: '0 2px 4px rgba(0, 0, 0, 0.1)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    flexWrap: 'wrap',
                    gap: 2,
                }}
            >
                <Box>
                    <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem', letterSpacing: '-0.01em' }}>
                        Usage Dashboard
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.25, fontSize: '0.875rem' }}>
                        {TIME_RANGE_CONFIG[timeRange].label}
                    </Typography>
                </Box>

                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
                    {/* Provider Selector */}
                    <FormControl size="small" sx={{ minWidth: 150 }}>
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
                            {providers.map((p) => (
                                <MenuItem key={p.uuid} value={p.uuid}>
                                    {p.name}
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>

                    <Divider orientation="vertical" flexItem sx={{ mx: 0.5 }} />

                    {/* Auto Refresh & Refresh */}
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
                            label={<Typography variant="body2" sx={{ fontSize: '0.875rem' }}>Auto</Typography>}
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
                </Box>
            </Paper>

            {/* Main Content: Two Column Layout */}
            <Box
                sx={{
                    display: 'flex',
                    gap: 2,
                    mb: 3,
                    flexDirection: { xs: 'column', md: 'row' },
                }}
            >
                {/* Left Column (70%) */}
                <Box sx={{ flex: { xs: 1, md: 7 }, display: 'flex', flexDirection: 'column', gap: 2 }}>
                    {/* Stat Cards Row - 5 cards */}
                    <Grid container spacing={{ xs: 1.5, sm: 2 }}>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title={'Total\nRequests'}
                                value={totalRequests.toLocaleString()}
                                subtitle={TIME_RANGE_CONFIG[timeRange].label}
                                icon={<CallMadeIcon />}
                                color="primary"
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title={'Total\nTokens'}
                                value={formatNumber(totalTokens)}
                                subtitle={`Input: ${formatNumber(totalInputTokens)}\nOutput: ${formatNumber(totalOutputTokens)}`}
                                icon={<PaidIcon />}
                                color="success"
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title={'Cache Hit\nRate'}
                                value={`${cacheHitRate.toFixed(1)}%`}
                                subtitle={`${formatNumber(totalCacheTokens)} cached`}
                                icon={<CachedIcon />}
                                color={cacheHitRate >= 50 ? 'success' : cacheHitRate >= 20 ? 'info' : 'warning'}
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title={'Error\nRate'}
                                value={`${errorRate.toFixed(2)}%`}
                                subtitle={`${totalErrors} errors`}
                                icon={<ErrorOutlineIcon />}
                                color={errorRate > 5 ? 'error' : errorRate > 1 ? 'warning' : 'info'}
                            />
                        </Grid>
                        <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                            <StatCard
                                title={'Streamed\nRate'}
                                value={`${streamedRate.toFixed(1)}%`}
                                subtitle={`${totalStreamed} streamed`}
                                icon={<StreamIcon />}
                                color="secondary"
                            />
                        </Grid>
                    </Grid>

                    {/* Time Series Chart - Full Width */}
                    <Box sx={{ display: 'flex' }}>
                        {timeRange === 'today' || timeRange === 'yesterday' ? (
                            <HourlyTokenHistoryChart data={timeSeries} />
                        ) : (
                            <DailyTokenHistoryChart data={timeSeries} />
                        )}
                    </Box>
                </Box>

                {/* Right Column (30%) - Token Usage List */}
                <Box sx={{ flex: { xs: 1, md: 3 } }}>
                    <Paper
                        elevation={0}
                        sx={{
                            p: 2.5,
                            borderRadius: 2,
                            border: '1px solid',
                            borderColor: 'divider',
                            backgroundColor: 'background.paper',
                            boxShadow: '0 2px 4px rgba(0, 0, 0, 0.1)',
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
                                            sx={{
                                                display: 'flex',
                                                alignItems: 'center',
                                                gap: 1,
                                                py: 1,
                                                px: 1,
                                                borderRadius: 1,
                                                transition: 'all 0.15s ease',
                                                cursor: 'pointer',
                                                '&:hover': {
                                                    backgroundColor: 'action.hover',
                                                },
                                            }}
                                        >
                                            {/* Rank Badge */}
                                            <Box
                                                sx={{
                                                    minWidth: 20,
                                                    height: 20,
                                                    borderRadius: 1,
                                                    backgroundColor: 'action.selected',
                                                    color: 'text.secondary',
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    fontSize: '0.7rem',
                                                    fontWeight: 600,
                                                }}
                                            >
                                                {index + 1}
                                            </Box>

                                            {/* Content */}
                                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                                {/* Model Name */}
                                                <Typography
                                                    variant="body2"
                                                    sx={{
                                                        fontWeight: 500,
                                                        fontSize: '0.8rem',
                                                        overflow: 'hidden',
                                                        textOverflow: 'ellipsis',
                                                        whiteSpace: 'nowrap',
                                                        mb: 0.5,
                                                    }}
                                                >
                                                    {item.model}
                                                </Typography>

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
                                                            fontSize: '0.7rem',
                                                            color: 'text.secondary',
                                                            minWidth: 50,
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
