import OAuthDetailDialog from '@/components/OAuthDetailDialog';
import ProviderFormDialog, { type EnhancedProviderFormData } from '@/components/ProviderFormDialog';
import { isCloudAuthType } from '@/components/cloud/cloudCredentialSchema';
import { api } from '@/services/api';
import type { Provider } from '@/types/provider';
import { type FormEvent, useCallback, useMemo, useState } from 'react';

type ProviderFormData = EnhancedProviderFormData;

interface OAuthEditFormData {
    name: string;
    apiBase: string;
    apiBaseOpenAI?: string;
    apiBaseAnthropic?: string;
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
    const [existingToken, setExistingToken] = useState('');
    const [apiKeyDialogOpen, setApiKeyDialogOpen] = useState(false);
    const [providerFormData, setProviderFormData] = useState<ProviderFormData>(emptyProviderFormData);
    const [oauthDetailProvider, setOAuthDetailProvider] = useState<Provider | null>(null);
    const [oauthDetailDialogOpen, setOAuthDetailDialogOpen] = useState(false);

    const handleProviderFormChange = useCallback((field: keyof ProviderFormData, value: any) => {
        setProviderFormData(prev => ({ ...prev, [field]: value }));
    }, []);

    // Edit-flow wire payload. Note: this intentionally differs from
    // useProviderDialog's buildProviderData — here empty optional URLs are
    // sent as `?? ''` (explicitly clearing them) and token/no_key_required
    // are normalized, while the add flow drops empty URLs (`|| undefined`).
    // The backend treats the two endpoints differently, so don't merge the
    // mappers without checking both.
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
        };
    }, [providerFormData]);

    const closeApiKeyDialog = useCallback(() => {
        setApiKeyDialogOpen(false);
        setProviderFormData(emptyProviderFormData);
        setExistingToken('');
    }, []);

    const closeOAuthDetailDialog = useCallback(() => {
        setOAuthDetailDialogOpen(false);
        setOAuthDetailProvider(null);
    }, []);

    const editProvider = useCallback(async (uuid: string) => {
        try {
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
                    setExistingToken(provider.token || '');
                    setProviderFormData({
                        uuid: provider.uuid,
                        name: provider.name,
                        apiBase: provider.api_base,
                        apiStyle: provider.api_style || 'openai',
                        apiBaseOpenAI: provider.api_base_openai || '',
                        apiBaseAnthropic: provider.api_base_anthropic || '',
                        // Seed the field with the stored key (password-masked,
                        // eye toggles it). Saving unchanged resends the same
                        // value — a harmless no-op for the backend.
                        token: provider.token || '',
                        enabled: provider.enabled,
                        noKeyRequired: provider.no_key_required || false,
                        proxyUrl: provider.proxy_url || '',
                        authType: provider.auth_type || 'api_key',
                    } as any);
                    setApiKeyDialogOpen(true);
                }
            } else {
                showNotification?.(`Failed to load provider details: ${result.error}`, 'error');
            }
        } catch (error) {
            showNotification?.(`Failed to load provider details: ${error instanceof Error ? error.message : 'Unknown error'}`, 'error');
        }
    }, [showNotification]);

    const handleProviderSubmit = useCallback(async (e: FormEvent, resolved?: Partial<ProviderFormData>) => {
        e.preventDefault();
        if (!providerFormData.uuid) return;

        const result = await api.updateProvider(providerFormData.uuid, buildEditProviderPayload(resolved));
        if (result.success) {
            showNotification?.('Provider updated successfully!', 'success');
            closeApiKeyDialog();
            try { await onUpdated?.(providerFormData.uuid); } catch (error) {
                showNotification?.(`Refresh failed: ${error instanceof Error ? error.message : 'Unknown error'}`, 'error');
            }
        } else {
            throw new Error(`Failed to update provider: ${result.error}`);
        }
    }, [buildEditProviderPayload, closeApiKeyDialog, onUpdated, providerFormData.uuid, showNotification]);

    const handleOAuthSubmit = useCallback(async (data: OAuthEditFormData) => {
        if (!oauthDetailProvider?.uuid) return;
        const result = await api.updateProvider(oauthDetailProvider.uuid, {
            name: data.name,
            api_base: data.apiBase,
            // OAuth credentials and API style are owned by the sign-in flow.
            // Only sent for dual providers; omitted otherwise so a single-URL
            // provider never accidentally gains dual URLs.
            ...(oauthDetailProvider.api_base_openai && oauthDetailProvider.api_base_anthropic
                ? {api_base_openai: data.apiBaseOpenAI ?? '', api_base_anthropic: data.apiBaseAnthropic ?? ''}
                : {}),
            enabled: data.enabled,
            proxy_url: data.proxyUrl ?? '',
        });
        if (!result.success) throw new Error(result.error || 'Failed to update provider');
        showNotification?.('Provider updated successfully!', 'success');
        try { await onUpdated?.(oauthDetailProvider.uuid); } catch (error) {
            showNotification?.(`Refresh failed: ${error instanceof Error ? error.message : 'Unknown error'}`, 'error');
        }
    }, [oauthDetailProvider, onUpdated, showNotification]);

    const providerEditDialogs = useMemo(() => (
        <>
            <ProviderFormDialog
                open={apiKeyDialogOpen}
                onClose={closeApiKeyDialog}
                onSubmit={handleProviderSubmit}
                data={providerFormData}
                onChange={handleProviderFormChange}
                mode="edit"
                existingToken={existingToken}
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
    ), [existingToken, apiKeyDialogOpen, closeApiKeyDialog, closeOAuthDetailDialog, handleOAuthSubmit, handleProviderFormChange, handleProviderSubmit, oauthDetailDialogOpen, oauthDetailProvider, providerFormData, showNotification]);

    return {
        editProvider,
        providerEditDialogs,
    };
}
