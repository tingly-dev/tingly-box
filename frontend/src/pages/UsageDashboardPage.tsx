import { useCallback, useEffect, useState } from 'react';
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
    ToggleButton,
    ToggleButtonGroup,
    Paper,
    Divider,
    alpha,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import CallMadeIcon from '@mui/icons-material/CallMade';
import PaidIcon from '@mui/icons-material/Paid';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import StreamIcon from '@mui/icons-material/Stream';
import SpeedIcon from '@mui/icons-material/Speed';
import { StatCard, TokenUsageChart, DailyTokenHistoryChart, HourlyTokenHistoryChart, ServiceStatsTable } from '@/components/dashboard';
import type { TimeSeriesData, AggregatedStat } from '@/components/dashboard';
import api from '../services/api';

interface Provider {
    uuid: string;
    name: string;
}

type TimeRange = 'today' | '7d' | '30d' | '90d';

const TIME_RANGE_CONFIG: Record<TimeRange, { label: string; days: number; interval: string }> = {
    today: { label: 'Today', days: 1, interval: 'hour' },
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

export default function UsageDashboardPage() {
    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [stats, setStats] = useState<AggregatedStat[]>([]);
    const [timeSeries, setTimeSeries] = useState<TimeSeriesData[]>([]);
    const [providers, setProviders] = useState<Provider[]>([]);
    const [selectedProvider, setSelectedProvider] = useState<string>('all');
    const [timeRange, setTimeRange] = useState<TimeRange>('7d');

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
    const totalTokens = totalInputTokens + totalOutputTokens;

    // Calculate average latency (weighted by request count)
    const totalLatencyWeight = stats.reduce((sum, s) => sum + (s.avg_latency_ms || 0) * (s.request_count || 0), 0);
    const avgLatency = totalRequests > 0 ? totalLatencyWeight / totalRequests : 0;

    // Calculate error rate
    const totalErrors = stats.reduce((sum, s) => sum + (s.error_count || 0), 0);
    const errorRate = totalRequests > 0 ? (totalErrors / totalRequests) * 100 : 0;

    // Calculate streamed rate
    const totalStreamed = stats.reduce((sum, s) => sum + (s.streamed_count || 0), 0);
    const streamedRate = totalRequests > 0 ? (totalStreamed / totalRequests) * 100 : 0;

    // Prepare chart data - include provider name to distinguish same model from different providers
    const tokenChartData = stats.slice(0, 10).map((stat) => {
        const provider = stat.provider_name || 'Unknown';
        const model = stat.model || stat.key || 'Unknown';
        const label = `${provider} - ${model}`;
        return {
            name: label.length > 30 ? label.substring(0, 30) + '...' : label,
            inputTokens: stat.total_input_tokens || 0,
            outputTokens: stat.total_output_tokens || 0,
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
                px: { xs: 3, sm: 4, md: 5 },
                py: { xs: 4, sm: 5, md: 6 },
                maxWidth: 1400,
                mx: 'auto',
                minHeight: '100vh',
                backgroundColor: 'background.default',
            }}
        >
            {/* Header with Filters */}
            <Paper
                sx={{
                    p: { xs: 2, sm: 2.5 },
                    mb: 4,
                    borderRadius: 2.5,
                    border: '1px solid',
                    borderColor: 'divider',
                    boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    flexWrap: 'wrap',
                    gap: 2,
                }}
            >
                <Typography variant="h4" sx={{ fontWeight: 700, fontSize: '1.5rem', letterSpacing: '-0.02em' }}>
                    Usage Dashboard
                </Typography>

                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
                    {/* Time Range Selector */}
                    <ToggleButtonGroup
                        value={timeRange}
                        exclusive
                        onChange={(_, value) => value && setTimeRange(value)}
                        size="small"
                        sx={{
                            backgroundColor: 'action.hover',
                            borderRadius: 2,
                            p: 0.5,
                            '& .MuiToggleButton-root': {
                                borderRadius: 1.5,
                                px: 2,
                                py: 0.75,
                                fontSize: '0.875rem',
                                fontWeight: 500,
                                textTransform: 'none',
                                border: 'none',
                                color: 'text.secondary',
                                transition: 'all 0.2s ease',
                                '&.Mui-selected': {
                                    backgroundColor: 'background.paper',
                                    color: 'primary.main',
                                    fontWeight: 600,
                                    boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
                                },
                                '&:hover': {
                                    backgroundColor: 'background.paper',
                                    color: 'text.primary',
                                },
                            },
                        }}
                    >
                        {Object.entries(TIME_RANGE_CONFIG).map(([key, config]) => (
                            <ToggleButton key={key} value={key}>
                                {config.label}
                            </ToggleButton>
                        ))}
                    </ToggleButtonGroup>

                    {/* Provider Selector */}
                    <FormControl size="small" sx={{ minWidth: 150 }}>
                        <InputLabel sx={{ fontWeight: 500 }}>Provider</InputLabel>
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
                                />
                            }
                            label={<Typography variant="body2">Auto</Typography>}
                            sx={{ mr: 0 }}
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

            {/* Stats Cards - Row 1 */}
            <Grid container spacing={2.5} sx={{ mb: 4 }}>
                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <StatCard
                        title="Total Requests"
                        value={totalRequests.toLocaleString()}
                        subtitle={TIME_RANGE_CONFIG[timeRange].label}
                        icon={<CallMadeIcon />}
                        color="primary"
                    />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <StatCard
                        title="Total Tokens"
                        value={formatNumber(totalTokens)}
                        subtitle={`${formatNumber(totalInputTokens)} in / ${formatNumber(totalOutputTokens)} out`}
                        icon={<PaidIcon />}
                        color="success"
                    />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <StatCard
                        title="Error Rate"
                        value={`${errorRate.toFixed(2)}%`}
                        subtitle={`${totalErrors} errors`}
                        icon={<ErrorOutlineIcon />}
                        color={errorRate > 5 ? 'error' : 'success'}
                    />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <StatCard
                        title="Streamed Rate"
                        value={`${streamedRate.toFixed(1)}%`}
                        subtitle={`${totalStreamed} streamed`}
                        icon={<StreamIcon />}
                        color="secondary"
                    />
                </Grid>
            </Grid>

            {/* Charts */}
            <Grid container spacing={2.5} sx={{ mb: 4 }}>
                <Grid size={{ xs: 12, md: 6 }}>
                    {timeRange === 'today' ? (
                        <HourlyTokenHistoryChart data={timeSeries} />
                    ) : (
                        <DailyTokenHistoryChart data={timeSeries} />
                    )}
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                    <TokenUsageChart data={tokenChartData} />
                </Grid>
            </Grid>

            {/* Stats Table */}
            <ServiceStatsTable stats={stats} />
        </Box>
    );
}
