import React, { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { fetchUIAPI } from '@/services/api';
import { usePageVisibility } from '@/hooks/usePageVisibility';
import type { ProviderQuota } from '@/types/quota';

type QuotaByProvider = Record<string, ProviderQuota>;

const ProviderQuotaContext = createContext<QuotaByProvider | undefined>(undefined);

// Re-read when the tab comes back after this long. The endpoint only reads the
// backend cache (a background refresher keeps it current), so this is cheap.
const STALE_AFTER_MS = 60_000;

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

    return <ProviderQuotaContext.Provider value={quota}>{children}</ProviderQuotaContext.Provider>;
}

/** Cached quota for one provider; undefined outside a ProviderQuotaProvider. */
export function useProviderQuotaOf(providerUuid?: string): ProviderQuota | undefined {
    const quota = useContext(ProviderQuotaContext);
    return providerUuid ? quota?.[providerUuid] : undefined;
}
