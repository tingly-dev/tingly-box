import { useCallback, useEffect, useRef, useState } from 'react';
import { Box, Paper, Skeleton, Tooltip, Typography } from '@mui/material';
import { Info as InfoIcon } from '@/components/icons';
import { format } from 'date-fns';
import api from '@/services/api';
import { type DailyUsage, TokenHeatmap } from './TokenHeatmap';

// The activity heatmap is a fixed, long-window overview: it always shows the
// last N days regardless of the dashboard's selected range, mirroring the
// GitHub contribution-graph convention (a full year → 52 week columns, which
// also gives the grid the right proportions for the wide chart pane). The
// Provider / Model / Identity filters are shared with the rest of the
// dashboard; only the time range is fixed.
const HEATMAP_DAYS = 365;

const toLocalISOString = (date: Date): string => {
    const tzOffset = -date.getTimezoneOffset();
    const sign = tzOffset >= 0 ? '+' : '-';
    const pad = (n: number) => String(Math.floor(Math.abs(n))).padStart(2, '0');
    return (
        date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) +
        'T' + pad(date.getHours()) + ':' + pad(date.getMinutes()) + ':' + pad(date.getSeconds()) +
        sign + pad(tzOffset / 60) + ':' + pad(tzOffset % 60)
    );
};

const getLocalMidnight = (date: Date): Date =>
    new Date(date.getFullYear(), date.getMonth(), date.getDate());

interface DashboardHeatmapSectionProps {
    /** Provider uuid filter, or 'all'. Shared with the rest of the dashboard. */
    provider: string;
    /** Model filter, or 'all'. Shared with the rest of the dashboard. */
    model?: string;
    /** Identity (user_id) filter, or 'all'. Shared with the rest of the dashboard. */
    user?: string;
    /** Bumping this triggers a refetch (e.g. on manual refresh). */
    refreshKey?: number;
}

export default function DashboardHeatmapSection({ provider, model = 'all', user = 'all', refreshKey = 0 }: DashboardHeatmapSectionProps) {
    const [dailyData, setDailyData] = useState<DailyUsage[]>([]);
    const [loading, setLoading] = useState(true);
    // Monotonic sequence to drop out-of-order responses when filters change
    // faster than requests complete (same pattern as DashboardPage.loadData).
    const requestSeq = useRef(0);

    const loadData = useCallback(async (providerFilter: string, modelFilter: string, userFilter: string) => {
        const seq = ++requestSeq.current;
        try {
            const todayStart = getLocalMidnight(new Date());
            const startTime = new Date(todayStart);
            startTime.setDate(startTime.getDate() - HEATMAP_DAYS + 1);
            const endTime = new Date(todayStart);
            endTime.setDate(endTime.getDate() + 1);

            const params: Record<string, string> = {
                start_time: toLocalISOString(startTime),
                end_time: toLocalISOString(endTime),
                interval: 'day',
            };
            if (providerFilter && providerFilter !== 'all') {
                params.provider = providerFilter;
            }
            if (modelFilter && modelFilter !== 'all') {
                params.model = modelFilter;
            }
            if (userFilter && userFilter !== 'all') {
                params.user_id = userFilter;
            }

            const result = await api.getUsageTimeSeries(params);
            if (seq !== requestSeq.current) {
                return;
            }

            const dataMap = new Map<string, { inputTokens: number; outputTokens: number; cacheTokens: number }>();
            if (result?.data) {
                for (const item of result.data) {
                    const timestampNum = parseInt(item.timestamp, 10);
                    const parsedDate = !isNaN(timestampNum) && timestampNum > 1000000000 && timestampNum < 9999999999
                        ? new Date(timestampNum * 1000)
                        : new Date(item.timestamp);
                    dataMap.set(format(parsedDate, 'yyyy-MM-dd'), {
                        inputTokens: item.input_tokens || 0,
                        outputTokens: item.output_tokens || 0,
                        cacheTokens: item.cache_input_tokens || 0,
                    });
                }
            }

            const daily: DailyUsage[] = [];
            const currentDay = new Date(startTime);
            while (currentDay < endTime) {
                const dateStr = format(currentDay, 'yyyy-MM-dd');
                const d = dataMap.get(dateStr) || { inputTokens: 0, outputTokens: 0, cacheTokens: 0 };
                daily.push({
                    date: dateStr,
                    inputTokens: d.inputTokens,
                    outputTokens: d.outputTokens,
                    cacheTokens: d.cacheTokens,
                    totalTokens: d.inputTokens + d.outputTokens + d.cacheTokens,
                });
                currentDay.setDate(currentDay.getDate() + 1);
            }
            setDailyData(daily);
        } catch (error) {
            console.error('Failed to load heatmap data:', error);
        } finally {
            if (seq === requestSeq.current) {
                setLoading(false);
            }
        }
    }, []);

    useEffect(() => {
        loadData(provider, model, user);
    }, [loadData, provider, model, user, refreshKey]);

    return (
        // Standard dashboard card (same style as RequestsView / Usage by
        // Model) so the view doesn't float bare on the page background.
        // flex: 1 lets it fill the chart pane (whose height is set by the
        // sibling column) so the grid can center vertically instead of
        // floating at the top with dead space below.
        <Paper
            elevation={0}
            sx={{
                flex: 1,
                display: 'flex',
                flexDirection: 'column',
                p: 2.5,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                backgroundColor: 'background.paper',
                boxShadow: 'none',
            }}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 2 }}>
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.9375rem' }}>
                    Token Activity
                </Typography>
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    · Last 12 months
                </Typography>
                <Tooltip
                    title={`Fixed ${HEATMAP_DAYS}-day window — not affected by the range selector (the Provider / Model / Identity filters still apply).`}
                    arrow
                >
                    <InfoIcon sx={{ fontSize: 15, color: 'text.disabled', cursor: 'default' }} />
                </Tooltip>
            </Box>

            <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
                {loading && dailyData.length === 0 ? (
                    // First load: a grid-shaped skeleton instead of flashing
                    // the "No activity" empty state before data arrives.
                    <Skeleton variant="rounded" height={160} sx={{ borderRadius: 1.5 }} />
                ) : dailyData.length > 0 ? (
                    <TokenHeatmap data={dailyData} />
                ) : (
                    <Box sx={{ py: 6, color: 'text.secondary', textAlign: 'center' }}>
                        <Typography variant="body2">No activity in the last {HEATMAP_DAYS} days.</Typography>
                    </Box>
                )}
            </Box>
        </Paper>
    );
}
