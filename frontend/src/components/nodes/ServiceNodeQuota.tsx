import { Box } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { useProviderQuotaOf } from '@/contexts/ProviderQuotaContext';
import { QUOTA_COLORS, formatNumber } from '../dashboard/chartStyles';
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

function formatDuration(ms: number): string {
    const mins = Math.max(0, Math.floor(ms / 60000));
    if (mins < 60) return `${mins}m`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ${mins % 60}m`;
    return `${Math.floor(hrs / 24)}d ${hrs % 24}h`;
}

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
    const { quota, refreshing, failed, refresh } = useProviderQuotaOf(providerUuid);
    const tightest = tightestWindow(quota);
    if (!quota || !tightest) return null;

    const remaining = quotaRemainingPercent(tightest);
    const color = remaining <= 20 ? QUOTA_COLORS.error : remaining <= 50 ? QUOTA_COLORS.warning : QUOTA_COLORS.success;
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
                <Box sx={{ mt: 0.5, color: QUOTA_COLORS.error }}>{t('rule.service.quota.refreshFailed')}</Box>
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
                    ...(refreshing && {
                        '@keyframes quota-ring-spin': {
                            '0%': { transform: 'rotate(0deg)' },
                            '100%': { transform: 'rotate(360deg)' },
                        },
                        animation: 'quota-ring-spin 1s linear infinite',
                    }),
                }}
            >
                {/* While refreshing, a fixed quarter arc spins like a loader — the
                    real arc can be empty (used up), and an empty ring shows no motion. */}
                <QuotaRing remaining={refreshing ? 25 : remaining} color={color} />
            </Box>
        </NodeTooltip>
    );
};

// Sized to match the 20px ApiStyleBadge circles on the other side of the row.
const RING_SIZE = 20;
const RING_STROKE = 2.5;

/** Remaining share as an arc running clockwise from 12 o'clock over a faint track. */
const QuotaRing: React.FC<{ remaining: number; color: string }> = ({ remaining, color }) => {
    const r = (RING_SIZE - RING_STROKE) / 2;
    const circumference = 2 * Math.PI * r;
    const c = RING_SIZE / 2;
    return (
        <svg width={RING_SIZE} height={RING_SIZE} viewBox={`0 0 ${RING_SIZE} ${RING_SIZE}`}>
            <circle cx={c} cy={c} r={r} fill="none" stroke={color} strokeOpacity={0.25} strokeWidth={RING_STROKE} />
            {/* No arc at all when used up — a round cap on a zero-length arc
                still paints a dot that reads as "a little left". */}
            {remaining > 0 && <circle
                cx={c}
                cy={c}
                r={r}
                fill="none"
                stroke={color}
                strokeWidth={RING_STROKE}
                strokeLinecap="round"
                strokeDasharray={`${circumference * remaining / 100} ${circumference}`}
                transform={`rotate(-90 ${c} ${c})`}
            />}
        </svg>
    );
};

export default ServiceNodeQuota;
