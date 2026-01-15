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
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import CallMadeIcon from '@mui/icons-material/CallMade';
import PaidIcon from '@mui/icons-material/Paid';
import BoltIcon from '@mui/icons-material/Bolt';
import { StatCard, TokenUsageChart, TokenHistoryChart, ServiceStatsTable } from '../components/dashboard';
import api from '../services/api';

interface AggregatedStat {
    key: string;
    provider_uuid?: string;
    provider_name?: string;
    model?: string;
    scenario?: string;
    request_count: number;
    total_tokens: number;
    total_input_tokens: number;
    total_output_tokens: number;
    avg_input_tokens: number;
    avg_output_tokens: number;
    avg_latency_ms: number;
    error_count: number;
    error_rate: number;
    streamed_count: number;
    streamed_rate: number;
}

interface TimeSeriesData {
    timestamp: string;
    request_count: number;
    total_tokens: number;
    input_tokens: number;
    output_tokens: number;
    error_count: number;
    avg_latency_ms: number;
}

interface Provider {
    uuid: string;
    name: string;
}

export default function UsageDashboardPage() {
    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [stats, setStats] = useState<AggregatedStat[]>([]);
    const [timeSeries, setTimeSeries] = useState<TimeSeriesData[]>([]);
    const [providers, setProviders] = useState<Provider[]>([]);
    const [selectedProvider, setSelectedProvider] = useState<string>('all');

    const loadData = useCallback(async (provider: string) => {
        try {
            // Build query params
            const now = new Date();
            const startTime = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000); // 7 days ago
            const params: Record<string, string> = {
                start_time: startTime.toISOString(),
                end_time: now.toISOString(),
            };
            if (provider && provider !== 'all') {
                params.provider = provider;
            }

            const [statsResult, timeSeriesResult, providersResult] = await Promise.all([
                api.getUsageStats({ ...params, group_by: 'model', limit: 100 }),
                api.getUsageTimeSeries({ ...params, interval: 'hour' }),
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
        loadData(selectedProvider);
    }, [loadData, selectedProvider]);

    useEffect(() => {
        if (autoRefresh) {
            const interval = setInterval(() => {
                loadData(selectedProvider);
            }, 60000);
            return () => clearInterval(interval);
        }
    }, [autoRefresh, loadData, selectedProvider]);

    const handleRefresh = () => {
        setRefreshing(true);
        loadData(selectedProvider);
    };

    // Calculate totals from stats
    const totalRequests = stats.reduce((sum, s) => sum + (s.request_count || 0), 0);
    const totalInputTokens = stats.reduce((sum, s) => sum + (s.total_input_tokens || 0), 0);
    const totalOutputTokens = stats.reduce((sum, s) => sum + (s.total_output_tokens || 0), 0);
    const totalTokens = totalInputTokens + totalOutputTokens;

    // Calculate today's tokens from time series
    const today = new Date().toDateString();
    const todayTokens = timeSeries
        .filter((d) => new Date(d.timestamp).toDateString() === today)
        .reduce((sum, d) => sum + d.input_tokens + d.output_tokens, 0);

    // Prepare chart data
    const tokenChartData = stats.slice(0, 10).map((stat) => ({
        name: stat.model?.length && stat.model.length > 25
            ? stat.model.substring(0, 25) + '...'
            : (stat.model || stat.key),
        inputTokens: stat.total_input_tokens || 0,
        outputTokens: stat.total_output_tokens || 0,
    }));

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
        <Box sx={{ p: 3 }}>
            {/* Header */}
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
                <Typography variant="h5" sx={{ fontWeight: 600 }}>
                    Usage Dashboard
                </Typography>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                    <FormControl size="small" sx={{ minWidth: 150 }}>
                        <InputLabel>Provider</InputLabel>
                        <Select
                            value={selectedProvider}
                            label="Provider"
                            onChange={(e) => setSelectedProvider(e.target.value)}
                        >
                            <MenuItem value="all">All Providers</MenuItem>
                            {providers.map((p) => (
                                <MenuItem key={p.uuid} value={p.uuid}>
                                    {p.name}
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                    <FormControlLabel
                        control={
                            <Switch
                                size="small"
                                checked={autoRefresh}
                                onChange={(e) => setAutoRefresh(e.target.checked)}
                            />
                        }
                        label={<Typography variant="body2">Auto Refresh</Typography>}
                    />
                    <Tooltip title="Refresh">
                        <IconButton onClick={handleRefresh} disabled={refreshing}>
                            {refreshing ? <CircularProgress size={20} /> : <RefreshIcon />}
                        </IconButton>
                    </Tooltip>
                </Box>
            </Box>

            {/* Stats Cards */}
            <Grid container spacing={3} sx={{ mb: 3 }}>
                <Grid size={{ xs: 12, sm: 4 }}>
                    <StatCard
                        title="Total Requests"
                        value={totalRequests.toLocaleString()}
                        subtitle="Last 7 days"
                        icon={<CallMadeIcon />}
                        color="primary"
                    />
                </Grid>
                <Grid size={{ xs: 12, sm: 4 }}>
                    <StatCard
                        title="Total Tokens"
                        value={formatNumber(totalTokens)}
                        subtitle={`${formatNumber(totalInputTokens)} in / ${formatNumber(totalOutputTokens)} out`}
                        icon={<PaidIcon />}
                        color="success"
                    />
                </Grid>
                <Grid size={{ xs: 12, sm: 4 }}>
                    <StatCard
                        title="Today's Tokens"
                        value={formatNumber(todayTokens)}
                        subtitle="Since midnight"
                        icon={<BoltIcon />}
                        color="info"
                    />
                </Grid>
            </Grid>

            {/* Charts */}
            <Grid container spacing={3} sx={{ mb: 3 }}>
                <Grid size={{ xs: 12, md: 6 }}>
                    <TokenHistoryChart data={timeSeries} />
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
