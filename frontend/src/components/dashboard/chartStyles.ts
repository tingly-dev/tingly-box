// Token color palette with semantic meaning
// Blue → Calm, foundation (input is always present)
// Blue-Gray → Subtle, background optimization (cache is large but secondary)
// Green → Success, result (output is what we get)
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
        main: '#cbd5e1',   // Slate 300 - blue-gray, subtle
        light: '#e2e8f0',  // Slate 200
        dark: '#94a3b8',   // Slate 400
        gradient: 'rgba(203, 213, 225, 0.7)',
        gradientStart: 'rgba(203, 213, 225, 0.8)',
        gradientEnd: 'rgba(203, 213, 225, 0.6)',
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

// Quota bar colors based on usage percentage
export const QUOTA_COLORS = {
    success: '#10b981',  // emerald-500 - < 50%
    warning: '#f59e0b',  // amber-500 - 50-80%
    error: '#ef4444',    // red-500 - > 80%
    secondary: '#94a3b8', // slate-400 - secondary quota
    background: '#f1f5f9', // slate-100 - background bar
};

// Common grid style - very subtle
export const gridStyle = {
    stroke: '#f1f5f9',
    strokeDasharray: '4 4',
    strokeOpacity: 0.5,
};

// Common axis style
export const axisStyle = {
    stroke: '#e2e8f0',
    strokeWidth: 1,
};

// Common tooltip style
export const tooltipStyle = {
    borderRadius: 2,
    border: '1px solid #e2e8f0',
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.1)',
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

// Format numbers (50K, 1M, etc.)
export const formatNumber = (n: number): string => {
    if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
    if (n >= 1000) return `${(n / 1000).toFixed(0)}K`;
    return n.toString();
};
