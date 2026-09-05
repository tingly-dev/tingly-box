import { useCallback, useEffect, useState } from 'react';
import api from '@/services/api';
import type { Provider } from '@/types/provider';
import type { Rule } from '@/components/RoutingGraphTypes';

// useTargetCatalog loads everything the target picker can point at: every
// rule across every scenario, every provider, and each provider's model
// list. One catalog, one search box — rule-vs-provider is a property of the
// chosen entry, never a question asked first (.design/playground.md §3).

export interface TargetCatalog {
    rules: Rule[];
    providers: Provider[];
    modelsByProvider: Record<string, string[]>;
    loading: boolean;
    error?: string;
    reload: () => void;
}

const modelsOf = (info: any): string[] => {
    const models: string[] = Array.isArray(info?.models) ? info.models : [];
    const custom: string[] = Array.isArray(info?.custom_model) ? info.custom_model : [];
    return Array.from(new Set([...models, ...custom]));
};

export function useTargetCatalog(): TargetCatalog {
    const [rules, setRules] = useState<Rule[]>([]);
    const [providers, setProviders] = useState<Provider[]>([]);
    const [modelsByProvider, setModelsByProvider] = useState<Record<string, string[]>>({});
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | undefined>();
    const [tick, setTick] = useState(0);

    const reload = useCallback(() => setTick((n) => n + 1), []);

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        setError(undefined);
        (async () => {
            try {
                const [rulesRes, providersRes] = await Promise.all([api.getAllRules(), api.getProviders()]);
                if (cancelled) return;
                const ruleList: Rule[] = rulesRes?.success && Array.isArray(rulesRes.data) ? rulesRes.data : [];
                const providerList: Provider[] = providersRes?.success && Array.isArray(providersRes.data) ? providersRes.data : [];
                setRules(ruleList);
                setProviders(providerList);
                // Models per provider, in parallel; a provider whose list fails
                // simply shows no models rather than blocking the whole catalog.
                const entries = await Promise.all(
                    providerList.map(async (p) => {
                        try {
                            const r = await api.getProviderModelsByUUID(p.uuid);
                            return [p.uuid, r?.success ? modelsOf(r.data) : []] as const;
                        } catch {
                            return [p.uuid, []] as const;
                        }
                    }),
                );
                if (cancelled) return;
                setModelsByProvider(Object.fromEntries(entries));
            } catch (e: any) {
                if (!cancelled) setError(e?.message || 'Failed to load targets');
            } finally {
                if (!cancelled) setLoading(false);
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [tick]);

    return { rules, providers, modelsByProvider, loading, error, reload };
}
