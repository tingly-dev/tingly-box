// Sequential scales for activity heatmaps. These are data-encoding colours,
// not UI chrome, so they stay theme-independent; they live here so
// usage heatmaps (and their legends) read from one place.

// GitHub-style contribution ramp, level 0 (none) .. 4 (very high).
export const heatmapScale = ['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'] as const;
