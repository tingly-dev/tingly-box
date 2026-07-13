import { useCallback, useEffect, useRef, useState } from 'react';
import { Box, Paper, Tooltip, Typography } from '@mui/material';
import { Info as InfoIcon } from '@/components/icons';
import { format } from 'date-fns';
import api from '@/services/api';
import { type DailyUsage, TokenHeatmap } from './TokenHeatmap';

// The activity heatmap is a fixed, long-window overview: it always shows the
// last N days regardless of the dashboard's selected range, mirroring the
// GitHub contribution-graph convention. It lives at the bottom of the Usage
// Dashboard so the page has a single time-range control (the range selector
// drives the tiles / chart / table above); this section is a stable
// at-a-glance activity view. Only the Provider filter is shared.
const HEATMAP_DAYS = 180;

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
    /** Bumping this triggers a refetch (e.g. on manual refresh). */
    refreshKey?: number;
}

export default function DashboardHeatmapSection({ provider, refreshKey = 0 }: DashboardHeatmapSectionProps) {
    const [dailyData, setDailyData] = useState<DailyUsage[]>([]);
    const [cellSize, setCellSize] = useState(12);
    const gridContainerRef = useRef<HTMLDivElement>(null);

    // Responsive cell sizing — same heuristic as the former standalone heatmap
    // page, so the grid keeps its familiar density.
    useEffect(() => {
        if (!gridContainerRef.current || dailyData.length === 0) return;
        const observer = new ResizeObserver((entries) => {
            for (const entry of entries) {
                const width = entry.contentRect.width;
                const weeks = Math.ceil(dailyData.length / 4);
                const availableWidth = width - 45 /* day labels */ - 70 /* padding */ - 1 /* gap */;
                setCellSize(Math.max(9, Math.floor(availableWidth / weeks)));
            }
        });
        observer.observe(gridContainerRef.current);
        return () => observer.disconnect();
    }, [dailyData]);

    const loadData = useCallback(async (providerFilter: string) => {
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

            const result = await api.getUsageTimeSeries(params);

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
        }
    }, []);

    useEffect(() => {
        loadData(provider);
    }, [loadData, provider, refreshKey]);

    return (
        <Paper
            elevation={0}
            sx={{
                p: 2.5,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                backgroundColor: 'background.paper',
                boxShadow: 'none',
            }}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                    Token Activity
                </Typography>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        Last {HEATMAP_DAYS} days
                    </Typography>
                    <Tooltip
                        title={`Fixed ${HEATMAP_DAYS}-day window — not affected by the range selector above (the Provider filter still applies).`}
                        arrow
                    >
                        <InfoIcon sx={{ fontSize: 15, color: 'text.disabled', cursor: 'default' }} />
                    </Tooltip>
                </Box>
            </Box>

            <Box ref={gridContainerRef} sx={{ display: 'flex', justifyContent: 'center' }}>
                {dailyData.length > 0 ? (
                    <TokenHeatmap data={dailyData} cellSize={cellSize} gap={1} />
                ) : (
                    <Box sx={{ py: 6, color: 'text.secondary' }}>
                        <Typography variant="body2">No activity in the last {HEATMAP_DAYS} days.</Typography>
                    </Box>
                )}
            </Box>
        </Paper>
    );
}
