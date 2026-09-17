// Mock-mode-only overrides for first-run UI (onboarding guides, tours…)
// that pop up unprompted. Left alone, they're exactly what a real new user
// should see — but a screenshot/E2E script hits a fresh browser context on
// every run, so without this every single run looks like day one and the
// guide dialog steals focus and blocks clicks.
//
// Default in mock mode: OFF. Automation gets a quiet app by default and
// never needs to know these dialogs exist. A test that specifically wants
// to exercise the first-run experience opts back in per-run via a query
// param — no code change, no shared state between runs.
//
//   ?mockOnboarding=on   force the guide to show, as if never seen
//   ?mockOnboarding=off  force it to stay hidden (the default — this param
//                        only needs to be spelled out when a page or a
//                        prior run left it in the "seen" state and a test
//                        wants to explicitly assert the off state)
//
// Adding another first-run flow later: give it its own localStorage key in
// utils/onboardingFlags.ts and seed/clear it the same way below — the query
// param namespace (`mockOnboarding`) is shared, so keep this a single flag
// for now rather than inventing per-flow param names ahead of need.
import { ROUTING_GUIDE_SEEN_KEY } from '@/utils/onboardingFlags';

export type MockOnboardingMode = 'on' | 'off';

const QUERY_PARAM = 'mockOnboarding';

export function resolveMockOnboardingMode(search: string): MockOnboardingMode {
    return new URLSearchParams(search).get(QUERY_PARAM) === 'on' ? 'on' : 'off';
}

// Runs once at mock app boot, before React mounts — so by the time a guide's
// own auto-open effect checks localStorage, the answer is already the one
// this run asked for.
export function applyMockOnboardingOverride(search: string = window.location.search): void {
    const mode = resolveMockOnboardingMode(search);
    try {
        if (mode === 'on') {
            localStorage.removeItem(ROUTING_GUIDE_SEEN_KEY);
        } else {
            localStorage.setItem(ROUTING_GUIDE_SEEN_KEY, '1');
        }
    } catch {
        // Storage unavailable — nothing to override, the guide's own effect
        // already treats that case as "skip rather than risk re-opening".
    }
}
