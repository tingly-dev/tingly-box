import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Alert,
    Box,
    CircularProgress,
    Grid,
    IconButton,
    Skeleton,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
} from '@mui/material';
import {
    Autorenew as CachedIcon,
    BarChart,
    Cloud,
    ErrorOutline,
    Refresh,
    Server,
    Token,
    Users,
} from '@/components/icons';
import PageHeader from '@/components/PageHeader';
import {
    formatNumber,
    StatCard,
    RosterTopList,
    getTotalTokens,
    getCacheHitRateColor,
    formatCacheBreakdown,
    hasCacheWrites,
    getErrorRateColor,
    getUsageMetricColumns,
    computeUsageSummary,
    useRosterAxis,
} from '@/components/dashboard';
import type { AggregatedStat, SortField, UsageMetricLabels, ShareBarItem } from '@/components/dashboard';
import RosterTable from './userUsage/RosterTable';
import RosterDetailSection from './userUsage/RosterDetailSection';
import { useUserUsageData } from './userUsage/useUserUsageData';
import {
    getModelKey,
    getProviderKey,
    accountKey,
    accountName,
    accountSearchText,
    modelName,
    modelSearchText,
    providerName,
    providerSearchText,
    fetchModelsForAccount,
    fetchAccountsForModel,
    fetchAccountsForProvider,
} from './userUsage/userUsageModel';
import type { TimeRange, ViewMode, UserUsageRow, PrimaryColumn } from './userUsage/userUsageModel';

const UserUsageSkeleton = () => (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
        <Skeleton variant="rounded" height={72} />
        <Grid container spacing={2}>
            {Array.from({ length: 5 }).map((_, index) => (
                <Grid key={index} size={{ xs: 6, sm: 4, md: 2.4 }}>
                    <Skeleton variant="rounded" height={118} />
                </Grid>
            ))}
        </Grid>
        <Grid container spacing={2}>
            <Grid size={{ xs: 12, lg: 7, xl: 5 }}><Skeleton variant="rounded" height={520} /></Grid>
            <Grid size={{ xs: 12, lg: 5, xl: 7 }}><Skeleton variant="rounded" height={520} /></Grid>
        </Grid>
    </Box>
);

export default function UserUsagePage() {
    const { t } = useTranslation();
    const [range, setRange] = useState<TimeRange>('today');
    const [viewMode, setViewMode] = useState<ViewMode>('account');
    const [rowsPerPage, setRowsPerPage] = useState(10);
    const detailPanelRef = useRef<HTMLDivElement>(null);

    const {
        loadRosters,
        rows,
        modelRoster,
        providerRoster,
        loading,
        refreshing,
        error,
        accountDisplayName,
    } = useUserUsageData(range);

    // Each axis owns its own search/sort/pagination/selection/detail-load
    // lifecycle via the shared hook — see components/dashboard/RosterAxis.tsx.
    // All three run unconditionally (fixed hook-call count), so all three
    // rosters' detail queries are ready before the user ever toggles to them.
    const accountAxis = useRosterAxis<UserUsageRow, AggregatedStat, TimeRange>({
        roster: rows,
        getKey: accountKey,
        nameOf: accountName,
        searchText: accountSearchText,
        rowsPerPage,
        range,
        loadDetail: fetchModelsForAccount,
    });
    const modelAxis = useRosterAxis<AggregatedStat, AggregatedStat, TimeRange>({
        roster: modelRoster,
        getKey: getModelKey,
        nameOf: modelName,
        searchText: modelSearchText,
        rowsPerPage,
        range,
        loadDetail: fetchAccountsForModel,
    });
    const providerAxis = useRosterAxis<AggregatedStat, AggregatedStat, TimeRange>({
        roster: providerRoster,
        getKey: getProviderKey,
        nameOf: providerName,
        searchText: providerSearchText,
        rowsPerPage,
        range,
        loadDetail: fetchAccountsForProvider,
    });
    // Fields common to every axis (search/sort/page/handleSort/detailLoading)
    // are safe to read off this union without a three-way ternary at every
    // use site; axis-specific fields (selected/pagedRows/detail) are read
    // off the specific axis in the branch that renders them.
    const activeAxis = viewMode === 'account' ? accountAxis : viewMode === 'model' ? modelAxis : providerAxis;

    const selectedUser = accountAxis.selected;
    const selectedModel = modelAxis.selected;
    const selectedProvider = providerAxis.selected;
    const detailSubject = viewMode === 'account'
        ? selectedUser
        : viewMode === 'model'
        ? selectedModel
        : selectedProvider;

    // Share-bar input for the visual band — follows the active axis and uses
    // each axis' own key space, so a bar click selects the exact roster row.
    const shareItems = useMemo<ShareBarItem[]>(() => {
        if (viewMode === 'account') {
            return rows.map((row) => ({
                key: accountKey(row),
                name: row.display_name || row.user_id,
                tokens: row.total_tokens,
            }));
        }
        if (viewMode === 'model') {
            return modelRoster.map((row) => ({
                key: getModelKey(row),
                name: modelName(row),
                tokens: getTotalTokens(row),
            }));
        }
        return providerRoster.map((row) => ({
            key: getProviderKey(row),
            name: providerName(row),
            tokens: getTotalTokens(row),
        }));
    }, [viewMode, rows, modelRoster, providerRoster]);

    // Detail-panel Top list input — ranks the selected subject's breakdown
    // (their models, or the accounts using the selected model/provider).
    // Display-only: the breakdown table has no row-selection state to link.
    const detailList = viewMode === 'account'
        ? accountAxis.detail
        : viewMode === 'model'
        ? modelAxis.detail
        : providerAxis.detail;
    const showDetailTop = !activeAxis.detailLoading && detailList.length > 0;
    const detailShareItems = useMemo<ShareBarItem[]>(() => {
        if (viewMode === 'account') {
            return detailList.map((model) => ({
                key: `${model.provider_uuid}-${model.model || model.key}`,
                name: model.model || model.key,
                tokens: getTotalTokens(model),
            }));
        }
        return detailList.map((account) => ({
            key: account.user_id || account.key,
            name: accountDisplayName(account.user_id || account.key),
            tokens: getTotalTokens(account),
        }));
    }, [viewMode, detailList, accountDisplayName]);

    // Single pass over the active axis' rows for every summary aggregate.
    const summary = useMemo(() => {
        const source = viewMode === 'account' ? rows : viewMode === 'model' ? modelRoster : providerRoster;
        return computeUsageSummary(source);
    }, [rows, modelRoster, providerRoster, viewMode]);
    const {
        tokens: totalTokens,
        inputTokens: totalInputTokens,
        outputTokens: totalOutputTokens,
        cacheTokens: totalCacheTokens,
        cacheWriteTokens: totalCacheWriteTokens,
        requests: totalRequests,
        errors: totalErrors,
        cacheHitRate,
        errorRate,
    } = summary;
    const activeAccounts = useMemo(() => rows.filter((row) => row.request_count > 0).length, [rows]);
    const providerCount = useMemo(
        () => new Set(modelRoster.map((row) => row.provider_uuid || row.provider_name || row.key)).size,
        [modelRoster],
    );
    const showAccountCacheWrite = hasCacheWrites(rows);
    const showModelRosterCacheWrite = hasCacheWrites(modelRoster);
    const showProviderRosterCacheWrite = hasCacheWrites(providerRoster);

    const primarySummaryItem = viewMode === 'account'
        ? {
            label: t('dashboard.userUsage.registeredUsers', { defaultValue: 'Registered users' }),
            value: String(rows.length),
            hint: t('dashboard.userUsage.activeUsers', {
                count: activeAccounts,
                defaultValue: `${activeAccounts} active in this period`,
            }),
            icon: <Users />,
            color: 'primary' as const,
        }
        : viewMode === 'model'
        ? {
            label: t('dashboard.userUsage.modelsUsed', { defaultValue: 'Models used' }),
            value: String(modelRoster.length),
            hint: t('dashboard.userUsage.acrossProviders', {
                count: providerCount,
                defaultValue: `Across ${providerCount} provider${providerCount === 1 ? '' : 's'}`,
            }),
            icon: <Server />,
            color: 'primary' as const,
        }
        : {
            label: t('dashboard.userUsage.providersUsed', { defaultValue: 'Providers used' }),
            value: String(providerRoster.length),
            hint: t('dashboard.userUsage.acrossModels', {
                count: modelRoster.length,
                defaultValue: `Across ${modelRoster.length} model${modelRoster.length === 1 ? '' : 's'}`,
            }),
            icon: <Cloud />,
            color: 'primary' as const,
        };
    const summaryItems = [
        primarySummaryItem,
        {
            label: t('dashboard.userUsage.totalTokens', { defaultValue: 'Total tokens' }),
            value: formatNumber(totalTokens),
            hint: t('dashboard.userUsage.tokenBreakdown', {
                cache: formatNumber(totalCacheTokens),
                input: formatNumber(totalInputTokens),
                output: formatNumber(totalOutputTokens),
                defaultValue: `Cache: ${formatNumber(totalCacheTokens)} · Input: ${formatNumber(totalInputTokens)} · Output: ${formatNumber(totalOutputTokens)}`,
            }),
            icon: <Token />,
            color: 'secondary' as const,
        },
        {
            label: t('dashboard.userUsage.cacheHitRate', { defaultValue: 'Cache hit rate' }),
            value: `${cacheHitRate.toFixed(1)}%`,
            // Already a fully composed string; wrapping it in t() would be an
            // identity call against a key that does not exist.
            hint: formatCacheBreakdown(totalCacheTokens, totalCacheWriteTokens, formatNumber),
            icon: <CachedIcon />,
            color: getCacheHitRateColor(cacheHitRate),
        },
        {
            label: t('dashboard.userUsage.requests', { defaultValue: 'Requests' }),
            value: formatNumber(totalRequests),
            hint: t('dashboard.userUsage.averagePerUser', {
                value: activeAccounts ? formatNumber(Math.round(totalRequests / activeAccounts)) : '0',
                defaultValue: `${activeAccounts ? formatNumber(Math.round(totalRequests / activeAccounts)) : '0'} per active user`,
            }),
            icon: <BarChart />,
            color: 'secondary' as const,
        },
        {
            label: t('dashboard.userUsage.errors', { defaultValue: 'Errors' }),
            value: formatNumber(totalErrors),
            hint: `${errorRate.toFixed(1)}%`,
            icon: <ErrorOutline />,
            color: getErrorRateColor(errorRate),
        },
    ];

    const usageMetricLabels: UsageMetricLabels = {
        requests: t('dashboard.userUsage.requests', { defaultValue: 'Requests' }),
        total: t('dashboard.userUsage.total', { defaultValue: 'Total' }),
        cacheRead: t('dashboard.userUsage.cacheRead', { defaultValue: 'Cache Read' }),
        cacheWrite: t('dashboard.userUsage.cacheWrite', { defaultValue: 'Cache Write' }),
        cacheHit: t('dashboard.userUsage.cacheHit', { defaultValue: 'Cache Hit' }),
        input: t('dashboard.userUsage.input', { defaultValue: 'Input' }),
        output: t('dashboard.userUsage.output', { defaultValue: 'Output' }),
        reasoning: t('dashboard.userUsage.reasoning', { defaultValue: 'Reasoning' }),
        errorRate: t('dashboard.userUsage.errorRate', { defaultValue: 'Error rate' }),
    };
    const metricColumns = (showCacheWrite: boolean): PrimaryColumn[] => getUsageMetricColumns({
        showTotal: true,
        showCacheWrite,
    }, usageMetricLabels).map((column) => ({
        kind: 'sort' as const,
        field: column.key as SortField,
        label: column.label,
        align: 'right' as const,
        defaultDir: 'desc' as const,
    }));
    const accountColumns: PrimaryColumn[] = [
        { kind: 'sort', field: 'name', label: t('dashboard.userUsage.user', { defaultValue: 'User' }), defaultDir: 'asc' },
        ...metricColumns(showAccountCacheWrite),
    ];
    const modelColumns: PrimaryColumn[] = [
        { kind: 'label', label: t('dashboard.userUsage.provider', { defaultValue: 'Provider' }) },
        { kind: 'sort', field: 'name', label: t('dashboard.userUsage.model', { defaultValue: 'Model' }), defaultDir: 'asc' },
        ...metricColumns(showModelRosterCacheWrite),
    ];
    const providerColumns: PrimaryColumn[] = [
        { kind: 'sort', field: 'name', label: t('dashboard.userUsage.provider', { defaultValue: 'Provider' }), defaultDir: 'asc' },
        ...metricColumns(showProviderRosterCacheWrite),
    ];
    const primaryColumns = viewMode === 'account' ? accountColumns : viewMode === 'model' ? modelColumns : providerColumns;

    const scrollDetailIntoView = () => {
        requestAnimationFrame(() => {
            detailPanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
        });
    };

    if (loading) return <UserUsageSkeleton />;

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
            <PageHeader
                title={t('dashboard.userUsage.title', { defaultValue: 'Team usage' })}
                subtitle={t('dashboard.userUsage.subtitle', {
                    defaultValue: 'See how every registered user is consuming shared AI access.',
                })}
                actions={
                    <>
                        <ToggleButtonGroup
                            size="small"
                            exclusive
                            value={range}
                            onChange={(_, value: TimeRange | null) => value && setRange(value)}
                            aria-label={t('dashboard.userUsage.timeRange', { defaultValue: 'Time range' })}
                        >
                            <ToggleButton value="today">{t('layout.today')}</ToggleButton>
                            <ToggleButton value="7d">7D</ToggleButton>
                            <ToggleButton value="30d">30D</ToggleButton>
                            <ToggleButton value="90d">90D</ToggleButton>
                        </ToggleButtonGroup>
                        <Tooltip title={t('common.refresh', { defaultValue: 'Refresh' })}>
                            <span>
                                <IconButton
                                    onClick={() => loadRosters(range, true)}
                                    disabled={refreshing}
                                    aria-label={t('common.refresh', { defaultValue: 'Refresh' })}
                                >
                                    {refreshing ? <CircularProgress size={20} /> : <Refresh />}
                                </IconButton>
                            </span>
                        </Tooltip>
                    </>
                }
            />

            {error && <Alert severity="error">{error}</Alert>}

            <Grid container spacing={{ xs: 1.5, sm: 2 }}>
                {summaryItems.map((item) => (
                    <Grid key={item.label} size={{ xs: 6, sm: 4, md: 2.4 }}>
                        <StatCard
                            title={item.label}
                            value={item.value}
                            subtitle={item.hint}
                            icon={item.icon}
                            color={item.color}
                        />
                    </Grid>
                ))}
            </Grid>

            {/* Roster table with the ranked Top list beside it (right) —
                same active axis, one selection state; clicking a Top entry
                selects the same subject the roster row would. Trend over time
                is the personal Usage Dashboard's job, not this page's. */}
            <Grid container spacing={2} sx={{ alignItems: 'stretch' }}>
                <Grid size={{ xs: 12, lg: 9 }} sx={{ display: 'flex', minWidth: 0 }}>
                    <RosterTable
                        viewMode={viewMode}
                        onViewModeChange={setViewMode}
                        activeAxis={activeAxis}
                        primaryColumns={primaryColumns}
                        showCacheWrite={viewMode === 'account'
                            ? showAccountCacheWrite
                            : viewMode === 'model'
                            ? showModelRosterCacheWrite
                            : showProviderRosterCacheWrite}
                        rowsPerPage={rowsPerPage}
                        onRowsPerPageChange={(newRowsPerPage) => {
                            setRowsPerPage(newRowsPerPage);
                            accountAxis.setPage(0);
                            modelAxis.setPage(0);
                            providerAxis.setPage(0);
                        }}
                        onSelectRow={(key) => { activeAxis.setSelectedKey(key); scrollDetailIntoView(); }}
                    />
                </Grid>
                <Grid size={{ xs: 12, lg: 3, xl: 2.4 }} sx={{ display: 'flex', minWidth: 0 }}>
                    <RosterTopList
                        items={shareItems}
                        selectedKey={activeAxis.selectedKey}
                        onSelect={(key) => { activeAxis.setSelectedKey(key); scrollDetailIntoView(); }}
                        title={viewMode === 'account'
                            ? t('dashboard.userUsage.topAccounts', { defaultValue: 'Top accounts' })
                            : viewMode === 'model'
                            ? t('dashboard.userUsage.topModels', { defaultValue: 'Top models' })
                            : t('dashboard.userUsage.topProviders', { defaultValue: 'Top providers' })}
                        othersLabel={t('dashboard.userUsage.others', { defaultValue: 'Others' })}
                        emptyLabel={t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                        emptyHint={t('dashboard.userUsage.noUsageHintShort', { defaultValue: 'Try a longer time range.' })}
                    />
                </Grid>

            </Grid>

            <RosterDetailSection
                panelRef={detailPanelRef}
                viewMode={viewMode}
                detailSubject={detailSubject}
                showDetailTop={showDetailTop}
                activeAxis={activeAxis}
                selectedUser={selectedUser}
                selectedModel={selectedModel}
                selectedProvider={selectedProvider}
                usageMetricLabels={usageMetricLabels}
                accountDisplayName={accountDisplayName}
                detailShareItems={detailShareItems}
            />
        </Box>
    );
}
