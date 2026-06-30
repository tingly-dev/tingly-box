import ApiKeyTable from '@/components/ApiKeyTable.tsx';
import ConnectProviderDialog, { type ConnectSelection } from '@/components/ConnectProviderDialog';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import ImportModal from '@/components/ImportModal';
import OAuthDetailDialog from '@/components/OAuthDetailDialog.tsx';
import OAuthDialog from '@/components/OAuthDialog.tsx';
import OAuthTable from '@/components/OAuthTable.tsx';
import PageHeader from '@/components/PageHeader';
import { PageLayout } from '@/components/PageLayout';
import ProviderFormDialog, { type EnhancedProviderFormData } from '@/components/ProviderFormDialog.tsx';
import Surface from '@/components/Surface';
import { useProviderQuota } from '@/hooks/useProviderQuota';
import { buildProviderFormData } from '@/hooks/useProviderDialog';
import { Add, ListAlt, Upload, VpnKey } from '@/components/icons';
import {
    Alert,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Stack,
    Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { api } from '../services/api';
import { useNotify } from '@/hooks/useNotify';

type ProviderFormData = EnhancedProviderFormData;

interface OAuthEditFormData {
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic';
    enabled: boolean;
    proxyUrl?: string;
}

const CredentialPage = () => {
    const [searchParams, setSearchParams] = useSearchParams();
    const [providers, setProviders] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const notify = useNotify();

    // API Key Dialog state
    const [apiKeyDialogOpen, setApiKeyDialogOpen] = useState(false);
    const [apiKeyDialogMode, setApiKeyDialogMode] = useState<'add' | 'edit'>('add');
    const [providerFormData, setProviderFormData] = useState<ProviderFormData>({
        uuid: undefined, name: '', apiBase: '', apiStyle: undefined, token: '', enabled: true,
    });

    // Unified "Connect Provider" picker state
    const [connectOpen, setConnectOpen] = useState(false);

    const [isLocalProvider, setIsLocalProvider] = useState(false);
    const [fromConnectPicker, setFromConnectPicker] = useState(false);

    // OAuth Dialog state
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);
    const [oauthAutoStartId, setOAuthAutoStartId] = useState<string | null>(null);
    const [oauthReauthUuid, setOAuthReauthUuid] = useState<string | null>(null);
    const [refreshFailPrompt, setRefreshFailPrompt] = useState<{
        open: boolean;
        providerUuid: string;
        providerName: string;
        reason: string;
    }>({ open: false, providerUuid: '', providerName: '', reason: '' });
    const [oauthDetailProvider, setOAuthDetailProvider] = useState<any | null>(null);
    const [oauthDetailDialogOpen, setOAuthDetailDialogOpen] = useState(false);

    // Import Dialog state
    const [showImportModal, setShowImportModal] = useState(false);
    const [importing, setImporting] = useState(false);

    // URL param handling for auto-opening dialogs
    useEffect(() => {
        const dialog = searchParams.get('dialog');
        const style = searchParams.get('style') as 'openai' | 'anthropic' | null;
        if (dialog === 'add') {
            setSearchParams({});
            if (style === 'oauth') {
                setOAuthDialogOpen(true);
            } else {
                const apiStyle = style === 'openai' || style === 'anthropic' ? style : undefined;
                setApiKeyDialogMode('add');
                setProviderFormData({
                    uuid: undefined, name: '', apiBase: '', apiStyle: apiStyle, token: '',
                    enabled: true, noKeyRequired: false, proxyUrl: '', userAgent: '',
                } as any);
                setApiKeyDialogOpen(true);
            }
        }
    }, [searchParams, setSearchParams]);

    useEffect(() => { loadProviders(); }, []);

    const { quotaData, refreshing, refreshQuota } = useProviderQuota(providers, { fetchOnMount: true });

    const showNotification = (message: string, severity: 'success' | 'error') => {
        notify[severity](message);
    };

    const handleAddApiKey = () => {
        setApiKeyDialogMode('add');
        setProviderFormData({
            uuid: undefined, name: '', apiBase: '', apiStyle: undefined, token: '',
            enabled: true, noKeyRequired: false, proxyUrl: '', userAgent: '',
        } as any);
        setApiKeyDialogOpen(true);
    };

    const handleAddOAuth = () => {
        setOAuthAutoStartId(null);
        setOAuthDialogOpen(true);
    };

    const handleConnectSelect = (selection: ConnectSelection) => {
        setConnectOpen(false);

        if (selection.kind === 'oauth') {
            setOAuthAutoStartId(selection.providerId);
            setOAuthDialogOpen(true);
            return;
        }
        if (selection.kind === 'import') {
            setShowImportModal(true);
            return;
        }

        const built = buildProviderFormData(selection)!;
        setIsLocalProvider(selection.kind === 'local');
        setFromConnectPicker(true);
        setApiKeyDialogMode('add');
        setProviderFormData(built.formData);
        setApiKeyDialogOpen(true);
    };

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) { setProviders(result.data); }
        else { showNotification(`Failed to load providers: ${result.error}`, 'error'); }
        setLoading(false);
    };

    // Build the body for an add request. Always produces exactly 1 provider record.
    const buildAddProviderPayload = (override?: Partial<ProviderFormData>): any => {
        const fd: any = { ...providerFormData, ...(override || {}) };
        return {
            name: fd.name,
            api_base: fd.apiBase,
            api_style: fd.apiStyle,
            api_base_openai: fd.apiBaseOpenAI || '',
            api_base_anthropic: fd.apiBaseAnthropic || '',
            token: fd.token,
            no_key_required: fd.noKeyRequired || false,
            enabled: true,
            proxy_url: fd.proxyUrl ?? '',
            user_agent: fd.userAgent ?? '',
            auth_type: 'api_key',
        };
    };

    const buildEditProviderPayload = (override?: Partial<ProviderFormData>) => {
        const fd: any = { ...providerFormData, ...(override || {}) };
        return {
            name: fd.name,
            api_base: fd.apiBase,
            api_style: fd.apiStyle,
            token: fd.token || undefined,
            no_key_required: fd.noKeyRequired || false,
            enabled: fd.enabled,
            proxy_url: fd.proxyUrl ?? '',
            user_agent: fd.userAgent ?? '',
            api_base_openai: fd.apiBaseOpenAI ?? '',
            api_base_anthropic: fd.apiBaseAnthropic ?? '',
        };
    };

    const handleProviderSubmit = async (e: React.FormEvent, resolved?: Partial<ProviderFormData>) => {
        e.preventDefault();
        if (apiKeyDialogMode === 'edit') {
            const providerData = buildEditProviderPayload(resolved);
            const result = await api.updateProvider(providerFormData.uuid!, providerData);
            if (result.success) {
                showNotification('Provider updated successfully!', 'success');
                setApiKeyDialogOpen(false);
                loadProviders();
            } else {
                showNotification(`Failed to update provider: ${result.error}`, 'error');
            }
        } else {
            const result = await api.addProvider(buildAddProviderPayload(resolved));
            if (result.success) {
                showNotification('Provider added successfully!', 'success');
                setApiKeyDialogOpen(false);
                loadProviders();
            } else {
                showNotification(`Failed to add provider: ${result.error}`, 'error');
            }
        }
    };

    const handleProviderForceAdd = async () => {
        if (apiKeyDialogMode === 'edit') {
            const providerData = buildEditProviderPayload();
            const result = await api.updateProvider(providerFormData.uuid!, providerData);
            if (result.success) {
                showNotification('Provider updated successfully!', 'success');
                setApiKeyDialogOpen(false);
                loadProviders();
            } else {
                showNotification(`Failed to update provider: ${result.error}`, 'error');
            }
        } else {
            const result = await api.addProvider(buildAddProviderPayload(), true);
            if (result.success) {
                showNotification('Provider added successfully!', 'success');
                setApiKeyDialogOpen(false);
                loadProviders();
            } else {
                showNotification(`Failed to add provider: ${result.error}`, 'error');
            }
        }
    };

    const handleDeleteProvider = async (uuid: string) => {
        const result = await api.deleteProvider(uuid);
        if (result.success) { showNotification('Provider deleted successfully!', 'success'); loadProviders(); }
        else { showNotification(`Failed to delete provider: ${result.error}`, 'error'); }
    };

    const handleToggleProvider = async (uuid: string) => {
        const result = await api.toggleProvider(uuid);
        if (result.success) { showNotification(result.message, 'success'); loadProviders(); }
        else { showNotification(`Failed to toggle provider: ${result.error}`, 'error'); }
    };

    const handleEditProvider = async (uuid: string) => {
        const result = await api.getProvider(uuid);
        if (result.success) {
            const provider = result.data;
            if (provider.auth_type === 'oauth') {
                setOAuthDetailProvider(result.data);
                setOAuthDetailDialogOpen(true);
            } else {
                setIsLocalProvider(false);
                setApiKeyDialogMode('edit');
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
                    userAgent: (provider as any).user_agent || '',
                    authType: provider.auth_type || 'api_key',
                } as any);
                setApiKeyDialogOpen(true);
            }
        } else {
            showNotification(`Failed to load provider details: ${result.error}`, 'error');
        }
    };

    // OAuth handlers
    const handleOAuthSuccess = () => {
        showNotification(
            oauthReauthUuid ? 'Provider reauthorized successfully!' : 'OAuth provider added successfully!',
            'success',
        );
        setOAuthReauthUuid(null);
        loadProviders();
    };

    const handleReauthorize = (providerUuid: string) => {
        const provider = oauthProviders.find((p: any) => p.uuid === providerUuid);
        const issuer = provider?.oauth_detail?.provider_type || provider?.oauth_detail?.issuer;
        if (!issuer) { showNotification('Cannot reauthorize: provider issuer is unknown', 'error'); return; }
        setOAuthReauthUuid(providerUuid);
        setOAuthAutoStartId(issuer);
        setOAuthDialogOpen(true);
    };

    const promptReauthAfterRefreshFailure = (providerUuid: string, reason: string) => {
        const provider = oauthProviders.find((p: any) => p.uuid === providerUuid);
        setRefreshFailPrompt({ open: true, providerUuid, providerName: provider?.name || 'this provider', reason: reason || 'Unknown error' });
    };

    const handleRefreshToken = async (providerUuid: string) => {
        try {
            const response = await api.oauthRefresh({ provider_uuid: providerUuid });
            if (response?.success) { showNotification('Token refreshed successfully!', 'success'); await loadProviders(); }
            else { promptReauthAfterRefreshFailure(providerUuid, response?.data?.error || response?.error || response?.message || 'Unknown error'); }
        } catch (error: any) {
            promptReauthAfterRefreshFailure(providerUuid, error?.response?.data?.error || error?.message || 'Unknown error');
        }
    };

    const handleImportClick = () => { setShowImportModal(true); };

    const handleImportData = async (data: string) => {
        setImporting(true);
        try {
            const result = await api.importProvider(data);
            if (result.success) {
                const providersCreated = result.data?.providers_created || 0;
                const providersUsed = result.data?.providers_used || 0;
                let message = 'Provider import completed';
                if (providersCreated > 0) message += `. ${providersCreated} new provider${providersCreated > 1 ? 's' : ''} created`;
                if (providersUsed > 0) message += `. ${providersUsed} existing provider${providersUsed > 1 ? 's' : ''} referenced`;
                if (providersCreated === 0 && providersUsed === 0) message = 'No providers found in import data';
                showNotification(message, 'success');
                setShowImportModal(false);
                await loadProviders();
            } else { showNotification(`Import failed: ${result.error || 'Unknown error'}`, 'error'); }
        } catch (err: any) { showNotification(`Import failed: ${err?.message || 'Unknown error'}`, 'error'); }
        finally { setImporting(false); }
    };

    const handleProviderFormChange = useCallback((field: keyof ProviderFormData, value: any) => {
        setProviderFormData(prev => ({ ...prev, [field]: value }));
    }, []);

    const { apiKeyProviders, oauthProviders, credentialCounts } = useMemo(() => {
        const apiKeys = providers.filter((p: any) => p.auth_type !== 'oauth' && p.auth_type !== 'vmodel');
        const oauth = providers.filter((p: any) => p.auth_type === 'oauth');
        return { apiKeyProviders: apiKeys, oauthProviders: oauth, credentialCounts: { apiKeys: apiKeys.length, oauth: oauth.length, total: apiKeys.length + oauth.length } };
    }, [providers]);

    return (
        <PageLayout loading={loading}>
            <Stack spacing={2.5}>
                <PageHeader
                    title="Credentials"
                    subtitle={`Managing ${credentialCounts.total} credential${credentialCounts.total !== 1 ? 's' : ''}`}
                    actions={
                        <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" sx={{ justifyContent: { xs: 'flex-start', sm: 'flex-end' } }}>
                            <Button component={Link} to="/onboarding" variant="outlined" startIcon={<ListAlt />} size="small" sx={{ minWidth: 130 }}>Providers</Button>
                            <Button variant="outlined" startIcon={<Upload />} onClick={handleImportClick} size="small" sx={{ minWidth: 110 }}>Import</Button>
                            <Button variant="contained" startIcon={<Add />} onClick={() => setConnectOpen(true)} size="small" sx={{ minWidth: 150 }}>Connect AI</Button>
                        </Stack>
                    }
                />

                {/* OAuth Section */}
                <Surface padding={{ xs: 2, sm: 2.5 }}>
                    <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1.5 }}>
                        <Typography variant="subtitle1" fontWeight={500}>OAuth</Typography>
                        <Chip label={credentialCounts.oauth} size="small" color="primary" variant="outlined" sx={{ height: 20, minWidth: 20, fontSize: '0.7rem' }}/>
                    </Stack>
                    {credentialCounts.oauth > 0 ? (
                        <OAuthTable providers={oauthProviders} onEdit={handleEditProvider} onToggle={handleToggleProvider} onDelete={handleDeleteProvider} onRefreshToken={handleRefreshToken} onReauthorize={handleReauthorize} onNotification={showNotification} providerQuotas={quotaData} refreshingQuotas={refreshing} onQuotaRefresh={refreshQuota}/>
                    ) : (
                        <EmptyStateGuide title="No OAuth Providers Configured" description="Configure OAuth providers like Claude Code, Gemini CLI, Qwen, etc." showOAuthButton={false} showHeroIcon={false} primaryButtonLabel="Add OAuth Provider" onAddApiKeyClick={handleAddOAuth}/>
                    )}
                </Surface>

                {/* API Keys Section */}
                <Surface padding={{ xs: 2, sm: 2.5 }}>
                    <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1.5 }}>
                        <Typography variant="subtitle1" fontWeight={500}>API Keys</Typography>
                        <Chip label={credentialCounts.apiKeys} size="small" color="primary" variant="outlined" sx={{ height: 20, minWidth: 20, fontSize: '0.7rem' }}/>
                    </Stack>
                    {credentialCounts.apiKeys > 0 ? (
                        <ApiKeyTable providers={apiKeyProviders} onEdit={handleEditProvider} onToggle={handleToggleProvider} onDelete={handleDeleteProvider} onNotification={showNotification} providerQuotas={quotaData} refreshingQuotas={refreshing} onQuotaRefresh={refreshQuota}/>
                    ) : (
                        <EmptyStateGuide title="No API Keys Configured" description="Configure API keys to access AI services like OpenAI, Anthropic, etc." showOAuthButton={false} showHeroIcon={false} primaryButtonLabel="Connect AI" onAddApiKeyClick={handleAddApiKey}/>
                    )}
                </Surface>
            </Stack>

            {/* API Key Provider Dialog — unified, no mode flags */}
            <ProviderFormDialog
                open={apiKeyDialogOpen}
                onClose={() => { setApiKeyDialogOpen(false); setIsLocalProvider(false); setFromConnectPicker(false); }}
                onBack={fromConnectPicker ? () => setConnectOpen(true) : undefined}
                onSubmit={handleProviderSubmit}
                onForceAdd={handleProviderForceAdd}
                data={providerFormData}
                onChange={handleProviderFormChange}
                mode={apiKeyDialogMode}
                optionalEditableToken={isLocalProvider}
            />

            {/* Unified provider picker */}
            <ConnectProviderDialog open={connectOpen} onClose={() => setConnectOpen(false)} onSelect={handleConnectSelect}/>

            {/* OAuth Add Dialog */}
            <OAuthDialog open={oauthDialogOpen} autoStartProviderId={oauthAutoStartId} reauthProviderUuid={oauthReauthUuid} onClose={() => { setOAuthDialogOpen(false); setOAuthAutoStartId(null); setOAuthReauthUuid(null); }} onSuccess={handleOAuthSuccess}/>

            {/* Refresh-failed → reauthorize guidance */}
            <Dialog open={refreshFailPrompt.open} onClose={() => setRefreshFailPrompt((s) => ({ ...s, open: false }))} maxWidth="xs" fullWidth>
                <DialogTitle>Token refresh failed</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ pt: 0.5 }}>
                        <Alert severity="warning">{refreshFailPrompt.reason}</Alert>
                        <Typography variant="body2" color="text.secondary">
                            Refreshing the token for <strong>{refreshFailPrompt.providerName}</strong> didn't work. If the credential was revoked or has fully expired, a refresh can't recover it — reauthorize to sign in again. This overwrites the credential in place, keeping the same provider so your routing rules and model keys stay intact.
                        </Typography>
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button color="inherit" onClick={() => setRefreshFailPrompt((s) => ({ ...s, open: false }))}>Dismiss</Button>
                    <Button variant="contained" startIcon={<VpnKey />} onClick={() => { const uuid = refreshFailPrompt.providerUuid; setRefreshFailPrompt((s) => ({ ...s, open: false })); handleReauthorize(uuid); }}>Reauthorize</Button>
                </DialogActions>
            </Dialog>

            {/* OAuth Detail/Edit Dialog */}
            <OAuthDetailDialog open={oauthDetailDialogOpen} provider={oauthDetailProvider} onClose={() => setOAuthDetailDialogOpen(false)} onSubmit={async (data: OAuthEditFormData) => {
                if (!oauthDetailProvider?.uuid) return;
                const result = await api.updateProvider(oauthDetailProvider.uuid, { name: data.name, api_base: data.apiBase, api_style: data.apiStyle, enabled: data.enabled, proxy_url: data.proxyUrl ?? '' });
                if (!result.success) throw new Error(result.error || 'Failed to update provider');
                showNotification('Provider updated successfully!', 'success');
                loadProviders();
            }}/>

            {/* Import Modal */}
            <ImportModal open={showImportModal} onClose={() => setShowImportModal(false)} onImport={handleImportData} loading={importing}/>
        </PageLayout>
    );
};

export default CredentialPage;
