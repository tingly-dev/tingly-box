import { Add } from '@mui/icons-material';
import { Alert, Box, Button, Snackbar, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { PageLayout } from '../components/PageLayout';
import PresetProviderFormDialog from '../components/PresetProviderFormDialog.tsx';
import { type ProviderFormData } from '../components/ProviderFormDialog.tsx';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';
import ApiKeyTable from '../components/ApiKeyTable.tsx';

const ApiKeyPage = () => {
    const [providers, setProviders] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error';
    }>({ open: false, message: '', severity: 'success' });

    // Dialog state
    const [dialogOpen, setDialogOpen] = useState(false);
    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>('add');
    const [providerFormData, setProviderFormData] = useState<ProviderFormData>({
        uuid: undefined,
        name: '',
        apiBase: '',
        apiStyle: undefined,
        token: '',
        enabled: true,
    });

    useEffect(() => {
        loadProviders();
    }, []);

    const showNotification = (message: string, severity: 'success' | 'error') => {
        setSnackbar({ open: true, message, severity });
    };

    const handleAddClick = () => {
        setDialogMode('add');
        setProviderFormData({
            uuid: undefined,
            name: '',
            apiBase: '',
            apiStyle: undefined,
            token: '',
            enabled: true,
        });
        setDialogOpen(true);
    };

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) {
            // Filter only API key providers
            setProviders(result.data.filter((p: any) => p.auth_type !== 'oauth'));
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
            setDialogOpen(false);
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
            setDialogMode('edit');
            setProviderFormData({
                uuid: provider.uuid,
                name: provider.name,
                apiBase: provider.api_base,
                apiStyle: provider.api_style || 'openai',
                token: provider.token || "",
                enabled: provider.enabled,
            });
            setDialogOpen(true);
        } else {
            showNotification(`Failed to load provider details: ${result.error}`, 'error');
        }
    };

    return (
        <PageLayout loading={loading}>
            {providers.length > 0 && (
                <UnifiedCard
                    title="API Keys"
                    subtitle={`Managing ${providers.length} API key${providers.length > 1 ? 's' : ''}`}
                    size="full"
                    rightAction={
                        <Button
                            variant="contained"
                            startIcon={<Add />}
                            onClick={handleAddClick}
                            size="small"
                        >
                            Add API Key
                        </Button>
                    }
                >
                    <ApiKeyTable
                        providers={providers}
                        onEdit={handleEditProvider}
                        onToggle={handleToggleProvider}
                        onDelete={handleDeleteProvider}
                    />
                </UnifiedCard>
            )}

            {providers.length === 0 && (
                <UnifiedCard
                    title="No API Keys Configured"
                    subtitle="Get started by adding your first API key"
                    size="large"
                >
                    <Box textAlign="center" py={3}>
                        <Typography color="text.secondary" gutterBottom>
                            Configure API keys to access AI services like OpenAI, Anthropic, etc.
                        </Typography>
                        <Button
                            variant="contained"
                            startIcon={<Add />}
                            onClick={handleAddClick}
                            sx={{ mt: 2 }}
                        >
                            Add API Key
                        </Button>
                    </Box>
                </UnifiedCard>
            )}

            {/* API Key Provider Dialog */}
            <PresetProviderFormDialog
                open={dialogOpen}
                onClose={() => setDialogOpen(false)}
                onSubmit={handleProviderSubmit}
                data={providerFormData}
                onChange={(field, value) => setProviderFormData(prev => ({ ...prev, [field]: value }))}
                mode={dialogMode}
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

export default ApiKeyPage;
