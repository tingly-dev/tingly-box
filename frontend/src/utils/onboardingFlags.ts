// The localStorage keys a first-run guide uses to record "the user has
// already seen this, don't auto-open it again". Each key is the single
// source of truth shared between the guide's own auto-open effect and the
// mock-mode test override that pre-seeds/clears it (see
// mocks/mockConfig.ts) — defining it in one place is what keeps the two
// from drifting apart if the key name ever changes.

// TemplatePage.tsx: the Direct routing guide that auto-opens once per user
// the first time they land on a populated routing page.
export const ROUTING_GUIDE_SEEN_KEY = 'tb.routingGuideAutoShown';
