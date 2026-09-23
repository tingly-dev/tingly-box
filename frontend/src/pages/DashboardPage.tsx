import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Box, Grid, Skeleton } from '@mui/material';
import { Outbound as CallMadeIcon, ErrorOutline as ErrorOutlineIcon, Token as PaidIcon, Stream as StreamIcon, Autorenew as CachedIcon } from '@/components/icons';
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
import { ToggleButtonGroup, ToggleButton } from '@mui/material';
import PageHeader from '@/components/PageHeader';
import { useTranslation } from 'react-i18next';
import { useDashboardData, TIME_RANGE_CONFIG } from '@/hooks/useDashboardData';
import type { TimeRange } from '@/hooks/useDashboardData';

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
    const { t } = useTranslation();
    const { timeRange: urlTimeRange } = useParams<{ timeRange: TimeRange }>();
    const navigate = useNavigate();

    // Validate and set time range from URL
    const validTimeRanges: TimeRange[] = ['today', 'yesterday', '3d', '7d', '30d', '90d'];
    const timeRange: TimeRange = validTimeRanges.includes(urlTimeRange as TimeRange)
        ? (urlTimeRange as TimeRange)
        : 'today';

    const isHourlyRange = timeRange === 'today' || timeRange === 'yesterday';

    // Analysis mode: token trend ('summary'), per-request list ('requests',
    // hourly ranges only), or the fixed 12-month heatmap ('activity').
    const [viewMode, setViewMode] = useState<'summary' | 'requests' | 'activity'>('summary');
    // "By Request" only exists for hourly ranges; fall back to the trend if a
    // stale 'requests' selection carries into a daily range.
    const effectiveViewMode = viewMode === 'requests' && !isHourlyRange ? 'summary' : viewMode;

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
        hasActiveFilters,
        handleClearFilters,
        groupedProviderOptions,
        modelOptions,
        usageIdentities,
        selectedIdentityLabel,
    } = useDashboardData({ timeRange, isHourlyRange, viewMode });

    // Reset view mode when switching away from hourly ranges
    useEffect(() => {
        if (!isHourlyRange) {
            setViewMode('summary');
        }
    }, [isHourlyRange]);

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

    const headerActions = (
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
            hasActiveFilters={hasActiveFilters}
            onClearFilters={handleClearFilters}
            autoRefresh={autoRefresh}
            onAutoRefreshChange={setAutoRefresh}
            refreshing={refreshing}
            onRefresh={handleRefresh}
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
            <PageHeader
                title={t('dashboard.overview.title', { defaultValue: 'Usage Dashboard' })}
                subtitle={t(TIME_RANGE_CONFIG[timeRange].labelKey)}
                actions={headerActions}
            />
            {/* Main Content */}
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {/* Stat Cards Row - 5 cards */}
                <Grid container spacing={{ xs: 1.5, sm: 2 }}>
                    <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={t('dashboard.overview.statCards.totalRequests', { defaultValue: 'Total Requests' })}
                            value={totalRequests.toLocaleString()}
                            subtitle={t(TIME_RANGE_CONFIG[timeRange].labelKey)}
                            icon={<CallMadeIcon />}
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
                            icon={<PaidIcon />}
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
                            icon={<CachedIcon />}
                            color={getCacheHitRateColor(cacheHitRate)}
                        />
                    </Grid>
                    <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={t('dashboard.overview.statCards.errorRate', { defaultValue: 'Error Rate' })}
                            value={`${errorRate.toFixed(2)}%`}
                            subtitle={t('dashboard.overview.statCards.errors', { count: totalErrors })}
                            icon={<ErrorOutlineIcon />}
                            color={getErrorRateColor(errorRate)}
                        />
                    </Grid>
                    <Grid size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={t('dashboard.overview.statCards.streamedRate', { defaultValue: 'Streamed Rate' })}
                            value={`${streamedRate.toFixed(1)}%`}
                            subtitle={t('dashboard.overview.statCards.streamed', { count: totalStreamed })}
                            icon={<StreamIcon />}
                            color="secondary"
                        />
                    </Grid>
                </Grid>

                {/* Chart view toggle: Summary trend, By Request (hourly only),
                    or the 12-month Activity heatmap. flex: 1 so the active
                    view can use the full pane height (the Activity grid
                    centers itself vertically in it). */}
                <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                    <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                        <ToggleButtonGroup
                            value={effectiveViewMode}
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
                            <ToggleButton value="summary">{t('dashboard.overview.viewModes.summary', { defaultValue: 'Summary' })}</ToggleButton>
                            {isHourlyRange && <ToggleButton value="requests">{t('dashboard.overview.viewModes.byRequest', { defaultValue: 'By Request' })}</ToggleButton>}
                            <ToggleButton value="activity">{t('dashboard.overview.viewModes.activity', { defaultValue: 'Activity' })}</ToggleButton>
                        </ToggleButtonGroup>
                    </Box>

                    {effectiveViewMode === 'activity' ? (
                        <DashboardHeatmapSection
                            provider={selectedProvider}
                            model={selectedModel}
                            user={selectedUser}
                            refreshKey={heatmapRefresh}
                        />
                    ) : effectiveViewMode === 'summary' ? (
                        <Box
                            sx={{
                                display: 'grid',
                                gridTemplateColumns: 'minmax(0, 1fr)',
                                '@media (min-width: 1280px)': {
                                    gridTemplateColumns: '320px minmax(0, 1fr)',
                                },
                                gap: 2,
                                alignItems: 'stretch',
                            }}
                        >
                            <Box sx={{ minWidth: 0, display: 'flex' }}>
                                <PerformanceSummary queryParams={recordsParams} />
                            </Box>
                            <Box sx={{ minWidth: 0, display: 'flex' }}>
                                {timeRange === 'today' || timeRange === 'yesterday' ? (
                                    <HourlyTokenHistoryChart data={timeSeries} />
                                ) : (
                                    <DailyTokenHistoryChart data={timeSeries} />
                                )}
                            </Box>
                        </Box>
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
            {/* Stats Table */}
            <ServiceStatsTable stats={stats} />
        </Box>
    );
}
