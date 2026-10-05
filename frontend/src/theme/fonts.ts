// Single source of truth for font stacks.
// Import these instead of writing font strings inline.

export const fontSans =
  '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei UI", "Microsoft YaHei", "Roboto", "Helvetica", "Arial", system-ui, sans-serif';

// Keeps the existing look (Fira/Cascadia first); ui-monospace / SF Mono only
// fill the gap on macOS before the generic fallbacks.
// Modern coding fonts first, then broad OS fallbacks.
export const fontMono =
  '"Fira Code", "Cascadia Code", ui-monospace, "SF Mono", Consolas, Menlo, Monaco, "Courier New", monospace';
