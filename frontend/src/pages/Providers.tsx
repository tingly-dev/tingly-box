import { Add, TableChart, ViewModule } from '@mui/icons-material';
import { Alert, Box, Button, Snackbar, Stack, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import AddProviderDialog from '../components/AddProviderDialog';
import CardGrid, { CardGridItem } from '../components/CardGrid';
import EditProviderDialog from '../components/EditProviderDialog';
import { PageLayout } from '../components/PageLayout';
import ProviderCard from '../components/ProviderCard';
import ProviderTable from '../components/ProviderTable';
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
    const [viewMode, setViewMode] = useState<'card' | 'table'>('table');

    // Add provider form
    const [providerName, setProviderName] = useState('');
    const [providerApiBase, setProviderApiBase] = useState('');
    const [providerApiStyle, setProviderApiStyle] = useState('openai');
    const [providerToken, setProviderToken] = useState('');

    // Add dialog
    const [addDialogOpen, setAddDialogOpen] = useState(false);

    // Edit dialog
    const [editDialogOpen, setEditDialogOpen] = useState(false);
    const [editingProvider, setEditingProvider] = useState<any>(null);
    const [editName, setEditName] = useState('');
    const [editApiBase, setEditApiBase] = useState('');
    const [editApiStyle, setEditApiStyle] = useState('openai');
    const [editToken, setEditToken] = useState('');
    const [editEnabled, setEditEnabled] = useState(true);

    useEffect(() => {
        loadProviders();
    }, []);

    const showNotification = (message: string, severity: 'success' | 'error') => {
        setSnackbar({ open: true, message, severity });
    };

    const handleAddProviderClick = () => {
        setProviderName('');
        setProviderApiBase('');
        setProviderApiStyle('openai');
        setProviderToken('');
        setAddDialogOpen(true);
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

    const handleAddProvider = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerName,
            api_base: providerApiBase,
            api_style: providerApiStyle,
            token: providerToken,
        };

        const result = await api.addProvider(providerData);

        if (result.success) {
            showNotification('Provider added successfully!', 'success');
            setProviderName('');
            setProviderApiBase('');
            setProviderApiStyle('openai');
            setProviderToken('');
            setAddDialogOpen(false);
            loadProviders();
        } else {
            showNotification(`Failed to add provider: ${result.error}`, 'error');
        }
    };

    const handleDeleteProvider = async (name: string) => {
        if (!confirm(`Are you sure you want to delete provider "${name}"?`)) {
            return;
        }

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
            setEditingProvider(provider);
            setEditName(provider.name);
            setEditApiBase(provider.api_base);
            setEditApiStyle(provider.api_style || 'openai');
            setEditToken('');
            setEditEnabled(provider.enabled);
            setEditDialogOpen(true);
        } else {
            showNotification(`Failed to load provider details: ${result.error}`, 'error');
        }
    };

    const handleUpdateProvider = async (e: React.FormEvent) => {
        e.preventDefault();

        // If token is empty, don't update it
        const providerData: any = {
            name: editName,
            api_base: editApiBase,
            api_style: editApiStyle,
            enabled: editEnabled,
        };

        if (editToken.trim() !== '') {
            providerData.token = editToken;
        }

        const result = await api.updateProvider(editingProvider.name, providerData);

        if (result.success) {
            showNotification('Provider updated successfully!', 'success');
            setEditDialogOpen(false);
            setEditingProvider(null);
            loadProviders();
        } else {
            showNotification(`Failed to update provider: ${result.error}`, 'error');
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
                            <ToggleButtonGroup
                                value={viewMode}
                                exclusive
                                onChange={(_, newMode) => newMode && setViewMode(newMode)}
                                size="small"
                                sx={{ mr: 1 }}
                            >
                                <ToggleButton value="card" sx={{ px: 1, py: 0.5 }}>
                                    <ViewModule fontSize="small" />
                                </ToggleButton>
                                <ToggleButton value="table" sx={{ px: 1, py: 0.5 }}>
                                    <TableChart fontSize="small" />
                                </ToggleButton>
                            </ToggleButtonGroup>
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
                            {viewMode === 'card' ? (
                                <CardGrid>
                                    {providers.map((provider) => (
                                        <CardGridItem xs={12} sm={6} md={4} lg={3} key={provider.name}>
                                            <ProviderCard
                                                provider={provider}
                                                variant="detailed"
                                                onEdit={handleEditProvider}
                                                onToggle={handleToggleProvider}
                                                onDelete={handleDeleteProvider}
                                            />
                                        </CardGridItem>
                                    ))}
                                </CardGrid>
                            ) : (
                                <ProviderTable
                                    providers={providers}
                                    onEdit={handleEditProvider}
                                    onToggle={handleToggleProvider}
                                    onDelete={handleDeleteProvider}
                                />
                            )}
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
                            onClick={() => setAddDialogOpen(true)}
                            sx={{ mt: 2 }}
                        >
                            Add Your First Provider
                        </Button>
                    </Box>
                </UnifiedCard>
            )}

            {/* Add Dialog */}
            <AddProviderDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddProvider}
                providerName={providerName}
                onProviderNameChange={setProviderName}
                providerApiBase={providerApiBase}
                onProviderApiBaseChange={setProviderApiBase}
                providerApiStyle={providerApiStyle}
                onProviderApiStyleChange={setProviderApiStyle}
                providerToken={providerToken}
                onProviderTokenChange={setProviderToken}
            />

            {/* Edit Dialog */}
            <EditProviderDialog
                open={editDialogOpen}
                onClose={() => setEditDialogOpen(false)}
                onSubmit={handleUpdateProvider}
                editName={editName}
                onEditNameChange={setEditName}
                editApiBase={editApiBase}
                onEditApiBaseChange={setEditApiBase}
                editApiStyle={editApiStyle}
                onEditApiStyleChange={setEditApiStyle}
                editToken={editToken}
                onEditTokenChange={setEditToken}
                editEnabled={editEnabled}
                onEditEnabledChange={setEditEnabled}
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
