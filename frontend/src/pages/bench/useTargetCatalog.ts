import { useEffect, useState } from 'react';
import api from '@/services/api';
import type { Provider } from '@/types/provider';

// useTargetCatalog loads every connected provider Bench can point at.
// ModelSelectDialog owns fetching each provider's own model list once a
// provider is picked, so this stays a thin provider fetch.

export interface TargetCatalog {
    providers: Provider[];
    loading: boolean;
    error?: string;
}

export function useTargetCatalog(): TargetCatalog {
    const [providers, setProviders] = useState<Provider[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | undefined>();

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        setError(undefined);
        api.getProviders()
            .then((res: any) => {
                if (cancelled) return;
                setProviders(res?.success && Array.isArray(res.data) ? res.data : []);
            })
            .catch((e: any) => {
                if (!cancelled) setError(e?.message || 'Failed to load providers');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    return { providers, loading, error };
}
