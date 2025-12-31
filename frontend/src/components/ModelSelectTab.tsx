import { CheckCircle } from '@mui/icons-material';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import NavigateBeforeIcon from '@mui/icons-material/NavigateBefore';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import RefreshIcon from '@mui/icons-material/Refresh';
import SearchIcon from '@mui/icons-material/Search';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    InputAdornment,
    Snackbar,
    Stack,
    Tab,
    Tabs,
    TextField,
    Typography,
} from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import { dispatchCustomModelUpdate, listenForCustomModelUpdates, useCustomModels } from '../hooks/useCustomModels';
import { useGridLayout } from '../hooks/useGridLayout';
import { usePagination } from '../hooks/usePagination';
import type { Provider, ProviderModelsData } from '../types/provider';
import { api } from '../services/api';
import { getModelTypeInfo, navigateToModelPage } from '../utils/modelUtils';
import { ApiStyleBadge } from "./ApiStyleBadge";
import CustomModelCard from './CustomModelCard';
import ModelCard from './ModelCard';
import { a11yProps, TabPanel } from './TabPanel';

export interface ProviderSelectTabOption {
    provider: Provider;
    model?: string;
}

interface ProviderSelectTabProps {
    providers: Provider[];
    providerModels?: ProviderModelsData;
    selectedProvider?: string; // This is now UUID
    selectedModel?: string;
    activeTab?: number;
    onSelected?: (option: ProviderSelectTabOption) => void;
    onProviderChange?: (provider: Provider) => void; // Called when switching to a provider tab
    onRefresh?: (provider: Provider) => void;
    onCustomModelSave?: (provider: Provider, customModel: string) => void;
    refreshingProviders?: string[]; // These are UUIDs
}

export default function ModelSelectTab({
    providers,
    providerModels,
    selectedProvider, // This is now UUID
    selectedModel,
    activeTab: externalActiveTab,
    onSelected,
    onProviderChange,
    onRefresh,
    onCustomModelSave,
    refreshingProviders = [], // These are UUIDs
}: ProviderSelectTabProps) {
    const [internalCurrentTab, setInternalCurrentTab] = useState(0);
    const [isInitialized, setIsInitialized] = useState(false);
    const { customModels, saveCustomModel, removeCustomModel } = useCustomModels();
    const gridLayout = useGridLayout();

    // State for model probing
    const [probingModels, setProbingModels] = useState<Set<string>>(new Set());
    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error';
    }>({ open: false, message: '', severity: 'error' });


    // Create provider name to UUID mapping for search functionality
    const providerNameToUuid = React.useMemo(() => {
        const map: { [name: string]: string } = {};
        providers.forEach(provider => {
            map[provider.name] = provider.uuid;
        });
        return map;
    }, [providers]);

    // Memoize enabled providers to avoid repeated filtering
    const enabledProviders = React.useMemo(
        () => (providers || []).filter(provider => provider.enabled),
        [providers]
    );

    // Pagination and search
    const { searchTerms, setCurrentPage, handleSearchChange, handlePageChange, getPaginatedData } =
        usePagination(
            enabledProviders.map(p => p.name),
            gridLayout.modelsPerPage
        );

    // Use external activeTab if provided, otherwise use internal state
    const currentTab = externalActiveTab !== undefined ? externalActiveTab : internalCurrentTab;

    const [customModelDialog, setCustomModelDialog] = useState<{ open: boolean; provider: Provider | null; value: string }>({
        open: false,
        provider: null,
        value: ''
    });

    // Listen for custom model updates from other components
    useEffect(() => {
        const cleanup = listenForCustomModelUpdates(() => {
            // The hook will automatically handle state updates
        });
        return cleanup;
    }, []);

    // Enhanced save function that also saves to local storage
    const handleCustomModelSave = () => {
        const customModel = customModelDialog.value?.trim();
        if (customModel && customModelDialog.provider) {
            // Save to local storage using hook
            if (saveCustomModel(customModelDialog.provider.name, customModel)) {
                dispatchCustomModelUpdate(customModelDialog.provider.name, customModel);
            }

            // Then save to persistence through parent component
            if (onCustomModelSave) {
                onCustomModelSave(customModelDialog.provider, customModel);
            }
        }
        setCustomModelDialog({ open: false, provider: null, value: '' });
    };

    const handleTabChange = (_: React.SyntheticEvent, newValue: number) => {
        if (externalActiveTab === undefined) {
            setInternalCurrentTab(newValue);
        }

        // Get the target provider
        const targetProvider = enabledProviders[newValue];
        if (!targetProvider) return;

        // Notify parent component about provider change
        // Parent component can then fetch models for this provider using UUID
        if (onProviderChange) {
            onProviderChange(targetProvider);
        }

        // Auto-navigate to page containing selected model when switching tabs
        if (selectedProvider === targetProvider.uuid && selectedModel) {
            const modelTypeInfo = getModelTypeInfo(targetProvider, providerModels, customModels);
            const { isCustomModel, allModelsForSearch } = modelTypeInfo;

            // Only navigate to page for standard models, not custom models
            if (!isCustomModel(selectedModel)) {
                const standardModels = allModelsForSearch.filter(model => !isCustomModel(model));
                navigateToModelPage(selectedModel, targetProvider, gridLayout.modelsPerPage, setCurrentPage, () => standardModels);
            }
        }
    };

    const handleModelSelect = useCallback(async (provider: Provider, model: string) => {
        // Check if provider is oauth type
        if (provider.auth_type === 'oauth') {
            const modelKey = `${provider.uuid}-${model}`;
            // Check if already probing
            if (probingModels.has(modelKey)) {
                return;
            }

            // Add to probing set
            setProbingModels(prev => new Set(prev).add(modelKey));

            try {
                // Probe model availability
                const result = await api.probeModel(provider.uuid, model);

                // Remove from probing set
                setProbingModels(prev => {
                    const next = new Set(prev);
                    next.delete(modelKey);
                    return next;
                });

                // Check if probe was successful
                if (result?.success === false || result?.error) {
                    console.log(result.error)
                    setSnackbar({
                        open: true,
                        message: `Model "${model}" is not available: ${result.error?.message || 'Unknown error'}`,
                        severity: 'error'
                    });
                    return; // Don't proceed with selection
                }

                // Success - proceed with selection
                if (onSelected) {
                    onSelected({ provider, model });
                }
            } catch (error: any) {
                // Remove from probing set
                setProbingModels(prev => {
                    const next = new Set(prev);
                    next.delete(modelKey);
                    return next;
                });

                console.log(error)
                setSnackbar({
                    open: true,
                    message: `Model "${model}" is not available: ${error || 'Network error'}`,
                    severity: 'error'
                });
            }
        } else {
            // Non-oauth provider - proceed directly
            if (onSelected) {
                onSelected({ provider, model });
            }
        }
    }, [probingModels, onSelected]);

    const handleDeleteCustomModel = (provider: Provider, customModel: string) => {
        removeCustomModel(provider.name, customModel);
        dispatchCustomModelUpdate(provider.name, customModel);
    };

    const handleCustomModelEdit = (provider: Provider, currentValue?: string) => {
        setCustomModelDialog({
            open: true,
            provider,
            value: currentValue || ''
        });
    };

    const handleCustomModelCancel = () => {
        setCustomModelDialog({ open: false, provider: null, value: '' });
    };

    // Auto-switch to selected provider tab and navigate to selected model on component mount (only once)
    React.useEffect(() => {
        if (!isInitialized && selectedProvider) {
            const targetProviderIndex = enabledProviders.findIndex(provider => provider.uuid === selectedProvider);

            // Auto-switch to the selected provider's tab
            if (targetProviderIndex !== -1) {
                if (externalActiveTab === undefined) {
                    setInternalCurrentTab(targetProviderIndex);
                }

                // Fetch models for the selected provider on initial load
                const targetProvider = enabledProviders[targetProviderIndex];
                if (onProviderChange) {
                    onProviderChange(targetProvider);
                }

                // Auto-navigate to selected model if also provided
                if (selectedModel) {
                    const modelTypeInfo = getModelTypeInfo(targetProvider, providerModels, customModels);
                    const { isCustomModel, allModelsForSearch } = modelTypeInfo;

                    // Only navigate to page for standard models, not custom models
                    if (!isCustomModel(selectedModel)) {
                        const standardModels = allModelsForSearch.filter(model => !isCustomModel(model));
                        navigateToModelPage(selectedModel, targetProvider, gridLayout.modelsPerPage, setCurrentPage, () => standardModels);
                    }
                }
            }

            // Mark as initialized to prevent further automatic switching
            setIsInitialized(true);
        }
    }, [isInitialized, selectedProvider, selectedModel, enabledProviders, providerModels, externalActiveTab, customModels, gridLayout.modelsPerPage, onProviderChange]);

    return (
        <Box sx={{ width: '100%' }}>
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>

                <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>
                    Credentials ({providers.length})
                </Typography>
                <Tabs
                    value={currentTab}
                    onChange={handleTabChange}
                    aria-label="Provider selection tabs"
                    variant="scrollable"
                    scrollButtons="auto"
                    allowScrollButtonsMobile
                >
                    {enabledProviders.map((provider, index) => {
                        const modelTypeInfo = getModelTypeInfo(provider, providerModels, customModels);
                        const isProviderSelected = selectedProvider === provider.uuid; // Compare UUIDs

                        return (
                            <Tab
                                key={provider.uuid} // Use UUID as key
                                label={
                                    <Stack direction="column" alignItems="center" spacing={0.5}>
                                        <Stack direction="row" alignItems="center" spacing={1}>
                                            <Typography variant="body1" fontWeight={600}>
                                                {provider.name}
                                            </Typography>
                                            {isProviderSelected && (
                                                <CheckCircle color="primary" sx={{ fontSize: 16 }} />
                                            )}
                                        </Stack>
                                        <Stack direction="row" alignItems="center" spacing={1}>
                                            {provider.api_style && <ApiStyleBadge apiStyle={provider.api_style} />}
                                        </Stack>
                                    </Stack>
                                }
                                {...a11yProps(index)}
                                sx={{
                                    textTransform: 'none',
                                    minWidth: 120,
                                    '&.Mui-selected': {
                                        color: 'primary.main',
                                        fontWeight: 600,
                                    },
                                }}
                            />
                        );
                    })}
                </Tabs>
            </Box>

            {enabledProviders.map((provider, index) => {
                const modelTypeInfo = getModelTypeInfo(provider, providerModels, customModels);
                const { standardModelsForDisplay, isCustomModel } = modelTypeInfo;

                const isProviderSelected = selectedProvider === provider.uuid; // Compare UUIDs
                const pagination = getPaginatedData(standardModelsForDisplay, provider.name);
                const isRefreshing = refreshingProviders.includes(provider.uuid); // Use UUID

                const backendCustomModel = providerModels?.[provider.name]?.custom_model;
                const localStorageCustomModels = customModels[provider.name] || [];

                return (
                    <TabPanel key={provider.uuid} value={currentTab} index={index}> {/* Use UUID as key */}


                        {/* Models Display */}
                        <Stack spacing={2}>
                            {/* Star Models Section */}
                            {providerModels?.[provider.name]?.star_models && providerModels[provider.name].star_models!.length > 0 && (
                                <Box>
                                    <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>
                                        Starred Models
                                    </Typography>
                                    <Box
                                        sx={{
                                            display: 'grid',
                                            gridTemplateColumns: `repeat(${gridLayout.columns}, 1fr)`,
                                            gap: 0.8,
                                        }}
                                    >
                                        {providerModels[provider.name].star_models!.map((starModel) => (
                                            <ModelCard
                                                key={starModel}
                                                model={starModel}
                                                isSelected={isProviderSelected && selectedModel === starModel}
                                                onClick={() => handleModelSelect(provider, starModel)}
                                                variant="starred"
                                                loading={provider.auth_type === 'oauth' && probingModels.has(`${provider.uuid}-${starModel}`)}
                                            />
                                        ))}
                                    </Box>
                                </Box>
                            )}

                            {/* All Models Section */}
                            <Box sx={{ minHeight: 200 }}>
                                {/* Title and Controls in same row */}
                                <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
                                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                        Models ({pagination.totalItems})
                                    </Typography>
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <TextField
                                            size="small"
                                            placeholder="Search models..."
                                            value={searchTerms[provider.name] || ''}
                                            onChange={(e) => handleSearchChange(provider.name, e.target.value)}
                                            slotProps={{
                                                input: {
                                                    startAdornment: (
                                                        <InputAdornment position="start">
                                                            <SearchIcon />
                                                        </InputAdornment>
                                                    ),
                                                },
                                            }}
                                            sx={{ width: 300 }}
                                        />
                                        <Button
                                            variant="outlined"
                                            startIcon={<AddCircleOutlineIcon />}
                                            onClick={() => handleCustomModelEdit(provider)}
                                            sx={{
                                                height: 40,
                                                borderColor: 'primary.main',
                                                color: 'primary.main',
                                                '&:hover': {
                                                    backgroundColor: 'primary.50',
                                                    borderColor: 'primary.dark',
                                                }
                                            }}
                                        >
                                            Customize
                                        </Button>
                                        <Button
                                            variant="outlined"
                                            startIcon={isRefreshing ? <CircularProgress size={16} /> : <RefreshIcon />}
                                            onClick={() => onRefresh && onRefresh(provider)}
                                            disabled={!onRefresh || isRefreshing}
                                            sx={{
                                                height: 40,
                                                borderColor: isRefreshing ? 'grey.300' : 'primary.main',
                                                color: isRefreshing ? 'grey.500' : 'primary.main',
                                                '&:hover': !isRefreshing ? {
                                                    backgroundColor: 'primary.50',
                                                    borderColor: 'primary.dark',
                                                } : {},
                                                '&:disabled': {
                                                    borderColor: 'grey.300',
                                                    color: 'grey.500',
                                                }
                                            }}
                                        >
                                            {isRefreshing ? 'Fetching...' : 'Refresh'}
                                        </Button>
                                    </Stack>
                                </Stack>

                                <Box
                                    sx={{
                                        display: 'grid',
                                        gridTemplateColumns: `repeat(${gridLayout.columns}, 1fr)`,
                                        gap: 0.8,
                                    }}
                                >
                                    {/* Custom models from local storage */}
                                    {customModels[provider.name]?.map((customModel, index) => (
                                        <CustomModelCard
                                            key={`localStorage-custom-model-${index}`}
                                            model={customModel}
                                            provider={provider}
                                            isSelected={isProviderSelected && selectedModel === customModel}
                                            onEdit={() => handleCustomModelEdit(provider, customModel)}
                                            onDelete={() => handleDeleteCustomModel(provider, customModel)}
                                            onSelect={() => handleModelSelect(provider, customModel)}
                                            variant="localStorage"
                                            loading={provider.auth_type === 'oauth' && probingModels.has(`${provider.uuid}-${customModel}`)}
                                        />
                                    ))}

                                    {/* Persisted custom model card (from backend) */}
                                    {backendCustomModel &&
                                        (!customModels[provider.name] || customModels[provider.name].length === 0) && (
                                            <CustomModelCard
                                                key="persisted-custom-model"
                                                model={backendCustomModel}
                                                provider={provider}
                                                isSelected={isProviderSelected && selectedModel === backendCustomModel}
                                                onEdit={() => handleCustomModelEdit(provider, backendCustomModel)}
                                                onDelete={() => handleDeleteCustomModel(provider, backendCustomModel)}
                                                onSelect={() => handleModelSelect(provider, backendCustomModel)}
                                                variant="backend"
                                                loading={provider.auth_type === 'oauth' && probingModels.has(`${provider.uuid}-${backendCustomModel}`)}
                                            />
                                        )}

                                    {/* Currently selected custom model card (not persisted) */}
                                    {isProviderSelected && selectedModel && isCustomModel(selectedModel) &&
                                        (!customModels[provider.name] || !customModels[provider.name].includes(selectedModel)) &&
                                        selectedModel !== backendCustomModel && (
                                            <CustomModelCard
                                                key="selected-custom-model"
                                                model={selectedModel}
                                                provider={provider}
                                                isSelected={true}
                                                onEdit={() => handleCustomModelEdit(provider, selectedModel)}
                                                onDelete={() => handleDeleteCustomModel(provider, selectedModel)}
                                                onSelect={() => handleModelSelect(provider, selectedModel)}
                                                variant="selected"
                                                loading={provider.auth_type === 'oauth' && probingModels.has(`${provider.uuid}-${selectedModel}`)}
                                            />
                                        )}

                                    {/* Standard models */}
                                    {pagination.items.map((model) => {
                                        const isModelSelected = isProviderSelected && selectedModel === model;
                                        return (
                                            <ModelCard
                                                key={model}
                                                model={model}
                                                isSelected={isModelSelected}
                                                onClick={() => handleModelSelect(provider, model)}
                                                variant="standard"
                                                loading={provider.auth_type === 'oauth' && probingModels.has(`${provider.uuid}-${model}`)}
                                            />
                                        );
                                    })}
                                </Box>
                                {pagination.totalItems === 0 &&
                                    (!customModels[provider.name] || customModels[provider.name].length === 0) &&
                                    !backendCustomModel &&
                                    !(isProviderSelected && selectedModel && isCustomModel(selectedModel)) && (
                                        <Box sx={{ textAlign: 'center', py: 4 }}>
                                            <Typography variant="body2" color="text.secondary">
                                                No models found matching "{searchTerms[provider.name] || ''}"
                                            </Typography>
                                        </Box>
                                    )}
                            </Box>

                            {/* Pagination Controls - Bottom Center */}
                            {pagination.totalPages > 1 && (
                                <Box sx={{ display: 'flex', justifyContent: 'center', mt: 3 }}>
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <IconButton
                                            size="small"
                                            disabled={pagination.currentPage === 1}
                                            onClick={() => handlePageChange(provider.name, pagination.currentPage - 1)}
                                        >
                                            <NavigateBeforeIcon />
                                        </IconButton>
                                        <Typography variant="body2" sx={{ minWidth: 60, textAlign: 'center' }}>
                                            {pagination.currentPage} / {pagination.totalPages}
                                        </Typography>
                                        <IconButton
                                            size="small"
                                            disabled={pagination.currentPage === pagination.totalPages}
                                            onClick={() => handlePageChange(provider.name, pagination.currentPage + 1)}
                                        >
                                            <NavigateNextIcon />
                                        </IconButton>
                                    </Stack>
                                </Box>
                            )}
                        </Stack>
                    </TabPanel>
                );
            })}

            {/* Custom Model Dialog */}
            <Dialog
                open={customModelDialog.open}
                onClose={handleCustomModelCancel}
                maxWidth="sm"
                fullWidth
            >
                <DialogTitle>
                    {customModelDialog.value ? 'Edit Custom Model' : 'Add Custom Model'}
                </DialogTitle>
                <DialogContent>
                    <TextField
                        autoFocus
                        margin="dense"
                        label="Model Name"
                        fullWidth
                        variant="outlined"
                        value={customModelDialog.value}
                        onChange={(e) => setCustomModelDialog(prev => ({ ...prev, value: e.target.value }))}
                        placeholder="Enter custom model name..."
                        sx={{
                            mt: 1,
                            '& .MuiOutlinedInput-root': {
                                borderRadius: 1.5,
                            }
                        }}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={handleCustomModelCancel}>
                        Cancel
                    </Button>
                    <Button
                        onClick={handleCustomModelSave}
                        variant="contained"
                        disabled={!customModelDialog.value?.trim()}
                    >
                        {customModelDialog.value ? 'Update' : 'Add'}
                    </Button>
                </DialogActions>
            </Dialog>

            {/* Snackbar for notifications */}
            <Snackbar
                open={snackbar.open}
                autoHideDuration={6000}
                onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
            >
                <Alert
                    onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
                    severity={snackbar.severity}
                    sx={{ width: '100%' }}
                >
                    {snackbar.message}
                </Alert>
            </Snackbar>
        </Box>
    );
}