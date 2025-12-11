import { Add } from '@mui/icons-material';
import { Alert, Box, Button, Snackbar, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { PageLayout } from '../components/PageLayout';
import ProviderTable from '../components/ProviderTable';
import ProviderFormDialog, { type ProviderFormData } from '../components/ui/ProviderFormDialog';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';

const Providers = () => {
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
        name: '',
        apiBase: '',
        apiStyle: 'openai',
        token: '',
        enabled: true,
    });

    useEffect(() => {
        loadProviders();
    }, []);

    const showNotification = (message: string, severity: 'success' | 'error') => {
        setSnackbar({ open: true, message, severity });
    };

    const handleAddProviderClick = () => {
        setDialogMode('add');
        setProviderFormData({
            name: '',
            apiBase: '',
            apiStyle: 'openai',
            token: '',
            enabled: true,
        });
        setDialogOpen(true);
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
            : await api.updateProvider(providerFormData.name, {
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

    const handleDeleteProvider = async (name: string) => {
        const result = await api.deleteProvider(name);

        if (result.success) {
            showNotification('Provider deleted successfully!', 'success');
            loadProviders();
        } else {
            showNotification(`Failed to delete provider: ${result.error}`, 'error');
        }
    };

    const handleToggleProvider = async (name: string) => {
        const result = await api.toggleProvider(name);

        if (result.success) {
            showNotification(result.message, 'success');
            loadProviders();
        } else {
            showNotification(`Failed to toggle provider: ${result.error}`, 'error');
        }
    };

    const handleEditProvider = async (name: string) => {
        const result = await api.getProvider(name);

        if (result.success) {
            const provider = result.data;
            setDialogMode('edit');
            setProviderFormData({
                name: provider.name,
                apiBase: provider.api_base,
                apiStyle: provider.api_style || 'openai',
                token: '',
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
                    title="Current Providers"
                    subtitle={providers.length > 0 ? `Managing ${providers.length} provider(s)` : "No providers configured yet"}
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1} alignItems="center">
                            <Button
                                variant="contained"
                                startIcon={<Add />}
                                onClick={handleAddProviderClick}
                                size="small"
                            >
                                Add Provider
                            </Button>
                        </Stack>
                    }
                >
                    {providers.length > 0 ? (
                        <Box sx={{ flex: 1 }}>
                                <ProviderTable
                                    providers={providers}
                                    onEdit={handleEditProvider}
                                    onToggle={handleToggleProvider}
                                    onDelete={handleDeleteProvider}
                                />
                        </Box>
                    ) : (
                        <Box textAlign="center" py={5}>
                            <Typography variant="h6" color="text.secondary" gutterBottom>
                                No Providers Configured
                            </Typography>
                            <Typography color="text.secondary">
                                Add your first AI provider using the form below to get started.
                            </Typography>
                        </Box>
                    )}
                </UnifiedCard>
            )}

            {providers.length === 0 && (
                <UnifiedCard
                    title="No Providers Configured"
                    subtitle="Get started by adding your first AI provider"
                    size="large"
                >
                    <Box textAlign="center" py={3}>
                        <Typography color="text.secondary" gutterBottom>
                            Click the + button on any card to add a new provider
                        </Typography>
                        <Button
                            variant="contained"
                            startIcon={<Add />}
                            onClick={() => setDialogOpen(true)}
                            sx={{ mt: 2 }}
                        >
                            Add Your First Provider
                        </Button>
                    </Box>
                </UnifiedCard>
            )}

            {/* Provider Dialog */}
            <ProviderFormDialog
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

export default Providers;
