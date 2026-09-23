import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getTotalTokens } from '@/components/dashboard';
import type { AggregatedStat } from '@/components/dashboard';
import api from '@/services/api';
import { buildTimeParams } from './userUsageModel';
import type { APITokenInfo, TimeRange, UserUsageRow } from './userUsageModel';

/**
 * Data layer of the team usage page: the race-guarded roster fetch (registered
 * tokens + user/model/provider rosters) and the account-row join the account
 * axis consumes. `range` is owned by the page (header toggle); everything
 * here only reacts to it.
 */
export function useUserUsageData(range: TimeRange) {
    const { t } = useTranslation();
    const [tokens, setTokens] = useState<APITokenInfo[]>([]);
    const [userStats, setUserStats] = useState<AggregatedStat[]>([]);
    const [modelRoster, setModelRoster] = useState<AggregatedStat[]>([]);
    const [providerRoster, setProviderRoster] = useState<AggregatedStat[]>([]);
    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [error, setError] = useState('');
    const requestSeq = useRef(0);

    const loadRosters = useCallback(async (selectedRange: TimeRange, manual = false) => {
        const seq = ++requestSeq.current;
        if (manual) setRefreshing(true);
        setError('');
        try {
            const timeParams = buildTimeParams(selectedRange);
            const [tokensResult, userStatsResult, modelStatsResult, providerStatsResult] = await Promise.all([
                api.listAPITokens({ limit: 500 }),
                api.getUsageStats({
                    ...timeParams,
                    group_by: 'user',
                    sort_by: 'total_tokens',
                    sort_order: 'desc',
                    limit: 500,
                }),
                api.getUsageStats({
                    ...timeParams,
                    group_by: 'model',
                    sort_by: 'total_tokens',
                    sort_order: 'desc',
                    limit: 1000,
                }),
                api.getUsageStats({
                    ...timeParams,
                    group_by: 'provider',
                    sort_by: 'total_tokens',
                    sort_order: 'desc',
                    limit: 200,
                }),
            ]);
            if (seq !== requestSeq.current) return;
            if (!tokensResult?.success) throw new Error(tokensResult?.error || 'Unable to load registered users');
            const tokenData = Array.isArray(tokensResult.data)
                ? tokensResult.data
                : tokensResult.data?.tokens || [];
            const sharingUsers: APITokenInfo[] = tokenData
                .filter((token: APITokenInfo) => token.user_id !== 'admin')
                .map((token: APITokenInfo) => ({ ...token, account_type: 'sharing' }));
            setTokens([
                {
                    token_id: 'primary-account',
                    user_id: 'admin',
                    display_name: '',
                    enabled: true,
                    account_type: 'primary',
                },
                ...sharingUsers,
            ]);
            setUserStats(userStatsResult?.data || []);
            setModelRoster(modelStatsResult?.data || []);
            setProviderRoster(providerStatsResult?.data || []);
        } catch (loadError) {
            if (seq === requestSeq.current) {
                setError(loadError instanceof Error ? loadError.message : 'Unable to load team usage');
            }
        } finally {
            if (seq === requestSeq.current) {
                setLoading(false);
                setRefreshing(false);
            }
        }
    }, []);

    useEffect(() => {
        loadRosters(range);
    }, [loadRosters, range]);

    const rows = useMemo<UserUsageRow[]>(() => {
        const statsByUser = new Map(
            userStats.map((stat) => [stat.user_id || stat.key, stat]),
        );
        return tokens.map((token) => {
            const stat = statsByUser.get(token.user_id);
            return {
                ...token,
                display_name: token.account_type === 'primary'
                    ? t('dashboard.userUsage.primaryAccount', { defaultValue: 'Primary account' })
                    : token.display_name,
                request_count: stat?.request_count || 0,
                // total_tokens is derived via the shared helper, not read from
                // the API's total_tokens field (input+output only, excludes
                // cache — see .design/stream-usage-tracking.md).
                total_tokens: getTotalTokens(stat ?? {}),
                total_input_tokens: stat?.total_input_tokens || 0,
                total_output_tokens: stat?.total_output_tokens || 0,
                cache_read_tokens: stat?.cache_read_tokens || 0,
                cache_write_tokens: stat?.cache_write_tokens || 0,
                error_count: stat?.error_count || 0,
                error_rate: stat?.error_rate || 0,
            };
        });
    }, [t, tokens, userStats]);

    const tokenByUserID = useMemo(
        () => new Map(tokens.map((token) => [token.user_id, token])),
        [tokens],
    );
    const accountDisplayName = useCallback((userID: string) => {
        const token = tokenByUserID.get(userID);
        if (token?.account_type === 'primary') {
            return t('dashboard.userUsage.primaryAccount', { defaultValue: 'Primary account' });
        }
        return token?.display_name || userID;
    }, [t, tokenByUserID]);

    return {
        loadRosters,
        rows,
        modelRoster,
        providerRoster,
        loading,
        refreshing,
        error,
        accountDisplayName,
    };
}
