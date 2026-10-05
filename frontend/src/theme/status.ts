import { getContrastRatio, type Theme } from '@mui/material/styles';

// One place that answers "what colour is success / error / warning here?".
// Status colours used to be hand-written at each call site (#10b981, #22c55e,
// #059669, #34D399 for "success"; #ef4444, #dc2626, #f87171 for "error"), so
// they disagreed with each other and ignored themes that define their own
// (claude's olive green, for one). Everything now reads the active palette.

export type StatusTone =
  | 'success'
  | 'info'
  | 'warning'
  | 'error'
  | 'critical' // worse than error: panic / fatal
  | 'neutral' // known but unremarkable
  | 'muted'; // debug / unknown / disabled

export const getStatusColor = (theme: Theme, tone: StatusTone): string => {
  switch (tone) {
    case 'success':
      return theme.palette.success.main;
    case 'info':
      return theme.palette.info.main;
    case 'warning':
      return theme.palette.warning.main;
    case 'error':
      return theme.palette.error.main;
    case 'critical':
      return theme.palette.error.dark;
    case 'neutral':
      return theme.palette.text.secondary;
    case 'muted':
      return theme.palette.text.disabled;
  }
};

/** Traffic-light tone for a remaining share (quota bars, rings). */
export const quotaTone = (remainingPercent: number): StatusTone =>
  remainingPercent <= 20 ? 'error' : remainingPercent <= 50 ? 'warning' : 'success';

/** Tone for an HTTP status code; no/unknown code is muted. */
export const httpStatusTone = (statusCode?: number): StatusTone => {
  if (!statusCode) return 'muted';
  if (statusCode >= 200 && statusCode < 300) return 'success';
  if (statusCode >= 300 && statusCode < 400) return 'info';
  if (statusCode >= 400 && statusCode < 500) return 'warning';
  if (statusCode >= 500) return 'error';
  return 'muted';
};

/** Tone for a log level. Unknown levels read as success, as before. */
export const logLevelTone = (level: string): StatusTone => {
  switch (level.toLowerCase()) {
    case 'panic':
    case 'fatal':
      return 'critical';
    case 'error':
      return 'error';
    case 'warning':
    case 'warn':
      return 'warning';
    case 'info':
      return 'info';
    case 'debug':
      return 'muted';
    default:
      return 'success';
  }
};

export type AccentTone = 'primary' | 'secondary' | 'success' | 'info' | 'warning' | 'error';

/**
 * A palette tone as a *foreground on a card* (icon, tint source): the tone's
 * `main` when it is legible on `background.paper` (WCAG 3:1 for graphics),
 * otherwise the nearest variant that is. Palettes pick `main` for fills, so
 * some are too pale or too deep to read as an icon — claude's secondary
 * (#B0AEA5) on white, or a mid-green success on a dark card.
 */
export const getReadableAccent = (theme: Theme, tone: AccentTone): string => {
  const { palette } = theme;
  const bg = palette.background.paper;
  const c = palette[tone];
  const ratio = (color: string) => getContrastRatio(color, bg);
  if (palette.mode === 'dark') return ratio(c.light) > ratio(c.main) ? c.light : c.main;
  return ratio(c.main) >= 3 ? c.main : ratio(c.dark) > ratio(c.main) ? c.dark : c.main;
};
