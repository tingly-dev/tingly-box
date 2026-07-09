import ApiKeyTable from '@/components/ApiKeyTable.tsx';
import ConnectProviderDialog from '@/components/ConnectProviderDialog';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import ImportModal from '@/components/ImportModal';
import OAuthDialog from '@/components/OAuthDialog.tsx';
import OAuthTable from '@/components/OAuthTable.tsx';
import PageHeader from '@/components/PageHeader';
import { PageLayout } from '@/components/PageLayout';
import ProviderFormDialog from '@/components/ProviderFormDialog.tsx';
import Surface from '@/components/Surface';
import { useProviderQuota } from '@/hooks/useProviderQuota';
import { useProviderEditDialog } from '@/hooks/useProviderEditDialog';
import { useProviderDialog } from '@/hooks/useProviderDialog';
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

const CredentialPage = () => {
    const [searchParams, setSearchParams] = useSearchParams();
    const [providers, setProviders] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const notify = useNotify();

    // OAuth Dialog state (page-local: needs reauth/refresh handling the shared
    // add-form hook doesn't cover)
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);
    const [oauthAutoStartId, setOAuthAutoStartId] = useState<string | null>(null);
    const [oauthReauthUuid, setOAuthReauthUuid] = useState<string | null>(null);
    const [refreshFailPrompt, setRefreshFailPrompt] = useState<{
        open: boolean;
        providerUuid: string;
        providerName: string;
        reason: string;
    }>({ open: false, providerUuid: '', providerName: '', reason: '' });

    // Import Dialog state
    const [showImportModal, setShowImportModal] = useState(false);
    const [importing, setImporting] = useState(false);

    useEffect(() => { loadProviders(); }, []);

    const { quotaData, refreshing, refreshQuota } = useProviderQuota(providers, { fetchOnMount: true });

    const showNotification = useCallback((message: string, severity: 'success' | 'error') => {
        notify[severity](message);
    }, [notify]);

    // Standard "Connect AI" add flow: picker + form (add/force-add/import/oauth routing).
    const {
        providerDialogOpen,
        providerFormData,
        handleProviderSubmit,
        handleProviderForceAdd,
        handleCloseDialog,
        handleFieldChange,
        connectDialogOpen,
        handleConnectAIClick,
        handleConnectSelect,
        handleCloseConnect,
        fromConnectPicker,
        optionalEditableToken,
    } = useProviderDialog(showNotification, {
        onProviderAdded: () => loadProviders(),
        onImport: () => setShowImportModal(true),
        onOAuth: (providerId) => {
            setOAuthAutoStartId(providerId);
            setOAuthDialogOpen(true);
        },
    });

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) { setProviders(result.data); }
        else { showNotification(`Failed to load providers: ${result.error}`, 'error'); }
        setLoading(false);
    };

    const { editProvider: handleEditProvider, providerEditDialogs } = useProviderEditDialog({
        showNotification,
        onUpdated: loadProviders,
    });

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


    // URL param handling for auto-opening dialogs
    useEffect(() => {
        const editProvider = searchParams.get('editProvider');
        if (editProvider) {
            const nextParams = new URLSearchParams(searchParams);
            nextParams.delete('editProvider');
            setSearchParams(nextParams, { replace: true });
            handleEditProvider(editProvider);
            return;
        }

        const dialog = searchParams.get('dialog');
        if (dialog === 'add') {
            const nextParams = new URLSearchParams(searchParams);
            nextParams.delete('dialog');
            setSearchParams(nextParams, { replace: true });
            // All "add credential" entry points funnel through the unified Connect AI picker.
            handleConnectAIClick();
        }
    }, [searchParams, setSearchParams, handleConnectAIClick]);

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
                            <Button variant="contained" startIcon={<Add />} onClick={handleConnectAIClick} size="small" sx={{ minWidth: 150 }}>Connect AI</Button>
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
                        <EmptyStateGuide title="No OAuth Providers Configured" description="Connect AI providers like Claude Code, Gemini CLI, Qwen, etc. via OAuth sign-in." showHeroIcon={false} primaryButtonLabel="Connect AI" onAddApiKeyClick={handleConnectAIClick}/>
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
                        <EmptyStateGuide title="No API Keys Configured" description="Connect AI providers like OpenAI, Anthropic, etc. via API key." showHeroIcon={false} primaryButtonLabel="Connect AI" onAddApiKeyClick={handleConnectAIClick}/>
                    )}
                </Surface>
            </Stack>

            {/* API Key Provider Dialog — unified add flow (edit goes through useProviderEditDialog) */}
            <ProviderFormDialog
                open={providerDialogOpen}
                onClose={handleCloseDialog}
                onBack={fromConnectPicker ? () => { handleCloseDialog(); handleConnectAIClick(); } : undefined}
                onSubmit={handleProviderSubmit}
                onForceAdd={handleProviderForceAdd}
                data={providerFormData}
                onChange={handleFieldChange}
                mode="add"
                optionalEditableToken={optionalEditableToken}
            />

            {/* Unified provider picker */}
            <ConnectProviderDialog open={connectDialogOpen} onClose={handleCloseConnect} onSelect={handleConnectSelect}/>

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

            {providerEditDialogs}

            {/* Import Modal */}
            <ImportModal open={showImportModal} onClose={() => setShowImportModal(false)} onImport={handleImportData} loading={importing}/>
        </PageLayout>
    );
};

export default CredentialPage;
