// Sequential scales for activity heatmaps. These are data-encoding colours,
// not UI chrome, so they stay theme-independent; they live here so
// usage heatmaps (and their legends) read from one place.

// GitHub-style contribution ramp, level 0 (none) .. 4 (very high).
export const heatmapScale = ['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'] as const;

// Empty heatmap cell and its hairline, per theme mode.
export const heatmapEmpty = { light: '#ebedf0', dark: 'rgba(255, 255, 255, 0.08)' } as const;
export const heatmapCellLine = { light: 'rgba(27, 31, 35, 0.06)', dark: 'rgba(255, 255, 255, 0.06)' } as const;

// Highlight behind the hovered bar of a recharts bar chart.
export const chartCursorFill = 'rgba(0, 0, 0, 0.05)';

// QR codes must stay black on white to scan, in every theme.
export const qrColors = { background: '#ffffff', foreground: '#000000' } as const;
