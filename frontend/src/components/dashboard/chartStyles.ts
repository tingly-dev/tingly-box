// Token color palette with semantic meaning
// Blue → Calm, foundation (input is always present)
// Amber → Warm, highlights optimization (cache is savings)
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
        main: '#F59E0B',   // Amber 500
        light: '#FBBF24',  // Amber 400
        dark: '#D97706',   // Amber 600
        gradient: 'rgba(245, 158, 11, 0.85)',
        gradientStart: 'rgba(245, 158, 11, 0.95)',
        gradientEnd: 'rgba(245, 158, 11, 0.75)',
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
};

// Bar radius for rounded corners
export const barRadius = [0, 4, 4, 0];

// Animation duration for chart transitions
export const ANIMATION_DURATION = 600;
