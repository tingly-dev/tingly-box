// Shared, in-memory, stale-while-revalidate caches for data that many
// scenario-page hooks fetch independently (providers, per-scenario rules).
//
// Why this exists: every "Use <Agent>" page mounts `useFunctionPanelData` /
// `useRuleManagement` up to three times in the same render tree
// (ScenarioPageModalProvider, the page's own useScenarioPageInternal call,
// and TemplatePage's internal useScenarioPageInternal call). Without
// dedup, that's 3x `getProviders()` and 2x `getRules(scenario)` per page
// view, and a fresh full-page loading spinner on every navigation even
// when the data was fetched seconds ago. A small resource cache collapses
// concurrent callers onto one in-flight request and lets repeat mounts
// paint instantly from cache while quietly revalidating in the background.
//
// Scope: only consumed by the scenario-page hooks and AgentSetupCard.
// Everywhere else in the app keeps calling `api.getProviders()` /
// `api.getRules()` directly and gets a true fetch every time.

import { api } from './api';
import type { Provider } from '@/types/provider';

type Listener<T> = (data: T) => void;

function createResourceCache<T>(fetcher: () => Promise<T>) {
    let cache: T | undefined;
    let inflight: Promise<T> | null = null;
    const listeners = new Set<Listener<T>>();

    const fetchFresh = (): Promise<T> => {
        if (!inflight) {
            inflight = fetcher()
                .then((data) => {
                    cache = data;
                    listeners.forEach((l) => l(data));
                    return data;
                })
                .finally(() => {
                    inflight = null;
                });
        }
        return inflight;
    };

    return {
        /** Synchronous cache peek; undefined if nothing has been fetched yet. */
        getCached: () => cache,
        /** Subscribe to future cache updates (fresh fetch or revalidation). */
        subscribe: (fn: Listener<T>): (() => void) => {
            listeners.add(fn);
            return () => {
                listeners.delete(fn);
            };
        },
        /**
         * Always performs a true fetch, but concurrent callers (e.g. the
         * same page mounting this hook 2-3 times) share one in-flight
         * request instead of firing one each.
         */
        refresh: () => fetchFresh(),
        /** Seed the cache directly, e.g. from a request made elsewhere. */
        prime: (data: T) => {
            cache = data;
            listeners.forEach((l) => l(data));
        },
    };
}

export const providersDataCache = createResourceCache<Provider[]>(async () => {
    const result = await api.getProviders();
    return result?.success && Array.isArray(result.data) ? result.data : [];
});

const ruleCachesByScenario = new Map<string, ReturnType<typeof createResourceCache<any[]>>>();

const getRulesCacheFor = (scenario: string) => {
    let entry = ruleCachesByScenario.get(scenario);
    if (!entry) {
        entry = createResourceCache<any[]>(async () => {
            const result = await api.getRules(scenario);
            return result?.success && Array.isArray(result.data) ? result.data : [];
        });
        ruleCachesByScenario.set(scenario, entry);
    }
    return entry;
};

export const rulesDataCache = {
    getCached: (scenario: string) => getRulesCacheFor(scenario).getCached(),
    subscribe: (scenario: string, fn: Listener<any[]>) => getRulesCacheFor(scenario).subscribe(fn),
    refresh: (scenario: string) => getRulesCacheFor(scenario).refresh(),
    prime: (scenario: string, data: any[]) => getRulesCacheFor(scenario).prime(data),
};
