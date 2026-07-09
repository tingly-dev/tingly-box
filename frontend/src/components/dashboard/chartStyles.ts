import type { Theme } from '@mui/material/styles';

// Token color palette with semantic meaning
// These colors should be used with theme palette in components
// This file is kept for backward compatibility and constants

export const TOKEN_COLORS = {
    input: {
        main: '#3B82F6',   // Blue 500
        light: '#60A5FA',  // Blue 400
        dark: '#2563EB',   // Blue 600
        gradient: 'rgba(59, 130, 246, 0.8)',
        gradientStart: 'rgba(59, 130, 246, 0.9)',
        gradientEnd: 'rgba(59, 130, 246, 0.6)',
    },
    cache: {
        main: '#94a3b8',   // Slate 400 - visible gray
        light: '#cbd5e1',  // Slate 300
        dark: '#64748b',   // Slate 500
        gradient: 'rgba(148, 163, 184, 0.7)',
        gradientStart: 'rgba(148, 163, 184, 0.8)',
        gradientEnd: 'rgba(148, 163, 184, 0.6)',
    },
    output: {
        main: '#10B981',  // Emerald 500
        light: '#34D399',  // Emerald 400
        dark: '#059669',   // Emerald 600
        gradient: 'rgba(16, 185, 129, 0.8)',
        gradientStart: 'rgba(16, 185, 129, 0.9)',
        gradientEnd: 'rgba(16, 185, 129, 0.6)',
    },
};

// Get theme-aware chart styles
export const getThemeChartStyles = (theme: Theme) => {
    const palette = theme.palette as any;
    const dashboardColors = palette?.dashboard || LIGHT_DASHBOARD_COLORS;

    return {
        token: dashboardColors.token || TOKEN_COLORS,
        chart: dashboardColors.chart || {
            grid: '#f1f5f9',
            axis: '#e2e8f0',
            tooltipBg: '#ffffff',
            tooltipBorder: '#e2e8f0',
        },
        statCard: dashboardColors.statCard || {
            boxShadow: 'none',
            emptyIconBg: 'rgba(100, 116, 139, 0.1)',
        },
    };
};

// Default dashboard colors (light theme)
const LIGHT_DASHBOARD_COLORS = {
    token: TOKEN_COLORS,
    chart: {
        grid: '#f1f5f9',
        axis: '#e2e8f0',
        tooltipBg: '#ffffff',
        tooltipBorder: '#e2e8f0',
    },
    statCard: {
        boxShadow: 'none',
        emptyIconBg: 'rgba(100, 116, 139, 0.1)',
    },
};

// Quota bar colors based on usage percentage
export const QUOTA_COLORS = {
    success: '#10b981',  // emerald-500 - < 50%
    warning: '#f59e0b',  // amber-500 - 50-80%
    error: '#ef4444',    // red-500 - > 80%
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
        fontSize: '0.75rem',
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
