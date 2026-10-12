import { getContrastRatio, type Theme } from '@mui/material/styles';

// Status colours are deliberately NOT read from the theme palette.
//
// They used to be hand-written at each call site (#10b981, #22c55e, #059669,
// #34D399 for "success"), so they disagreed with each other. Reading them from
// the palette fixed that but made them theme-dependent, and a muted theme
// colour (claude's olive green / brick red) is a poor traffic light: on a
// 12px quota ring it reads as muddy and loses its "ok / warning / stop"
// signal. So status is its own fixed set, shared by every theme:
//   fill    - rings, bars, chips, dots (white text sits on it in chips)
//   ink     - the same hue as text/icon on a light surface (>=3:1 on white)
//   inkDark - the same hue as text/icon on a dark surface
const STATUS = {
  success: { fill: '#10b981', ink: '#059669', inkDark: '#34d399' },
  info: { fill: '#3b82f6', ink: '#2563eb', inkDark: '#60a5fa' },
  warning: { fill: '#f59e0b', ink: '#d97706', inkDark: '#fbbf24' },
  error: { fill: '#ef4444', ink: '#dc2626', inkDark: '#f87171' },
  critical: { fill: '#991b1b', ink: '#991b1b', inkDark: '#ef4444' }, // worse than error: panic / fatal
} as const;

export type StatusTone =
  | keyof typeof STATUS
  | 'neutral' // known but unremarkable
  | 'muted'; // debug / unknown / disabled

/** Fill colour for a status tone (ring, bar, chip, dot). Same in every theme. */
export const getStatusColor = (theme: Theme, tone: StatusTone): string => {
  if (tone === 'neutral') return theme.palette.text.secondary;
  if (tone === 'muted') return theme.palette.text.disabled;
  return STATUS[tone].fill;
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
 * A tone as a *foreground on a card* (icon, tint source). Status tones use the
 * fixed status set (ink on light surfaces, inkDark on dark ones). primary and
 * secondary are the theme's own accents, so they follow the palette: `main`
 * when it is legible on `background.paper` (WCAG 3:1 for graphics), otherwise
 * the nearest variant that is (claude's pale secondary #B0AEA5 on white).
 */
export const getReadableAccent = (theme: Theme, tone: AccentTone): string => {
  const { palette } = theme;
  if (tone !== 'primary' && tone !== 'secondary') {
    return palette.mode === 'dark' ? STATUS[tone].inkDark : STATUS[tone].ink;
  }
  const bg = palette.background.paper;
  const c = palette[tone];
  const ratio = (color: string) => getContrastRatio(color, bg);
  if (palette.mode === 'dark') return ratio(c.light) > ratio(c.main) ? c.light : c.main;
  return ratio(c.main) >= 3 ? c.main : ratio(c.dark) > ratio(c.main) ? c.dark : c.main;
};
