// Sequential scales for activity heatmaps. These are data-encoding colours,
// not UI chrome, so they stay theme-independent; they live here so the two
// calendars/heatmaps (and their legends) read from one place.

// GitHub-style contribution ramp, level 0 (none) .. 4 (very high).
export const heatmapScale = ['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'] as const;

// Recording calendar ramp, level 1 (low) .. 4 (very high). Level 0 is
// transparent in the grid and `activityEmpty` in the legend.
export const activityScale = ['#dcfce7', '#86efac', '#22c55e', '#15803d'] as const;
export const activityEmpty = '#f3f4f6';
