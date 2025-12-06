import {
    Alert,
    Box,
    CircularProgress,
    Typography,
    Button,
    ToggleButton,
    ToggleButtonGroup,
    Stack,
} from '@mui/material';
import { Add, ViewModule, TableChart } from '@mui/icons-material';
import { useEffect, useState } from 'react';
import CardGrid, { CardGridItem } from '../components/CardGrid';
import UnifiedCard from '../components/UnifiedCard';
import ProviderCard from '../components/ProviderCard';
import ProviderTable from '../components/ProviderTable';
import AddProviderDialog from '../components/AddProviderDialog';
import EditProviderDialog from '../components/EditProviderDialog';
import { api } from '../services/api';

const Providers = () => {
    const [providers, setProviders] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [viewMode, setViewMode] = useState<'card' | 'table'>('table');

    // Add provider form
    const [providerName, setProviderName] = useState('');
    const [providerApiBase, setProviderApiBase] = useState('');
    const [providerApiVersion, setProviderApiVersion] = useState('openai');
    const [providerToken, setProviderToken] = useState('');

    // Add dialog
    const [addDialogOpen, setAddDialogOpen] = useState(false);

    // Edit dialog
    const [editDialogOpen, setEditDialogOpen] = useState(false);
    const [editingProvider, setEditingProvider] = useState<any>(null);
    const [editName, setEditName] = useState('');
    const [editApiBase, setEditApiBase] = useState('');
    const [editApiVersion, setEditApiVersion] = useState('openai');
    const [editToken, setEditToken] = useState('');
    const [editEnabled, setEditEnabled] = useState(true);

    useEffect(() => {
        loadProviders();
    }, []);

    const handleAddProviderClick = () => {
        setProviderName('');
        setProviderApiBase('');
        setProviderApiVersion('openai');
        setProviderToken('');
        setAddDialogOpen(true);
    };

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        } else {
            setMessage({ type: 'error', text: `Failed to load providers: ${result.error}` });
        }
        setLoading(false);
    };

    const handleAddProvider = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerName,
            api_base: providerApiBase,
            api_version: providerApiVersion,
            token: providerToken,
        };

        const result = await api.addProvider(providerData);

        if (result.success) {
            setMessage({ type: 'success', text: 'Provider added successfully!' });
            setProviderName('');
            setProviderApiBase('');
            setProviderApiVersion('openai');
            setProviderToken('');
            setAddDialogOpen(false);
            loadProviders();
        } else {
            setMessage({ type: 'error', text: `Failed to add provider: ${result.error}` });
        }
    };

    const handleDeleteProvider = async (name: string) => {
        if (!confirm(`Are you sure you want to delete provider "${name}"?`)) {
            return;
        }

        const result = await api.deleteProvider(name);

        if (result.success) {
            setMessage({ type: 'success', text: 'Provider deleted successfully!' });
            loadProviders();
        } else {
            setMessage({ type: 'error', text: `Failed to delete provider: ${result.error}` });
        }
    };

    const handleToggleProvider = async (name: string) => {
        const result = await api.toggleProvider(name);

        if (result.success) {
            setMessage({ type: 'success', text: result.message });
            loadProviders();
        } else {
            setMessage({ type: 'error', text: `Failed to toggle provider: ${result.error}` });
        }
    };

    const handleEditProvider = async (name: string) => {
        const result = await api.getProvider(name);

        if (result.success) {
            const provider = result.data;
            setEditingProvider(provider);
            setEditName(provider.name);
            setEditApiBase(provider.api_base);
            setEditApiVersion(provider.api_version || 'openai');
            setEditToken('');
            setEditEnabled(provider.enabled);
            setEditDialogOpen(true);
        } else {
            setMessage({ type: 'error', text: `Failed to load provider details: ${result.error}` });
        }
    };

    const handleUpdateProvider = async (e: React.FormEvent) => {
        e.preventDefault();

        // If token is empty, don't update it
        const providerData: any = {
            name: editName,
            api_base: editApiBase,
            api_version: editApiVersion,
            enabled: editEnabled,
        };

        if (editToken.trim() !== '') {
            providerData.token = editToken;
        }

        const result = await api.updateProvider(editingProvider.name, providerData);

        if (result.success) {
            setMessage({ type: 'success', text: 'Provider updated successfully!' });
            setEditDialogOpen(false);
            setEditingProvider(null);
            loadProviders();
        } else {
            setMessage({ type: 'error', text: `Failed to update provider: ${result.error}` });
        }
    };

    if (loading) {
        return (
            <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
                <CircularProgress />
            </Box>
        );
    }

    return (
        <Box>
            {message && (
                <Alert
                    severity={message.type}
                    sx={{ mb: 2 }}
                    onClose={() => setMessage(null)}
                >
                    {message.text}
                </Alert>
            )}

            <CardGrid>
                <CardGridItem xs={12}>
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
                </CardGridItem>

                {providers.length === 0 && (
                    <CardGridItem xs={12}>
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
                    </CardGridItem>
                )}
            </CardGrid>

            {/* Add Dialog */}
            <AddProviderDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddProvider}
                providerName={providerName}
                onProviderNameChange={setProviderName}
                providerApiBase={providerApiBase}
                onProviderApiBaseChange={setProviderApiBase}
                providerApiVersion={providerApiVersion}
                onProviderApiVersionChange={setProviderApiVersion}
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
                editApiVersion={editApiVersion}
                onEditApiVersionChange={setEditApiVersion}
                editToken={editToken}
                onEditTokenChange={setEditToken}
                editEnabled={editEnabled}
                onEditEnabledChange={setEditEnabled}
            />
        </Box>
    );
};

export default Providers;
