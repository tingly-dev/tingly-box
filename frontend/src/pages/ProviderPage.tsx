import { Add, VpnKey } from '@mui/icons-material';
import { Alert, Box, Button, Chip, Snackbar, Stack, Tab, Tabs, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { PageLayout } from '../components/PageLayout';
import PresetProviderFormDialog from '../components/PresetProviderFormDialog.tsx';
import OAuthDialog from '../components/OAuthDialog.tsx';
import OAuthDetailDialog from '../components/OAuthDetailDialog.tsx';
import { type ProviderFormData } from '../components/ProviderFormDialog.tsx';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';
import ApiKeyTable from '../components/ApiKeyTable.tsx';
import OAuthTable from '../components/OAuthTable.tsx';

const ProviderPage = () => {
    const [providers, setProviders] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const [tabValue, setTabValue] = useState(0);
    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error';
    }>({ open: false, message: '', severity: 'success' });

    // API Key Dialog state
    const [apiKeyDialogOpen, setApiKeyDialogOpen] = useState(false);
    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>('add');
    const [providerFormData, setProviderFormData] = useState<ProviderFormData>({
        uuid: undefined,
        name: '',
        apiBase: '',
        apiStyle: undefined,
        token: '',
        enabled: true,
    });

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

    const handleAddApiKeyClick = () => {
        setDialogMode('add');
        setProviderFormData({
            uuid: undefined,
            name: '',
            apiBase: '',
            apiStyle: undefined,
            token: '',
            enabled: true,
        });
        setApiKeyDialogOpen(true);
    };

    const handleAddOAuthClick = () => {
        setOAuthDialogOpen(true);
    };

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        } else {
            showNotification(`Failed to load providers: ${result.error}`, 'error');
        }
        setLoading(false);
    };

    const handleProviderSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            token: providerFormData.token,
            ...(dialogMode === 'edit' && { enabled: providerFormData.enabled }),
        };

        const result = dialogMode === 'add'
            ? await api.addProvider(providerData)
            : await api.updateProvider(providerFormData.uuid!, {
                ...providerData,
                token: providerFormData.token || undefined,
            });

        if (result.success) {
            showNotification(`Provider ${dialogMode === 'add' ? 'added' : 'updated'} successfully!`, 'success');
            setApiKeyDialogOpen(false);
            loadProviders();
        } else {
            showNotification(`Failed to ${dialogMode === 'add' ? 'add' : 'update'} provider: ${result.error}`, 'error');
        }
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
            const provider = result.data;

            // Route to appropriate dialog based on auth type
            if (provider.auth_type === 'oauth') {
                // Open OAuth detail dialog (read-only credentials, editable settings)
                setOAuthDetailProvider(provider);
                setOAuthDetailDialogOpen(true);
            } else {
                // Open API Key edit dialog
                setDialogMode('edit');
                setProviderFormData({
                    uuid: provider.uuid,
                    name: provider.name,
                    apiBase: provider.api_base,
                    apiStyle: provider.api_style || 'openai',
                    token: provider.token || "",
                    enabled: provider.enabled,
                });
                setApiKeyDialogOpen(true);
            }
        } else {
            showNotification(`Failed to load provider details: ${result.error}`, 'error');
        }
    };

    const handleReauthorizeOAuth = async (_uuid: string) => {
        // TODO: Implement reauthorize flow
        showNotification('Reauthorize functionality coming soon!', 'error');
    };

    // Separate providers by auth type
    const apiKeyProviders = providers.filter(p => p.auth_type !== 'oauth');
    const oauthProviders = providers.filter(p => p.auth_type === 'oauth');

    return (
        <PageLayout loading={loading}>
            {providers.length > 0 && (
                <UnifiedCard
                    title="Credential"
                    subtitle={providers.length > 0 ? `Managing ${providers.length} providers and api keys` : "No model API key configured yet"}
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1} alignItems="center">
                            {tabValue === 0 && (
                                <Button
                                    variant="contained"
                                    startIcon={<Add />}
                                    onClick={handleAddApiKeyClick}
                                    size="small"
                                >
                                    Add API Key
                                </Button>
                            )}
                            {tabValue === 1 && (
                                <Button
                                    variant="contained"
                                    startIcon={<VpnKey />}
                                    onClick={handleAddOAuthClick}
                                    size="small"
                                >
                                    Add OAuth
                                </Button>
                            )}
                        </Stack>
                    }
                >
                    <Box sx={{ flex: 1 }}>
                        <Tabs value={tabValue} onChange={(_, newValue) => setTabValue(newValue)} sx={{ borderBottom: 1, borderColor: 'divider' }}>
                            <Tab
                                label={
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <Typography variant="body2">API Keys</Typography>
                                        <Chip label={apiKeyProviders.length} size="small" sx={{ height: 18, fontSize: '0.7rem' }} />
                                    </Stack>
                                }
                            />
                            <Tab
                                label={
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <Typography variant="body2">OAuth</Typography>
                                        <Chip label={oauthProviders.length} size="small" sx={{ height: 18, fontSize: '0.7rem' }} />
                                    </Stack>
                                }
                            />
                        </Tabs>

                        <Box sx={{ mt: 2 }}>
                            {tabValue === 0 && (
                                <ApiKeyTable
                                    providers={apiKeyProviders}
                                    onEdit={handleEditProvider}
                                    onToggle={handleToggleProvider}
                                    onDelete={handleDeleteProvider}
                                />
                            )}
                            {tabValue === 1 && (
                                <OAuthTable
                                    providers={oauthProviders}
                                    onEdit={handleEditProvider}
                                    onToggle={handleToggleProvider}
                                    onDelete={handleDeleteProvider}
                                    onReauthorize={handleReauthorizeOAuth}
                                />
                            )}
                        </Box>
                    </Box>
                </UnifiedCard>
            )}

            {providers.length === 0 && (
                <UnifiedCard
                    title="No Model API Key Configured"
                    subtitle="Get started by adding your first API token or key"
                    size="large"
                >
                    <Box textAlign="center" py={3}>
                        <Typography color="text.secondary" gutterBottom>
                            Configure API keys or OAuth providers to access AI services
                        </Typography>
                        <Stack direction="row" spacing={2} justifyContent="center" sx={{ mt: 2 }}>
                            <Button
                                variant="outlined"
                                startIcon={<VpnKey />}
                                onClick={handleAddOAuthClick}
                            >
                                Add OAuth
                            </Button>
                            <Button
                                variant="contained"
                                startIcon={<Add />}
                                onClick={handleAddApiKeyClick}
                            >
                                Add API Key
                            </Button>
                        </Stack>
                    </Box>
                </UnifiedCard>
            )}

            {/* API Key Provider Dialog */}
            <PresetProviderFormDialog
                open={apiKeyDialogOpen}
                onClose={() => setApiKeyDialogOpen(false)}
                onSubmit={handleProviderSubmit}
                data={providerFormData}
                onChange={(field, value) => setProviderFormData(prev => ({ ...prev, [field]: value }))}
                mode={dialogMode}
            />

            {/* OAuth Add Dialog */}
            <OAuthDialog
                open={oauthDialogOpen}
                onClose={() => setOAuthDialogOpen(false)}
            />

            {/* OAuth Detail/Edit Dialog */}
            <OAuthDetailDialog
                open={oauthDetailDialogOpen}
                provider={oauthDetailProvider}
                onClose={() => setOAuthDetailDialogOpen(false)}
                onSubmit={async (data) => {
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

export default ProviderPage;
