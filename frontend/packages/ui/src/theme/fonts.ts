// Single source of truth for font stacks.
// Import these instead of writing font strings inline.

export const fontSans =
  '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei UI", "Microsoft YaHei", "Roboto", "Helvetica", "Arial", system-ui, sans-serif';

// Keeps the existing look (Fira/Cascadia first); ui-monospace / SF Mono only
// fill the gap on macOS before the generic fallbacks.
// Modern coding fonts first, then broad OS fallbacks.
export const fontMono =
  '"Fira Code", "Cascadia Code", ui-monospace, "SF Mono", Consolas, Menlo, Monaco, "Courier New", monospace';

// Small-text size scale. Dozens of call sites used to hand-pick values between
// 0.6rem and 0.86rem (0.62, 0.68, 0.72, 0.76, 0.78, 0.8125, 0.82 ...), so the
// same "small label" came out at a different size on every page. Use these
// instead of a literal; values are the nearest step of the old spread, so no
// call site moves by more than ~0.5px.
export const fontSizes = {
  micro: '0.65rem', // 10.4px — badges, kbd hints, dense chips
  xs: '0.7rem', //   11.2px — secondary meta
  sm: '0.75rem', //  12px   — captions, helper text
  md: '0.8rem', //   12.8px — compact body (matches theme body2)
  lg: '0.85rem', //  13.6px — compact emphasis
} as const;
