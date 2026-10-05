import React from 'react';
import type { Theme } from '@mui/material/styles';
import { getStatusColor, quotaTone } from '@/theme/status';

/** Traffic-light color for a remaining share, shared by every quota ring. */
export function quotaRingColor(theme: Theme, remaining: number): string {
    return getStatusColor(theme, quotaTone(remaining));
}

/** "5m" / "3h 12m" / "2d 4h" — used for "updated … ago" and "resets in …". */
export function formatQuotaDuration(ms: number): string {
    const mins = Math.max(0, Math.floor(ms / 60000));
    if (mins < 60) return `${mins}m`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ${mins % 60}m`;
    return `${Math.floor(hrs / 24)}d ${hrs % 24}h`;
}

const STROKE = 2.5;

/**
 * Remaining share as an arc running clockwise from 12 o'clock over a faint
 * track. Sized by default to match the 20px ApiStyleBadge circles it usually
 * sits next to.
 */
export const QuotaRing: React.FC<{ remaining: number; color: string; size?: number }> = ({
    remaining,
    color,
    size = 20,
}) => {
    const r = (size - STROKE) / 2;
    const circumference = 2 * Math.PI * r;
    const c = size / 2;
    return (
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flexShrink: 0 }}>
            <circle cx={c} cy={c} r={r} fill="none" stroke={color} strokeOpacity={0.25} strokeWidth={STROKE} />
            {/* No arc at all when used up — a round cap on a zero-length arc
                still paints a dot that reads as "a little left". */}
            {remaining > 0 && <circle
                cx={c}
                cy={c}
                r={r}
                fill="none"
                stroke={color}
                strokeWidth={STROKE}
                strokeLinecap="round"
                strokeDasharray={`${circumference * remaining / 100} ${circumference}`}
                transform={`rotate(-90 ${c} ${c})`}
            />}
        </svg>
    );
};

/** Spread onto a ring's wrapper while refreshing: the ring spins like a loader. */
export const quotaRingSpinSx = {
    '@keyframes quota-ring-spin': {
        '0%': { transform: 'rotate(0deg)' },
        '100%': { transform: 'rotate(360deg)' },
    },
    animation: 'quota-ring-spin 1s linear infinite',
} as const;
