import { useState } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { Box, Button, Grid, Skeleton, Stack, Tab, Tabs, Typography } from '@mui/material';
import {
    StatCard,
    DailyTokenHistoryChart,
    HourlyTokenHistoryChart,
    ServiceStatsTable,
    RequestsView,
    PerformanceSummary,
    DashboardHeatmapSection,
    DashboardFilterBar,
    formatNumber,
    getCacheHitRateColor,
    formatCacheBreakdown,
    getErrorRateColor,
    computeUsageSummary,
} from '@/components/dashboard';
import { visuallyHidden } from '@/components/dashboard/visuallyHidden';
import { useTranslation } from 'react-i18next';
import { useDashboardData, TIME_RANGE_CONFIG } from '@/hooks/useDashboardData';
import type { TimeRange } from '@/hooks/useDashboardData';
import { fontSizes } from '@/theme/fonts';

const DashboardSkeleton = () => (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
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
    const { t } = useTranslation();
    const { timeRange: urlTimeRange } = useParams<{ timeRange: TimeRange }>();
    const navigate = useNavigate();
    // `?scenario=<agent>` opens the dashboard already filtered to one agent
    // (the agent pages' usage panel links here). Read once; the filter bar
    // owns it from then on.
    const [searchParams] = useSearchParams();
    const initialScenario = searchParams.get('scenario') || 'all';

    // Validate and set time range from URL
    const validTimeRanges: TimeRange[] = ['today', 'yesterday', '3d', '7d', '30d', '90d'];
    const timeRange: TimeRange = validTimeRanges.includes(urlTimeRange as TimeRange)
        ? (urlTimeRange as TimeRange)
        : 'today';

    const isHourlyRange = timeRange === 'today' || timeRange === 'yesterday';

    // Analysis mode: token trend ('summary'), per-request list ('requests'),
    // or the fixed 12-month heatmap ('activity').
    const [viewMode, setViewMode] = useState<'summary' | 'requests' | 'activity'>('summary');
    // Single requests are only listed for hourly ranges. The tab stays put on
    // a daily range and says so, rather than disappearing under the user.
    const requestsUnavailable = viewMode === 'requests' && !isHourlyRange;

    const {
        loading,
        refreshing,
        autoRefresh,
        setAutoRefresh,
        handleRefresh,
        stats,
        timeSeries,
        records,
        recordsLoading,
        recordsTotal,
        recordsParams,
        heatmapRefresh,
        selectedProvider,
        setSelectedProvider,
        selectedModel,
        setSelectedModel,
        selectedUser,
        setSelectedUser,
        selectedScenario,
        setSelectedScenario,
        scenarioOptions,
        hasActiveFilters,
        handleClearFilters,
        groupedProviderOptions,
        modelOptions,
        usageIdentities,
        selectedIdentityLabel,
    } = useDashboardData({
        timeRange,
        isHourlyRange,
        viewMode: requestsUnavailable ? 'summary' : viewMode,
        initialScenario,
    });

    // Calculate totals from stats — single shared pass over the rows (same
    // helper the Team Usage page uses for its stat cards). The streamed rate
    // is not part of computeUsageSummary, so it stays a local reduce.
    const summary = computeUsageSummary(stats);
    const {
        requests: totalRequests,
        tokens: totalTokens,
        inputTokens: totalInputTokens,
        outputTokens: totalOutputTokens,
        cacheTokens: totalCacheTokens,
        // Cache writes are already inside total_input_tokens (they are billed at a
        // premium but are still this prompt's input), so they are reported next to
        // the read hits rather than added to any total.
        cacheWriteTokens: totalCacheWriteTokens,
        errors: totalErrors,
        errorRate,
        cacheHitRate,
    } = summary;

    // Calculate streamed rate
    const totalStreamed = stats.reduce((sum, s) => sum + (s.streamed_count || 0), 0);
    const streamedRate = totalRequests > 0 ? (totalStreamed / totalRequests) * 100 : 0;

    if (loading) {
        return <DashboardSkeleton />;
    }

    const toolbar = (
        <DashboardFilterBar
            providerGroups={groupedProviderOptions}
            modelOptions={modelOptions}
            usageIdentities={usageIdentities}
            selectedProvider={selectedProvider}
            onProviderChange={setSelectedProvider}
            selectedModel={selectedModel}
            onModelChange={setSelectedModel}
            selectedUser={selectedUser}
            onUserChange={setSelectedUser}
            selectedIdentityLabel={selectedIdentityLabel}
            scenarioOptions={scenarioOptions}
            selectedScenario={selectedScenario}
            onScenarioChange={setSelectedScenario}
            hasActiveFilters={hasActiveFilters}
            onClearFilters={handleClearFilters}
            autoRefresh={autoRefresh}
            onAutoRefreshChange={setAutoRefresh}
            refreshing={refreshing}
            onRefresh={handleRefresh}
            // The time range is a filter on this page, not a place to go — it
            // used to be six rows in the Dashboard sidebar. Each range keeps
            // its own URL (/dashboard/7d, …).
            timeRange={timeRange}
            onTimeRangeChange={(range) => navigate(`/dashboard/${range}`)}
        />
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
            {/* The sidebar already says where you are ("Usage"); keep the h1 for
                screen readers only instead of a second, visible title. */}
            <Box component="h1" sx={visuallyHidden}>
                {t('dashboard.overview.title', { defaultValue: 'Usage Dashboard' })}
            </Box>
            {toolbar}
            {/* Main Content */}
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {/* Stat Cards Row - 5 cards */}
                <Grid container spacing={{ xs: 1.5, sm: 2 }}>
                    <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={t('dashboard.overview.statCards.totalRequests', { defaultValue: 'Total Requests' })}
                            value={totalRequests.toLocaleString()}
                            subtitle={t(TIME_RANGE_CONFIG[timeRange].labelKey)}
                            // Volume metric — no health judgment, so keep it neutral.
                            color="secondary"
                        />
                    </Grid>
                    <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={t('dashboard.overview.statCards.totalTokens', { defaultValue: 'Total Tokens' })}
                            value={formatNumber(totalTokens)}
                            subtitle={t('dashboard.overview.statCards.tokenBreakdown', {
                                input: formatNumber(totalInputTokens),
                                cache: formatNumber(totalCacheTokens),
                                output: formatNumber(totalOutputTokens),
                            })}
                            // Volume metric — no health judgment, so keep it neutral.
                            color="secondary"
                        />
                    </Grid>
                    <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={t('dashboard.overview.statCards.cacheHitRate', { defaultValue: 'Cache Hit Rate' })}
                            value={`${cacheHitRate.toFixed(1)}%`}
                            subtitle={formatCacheBreakdown(totalCacheTokens, totalCacheWriteTokens, formatNumber, {
                                read: t('dashboard.overview.statCards.cacheRead', { defaultValue: 'read' }),
                                written: t('dashboard.overview.statCards.cacheWrite', { defaultValue: 'written' }),
                            })}
                            statusDot
                            color={getCacheHitRateColor(cacheHitRate)}
                        />
                    </Grid>
                    <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={t('dashboard.overview.statCards.errorRate', { defaultValue: 'Error Rate' })}
                            value={`${errorRate.toFixed(2)}%`}
                            subtitle={t('dashboard.overview.statCards.errors', { count: totalErrors })}
                            statusDot
                            color={getErrorRateColor(errorRate)}
                        />
                    </Grid>
                    <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={t('dashboard.overview.statCards.streamedRate', { defaultValue: 'Streamed Rate' })}
                            value={`${streamedRate.toFixed(1)}%`}
                            subtitle={t('dashboard.overview.statCards.streamed', { count: totalStreamed })}
                            color="secondary"
                        />
                    </Grid>
                </Grid>

                {/* View tabs: Summary trend, By Request (hourly only), or the
                    12-month Activity heatmap. They sit at the top of the content
                    panel; flex: 1 so the active view can use the full pane
                    height (the Activity grid centers itself vertically in it). */}
                <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                    <Tabs
                        value={viewMode}
                        onChange={(_, v) => setViewMode(v)}
                        sx={{
                            minHeight: 40,
                            borderBottom: '1px solid',
                            borderColor: 'divider',
                            '& .MuiTab-root': {
                                minHeight: 40,
                                fontSize: fontSizes.md,
                                textTransform: 'none',
                            },
                        }}
                    >
                        <Tab value="summary" label={t('dashboard.overview.viewModes.summary', { defaultValue: 'Summary' })} />
                        <Tab value="requests" label={t('dashboard.overview.viewModes.byRequest', { defaultValue: 'By Request' })} />
                        <Tab value="activity" label={t('dashboard.overview.viewModes.activity', { defaultValue: 'Activity' })} />
                    </Tabs>
                    {/* One level of cards: each panel below is its own card on the
                        page, not a card inside a card. */}
                    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
                        {viewMode === 'activity' ? (
                            <DashboardHeatmapSection
                                provider={selectedProvider}
                                model={selectedModel}
                                user={selectedUser}
                                scenario={selectedScenario}
                                refreshKey={heatmapRefresh}
                            />
                        ) : viewMode === 'summary' ? (
                            <>
                            <Box
                                sx={{
                                    display: 'grid',
                                    gridTemplateColumns: 'minmax(0, 1fr)',
                                    '@media (min-width: 1280px)': {
                                        gridTemplateColumns: 'minmax(0, 1fr) 400px',
                                    },
                                    gap: 2,
                                    alignItems: 'stretch',
                                }}
                            >
                                <Box sx={{ minWidth: 0, display: 'flex' }}>
                                    {timeRange === 'today' || timeRange === 'yesterday' ? (
                                        <HourlyTokenHistoryChart data={timeSeries} />
                                    ) : (
                                        <DailyTokenHistoryChart data={timeSeries} />
                                    )}
                                </Box>
                                <Box sx={{ minWidth: 0, display: 'flex' }}>
                                    <PerformanceSummary queryParams={recordsParams} />
                                </Box>
                            </Box>
                            {/* Per-model totals belong to the summary, not to every tab. */}
                            <ServiceStatsTable stats={stats} />
                            </>
                        ) : requestsUnavailable ? (
                            <Stack spacing={1.5} sx={{ alignItems: 'center', justifyContent: 'center', py: 8, textAlign: 'center' }}>
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    {t('dashboard.overview.byRequestHourlyOnly', {
                                        defaultValue: 'Single requests are listed for today and yesterday.',
                                    })}
                                </Typography>
                                <Stack direction="row" spacing={1}>
                                    {(['today', 'yesterday'] as const).map((range) => (
                                        <Button key={range} variant="outlined" size="small" onClick={() => navigate(`/dashboard/${range}`)}>
                                            {t(TIME_RANGE_CONFIG[range].labelKey)}
                                        </Button>
                                    ))}
                                </Stack>
                            </Stack>
                        ) : (
                            <RequestsView
                                records={records}
                                loading={recordsLoading}
                                totalCount={recordsTotal}
                                queryParams={recordsParams}
                            />
                        )}
                    </Box>
                </Box>
            </Box>
        </Box>
    );
}
