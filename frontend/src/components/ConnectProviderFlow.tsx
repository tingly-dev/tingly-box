import ConnectProviderDialog, { type ConnectSelection } from '@/components/ConnectProviderDialog';
import OAuthDialog from '@/components/OAuthDialog';
import ProviderFormDialog, { type EnhancedProviderFormData } from '@/components/ProviderFormDialog';
import { buildProviderFormData } from '@/hooks/useProviderDialog';
import { useState, useCallback } from 'react';
import { api } from '@/services/api';

interface ConnectProviderFlowProps {
    open: boolean;
    onClose: () => void;
    onProviderAdded?: () => void;
    showNotification?: (message: string, severity: 'success' | 'error') => void;
}

const ConnectProviderFlow: React.FC<ConnectProviderFlowProps> = ({
    open,
    onClose,
    onProviderAdded,
    showNotification,
}) => {
    const [apiKeyDialogOpen, setApiKeyDialogOpen] = useState(false);
    const [apiKeyDialogMode] = useState<'add'>('add');
    const [optionalEditableToken, setOptionalEditableToken] = useState(false);
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);
    const [oauthAutoStartId, setOAuthAutoStartId] = useState<string | null>(null);
    const [providerFormData, setProviderFormData] = useState<EnhancedProviderFormData>({
        name: '', apiBase: '', apiStyle: undefined, token: '', enabled: true, noKeyRequired: false, proxyUrl: '',
    });

    const handleConnectSelect = useCallback((selection: ConnectSelection) => {
        onClose();

        // oauth / import are handled separately
        if (selection.kind === 'oauth') {
            setOAuthAutoStartId(selection.providerId);
            setOAuthDialogOpen(true);
            return;
        }
        if (selection.kind === 'import') return;

        const built = buildProviderFormData(selection)!;
        console.log('[ConnectProviderFlow] screen1 → screen2:', {
            kind: selection.kind,
            selectedProviderId: built.formData.selectedProviderId,
        });
        setOptionalEditableToken(built.optionalEditableToken);
        setProviderFormData(built.formData);
        setApiKeyDialogOpen(true);
    }, [onClose]);

    const handleProviderSubmit = async (_e: React.FormEvent, resolved?: Partial<EnhancedProviderFormData>) => {
        const fd = { ...providerFormData, ...(resolved || {}) };
        const result = await api.addProvider({
            name: fd.name, api_base: fd.apiBase, api_style: fd.apiStyle,
            api_base_openai: fd.apiBaseOpenAI || undefined,
            api_base_anthropic: fd.apiBaseAnthropic || undefined,
            token: fd.token, no_key_required: fd.noKeyRequired, proxy_url: fd.proxyUrl,
        });
        if (result.success) {
            showNotification?.('Provider added successfully!', 'success');
            setApiKeyDialogOpen(false);
            onProviderAdded?.();
        } else {
            showNotification?.(`Failed to add provider: ${result.error}`, 'error');
        }
    };

    const handleProviderForceAdd = async () => {
        const result = await api.addProvider({
            name: providerFormData.name, api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            api_base_openai: providerFormData.apiBaseOpenAI || undefined,
            api_base_anthropic: providerFormData.apiBaseAnthropic || undefined,
            token: providerFormData.token, no_key_required: providerFormData.noKeyRequired,
            proxy_url: providerFormData.proxyUrl,
        }, true);
        if (result.success) {
            showNotification?.('Provider added successfully!', 'success');
            setApiKeyDialogOpen(false);
            onProviderAdded?.();
        } else {
            showNotification?.(`Failed to add provider: ${result.error}`, 'error');
        }
    };

    const handleFieldChange = (field: keyof EnhancedProviderFormData, value: any) => {
        setProviderFormData(prev => ({ ...prev, [field]: value }));
    };

    return (
        <>
            <ConnectProviderDialog
                open={open}
                onClose={onClose}
                onSelect={handleConnectSelect}
            />
            <ProviderFormDialog
                open={apiKeyDialogOpen}
                onClose={() => { setApiKeyDialogOpen(false); setOptionalEditableToken(false); }}
                onBack={() => { setApiKeyDialogOpen(false); onClose(); }}
                onSubmit={handleProviderSubmit}
                onForceAdd={handleProviderForceAdd}
                data={providerFormData}
                onChange={handleFieldChange}
                mode={apiKeyDialogMode}
                optionalEditableToken={optionalEditableToken}
            />
            <OAuthDialog
                open={oauthDialogOpen}
                autoStartProviderId={oauthAutoStartId}
                onClose={() => { setOAuthDialogOpen(false); setOAuthAutoStartId(null); }}
                onSuccess={() => {
                    setOAuthDialogOpen(false);
                    setOAuthAutoStartId(null);
                    showNotification?.('Provider connected via OAuth!', 'success');
                    onProviderAdded?.();
                }}
            />
        </>
    );
};

export default ConnectProviderFlow;
