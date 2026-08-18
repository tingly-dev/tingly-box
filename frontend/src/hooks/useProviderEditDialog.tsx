import OAuthDetailDialog from '@/components/OAuthDetailDialog';
import ProviderFormDialog, { type EnhancedProviderFormData } from '@/components/ProviderFormDialog';
import { isCloudAuthType } from '@/components/cloud/cloudCredentialSchema';
import { api } from '@/services/api';
import { apiToFlags, flagsToApi } from '@/components/rule-card/flagHelpers';
import type { Provider } from '@/types/provider';
import { type FormEvent, useCallback, useMemo, useState } from 'react';

type ProviderFormData = EnhancedProviderFormData;

interface OAuthEditFormData {
    name: string;
    apiBase: string;
    apiStyle: string;
    enabled: boolean;
    proxyUrl?: string;
}

interface UseProviderEditDialogOptions {
    onUpdated?: (providerUuid: string) => void | Promise<void>;
    showNotification?: (message: string, severity: 'success' | 'error') => void;
}

const emptyProviderFormData: ProviderFormData = {
    uuid: undefined,
    name: '',
    apiBase: '',
    apiStyle: undefined,
    token: '',
    enabled: true,
};

export function useProviderEditDialog({ onUpdated, showNotification }: UseProviderEditDialogOptions = {}) {
    const [apiKeyDialogOpen, setApiKeyDialogOpen] = useState(false);
    const [providerFormData, setProviderFormData] = useState<ProviderFormData>(emptyProviderFormData);
    const [oauthDetailProvider, setOAuthDetailProvider] = useState<Provider | null>(null);
    const [oauthDetailDialogOpen, setOAuthDetailDialogOpen] = useState(false);

    const handleProviderFormChange = useCallback((field: keyof ProviderFormData, value: any) => {
        setProviderFormData(prev => ({ ...prev, [field]: value }));
    }, []);

    const buildEditProviderPayload = useCallback((override?: Partial<ProviderFormData>) => {
        const fd: any = { ...providerFormData, ...(override || {}) };
        return {
            name: fd.name,
            api_base: fd.apiBase,
            api_style: fd.apiStyle,
            token: fd.token || undefined,
            no_key_required: fd.noKeyRequired || false,
            enabled: fd.enabled,
            proxy_url: fd.proxyUrl ?? '',
            api_base_openai: fd.apiBaseOpenAI ?? '',
            api_base_anthropic: fd.apiBaseAnthropic ?? '',
            // Whole-object replace, mirroring the form's edit surface: the
            // dialog seeded the stored flags, so an empty object here means
            // the user cleared them (backend: non-null replaces wholesale).
            flags: flagsToApi(fd.flags),
        };
    }, [providerFormData]);

    const closeApiKeyDialog = useCallback(() => {
        setApiKeyDialogOpen(false);
        setProviderFormData(emptyProviderFormData);
    }, []);

    const closeOAuthDetailDialog = useCallback(() => {
        setOAuthDetailDialogOpen(false);
        setOAuthDetailProvider(null);
    }, []);

    const editProvider = useCallback(async (uuid: string) => {
        const result = await api.getProvider(uuid);
        if (result.success) {
            const provider = result.data as Provider;
            if (provider.auth_type === 'oauth') {
                setOAuthDetailProvider(provider);
                setOAuthDetailDialogOpen(true);
            } else if (isCloudAuthType(provider.auth_type)) {
                // Cloud providers (Bedrock/Vertex/Azure) hold multi-field
                // credentials the token-based form can neither show nor edit —
                // opening it would present the wrong fields and risk a bad
                // update. A dedicated cloud edit flow is a declared follow-up
                // (.design/third-party-credentials.md §11); until then, be
                // honest instead of opening a broken form.
                showNotification?.(
                    'Editing cloud credentials is not supported yet — delete the provider and re-add it to change them.',
                    'error'
                );
            } else {
                setProviderFormData({
                    uuid: provider.uuid,
                    name: provider.name,
                    apiBase: provider.api_base,
                    apiStyle: provider.api_style || 'openai',
                    apiBaseOpenAI: provider.api_base_openai || '',
                    apiBaseAnthropic: provider.api_base_anthropic || '',
                    token: provider.token || '',
                    enabled: provider.enabled,
                    noKeyRequired: provider.no_key_required || false,
                    proxyUrl: provider.proxy_url || '',
                    authType: provider.auth_type || 'api_key',
                    flags: apiToFlags(provider.flags),
                } as any);
                setApiKeyDialogOpen(true);
            }
        } else {
            showNotification?.(`Failed to load provider details: ${result.error}`, 'error');
        }
    }, [showNotification]);

    const handleProviderSubmit = useCallback(async (e: FormEvent, resolved?: Partial<ProviderFormData>) => {
        e.preventDefault();
        if (!providerFormData.uuid) return;

        const result = await api.updateProvider(providerFormData.uuid, buildEditProviderPayload(resolved));
        if (result.success) {
            showNotification?.('Provider updated successfully!', 'success');
            closeApiKeyDialog();
            await onUpdated?.(providerFormData.uuid);
        } else {
            showNotification?.(`Failed to update provider: ${result.error}`, 'error');
        }
    }, [buildEditProviderPayload, closeApiKeyDialog, onUpdated, providerFormData.uuid, showNotification]);

    const handleOAuthSubmit = useCallback(async (data: OAuthEditFormData) => {
        if (!oauthDetailProvider?.uuid) return;
        const result = await api.updateProvider(oauthDetailProvider.uuid, {
            name: data.name,
            api_base: data.apiBase,
            api_style: data.apiStyle,
            enabled: data.enabled,
            proxy_url: data.proxyUrl ?? '',
        });
        if (!result.success) throw new Error(result.error || 'Failed to update provider');
        showNotification?.('Provider updated successfully!', 'success');
        await onUpdated?.(oauthDetailProvider.uuid);
    }, [oauthDetailProvider?.uuid, onUpdated, showNotification]);

    const providerEditDialogs = useMemo(() => (
        <>
            <ProviderFormDialog
                open={apiKeyDialogOpen}
                onClose={closeApiKeyDialog}
                onSubmit={handleProviderSubmit}
                data={providerFormData}
                onChange={handleProviderFormChange}
                mode="edit"
                onNotification={showNotification}
            />
            <OAuthDetailDialog
                open={oauthDetailDialogOpen}
                provider={oauthDetailProvider}
                onClose={closeOAuthDetailDialog}
                onSubmit={handleOAuthSubmit}
                onNotification={showNotification}
            />
        </>
    ), [apiKeyDialogOpen, closeApiKeyDialog, closeOAuthDetailDialog, handleOAuthSubmit, handleProviderFormChange, handleProviderSubmit, oauthDetailDialogOpen, oauthDetailProvider, providerFormData, showNotification]);

    return {
        editProvider,
        providerEditDialogs,
    };
}
