// Brand colours: a vendor's own colour, fixed by the vendor, the same in every
// theme. UI chrome never uses these; they mark a provider or platform (a
// connect card's accent, an API-style badge, a guide's header).

/** OAuth provider card accents. */
export const oauthBrandColors = {
  claude: '#D97757',
  gemini: '#4285F4',
  antigravity: '#7B1FA2',
  qwen: '#00A8E1',
  codex: '#10A37F',
  kimi: '#6366F1',
  xai: '#000000',
  zcode: '#3859FF',
  workbuddy: '#14C38E',
  generic: '#9E9E9E',
} as const;

/** IM platform guide accents. */
export const platformBrandColors = {
  telegram: '#0088cc',
  feishu: '#00d6b9',
  dingtalk: '#0089ff',
  weixin: '#07c160',
  wecom: '#10a800',
  other: '#888',
} as const;

/** API-style badge tints. */
export const apiStyleTints = {
  anthropic: '#E07A5F',
  google: '#4285F4',
} as const;

/** Single-colour brand artwork rendered through an <img>: no `currentColor` there. */
export const monoArtwork = {
  onLight: '#000',
  onDark: '#fff',
} as const;
