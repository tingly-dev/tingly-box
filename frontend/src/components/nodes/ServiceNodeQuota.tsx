import { Box } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { useProviderQuotaOf } from '@/contexts/ProviderQuotaContext';
import { useTheme } from '@mui/material/styles';
import { formatNumber } from '../dashboard/chartStyles';
import { QuotaRing, formatQuotaDuration as formatDuration, quotaRingColor, quotaRingSpinSx } from '../credential/QuotaRing';
import {
    formatQuotaAvailable,
    formatQuotaRemaining,
    isCountable,
    quotaRemainingPercent,
    quotaToWindows,
    tightestWindow,
} from '@/types/quota';
import NodeTooltip from './NodeTooltip.tsx';

// Older than this, the figure is dimmed: the cache is refreshed in the
// background, so a stale snapshot means the refresher could not reach upstream.
const STALE_AFTER_MS = 60 * 60 * 1000;

/**
 * The provider's binding quota on a service node: a ring showing the remaining
 * share of its tightest window, mirroring the api style badges on the right.
 * Exact figures, reset times and balances live in the tooltip; clicking the
 * ring asks upstream for a fresh reading.
 * Renders nothing when the provider reports no comparable figure
 * (.design/quota-semantics.md §3.6).
 */
export const ServiceNodeQuota: React.FC<{ providerUuid: string }> = ({ providerUuid }) => {
    const { t } = useTranslation();
    const theme = useTheme();
    const { quota, refreshing, failed, refresh } = useProviderQuotaOf(providerUuid);
    const tightest = tightestWindow(quota);
    if (!quota || !tightest) return null;

    const remaining = quotaRemainingPercent(tightest);
    const color = quotaRingColor(theme, remaining);
    const now = Date.now();
    const fetchedAt = quota.fetched_at ? new Date(quota.fetched_at).getTime() : NaN;
    const stale = Number.isFinite(fetchedAt) && now - fetchedAt > STALE_AFTER_MS;

    const tooltip = (
        <Box sx={{ minWidth: 160 }}>
            {quotaToWindows(quota).map(({ key, label, window }) => {
                const value = isCountable(window)
                    ? t('rule.service.quota.left', { value: formatQuotaRemaining(window, formatNumber) })
                    : formatQuotaAvailable(window, formatNumber);
                if (!value) return null;
                const resetsAt = window.resets_at ? new Date(window.resets_at).getTime() : NaN;
                return (
                    <Box key={key} sx={{ mb: 0.25, fontWeight: window === tightest ? 700 : 400 }}>
                        {label}: {value}
                        {Number.isFinite(resetsAt) && resetsAt > now && (
                            <> · {t('rule.service.quota.resetsIn', { duration: formatDuration(resetsAt - now) })}</>
                        )}
                    </Box>
                );
            })}
            {failed && (
                <Box sx={{ mt: 0.5, color: 'error.main' }}>{t('rule.service.quota.refreshFailed')}</Box>
            )}
            <Box sx={{ mt: 0.5, opacity: 0.7 }}>
                {refreshing
                    ? t('rule.service.quota.refreshing')
                    : [
                        Number.isFinite(fetchedAt) && t('rule.service.quota.updated', { duration: formatDuration(now - fetchedAt) }),
                        refresh && t('rule.service.quota.clickToRefresh'),
                    ].filter(Boolean).join(' · ')}
            </Box>
        </Box>
    );

    return (
        <NodeTooltip title={tooltip} placement="bottom">
            <Box
                component="span"
                role="button"
                tabIndex={0}
                aria-label={t('rule.service.quota.left', { value: `${Math.round(remaining)}%` })}
                aria-busy={refreshing}
                onClick={(e) => {
                    e.stopPropagation();
                    refresh?.();
                }}
                onKeyDown={(e) => {
                    if (e.key !== 'Enter' && e.key !== ' ') return;
                    e.preventDefault();
                    e.stopPropagation();
                    refresh?.();
                }}
                sx={{
                    display: 'inline-flex',
                    borderRadius: '50%',
                    opacity: stale && !refreshing ? 0.5 : 1,
                    cursor: refreshing ? 'progress' : 'pointer',
                    ...(refreshing && quotaRingSpinSx),
                }}
            >
                {/* While refreshing, a fixed quarter arc spins like a loader — the
                    real arc can be empty (used up), and an empty ring shows no motion. */}
                <QuotaRing remaining={refreshing ? 25 : remaining} color={color} />
            </Box>
        </NodeTooltip>
    );
};

export default ServiceNodeQuota;
