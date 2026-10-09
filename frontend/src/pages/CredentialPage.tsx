import ApiKeyTable from '@/components/ApiKeyTable.tsx';
import ConnectAIDialogs from '@/components/ConnectAIDialogs';
import EmptyState from '@/components/EmptyState';
import OAuthDialog from '@/components/OAuthDialog.tsx';
import OAuthTable from '@/components/OAuthTable.tsx';
import PageHeader from '@/components/PageHeader';
import { PageLayout } from '@/components/PageLayout';
import Surface from '@/components/Surface';
import { useProviderQuota } from '@/hooks/useProviderQuota';
import { useProviderEditDialog } from '@/hooks/useProviderEditDialog';
import { useProviderDialog } from '@/hooks/useProviderDialog';
import { Add, VpnKey } from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Divider,
    Stack,
    Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation, Trans } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import { api } from '../services/api';
import { useNotify } from '@/hooks/useNotify';
import { fontSizes } from '@/theme/fonts';

const SectionTitle = ({ label, count }: { label: string; count: number }) => (
    <Stack direction="row" spacing={1} sx={{ alignItems: "center", mb: 1.5 }}>
        <Typography variant="subtitle1" sx={{ fontWeight: 500 }}>{label}</Typography>
        <Chip label={count} size="small" color="primary" variant="outlined" sx={{ height: 20, minWidth: 20, fontSize: fontSizes.xs }}/>
    </Stack>
);

const CredentialPage = () => {
    const [searchParams, setSearchParams] = useSearchParams();
    const [providers, setProviders] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const notify = useNotify();
    const { t } = useTranslation();

    // Reauthorize dialog state (page-local: re-authenticates an existing OAuth
    // provider in place — the shared Connect AI flow only covers adding).
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);
    const [oauthAutoStartId, setOAuthAutoStartId] = useState<string | null>(null);
    const [oauthReauthUuid, setOAuthReauthUuid] = useState<string | null>(null);
    const [refreshFailPrompt, setRefreshFailPrompt] = useState<{
        open: boolean;
        providerUuid: string;
        providerName: string;
        reason: string;
    }>({ open: false, providerUuid: '', providerName: '', reason: '' });

    useEffect(() => { loadProviders(); }, []);

    const { quotaData, refreshing, refreshQuota } = useProviderQuota(providers, { fetchOnMount: true });

    const showNotification = useCallback((message: string, severity: 'success' | 'error') => {
        notify[severity](message);
    }, [notify]);

    // Standard "Connect AI" add flow: picker + every downstream dialog
    // (form / OAuth / paste / import) via the shared hook + ConnectAIDialogs.
    const connectAI = useProviderDialog(showNotification, {
        onProviderAdded: () => loadProviders(),
    });
    const { handleConnectAIClick } = connectAI;

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) { setProviders(result.data); }
        else { showNotification(t('credentialPage.loadFailed', { error: result.error }), 'error'); }
        setLoading(false);
    };

    const { editProvider: handleEditProvider, providerEditDialogs } = useProviderEditDialog({
        showNotification,
        onUpdated: loadProviders,
    });

    const handleDeleteProvider = async (uuid: string) => {
        const result = await api.deleteProvider(uuid);
        if (result.success) { showNotification(t('credentialPage.deleted'), 'success'); loadProviders(); }
        else { showNotification(t('credentialPage.deleteFailed', { error: result.error }), 'error'); }
    };

    const handleToggleProvider = async (uuid: string) => {
        const result = await api.toggleProvider(uuid);
        if (result.success) { showNotification(result.message, 'success'); loadProviders(); }
        else { showNotification(t('credentialPage.toggleFailed', { error: result.error }), 'error'); }
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

    // Reauthorize handlers (add-flow OAuth success is handled by the shared hook)
    const handleReauthSuccess = () => {
        showNotification(t('credentialPage.reauthorized'), 'success');
        setOAuthReauthUuid(null);
        loadProviders();
    };

    const handleReauthorize = (providerUuid: string) => {
        const provider = oauthProviders.find((p: any) => p.uuid === providerUuid);
        const issuer = provider?.oauth_detail?.provider_type || provider?.oauth_detail?.issuer;
        if (!issuer) { showNotification(t('credentialPage.reauthorizeUnknownIssuer'), 'error'); return; }
        setOAuthReauthUuid(providerUuid);
        setOAuthAutoStartId(issuer);
        setOAuthDialogOpen(true);
    };

    const promptReauthAfterRefreshFailure = (providerUuid: string, reason: string) => {
        const provider = oauthProviders.find((p: any) => p.uuid === providerUuid);
        setRefreshFailPrompt({ open: true, providerUuid, providerName: provider?.name || t('credentialPage.fallbackProviderName'), reason: reason || t('credentialPage.unknownError') });
    };

    const handleRefreshToken = async (providerUuid: string) => {
        try {
            const response = await api.oauthRefresh({ provider_uuid: providerUuid });
            if (response?.success) { showNotification(t('credentialPage.tokenRefreshed'), 'success'); await loadProviders(); }
            else { promptReauthAfterRefreshFailure(providerUuid, response?.data?.error || response?.error || response?.message || t('credentialPage.unknownError')); }
        } catch (error: any) {
            promptReauthAfterRefreshFailure(providerUuid, error?.response?.data?.error || error?.message || t('credentialPage.unknownError'));
        }
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
                    title={t('layout.credentials')}
                    subtitle={credentialCounts.total === 0
                        ? t('credentialPage.empty')
                        : t('credentialPage.managing', { count: credentialCounts.total })}
                    // Empty: the landing below carries Connect AI — one CTA, not two.
                    // The provider catalog is reached from inside Connect AI.
                    actions={credentialCounts.total === 0 ? undefined : (
                        <Button variant="contained" startIcon={<Add />} onClick={handleConnectAIClick} size="small" sx={{ minWidth: 150 }}>{t('templateActions.connectAI')}</Button>
                    )}
                />

                {/* Both credential kinds share one card so the page reads as a
                    single "Credentials" surface, not two unrelated panels — but
                    each keeps its own table, since OAuth and API key credentials
                    show different columns (issuer/expiry vs. base URL/key) that
                    don't collapse into shared columns without losing detail.

                    Only kinds that exist get a section. An empty kind used to
                    render its own "No … Configured" block with its own Connect
                    AI button, so a new user met the page split in two with three
                    identical buttons, and a user with only API keys kept an
                    empty OAuth block on top. The header's Connect AI adds either
                    kind; with nothing at all, one landing replaces both. */}
                <Surface padding={{ xs: 2, sm: 2.5 }}>
                    {credentialCounts.total === 0 ? (
                        <EmptyState
                            icon={<VpnKey />}
                            title={t('credentialPage.emptyTitle')}
                            description={t('credentialPage.emptyDescription')}
                            primaryAction={{ label: t('templateActions.connectAI'), icon: <Add />, onClick: handleConnectAIClick }}
                        />
                    ) : (
                        <Stack spacing={3} divider={<Divider />}>
                            {credentialCounts.oauth > 0 && (
                                <Box>
                                    <SectionTitle label={t('credentialPage.sectionOAuth')} count={credentialCounts.oauth}/>
                                    <OAuthTable providers={oauthProviders} onEdit={handleEditProvider} onToggle={handleToggleProvider} onDelete={handleDeleteProvider} onRefreshToken={handleRefreshToken} onReauthorize={handleReauthorize} onNotification={showNotification} providerQuotas={quotaData} refreshingQuotas={refreshing} onQuotaRefresh={refreshQuota}/>
                                </Box>
                            )}
                            {credentialCounts.apiKeys > 0 && (
                                <Box>
                                    <SectionTitle label={t('credentialPage.sectionApiKeys')} count={credentialCounts.apiKeys}/>
                                    <ApiKeyTable providers={apiKeyProviders} onEdit={handleEditProvider} onToggle={handleToggleProvider} onDelete={handleDeleteProvider} onNotification={showNotification} providerQuotas={quotaData} refreshingQuotas={refreshing} onQuotaRefresh={refreshQuota}/>
                                </Box>
                            )}
                        </Stack>
                    )}
                </Surface>
            </Stack>
            {/* Unified Connect AI add flow: picker + form/OAuth/paste/import dialogs
                (edit goes through useProviderEditDialog) */}
            <ConnectAIDialogs flow={connectAI}/>
            {/* Reauthorize dialog (existing OAuth provider, in place) */}
            <OAuthDialog open={oauthDialogOpen} autoStartProviderId={oauthAutoStartId} reauthProviderUuid={oauthReauthUuid} onClose={() => { setOAuthDialogOpen(false); setOAuthAutoStartId(null); setOAuthReauthUuid(null); }} onSuccess={handleReauthSuccess}/>
            {/* Refresh-failed → reauthorize guidance */}
            <Dialog open={refreshFailPrompt.open} onClose={() => setRefreshFailPrompt((s) => ({ ...s, open: false }))} maxWidth="xs" fullWidth>
                <DialogTitle>{t('credentialPage.refreshFailedTitle')}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ pt: 0.5 }}>
                        <Alert severity="warning">{refreshFailPrompt.reason}</Alert>
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>
                            <Trans i18nKey="credentialPage.refreshFailedBody" values={{ name: refreshFailPrompt.providerName }} components={{ 1: <strong /> }} />
                        </Typography>
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button color="inherit" onClick={() => setRefreshFailPrompt((s) => ({ ...s, open: false }))}>{t('credentialPage.dismiss')}</Button>
                    <Button variant="contained" startIcon={<VpnKey />} onClick={() => { const uuid = refreshFailPrompt.providerUuid; setRefreshFailPrompt((s) => ({ ...s, open: false })); handleReauthorize(uuid); }}>{t('credentialPage.reauthorize')}</Button>
                </DialogActions>
            </Dialog>
            {providerEditDialogs}
        </PageLayout>
    );
};

export default CredentialPage;
