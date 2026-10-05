import type { Theme } from '@mui/material/styles';
import { fontSizes } from '@/theme/fonts';
import { lightPalette } from '@/theme/palettes/light';

// Token color palette with semantic meaning
// These colors should be used with theme palette in components
// This file is kept for backward compatibility and constants

export const TOKEN_COLORS = lightPalette.dashboard.token;

// Fallback when the active theme has no dashboard tokens: the light palette's own.
const LIGHT_DASHBOARD_COLORS = lightPalette.dashboard;

// Get theme-aware chart styles
export const getThemeChartStyles = (theme: Theme) => {
    const palette = theme.palette as any;
    const dashboardColors = palette?.dashboard || LIGHT_DASHBOARD_COLORS;

    return {
        token: dashboardColors.token || TOKEN_COLORS,
        chart: dashboardColors.chart || LIGHT_DASHBOARD_COLORS.chart,
        statCard: dashboardColors.statCard || LIGHT_DASHBOARD_COLORS.statCard,
    };
};

// Quota bar colors based on remaining percentage
export const QUOTA_COLORS = {
    success: '#10b981',  // emerald-500 - > 50% remaining
    warning: '#f59e0b',  // amber-500 - 20-50% remaining
    error: '#ef4444',    // red-500 - <= 20% remaining
    secondary: '#94a3b8', // slate-400 - secondary quota
    background: '#f1f5f9', // slate-100 - background bar
};

// Common grid style - very subtle (deprecated, use theme)
export const gridStyle = {
    stroke: '#f1f5f9',
    strokeDasharray: '4 4',
    strokeOpacity: 0.5,
};

// Common axis style (deprecated, use theme)
export const axisStyle = {
    stroke: '#e2e8f0',
    strokeWidth: 1,
};

// Common tooltip style (deprecated, use theme)
export const tooltipStyle = {
    borderRadius: 2,
    border: '1px solid #e2e8f0',
    boxShadow: 'none',
    backgroundColor: 'white',
    padding: '12px',
    minWidth: 200,
};

// Tooltip text styles
export const tooltipTextStyles = {
    title: {
        fontWeight: 600,
        mb: 1,
        fontSize: '0.875rem',
        color: '#0f172a',
    },
    body: {
        color: '#0f172a',
        fontSize: '0.875rem',
    },
    caption: {
        color: '#64748b',
        fontSize: fontSizes.sm,
    },
    divider: '1px solid #e2e8f0',
};

// Bar radius for rounded corners
export const barRadius: [number, number, number, number] = [4, 4, 0, 0];

// Animation duration for chart transitions
export const ANIMATION_DURATION = 600;

// Format large numbers compactly (999, 50K, 1.5M, 20.4B, 1.1T).
//
// Backed by Intl's compact notation instead of a hand-rolled unit ladder: a
// manual "divide, then toFixed" approach rounds within the chosen unit and
// can strand a value just below a boundary — e.g. 999999 divided by 1e3 and
// rounded to 0 decimals gives "1000K" instead of carrying into "1M". Intl
// rounds to the target significant digits first and picks the unit that
// carry produces, so boundary values always land in the right unit.
const compactNumberFormatter = new Intl.NumberFormat('en-US', {
    notation: 'compact',
    compactDisplay: 'short',
    maximumSignificantDigits: 3,
});

export const formatNumber = (n: number): string => compactNumberFormatter.format(n);

// The backend's total_tokens field is deliberately input+output only (cache
// is billed separately — see .design/usage-tracking.md), so any
// surface displaying a true grand total must derive it from the three raw
// fields instead of trusting total_tokens.
export const getTotalTokens = (stat: {
    total_input_tokens?: number;
    total_output_tokens?: number;
    cache_read_tokens?: number;
}): number => (stat.total_input_tokens || 0) + (stat.total_output_tokens || 0) + (stat.cache_read_tokens || 0);

// Cache / (cache + input), as a percentage.
export const getCacheHitRate = (cacheTokens: number, inputTokens: number): number =>
    (cacheTokens + inputTokens) > 0 ? (cacheTokens / (cacheTokens + inputTokens)) * 100 : 0;

// Formats the read/write breakdown shown under a cache stat. Cache writes are
// only reported by gpt-5.6+ and Anthropic; on every other channel the count is
// permanently zero, so the write half is omitted rather than shown as noise.
export const formatCacheBreakdown = (
    cacheReadTokens: number,
    cacheWriteTokens: number,
    format: (n: number) => string,
    labels: { read: string; written: string } = { read: 'read', written: 'written' },
): string =>
    cacheWriteTokens > 0
        ? `${format(cacheReadTokens)} ${labels.read} \u00b7 ${format(cacheWriteTokens)} ${labels.written}`
        : `${format(cacheReadTokens)} ${labels.read}`;

// Whether any row carries a cache write, i.e. whether the write dimension is
// worth showing at all. Owns the "omit when there is nothing to attribute"
// policy for the tables, the same way formatCacheBreakdown owns it for the
// stat cards — keep both readings of that policy in this one file.
export const hasCacheWrites = (rows: { cache_write_tokens?: number }[]): boolean =>
    rows.some((r) => (r.cache_write_tokens ?? 0) > 0);

// Health-gauge color for a Cache Hit Rate stat card (higher is better).
export const getCacheHitRateColor = (percent: number): 'success' | 'warning' | 'error' =>
    percent >= 50 ? 'success' : percent >= 20 ? 'warning' : 'error';

// Health-gauge color for an Error Rate stat card (lower is better; percent is 0-100 scale).
export const getErrorRateColor = (percent: number): 'success' | 'warning' | 'error' =>
    percent > 5 ? 'error' : percent > 1 ? 'warning' : 'success';
