import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { fetchUIAPI } from '@/services/api';
import { usePageVisibility } from '@/hooks/usePageVisibility';
import type { ProviderQuota } from '@/types/quota';

type QuotaByProvider = Record<string, ProviderQuota>;

interface ProviderQuotaContextValue {
    quota: QuotaByProvider;
    refreshing: Set<string>;
    failed: Set<string>;
    refresh: (providerUuid: string) => Promise<void>;
}

const ProviderQuotaContext = createContext<ProviderQuotaContextValue | undefined>(undefined);

// Re-read when the tab comes back after this long. The endpoint only reads the
// backend cache (a background refresher keeps it current), so this is cheap.
const STALE_AFTER_MS = 60_000;

function without(set: Set<string>, uuid: string): Set<string> {
    if (!set.has(uuid)) return set;
    const next = new Set(set);
    next.delete(uuid);
    return next;
}

/**
 * Loads cached quota for a page's providers with one batch request, so every
 * graph node under it can show its provider's quota without fetching on its
 * own. Failures stay quiet: a node's quota figure is a glance, not a feature
 * worth a toast — the Credentials page is where quota errors are surfaced.
 */
export function ProviderQuotaProvider({ providerUuids, children }: {
    providerUuids: string[];
    children: React.ReactNode;
}) {
    const [quota, setQuota] = useState<QuotaByProvider>({});
    const [refreshing, setRefreshing] = useState<Set<string>>(new Set());
    const [failed, setFailed] = useState<Set<string>>(new Set());
    const key = [...providerUuids].sort().join(',');

    const load = useCallback(async () => {
        if (!key) return;
        try {
            const response = await fetchUIAPI('/provider-quota/batch', {
                method: 'POST',
                body: JSON.stringify({ provider_uuids: key.split(',') }),
            });
            setQuota(response?.data ?? {});
        } catch (error) {
            console.debug('[ProviderQuotaProvider] quota unavailable:', error);
        }
    }, [key]);

    useEffect(() => {
        load();
    }, [load]);
    usePageVisibility(load, STALE_AFTER_MS);

    // Unlike load, this asks the upstream provider, so a click while one is in
    // flight is ignored rather than stacked. A failure keeps the old snapshot
    // and is only reported in the node's tooltip.
    const inFlight = useRef<Set<string>>(new Set());
    const refresh = useCallback(async (providerUuid: string) => {
        if (inFlight.current.has(providerUuid)) return;
        inFlight.current.add(providerUuid);
        setRefreshing(r => new Set(r).add(providerUuid));
        try {
            const usage = await fetchUIAPI(`/provider-quota/${providerUuid}/refresh`, { method: 'POST' });
            // A refused upstream still answers 200, as a record carrying
            // last_error and no windows; replacing the old reading with it
            // would make the ring vanish instead of saying the refresh failed.
            if (!usage?.provider_uuid || (usage.last_error && !usage.windows?.length)) {
                setFailed(f => new Set(f).add(providerUuid));
                return;
            }
            setQuota(q => ({ ...q, [providerUuid]: usage }));
            setFailed(f => without(f, providerUuid));
        } catch (error) {
            console.debug('[ProviderQuotaProvider] refresh failed:', error);
            setFailed(f => new Set(f).add(providerUuid));
        } finally {
            inFlight.current.delete(providerUuid);
            setRefreshing(r => without(r, providerUuid));
        }
    }, []);

    const value = useMemo(() => ({ quota, refreshing, failed, refresh }), [quota, refreshing, failed, refresh]);
    return <ProviderQuotaContext.Provider value={value}>{children}</ProviderQuotaContext.Provider>;
}

/** Quota state for one provider; quota is undefined outside a ProviderQuotaProvider. */
export function useProviderQuotaOf(providerUuid?: string) {
    const ctx = useContext(ProviderQuotaContext);
    return {
        quota: providerUuid ? ctx?.quota[providerUuid] : undefined,
        refreshing: !!providerUuid && !!ctx?.refreshing.has(providerUuid),
        failed: !!providerUuid && !!ctx?.failed.has(providerUuid),
        refresh: ctx && providerUuid ? () => ctx.refresh(providerUuid) : undefined,
    };
}
