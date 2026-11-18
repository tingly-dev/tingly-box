import { TextField } from '@mui/material';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ConfirmDialog from '@/components/ConfirmDialog';
import type { ProxyControl } from '@/components/credential/ProxySwitch';
import { isProxyUrl } from '@/components/provider-form-dialog/validation';
import { api } from '@/services/api';

interface ProxyProvider {
    uuid: string;
    name: string;
    proxy_url?: string | null;
}

interface Options<P extends ProxyProvider> {
    setProviders: (update: (previous: P[]) => P[]) => void;
    notify: (message: string, severity: 'success' | 'error') => void;
}

/**
 * Credential proxy on/off built on the ordinary provider update: off clears
 * `proxy_url`; on stores a URL the user confirms in a small dialog that is
 * prefilled with the global quick proxy. The row changes at once; a failed
 * save restores it and says why. A row with a save in flight ignores clicks.
 */
export function useProviderProxy<P extends ProxyProvider>(providers: P[], { setProviders, notify }: Options<P>) {
    const { t } = useTranslation();
    const [pending, setPending] = useState<ReadonlySet<string>>(new Set());
    const inFlight = useRef(new Set<string>());
    const providersRef = useRef(providers);
    useEffect(() => { providersRef.current = providers; });
    const [target, setTarget] = useState<{ uuid: string; name: string } | null>(null);
    const [url, setUrl] = useState('');
    const [fromGlobal, setFromGlobal] = useState(false);
    const [invalid, setInvalid] = useState(false);

    const save = useCallback(async (uuid: string, next: string): Promise<boolean> => {
        const provider = providersRef.current.find((p) => p.uuid === uuid);
        if (!provider || inFlight.current.has(uuid)) return false;
        const previous = provider.proxy_url ?? '';
        const patch = (proxy_url: string) => setProviders((list) => list.map((p) => (p.uuid === uuid ? { ...p, proxy_url } : p)));
        inFlight.current.add(uuid);
        setPending(new Set(inFlight.current));
        patch(next);
        try {
            const result = await api.updateProvider(uuid, { proxy_url: next });
            if (!result?.success) throw new Error(result?.error || t('credentialPage.unknownError'));
            notify(t(next ? 'credentialPage.proxyEnabledToast' : 'credentialPage.proxyDisabledToast', { name: provider.name }), 'success');
            return true;
        } catch (error) {
            patch(previous);
            notify(t('credentialPage.proxyToggleFailed', { name: provider.name, error: error instanceof Error ? error.message : String(error) }), 'error');
            return false;
        } finally {
            inFlight.current.delete(uuid);
            setPending(new Set(inFlight.current));
        }
    }, [notify, setProviders, t]);

    const onDisable = useCallback((uuid: string) => { void save(uuid, ''); }, [save]);

    const onEnable = useCallback((uuid: string) => {
        const provider = providersRef.current.find((p) => p.uuid === uuid);
        if (!provider || inFlight.current.has(uuid)) return;
        setTarget({ uuid, name: provider.name });
        setUrl('');
        setFromGlobal(false);
        setInvalid(false);
        void api.getConfig().then((result: any) => {
            const global: string = result?.data?.http_transport?.global_proxy_url ?? '';
            if (global) { setUrl((current) => current || global); setFromGlobal(true); }
        }).catch(() => { /* Prefill is a convenience; the user can still type a URL. */ });
    }, []);

    const confirm = async () => {
        if (!target) return;
        if (!isProxyUrl(url)) { setInvalid(true); return; }
        const done = await save(target.uuid, url.trim());
        if (done) setTarget(null);
    };

    const control: ProxyControl = { pending, onDisable, onEnable };
    const dialog = (
        <ConfirmDialog
            open={!!target}
            title={t('credentialPage.proxyEnableTitle', { name: target?.name })}
            description={
                <TextField
                    autoFocus
                    size="small"
                    fullWidth
                    margin="dense"
                    label={t('providerDialog.advanced.proxyUrl.label')}
                    placeholder={t('providerDialog.advanced.proxyUrl.placeholder')}
                    value={url}
                    error={invalid}
                    helperText={invalid ? t('credentialPage.proxyUrlInvalid') : fromGlobal ? t('credentialPage.proxyFromGlobal') : t('credentialPage.proxyEnterUrl')}
                    onChange={(e) => { setUrl(e.target.value); setInvalid(false); }}
                    onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void confirm(); } }}
                />
            }
            confirmLabel={t('credentialPage.proxyEnableConfirm')}
            cancelLabel={t('common.cancel')}
            loading={!!target && pending.has(target.uuid)}
            onClose={() => setTarget(null)}
            onConfirm={() => void confirm()}
        />
    );

    return { proxyControl: control, proxyDialog: dialog };
}
