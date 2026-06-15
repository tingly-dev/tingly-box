import ConnectProviderDialog, { type ConnectSelection } from '@/components/ConnectProviderDialog';
import OAuthDialog from '@/components/OAuthDialog';
import ProviderFormDialog, { type EnhancedProviderFormData } from '@/components/ProviderFormDialog';
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
    const [isLocalProvider, setIsLocalProvider] = useState(false);
    const [isDualMode, setIsDualMode] = useState(false);
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);
    const [oauthAutoStartId, setOAuthAutoStartId] = useState<string | null>(null);
    const [providerFormData, setProviderFormData] = useState<EnhancedProviderFormData>({
        name: '', apiBase: '', apiStyle: undefined, token: '', enabled: true, noKeyRequired: false, proxyUrl: '',
    });

    const handleConnectSelect = useCallback((selection: ConnectSelection) => {
        onClose();
        setIsDualMode(false);
        if (selection.kind === 'oauth') {
            setOAuthAutoStartId(selection.providerId);
            setOAuthDialogOpen(true);
            return;
        }
        if (selection.kind === 'import') {
            return;
        }
        if (selection.kind === 'custom') {
            setIsLocalProvider(false);
            setProviderFormData({
                name: '', apiBase: '', apiStyle: undefined, token: '', enabled: true, noKeyRequired: false, proxyUrl: '',
            });
            setApiKeyDialogOpen(true);
            return;
        }
        if (selection.kind === 'dual') {
            // Dual endpoint: two URLs (OpenAI + Anthropic) under one key, always
            // saved as a single fused record. No protocol selector / topology toggle.
            setIsLocalProvider(false);
            setIsDualMode(true);
            setProviderFormData({
                name: '', apiBase: '', apiStyle: 'openai', token: '', enabled: true, noKeyRequired: false, proxyUrl: '',
                apiBaseOpenAI: '', apiBaseAnthropic: '', createDualProvider: true,
                protocols: ['openai', 'anthropic'],
            } as any);
            setApiKeyDialogOpen(true);
            return;
        }
        if (selection.kind === 'local') {
            const lp = selection.provider;
            setIsLocalProvider(true);
            setProviderFormData({
                name: lp.name, apiBase: lp.baseUrlOpenAI || lp.baseUrlAnthropic || '', apiStyle: 'openai', token: '',
                enabled: true, noKeyRequired: true,
            } as any);
            setApiKeyDialogOpen(true);
            return;
        }
        const p = selection.provider;
        setIsLocalProvider(false);
        setProviderFormData({
            uuid: undefined, name: p.alias || p.name,
            apiBase: p.baseUrlOpenAI || p.baseUrlAnthropic || '',
            apiStyle: undefined, token: '', enabled: true, noKeyRequired: false,
            proxyUrl: '', userAgent: '', createDualProvider: false,
            providerBaseUrls: { openai: p.baseUrlOpenAI, anthropic: p.baseUrlAnthropic },
        } as any);
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
                onClose={() => { setApiKeyDialogOpen(false); setIsLocalProvider(false); setIsDualMode(false); }}
                onBack={() => { setApiKeyDialogOpen(false); onClose(); /* re-open handled by parent */ }}
                onSubmit={handleProviderSubmit}
                onForceAdd={handleProviderForceAdd}
                data={providerFormData}
                onChange={handleFieldChange}
                mode={apiKeyDialogMode}
                optionalEditableToken={isLocalProvider}
                dualMode={isDualMode}
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
