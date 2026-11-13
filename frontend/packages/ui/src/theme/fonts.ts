// Single source of truth for font stacks.
// Import these instead of writing font strings inline.

export const fontSans =
  '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei UI", "Microsoft YaHei", "Roboto", "Helvetica", "Arial", system-ui, sans-serif';

// Keeps the existing look (Fira/Cascadia first); ui-monospace / SF Mono only
// fill the gap on macOS before the generic fallbacks.
// Modern coding fonts first, then broad OS fallbacks.
export const fontMono =
  '"Fira Code", "Cascadia Code", ui-monospace, "SF Mono", Consolas, Menlo, Monaco, "Courier New", monospace';

// Small-text size scale, matching the theme's type scale (base.ts): 11 for
// badges and counts, 12 for meta, 13 for compact body, 14 for body and
// titles. Use these instead of a literal.
export const fontSizes = {
  micro: '0.6875rem', // 11px — badges, kbd hints, counts, rail labels
  sm: '0.75rem', //      12px — captions, helper text, meta
  md: '0.8125rem', //    13px — compact body (theme body2)
  base: '0.875rem', //   14px — body and titles
  /** @deprecated Use `micro` (the 11.2px step folded into it). */
  xs: '0.6875rem',
  /** @deprecated Use `md` (the 13.6px step folded into it). */
  lg: '0.8125rem',
} as const;
