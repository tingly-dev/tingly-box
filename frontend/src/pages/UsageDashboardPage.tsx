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

interface ServiceStat {
    service_id: string;
    request_count: number;
    window_request_count: number;
    window_input_tokens: number;
    window_output_tokens: number;
    window_tokens_consumed: number;
    last_used: string;
}

interface Provider {
    uuid: string;
    name: string;
}

interface HistoryDataPoint {
    hour: string;
    input_tokens: number;
    output_tokens: number;
    request_count: number;
}

export default function UsageDashboardPage() {
    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [stats, setStats] = useState<Record<string, ServiceStat>>({});
    const [providers, setProviders] = useState<Provider[]>([]);
    const [selectedProvider, setSelectedProvider] = useState<string>('all');
    const [historyData, setHistoryData] = useState<HistoryDataPoint[]>([]);
    const [totalTokens, setTotalTokens] = useState({ input: 0, output: 0 });

    const loadData = useCallback(async (provider: string) => {
        try {
            const [statsResult, providersResult, historyResult] = await Promise.all([
                api.getLoadBalancerStats(),
                api.getProviders(),
                api.getStatsHistory(7, provider),
            ]);

            if (statsResult?.stats) {
                setStats(statsResult.stats);
            }
            if (providersResult?.success && providersResult?.data) {
                setProviders(providersResult.data);
            }
            if (historyResult?.history) {
                setHistoryData(historyResult.history);
            }
            if (historyResult?.total_tokens) {
                setTotalTokens(historyResult.total_tokens);
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

    // Create provider UUID -> name map
    const providerMap = providers.reduce((acc, p) => {
        acc[p.uuid] = p.name;
        return acc;
    }, {} as Record<string, string>);

    // Filter stats by selected provider
    const filteredStats = selectedProvider === 'all'
        ? stats
        : Object.fromEntries(
            Object.entries(stats).filter(([key]) => key.startsWith(selectedProvider))
        );

    // Calculate metrics from filtered stats
    const totalRequests = Object.values(filteredStats).reduce((sum, s) => sum + (s.request_count || 0), 0);
    const totalTokensConsumed = totalTokens.input + totalTokens.output;

    // Calculate today's tokens from history data
    const today = new Date().toDateString();
    const todayTokens = historyData
        .filter(d => new Date(d.hour).toDateString() === today)
        .reduce((sum, d) => sum + d.input_tokens + d.output_tokens, 0);

    // Prepare chart data - use model name
    const tokenChartData = Object.entries(filteredStats).map(([key, stat]) => {
        const parts = (stat.service_id || key).split(':');
        const modelName = parts[1] || key;
        // Shorten long model names for display
        const displayName = modelName.length > 20 ? modelName.substring(0, 20) + '...' : modelName;
        return {
            name: displayName,
            inputTokens: stat.window_input_tokens || 0,
            outputTokens: stat.window_output_tokens || 0,
        };
    });

    // Format large numbers
    const formatNumber = (num: number): string => {
        if (num >= 1000000) {
            return (num / 1000000).toFixed(1) + 'M';
        }
        if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'K';
        }
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
                        subtitle="All time"
                        icon={<CallMadeIcon />}
                        color="primary"
                    />
                </Grid>
                <Grid size={{ xs: 12, sm: 4 }}>
                    <StatCard
                        title="Total Tokens"
                        value={formatNumber(totalTokensConsumed)}
                        subtitle={`${formatNumber(totalTokens.input)} in / ${formatNumber(totalTokens.output)} out`}
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
                    <TokenHistoryChart data={historyData} />
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                    <TokenUsageChart data={tokenChartData} />
                </Grid>
            </Grid>

            {/* Stats Table */}
            <ServiceStatsTable stats={filteredStats} providerMap={providerMap} />
        </Box>
    );
}
