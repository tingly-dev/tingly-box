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
 * Exact figures, reset times and balances live in the tooltip.
 * Renders nothing when the provider reports no comparable figure
 * (.design/quota-semantics.md §3.6).
 */
export const ServiceNodeQuota: React.FC<{ providerUuid: string }> = ({ providerUuid }) => {
    const { t } = useTranslation();
    const quota = useProviderQuotaOf(providerUuid);
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
            {Number.isFinite(fetchedAt) && (
                <Box sx={{ mt: 0.5, opacity: 0.7 }}>
                    {t('rule.service.quota.updated', { duration: formatDuration(now - fetchedAt) })}
                </Box>
            )}
        </Box>
    );

    return (
        <NodeTooltip title={tooltip} placement="bottom">
            <Box
                component="span"
                role="img"
                aria-label={t('rule.service.quota.left', { value: `${Math.round(remaining)}%` })}
                onClick={(e) => e.stopPropagation()}
                sx={{ display: 'inline-flex', opacity: stale ? 0.5 : 1, cursor: 'default' }}
            >
                <QuotaRing remaining={remaining} color={color} />
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
