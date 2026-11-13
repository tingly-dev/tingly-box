// Single source of truth for font stacks.
// Import these instead of writing font strings inline.

// Inter is bundled (@fontsource-variable/inter, SIL OFL 1.1 — loaded by each
// app's main.tsx) so Latin text looks the same on macOS, Windows, Linux and the
// desktop WebView instead of falling to whatever sans the OS has. Its
// @font-face rules cover Latin, Cyrillic, Greek and Vietnamese only, so
// Chinese characters fall through to the system CJK fonts below.
export const fontSans =
  '"Inter Variable", -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei UI", "Microsoft YaHei", "Roboto", "Helvetica", "Arial", system-ui, sans-serif';

// Keeps the existing look (Fira/Cascadia first); ui-monospace / SF Mono only
// fill the gap on macOS before the generic fallbacks.
// Modern coding fonts first, then broad OS fallbacks.
export const fontMono =
  '"Fira Code", "Cascadia Code", ui-monospace, "SF Mono", Consolas, Menlo, Monaco, "Courier New", monospace';

// Text size scale, the same steps as the theme's typography (base.ts). Use
// these — or a typography variant — instead of a literal; the style-literal
// lint rejects rem/px font sizes outside the theme. Icon sizes are plain px
// numbers (`fontSize: 16`), not text sizes.
export const fontSizes = {
  micro: '0.6875rem', //  11px — badges, kbd hints, counts, rail labels
  sm: '0.75rem', //       12px — captions, helper text, meta, table heads
  md: '0.8125rem', //     13px — compact body (theme body2)
  base: '0.875rem', //    14px — body, card and row titles
  title: '1rem', //       16px — section and dialog titles (theme h4)
  lead: '1.125rem', //    18px — second-level headings in rendered Markdown
  heading: '1.25rem', //  20px — page titles, headline numbers (theme h3)
  display: '1.5rem', //   24px — the largest figures (theme h2)
  inlineCode: '0.85em', // relative — code inside running text
} as const;
