import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getTotalTokens } from '@/components/dashboard';
import type { TimeSeriesData, AggregatedStat, UsageRecord } from '@/components/dashboard';
import api from '@/services/api';
import { toLocalISOString, getLocalMidnight } from '@/utils/datetime';

export interface Provider {
    uuid: string;
    name: string;
    auth_type?: string;
}

export interface APIToken {
    user_id?: string;
    display_name?: string;
    enabled?: boolean;
}

export interface UsageIdentity {
    userId: string;
    label: string;
    type: 'owner' | 'sharing_key';
    enabled: boolean;
}

export type TimeRange = 'today' | 'yesterday' | '3d' | '7d' | '30d' | '90d';

export const TIME_RANGE_CONFIG: Record<TimeRange, { labelKey: string; days: number; interval: string }> = {
    today: { labelKey: 'dashboard.overview.range.today', days: 1, interval: 'minute' },
    yesterday: { labelKey: 'dashboard.overview.range.yesterday', days: 1, interval: 'minute' },
    '3d': { labelKey: 'dashboard.overview.range.3d', days: 3, interval: 'day' },
    '7d': { labelKey: 'dashboard.overview.range.7d', days: 7, interval: 'day' },
    '30d': { labelKey: 'dashboard.overview.range.30d', days: 30, interval: 'day' },
    '90d': { labelKey: 'dashboard.overview.range.90d', days: 90, interval: 'day' },
};

export const shortenUserId = (userId: string): string => {
    if (userId.length <= 12) return userId;
    return `${userId.slice(0, 4)}…${userId.slice(-4)}`;
};

export interface DashboardRecordsParams {
    start_time: string;
    end_time: string;
    provider: string;
    model: string;
    user: string;
}

export interface ProviderOptionGroup {
    authType: string;
    label: string;
    providers: Provider[];
}

const MAIN_ACCOUNT_USER_ID = 'admin';

/**
 * Data layer of the usage dashboard: seq-guarded stat/time-series loads,
 * one-shot filter-option metadata (providers + sharing keys), the records
 * query for the requests view, auto-refresh, and the filter-option
 * snapshots (providers/models currently in the data). Filter/UI state that
 * only shapes rendering (view mode) stays with the page.
 */
export function useDashboardData({
    timeRange,
    isHourlyRange,
    viewMode,
}: {
    timeRange: TimeRange;
    isHourlyRange: boolean;
    viewMode: 'summary' | 'requests' | 'activity';
}) {
    const { t } = useTranslation();

    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(false);
    const [stats, setStats] = useState<AggregatedStat[]>([]);
    const [timeSeries, setTimeSeries] = useState<TimeSeriesData[]>([]);
    const [providers, setProviders] = useState<Provider[]>([]);
    const [usageIdentities, setUsageIdentities] = useState<UsageIdentity[]>([
        { userId: MAIN_ACCOUNT_USER_ID, label: t('dashboard.overview.mainAccount', { defaultValue: 'Main account' }), type: 'owner', enabled: true },
    ]);
    const [selectedProvider, setSelectedProvider] = useState<string>('all');
    const [selectedModel, setSelectedModel] = useState<string>('all');
    const [selectedUser, setSelectedUser] = useState<string>('all');
    // Bumped on manual refresh so the fixed-window activity heatmap refetches too.
    const [heatmapRefresh, setHeatmapRefresh] = useState(0);
    const [records, setRecords] = useState<UsageRecord[]>([]);
    const [recordsLoading, setRecordsLoading] = useState(false);
    // Real total in range from the server (records itself is capped at 500).
    const [recordsTotal, setRecordsTotal] = useState(0);
    // Full parameter set for the records query (time window + filters),
    // written by loadData after each load. A fresh object per load means the
    // requests-view effect refires exactly once per dashboard load — records
    // used to be fetched twice per filter change (once from the filter deps,
    // once from the new time params).
    const [recordsParams, setRecordsParams] = useState<DashboardRecordsParams | null>(null);

    const buildTimeParams = useCallback((provider: string, model: string, user: string, range: TimeRange) => {
        const now = new Date();
        const config = TIME_RANGE_CONFIG[range];
        const todayStart = getLocalMidnight(now);
        const startTime = new Date(todayStart);
        let endTime: Date;

        if (range === 'today') {
            endTime = now;
        } else if (range === 'yesterday') {
            startTime.setDate(startTime.getDate() - 1);
            endTime = new Date(todayStart);
        } else {
            startTime.setDate(startTime.getDate() - (config.days - 1));
            endTime = new Date(todayStart);
            endTime.setDate(endTime.getDate() + 1);
        }

        const params: Record<string, string> = {
            start_time: toLocalISOString(startTime),
            end_time: toLocalISOString(endTime),
        };
        if (provider && provider !== 'all') {
            params.provider = provider;
        }
        if (model && model !== 'all') {
            params.model = model;
        }
        if (user && user !== 'all') {
            params.user_id = user;
        }
        return params;
    }, []);

    // Monotonic sequence used to drop out-of-order responses when filters
    // change faster than requests complete.
    const requestSeq = useRef(0);

    // Providers and API tokens are filter metadata that doesn't depend on the
    // selected time range or filters — fetch them once (and on manual
    // refresh) instead of on every filter change / auto-refresh tick.
    const loadFilterOptions = useCallback(async () => {
        try {
            const [providersResult, tokensResult] = await Promise.all([
                api.getProviders(),
                api.listAPITokens({ limit: 500 }),
            ]);

            if (providersResult?.success && providersResult?.data) {
                setProviders(providersResult.data);
            }
            if (tokensResult?.success && tokensResult?.data) {
                const tokens: APIToken[] = Array.isArray(tokensResult.data) ? tokensResult.data : tokensResult.data.tokens || [];
                const sharingKeysByUserId = new Map<string, UsageIdentity>();
                tokens.forEach((token) => {
                    if (!token.user_id) return;
                    sharingKeysByUserId.set(token.user_id, {
                        userId: token.user_id,
                        label: token.display_name?.trim() || t('dashboard.overview.unnamedSharingKey', { defaultValue: 'Unnamed sharing key' }),
                        type: 'sharing_key',
                        enabled: token.enabled !== false,
                    });
                });
                const sharingKeys = Array.from(sharingKeysByUserId.values())
                    .sort((a, b) => a.label.localeCompare(b.label));
                setUsageIdentities([
                    { userId: MAIN_ACCOUNT_USER_ID, label: t('dashboard.overview.mainAccount', { defaultValue: 'Main account' }), type: 'owner', enabled: true },
                    ...sharingKeys,
                ]);
            }
        } catch (error) {
            console.error('Failed to load dashboard filter options:', error);
        }
    }, []);

    const loadData = useCallback(async (provider: string, model: string, user: string, range: TimeRange) => {
        const seq = ++requestSeq.current;
        try {
            const config = TIME_RANGE_CONFIG[range];
            const params = buildTimeParams(provider, model, user, range);

            const [statsResult, timeSeriesResult] = await Promise.all([
                // limit is the server-side max (1000): the stat-card totals are
                // summed from these groups, so a low limit silently under-counts.
                api.getUsageStats({ ...params, group_by: 'model', limit: 1000 }),
                api.getUsageTimeSeries({ ...params, interval: config.interval }),
            ]);

            // A newer request was issued while this one was in flight —
            // discard the stale response instead of overwriting fresh data.
            if (seq !== requestSeq.current) {
                return;
            }

            if (statsResult?.data) {
                setStats(statsResult.data);
            }
            if (timeSeriesResult?.data) {
                setTimeSeries(timeSeriesResult.data);
            }

            // Store the records query params for the requests view
            setRecordsParams({ start_time: params.start_time, end_time: params.end_time, provider, model, user });
        } catch (error) {
            console.error('Failed to load dashboard data:', error);
        } finally {
            if (seq === requestSeq.current) {
                setLoading(false);
                setRefreshing(false);
            }
        }
    }, [buildTimeParams]);

    // Same out-of-order protection as loadData: without it, a slow earlier
    // response could overwrite the requests view after a newer one landed.
    const recordsSeq = useRef(0);

    const loadRecords = useCallback(async (params: DashboardRecordsParams | null) => {
        if (!params) return;
        const seq = ++recordsSeq.current;
        setRecordsLoading(true);
        try {
            const filters: Record<string, any> = {
                start_time: params.start_time,
                end_time: params.end_time,
                limit: 500,
                offset: 0,
            };
            if (params.provider !== 'all') {
                filters.provider = params.provider;
            }
            if (params.model !== 'all') {
                filters.model = params.model;
            }
            if (params.user !== 'all') {
                filters.user_id = params.user;
            }
            const result = await api.getUsageRecords(filters);
            if (seq !== recordsSeq.current) {
                return;
            }
            if (result?.data) {
                setRecords(result.data);
                setRecordsTotal(result.meta?.total ?? result.data.length);
            }
        } catch (error) {
            console.error('Failed to load records:', error);
        } finally {
            if (seq === recordsSeq.current) {
                setRecordsLoading(false);
            }
        }
    }, []);

    useEffect(() => {
        loadFilterOptions();
    }, [loadFilterOptions]);

    useEffect(() => {
        loadData(selectedProvider, selectedModel, selectedUser, timeRange);
    }, [loadData, selectedProvider, selectedModel, selectedUser, timeRange]);

    // Provider/model options are snapshotted from the current range's stats, so a
    // selection from one range can be stale (or simply absent) in another. Clear
    // them when the user switches time range. The user filter is kept — it names
    // whose usage you're looking at, which stays meaningful across ranges.
    const prevTimeRangeRef = useRef(timeRange);
    useEffect(() => {
        if (prevTimeRangeRef.current !== timeRange) {
            setSelectedProvider('all');
            setSelectedModel('all');
            prevTimeRangeRef.current = timeRange;
        }
    }, [timeRange]);

    // Load records when entering the requests view or when a dashboard load
    // publishes new query params (filters are carried inside recordsParams).
    useEffect(() => {
        if (viewMode === 'requests') {
            loadRecords(recordsParams);
        }
    }, [viewMode, recordsParams, loadRecords]);

    // Reset a selection only when it disappears from the configured metadata
    // (a deleted provider / sharing key). Checking against the already-filtered
    // stats used to wipe BOTH provider and model back to "all" whenever a
    // combination simply had no data in the selected range.
    useEffect(() => {
        if (selectedProvider !== 'all' && providers.length > 0 && !providers.some((p) => p.uuid === selectedProvider)) {
            setSelectedProvider('all');
        }
        if (selectedUser !== 'all' && !usageIdentities.some((identity) => identity.userId === selectedUser)) {
            setSelectedUser('all');
        }
    }, [providers, usageIdentities, selectedProvider, selectedUser]);

    useEffect(() => {
        if (autoRefresh) {
            const interval = setInterval(() => {
                // loadData refreshes charts and, via the fresh recordsParams
                // object it publishes, the requests view. Bump the heatmap key
                // too — the Activity view used to go stale under auto-refresh.
                loadData(selectedProvider, selectedModel, selectedUser, timeRange);
                setHeatmapRefresh((n) => n + 1);
            }, 60000);
            return () => clearInterval(interval);
        }
    }, [autoRefresh, loadData, selectedProvider, selectedModel, selectedUser, timeRange]);

    const handleRefresh = () => {
        setRefreshing(true);
        loadFilterOptions();
        loadData(selectedProvider, selectedModel, selectedUser, timeRange);
        setHeatmapRefresh((n) => n + 1);
    };

    // Group providers by auth_type for the dropdown
    const authTypeLabel = (authType: string): string => {
        switch (authType) {
            case 'oauth': return 'OAuth';
            case 'api_key': return t('dashboard.overview.authType.apiKey', { defaultValue: 'API Key' });
            case 'bearer_token': return t('dashboard.overview.authType.bearerToken', { defaultValue: 'Bearer Token' });
            case 'basic_auth': return t('dashboard.overview.authType.basicAuth', { defaultValue: 'Basic Auth' });
            case 'vmodel': return t('dashboard.overview.authType.vmodel', { defaultValue: 'Virtual Model' });
            default: return authType || t('dashboard.overview.authType.other', { defaultValue: 'Other' });
        }
    };

    const AUTH_TYPE_ORDER = ['oauth', 'api_key', 'bearer_token', 'basic_auth', 'vmodel'];

    // Providers that appear in the data — snapshotted only while no provider
    // is selected. Deriving this from the live (already filtered) stats
    // collapsed the dropdown to just the selected provider, forcing a
    // clear-filters round-trip to switch to a different one.
    const [providerUuidsInData, setProviderUuidsInData] = useState<Set<string>>(() => new Set());
    useEffect(() => {
        if (selectedProvider === 'all') {
            setProviderUuidsInData(new Set(
                stats
                    .map(s => s.provider_uuid)
                    .filter((uuid): uuid is string => uuid != null && uuid !== '')
            ));
        }
    }, [stats, selectedProvider]);

    const groupedProviderOptions = useMemo<ProviderOptionGroup[]>(() => {
        const groups: Record<string, Provider[]> = {};
        providers
            .filter(p => providerUuidsInData.has(p.uuid))  // Only include providers in current data
            .forEach((p) => {
                const authType = p.auth_type || 'api_key';
                if (!groups[authType]) groups[authType] = [];
                groups[authType].push(p);
            });
        // Sort providers within each group by name
        Object.values(groups).forEach((list) => list.sort((a, b) => a.name.localeCompare(b.name)));

        // Return in predefined order, skip empty groups
        return AUTH_TYPE_ORDER
            .filter((t) => groups[t]?.length)
            .map((authType) => ({
                authType,
                label: authTypeLabel(authType),
                providers: groups[authType],
            }));
    }, [providers, providerUuidsInData]);

    // Unique models from stats, sorted by usage — same snapshot pattern as the
    // provider options: only recompute while no model is selected, so sibling
    // models stay selectable after picking one.
    const [modelOptions, setModelOptions] = useState<string[]>([]);
    useEffect(() => {
        if (selectedModel !== 'all') return;
        const modelMap = new Map<string, { model: string; totalTokens: number }>();
        stats.forEach((stat) => {
            const model = stat.model || stat.key || 'Unknown';
            const totalTokens = getTotalTokens(stat);
            const existing = modelMap.get(model);
            if (!existing || totalTokens > existing.totalTokens) {
                modelMap.set(model, { model, totalTokens });
            }
        });
        setModelOptions(Array.from(modelMap.values())
            .sort((a, b) => b.totalTokens - a.totalTokens)
            .map((m) => m.model));
    }, [stats, selectedModel]);

    const hasActiveFilters = selectedProvider !== 'all' || selectedModel !== 'all' || selectedUser !== 'all';

    // Owner label is rendered through t() so a live language switch updates it;
    // sharing-key labels carry their own display name instead.
    const identityLabel = (identity: UsageIdentity): string =>
        identity.type === 'owner'
            ? t('dashboard.overview.mainAccount', { defaultValue: 'Main account' })
            : identity.label;

    const selectedIdentity = usageIdentities.find((i) => i.userId === selectedUser);
    const selectedIdentityLabel = selectedUser === 'all'
        ? t('dashboard.overview.allIdentities', { defaultValue: 'All identities' })
        : selectedIdentity
            ? identityLabel(selectedIdentity)
            : shortenUserId(selectedUser);

    const handleClearFilters = () => {
        setSelectedProvider('all');
        setSelectedModel('all');
        setSelectedUser('all');
    };

    return {
        // load state
        loading,
        refreshing,
        autoRefresh,
        setAutoRefresh,
        handleRefresh,
        // data
        stats,
        timeSeries,
        records,
        recordsLoading,
        recordsTotal,
        recordsParams,
        heatmapRefresh,
        // filters
        selectedProvider,
        setSelectedProvider,
        selectedModel,
        setSelectedModel,
        selectedUser,
        setSelectedUser,
        hasActiveFilters,
        handleClearFilters,
        // filter options
        groupedProviderOptions,
        modelOptions,
        usageIdentities,
        selectedIdentityLabel,
    };
}
