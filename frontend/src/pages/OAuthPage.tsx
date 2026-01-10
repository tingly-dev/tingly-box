import { VpnKey } from '@mui/icons-material';
import { Alert, Box, Button, Snackbar, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { PageLayout } from '../components/PageLayout';
import OAuthDialog from '../components/OAuthDialog.tsx';
import OAuthDetailDialog from '../components/OAuthDetailDialog.tsx';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';
import OAuthTable from '../components/OAuthTable.tsx';

interface OAuthEditFormData {
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic';
    enabled: boolean;
}

const OAuthPage = () => {
    const [providers, setProviders] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error';
    }>({ open: false, message: '', severity: 'success' });

    // OAuth Dialog state
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);
    const [oauthDetailProvider, setOAuthDetailProvider] = useState<any | null>(null);
    const [oauthDetailDialogOpen, setOAuthDetailDialogOpen] = useState(false);

    useEffect(() => {
        loadProviders();
    }, []);

    const showNotification = (message: string, severity: 'success' | 'error') => {
        setSnackbar({ open: true, message, severity });
    };

    const handleAddOAuthClick = () => {
        setOAuthDialogOpen(true);
    };

    const handleOAuthSuccess = () => {
        showNotification('OAuth provider added successfully!', 'success');
        loadProviders();
    };

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) {
            // Filter only OAuth providers
            setProviders(result.data.filter((p: any) => p.auth_type === 'oauth'));
        } else {
            showNotification(`Failed to load providers: ${result.error}`, 'error');
        }
        setLoading(false);
    };

    const handleDeleteProvider = async (uuid: string) => {
        const result = await api.deleteProvider(uuid);

        if (result.success) {
            showNotification('Provider deleted successfully!', 'success');
            loadProviders();
        } else {
            showNotification(`Failed to delete provider: ${result.error}`, 'error');
        }
    };

    const handleToggleProvider = async (uuid: string) => {
        const result = await api.toggleProvider(uuid);

        if (result.success) {
            showNotification(result.message, 'success');
            loadProviders();
        } else {
            showNotification(`Failed to toggle provider: ${result.error}`, 'error');
        }
    };

    const handleEditProvider = async (uuid: string) => {
        const result = await api.getProvider(uuid);

        if (result.success) {
            setOAuthDetailProvider(result.data);
            setOAuthDetailDialogOpen(true);
        } else {
            showNotification(`Failed to load provider details: ${result.error}`, 'error');
        }
    };

    // Reauthorize - start new OAuth flow (not implemented in backend, placeholder for now)
    const handleReauthorizeOAuth = async (_uuid: string) => {
        // For now, this could open the OAuth dialog with pre-filled provider info
        // or could be removed if reauthorization is done through "Add OAuth" flow
        showNotification('Reauthorization flow - use "Add OAuth" to reauthorize', 'success');
    };

    // Refresh token - use refresh_token to get new access token
    const handleRefreshToken = async (providerUuid: string) => {
        try {
            const { oauthApi } = await api.instances();
            const response = await oauthApi.apiV1OauthRefreshPost({ provider_uuid: providerUuid });

            if (response.data.success) {
                showNotification('Token refreshed successfully!', 'success');
                await loadProviders();
            } else {
                showNotification(`Failed to refresh token: ${response.data.message || 'Unknown error'}`, 'error');
            }
        } catch (error: any) {
            const errorMessage = error?.response?.data?.error || error?.message || 'Unknown error';
            showNotification(`Failed to refresh token: ${errorMessage}`, 'error');
        }
    };

    return (
        <PageLayout loading={loading}>
            {/* Dev Mode Debug Panel */}
            {import.meta.env.DEV && (
                <Box sx={{ mb: 2 }}>
                    <Alert severity="info">
                        <Typography variant="subtitle2" gutterBottom>
                            Development Mode - OAuth Debug Panel
                        </Typography>
                        <Stack direction="row" spacing={2} sx={{ mt: 1 }}>
                            <Button
                                variant="outlined"
                                size="small"
                                onClick={() => {
                                    setOAuthDialogOpen(true);
                                }}
                            >
                                Open OAuth Dialog
                            </Button>
                            {providers.length > 0 && (
                                <Button
                                    variant="outlined"
                                    size="small"
                                    onClick={() => {
                                        setOAuthDetailProvider(providers[0]);
                                        setOAuthDetailDialogOpen(true);
                                    }}
                                >
                                    Open Detail Dialog
                                </Button>
                            )}
                            <Button
                                variant="outlined"
                                size="small"
                                color="secondary"
                                onClick={() => {
                                    showNotification('Test notification!', 'success');
                                }}
                            >
                                Test Notification
                            </Button>
                        </Stack>
                    </Alert>
                </Box>
            )}

            {providers.length > 0 && (
                <UnifiedCard
                    title="OAuth Providers"
                    subtitle={`Managing ${providers.length} OAuth provider${providers.length > 1 ? 's' : ''}`}
                    size="full"
                    rightAction={
                        <Button
                            variant="contained"
                            startIcon={<VpnKey />}
                            onClick={handleAddOAuthClick}
                            size="small"
                        >
                            Add OAuth
                        </Button>
                    }
                >
                    <OAuthTable
                        providers={providers}
                        onEdit={handleEditProvider}
                        onToggle={handleToggleProvider}
                        onDelete={handleDeleteProvider}
                        // onReauthorize={handleReauthorizeOAuth}
                        onRefreshToken={handleRefreshToken}
                    />
                </UnifiedCard>
            )}

            {providers.length === 0 && (
                <UnifiedCard
                    title="No OAuth Providers Configured"
                    subtitle="Get started by adding your first OAuth provider"
                    size="large"
                >
                    <Box textAlign="center" py={3}>
                        <Typography color="text.secondary" gutterBottom>
                            Configure OAuth providers like Claude Code, Gemini CLI, Qwen, etc.
                        </Typography>
                        <Button
                            variant="contained"
                            startIcon={<VpnKey />}
                            onClick={handleAddOAuthClick}
                            sx={{ mt: 2 }}
                        >
                            Add OAuth Provider
                        </Button>
                    </Box>
                </UnifiedCard>
            )}

            {/* OAuth Add Dialog */}
            <OAuthDialog
                open={oauthDialogOpen}
                onClose={() => setOAuthDialogOpen(false)}
                onSuccess={handleOAuthSuccess}
            />

            {/* OAuth Detail/Edit Dialog */}
            <OAuthDetailDialog
                open={oauthDetailDialogOpen}
                provider={oauthDetailProvider}
                onClose={() => setOAuthDetailDialogOpen(false)}
                onSubmit={async (data: OAuthEditFormData) => {
                    if (!oauthDetailProvider?.uuid) return;
                    const result = await api.updateProvider(oauthDetailProvider.uuid, {
                        name: data.name,
                        api_base: data.apiBase,
                        api_style: data.apiStyle,
                        enabled: data.enabled,
                    });
                    if (!result.success) {
                        throw new Error(result.error || 'Failed to update provider');
                    }
                    showNotification('Provider updated successfully!', 'success');
                    loadProviders();
                }}
            />

            {/* Snackbar for notifications */}
            <Snackbar
                open={snackbar.open}
                autoHideDuration={6000}
                onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            >
                <Alert
                    onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
                    severity={snackbar.severity}
                    variant="filled"
                    sx={{ width: '100%' }}
                >
                    {snackbar.message}
                </Alert>
            </Snackbar>
        </PageLayout>
    );
};

export default OAuthPage;
