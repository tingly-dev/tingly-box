import { Box, Button, Grid, Stack, Typography } from '@mui/material';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router-dom';
import { api } from '@/services/api';
import { getLocalMidnight, toLocalISOString } from '@/utils/datetime';
import { computeUsageSummary } from './RosterAxis';
import { formatCacheBreakdown, formatNumber, getCacheHitRateColor, getErrorRateColor } from './chartStyles';
import ServiceStatsTable, { type AggregatedStat } from './ServiceStatsTable';
import StatCard from './StatCard';

const REFRESH_MS = 30_000;

/**
 * Today's usage for one agent, as the Usage tab of its Log: the Dashboard's
 * cards and per-model table, filtered to the agent. The full analysis (other
 * ranges, charts, activity) stays one click away in the Dashboard.
 */
export default function AgentUsagePanel({ scenario }: { scenario: string }) {
    const { t } = useTranslation();
    const [stats, setStats] = useState<AggregatedStat[] | null>(null);
    const seq = useRef(0);

    const load = useCallback(async () => {
        const mine = ++seq.current;
        const now = new Date();
        try {
            const result = await api.getUsageStats({
                scenario,
                group_by: 'model',
                start_time: toLocalISOString(getLocalMidnight(now)),
                end_time: toLocalISOString(now),
                limit: 1000,
            });
            if (mine === seq.current) setStats(Array.isArray(result?.data) ? result.data : []);
        } catch {
            if (mine === seq.current) setStats((prev) => prev ?? []);
        }
    }, [scenario]);

    useEffect(() => {
        void load();
        const timer = window.setInterval(() => {
            if (document.visibilityState === 'visible') void load();
        }, REFRESH_MS);
        return () => window.clearInterval(timer);
    }, [load]);

    // Nothing until the first answer, so the cards don't flash zeros.
    if (stats === null) {
        return (
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {t('logViewer.loading', { defaultValue: 'Loading…' })}
            </Typography>
        );
    }

    const summary = computeUsageSummary(stats);
    const today = t('dashboard.overview.range.today');

    return (
        <Stack spacing={2}>
            <Grid container spacing={2}>
                <Grid size={{ xs: 6, md: 3 }}>
                    <StatCard
                        title={t('dashboard.overview.statCards.totalRequests', { defaultValue: 'Total Requests' })}
                        value={summary.requests.toLocaleString()}
                        subtitle={today}
                        color="secondary"
                    />
                </Grid>
                <Grid size={{ xs: 6, md: 3 }}>
                    <StatCard
                        title={t('dashboard.overview.statCards.totalTokens', { defaultValue: 'Total Tokens' })}
                        value={formatNumber(summary.tokens)}
                        subtitle={t('dashboard.overview.statCards.tokenBreakdown', {
                            input: formatNumber(summary.inputTokens),
                            cache: formatNumber(summary.cacheTokens),
                            output: formatNumber(summary.outputTokens),
                        })}
                        color="secondary"
                    />
                </Grid>
                <Grid size={{ xs: 6, md: 3 }}>
                    <StatCard
                        title={t('dashboard.overview.statCards.cacheHitRate', { defaultValue: 'Cache Hit Rate' })}
                        value={`${summary.cacheHitRate.toFixed(1)}%`}
                        subtitle={formatCacheBreakdown(summary.cacheTokens, summary.cacheWriteTokens, formatNumber, {
                            read: t('dashboard.overview.statCards.cacheRead', { defaultValue: 'read' }),
                            written: t('dashboard.overview.statCards.cacheWrite', { defaultValue: 'written' }),
                        })}
                        statusDot
                        color={getCacheHitRateColor(summary.cacheHitRate)}
                    />
                </Grid>
                <Grid size={{ xs: 6, md: 3 }}>
                    <StatCard
                        title={t('dashboard.overview.statCards.errorRate', { defaultValue: 'Error Rate' })}
                        value={`${summary.errorRate.toFixed(2)}%`}
                        subtitle={t('dashboard.overview.statCards.errors', { count: summary.errors })}
                        statusDot
                        color={getErrorRateColor(summary.errorRate)}
                    />
                </Grid>
            </Grid>
            <ServiceStatsTable stats={stats} />
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 2 }}>
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                    {t('logViewer.usageHint', { defaultValue: 'Other days, charts and performance are in the Dashboard.' })}
                </Typography>
                <Button
                    component={RouterLink}
                    to={`/dashboard/today?scenario=${encodeURIComponent(scenario)}`}
                    variant="outlined"
                    size="small"
                >
                    {t('logViewer.openDashboard', { defaultValue: 'Open in Dashboard' })}
                </Button>
            </Box>
        </Stack>
    );
}
