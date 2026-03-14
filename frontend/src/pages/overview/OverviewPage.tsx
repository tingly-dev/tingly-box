import { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
    Box,
    Paper,
    Typography,
    FormControl,
    InputLabel,
    Select,
    MenuItem,
    IconButton,
    Tooltip,
    CircularProgress,
    Divider,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import CalendarTodayIcon from '@mui/icons-material/CalendarToday';
import { TokenHeatmap, type DailyUsage } from '@/components/dashboard';
import api from '@/services/api';
import { format } from 'date-fns';

type TimeRange = '30d' | '90d' | '180d' | '365d';

const TIME_RANGE_CONFIG: Record<TimeRange, { label: string; days: number }> = {
    '30d': { label: '30 Days', days: 30 },
    '90d': { label: '90 Days', days: 90 },
    '180d': { label: '180 Days', days: 180 },
    '365d': { label: '1 Year', days: 365 },
};

// Format date to local ISO string (with timezone offset)
const toLocalISOString = (date: Date): string => {
    const tzOffset = -date.getTimezoneOffset();
    const sign = tzOffset >= 0 ? '+' : '-';
    const pad = (n: number) => String(Math.floor(Math.abs(n))).padStart(2, '0');
    return (
        date.getFullYear() +
        '-' +
        pad(date.getMonth() + 1) +
        '-' +
        pad(date.getDate()) +
        'T' +
        pad(date.getHours()) +
        ':' +
        pad(date.getMinutes()) +
        ':' +
        pad(date.getSeconds()) +
        sign +
        pad(tzOffset / 60) +
        ':' +
        pad(tzOffset % 60)
    );
};

// Get local midnight
const getLocalMidnight = (date: Date): Date => {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate());
};

export default function OverviewPage() {
    const { timeRange: urlTimeRange } = useParams<{ timeRange: TimeRange }>();
    const navigate = useNavigate();

    // Validate and set time range from URL
    const validTimeRanges: TimeRange[] = ['30d', '90d', '180d', '365d'];
    const timeRange: TimeRange = validTimeRanges.includes(urlTimeRange as TimeRange)
        ? (urlTimeRange as TimeRange)
        : '90d';

    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [dailyData, setDailyData] = useState<DailyUsage[]>([]);
    const [providers, setProviders] = useState<{ uuid: string; name: string }[]>([]);
    const [selectedProvider, setSelectedProvider] = useState<string>('all');

    const loadData = useCallback(
        async (provider: string, range: TimeRange) => {
            try {
                const config = TIME_RANGE_CONFIG[range];
                const now = new Date();
                const todayStart = getLocalMidnight(now);

                // Calculate FIXED date range based on user selection (not based on actual data)
                // From (N-1) days ago 00:00:00 to today 23:59:59
                const startTime = new Date(todayStart);
                startTime.setDate(startTime.getDate() - config.days + 1);
                const endTime = new Date(todayStart);
                endTime.setDate(endTime.getDate() + 1); // End of today

                const params: Record<string, string> = {
                    start_time: toLocalISOString(startTime),
                    end_time: toLocalISOString(endTime),
                    interval: 'day',
                };
                if (provider && provider !== 'all') {
                    params.provider = provider;
                }

                const [timeSeriesResult, providersResult] = await Promise.all([
                    api.getUsageTimeSeries(params),
                    api.getProviders(),
                ]);

                // Create a map of existing data from API response
                const dataMap = new Map<string, { inputTokens: number; outputTokens: number; cacheTokens: number }>();
                if (timeSeriesResult?.data) {
                    for (const item of timeSeriesResult.data) {
                        let parsedDate: Date;
                        const timestampNum = parseInt(item.timestamp, 10);
                        if (!isNaN(timestampNum) && timestampNum > 1000000000 && timestampNum < 9999999999) {
                            parsedDate = new Date(timestampNum * 1000);
                        } else {
                            parsedDate = new Date(item.timestamp);
                        }

                        const dateStr = format(parsedDate, 'yyyy-MM-dd');
                        dataMap.set(dateStr, {
                            inputTokens: item.input_tokens || 0,
                            outputTokens: item.output_tokens || 0,
                            cacheTokens: item.cache_input_tokens || 0,
                        });
                    }
                }

                // Fill ALL days in the fixed time range (from startTime to endTime)
                // This ensures we show exactly the number of days user selected (e.g., 365 days for "1 year")
                const daily: DailyUsage[] = [];
                const currentDay = new Date(startTime);

                while (currentDay < endTime) {
                    const dateStr = format(currentDay, 'yyyy-MM-dd');
                    const data = dataMap.get(dateStr) || { inputTokens: 0, outputTokens: 0, cacheTokens: 0 };
                    daily.push({
                        date: dateStr,
                        inputTokens: data.inputTokens,
                        outputTokens: data.outputTokens,
                        cacheTokens: data.cacheTokens,
                        totalTokens: data.inputTokens + data.outputTokens + data.cacheTokens,
                    });
                    currentDay.setDate(currentDay.getDate() + 1);
                }

                setDailyData(daily);

                if (providersResult?.success && providersResult?.data) {
                    setProviders(providersResult.data);
                }
            } catch (error) {
                console.error('Failed to load overview data:', error);
            } finally {
                setLoading(false);
                setRefreshing(false);
            }
        },
        []
    );

    useEffect(() => {
        loadData(selectedProvider, timeRange);
    }, [loadData, selectedProvider, timeRange]);

    const handleRefresh = () => {
        setRefreshing(true);
        loadData(selectedProvider, timeRange);
    };

    const handleTimeRangeChange = (newRange: TimeRange) => {
        navigate(`/overview/${newRange}`);
    };

    const handleProviderChange = (provider: string) => {
        setSelectedProvider(provider);
    };

    // Calculate start and end date for display
    const dateRange = (() => {
        if (dailyData.length === 0) return null;
        const dates = dailyData.map((d) => d.date).sort();
        const startDate = dates[0];
        const endDate = dates[dates.length - 1];
        return {
            start: format(new Date(startDate), 'MMM d, yyyy'),
            end: format(new Date(endDate), 'MMM d, yyyy'),
        };
    })();

    if (loading) {
        return (
            <Box
                sx={{
                    display: 'flex',
                    justifyContent: 'center',
                    alignItems: 'center',
                    height: '50vh',
                }}
            >
                <CircularProgress />
            </Box>
        );
    }

    return (
        <Box
            sx={{
                px: { xs: 3, sm: 4, md: 5 },
                py: { xs: 4, sm: 5, md: 6 },
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
                <Box>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography variant="h4" sx={{ fontWeight: 700, fontSize: '1.5rem', letterSpacing: '-0.02em' }}>
                            Token Heatmap
                        </Typography>
                    </Box>
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, ml: 0.5 }}>
                        {dateRange ? `${dateRange.start} - ${dateRange.end}` : TIME_RANGE_CONFIG[timeRange].label}
                    </Typography>
                </Box>

                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
                    {/* Time Range Selector */}
                    <FormControl size="small" sx={{ minWidth: 120 }}>
                        <InputLabel sx={{ fontWeight: 500 }}>Time Range</InputLabel>
                        <Select
                            value={timeRange}
                            label="Time Range"
                            onChange={(e) => handleTimeRangeChange(e.target.value as TimeRange)}
                            sx={{
                                borderRadius: 2,
                                '& .MuiOutlinedInput-input': { py: 1 },
                            }}
                        >
                            {Object.entries(TIME_RANGE_CONFIG).map(([key, config]) => (
                                <MenuItem key={key} value={key}>
                                    {config.label}
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>

                    {/* Provider Selector */}
                    <FormControl size="small" sx={{ minWidth: 150 }}>
                        <InputLabel sx={{ fontWeight: 500 }}>Provider</InputLabel>
                        <Select
                            value={selectedProvider}
                            label="Provider"
                            onChange={(e) => handleProviderChange(e.target.value)}
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

                    {/* Refresh Button */}
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
            </Paper>

            {/* Token Heatmap */}
            <Paper
                sx={{
                    p: 3,
                    borderRadius: 2.5,
                    border: '1px solid',
                    borderColor: 'divider',
                    boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
                }}
            >
                {dailyData.length > 0 ? (
                    <TokenHeatmap
                        data={dailyData}
                        cellSize={15}
                        gap={1}
                    />
                ) : (
                    <Box
                        sx={{
                            py: 8,
                            display: 'flex',
                            flexDirection: 'column',
                            alignItems: 'center',
                            justifyContent: 'center',
                            color: 'text.secondary',
                        }}
                    >
                        <CalendarTodayIcon sx={{ fontSize: 48, opacity: 0.3, mb: 2 }} />
                        <Typography variant="body1">No usage data for this period</Typography>
                        <Typography variant="caption" color="text.disabled" sx={{ mt: 0.5 }}>
                            Try a different time range
                        </Typography>
                    </Box>
                )}
            </Paper>
        </Box>
    );
}
