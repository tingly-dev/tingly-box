import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { proxyState } from '@/components/credential/proxyState';

interface ProxyProvider {
    uuid: string;
    name: string;
    proxy_url?: string | null;
    proxy_disabled?: boolean | null;
}

interface Options<P extends ProxyProvider> {
    setProviders: (update: (previous: P[]) => P[]) => void;
    notify: (message: string, severity: 'success' | 'error') => void;
}

/**
 * One-click proxy on/off for a credential row. The row flips at once, the
 * request follows, and a failure puts the previous value back and says why.
 * A row with a request in flight ignores further clicks (and is reported in
 * `pending` so the control can show it), so rapid clicks can't race.
 */
export function useProviderProxySwitch<P extends ProxyProvider>(providers: P[], { setProviders, notify }: Options<P>) {
    const { t } = useTranslation();
    const [pending, setPending] = useState<ReadonlySet<string>>(new Set());
    const inFlight = useRef(new Set<string>());
    const providersRef = useRef(providers);
    useEffect(() => { providersRef.current = providers; });

    const patch = useCallback((uuid: string, fields: Partial<ProxyProvider>) => {
        setProviders((previous) => previous.map((p) => (p.uuid === uuid ? { ...p, ...fields } : p)));
    }, [setProviders]);

    const toggleProxy = useCallback(async (uuid: string) => {
        const provider = providersRef.current.find((p) => p.uuid === uuid);
        if (!provider || inFlight.current.has(uuid)) return;
        const before = proxyState(provider);
        if (before === 'none') return;
        const enable = before === 'off';
        const previous = { proxy_disabled: !!provider.proxy_disabled };

        inFlight.current.add(uuid);
        setPending(new Set(inFlight.current));
        patch(uuid, { proxy_disabled: !enable });
        try {
            const result = await api.setProviderProxy(uuid, enable);
            if (!result?.success) throw new Error(result?.error || t('credentialPage.unknownError'));
            // The server's answer is the truth the other views will load too.
            patch(uuid, { proxy_url: result.data?.proxy_url ?? provider.proxy_url, proxy_disabled: !!result.data?.proxy_disabled });
            notify(t(enable ? 'credentialPage.proxyEnabledToast' : 'credentialPage.proxyDisabledToast', { name: provider.name }), 'success');
        } catch (error) {
            patch(uuid, previous);
            notify(t('credentialPage.proxyToggleFailed', { name: provider.name, error: error instanceof Error ? error.message : String(error) }), 'error');
        } finally {
            inFlight.current.delete(uuid);
            setPending(new Set(inFlight.current));
        }
    }, [notify, patch, t]);

    return { toggleProxy, proxyPending: pending };
}
