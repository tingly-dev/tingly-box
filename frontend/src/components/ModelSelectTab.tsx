import { CheckCircle } from '@mui/icons-material';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import NavigateBeforeIcon from '@mui/icons-material/NavigateBefore';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import RefreshIcon from '@mui/icons-material/Refresh';
import SearchIcon from '@mui/icons-material/Search';
import {
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    InputAdornment,
    Stack,
    Tab,
    Tabs,
    TextField,
    Typography,
} from '@mui/material';
import React, { useEffect, useState } from 'react';
import { dispatchCustomModelUpdate, listenForCustomModelUpdates, useCustomModels } from '../hooks/useCustomModels';
import { useGridLayout } from '../hooks/useGridLayout';
import { usePagination } from '../hooks/usePagination';
import type { Provider, ProviderModelsData } from '../types/provider';
import { getModelTypeInfo, navigateToModelPage } from '../utils/modelUtils';
import CustomModelCard from './CustomModelCard';
import ModelCard from './ModelCard';
import { TabPanel, a11yProps } from './TabPanel';

export interface ProviderSelectTabOption {
    provider: Provider;
    model?: string;
}

interface ProviderSelectTabProps {
    providers: Provider[];
    providerModels?: ProviderModelsData;
    selectedProvider?: string;
    selectedModel?: string;
    activeTab?: number;
    onSelected?: (option: ProviderSelectTabOption) => void;
    onRefresh?: (provider: Provider) => void;
    onCustomModelSave?: (provider: Provider, customModel: string) => void;
    refreshingProviders?: string[];
}

export default function ModelSelectTab({
    providers,
    providerModels,
    selectedProvider,
    selectedModel,
    activeTab: externalActiveTab,
    onSelected,
    onRefresh,
    onCustomModelSave,
    refreshingProviders = [],
}: ProviderSelectTabProps) {
    const [internalCurrentTab, setInternalCurrentTab] = useState(0);
    const [isInitialized, setIsInitialized] = useState(false);
    const { customModels, saveCustomModel, removeCustomModel } = useCustomModels();
    const gridLayout = useGridLayout();
    const [autoFetchedProviders, setAutoFetchedProviders] = useState<Set<string>>(new Set());

    // Pagination and search
    const { searchTerms, currentPage, setCurrentPage, handleSearchChange, handlePageChange, getPaginatedData } =
        usePagination(
            (providers || []).filter(provider => provider.enabled).map(p => p.name),
            gridLayout.modelsPerPage
        );

    // Use external activeTab if provided, otherwise use internal state
    const currentTab = externalActiveTab !== undefined ? externalActiveTab : internalCurrentTab;

    const [customModelDialog, setCustomModelDialog] = useState<{ open: boolean; provider: Provider | null; value: string }>({
        open: false,
        provider: null,
        value: ''
    });

    // Reset auto-fetched providers when provider models are updated
    useEffect(() => {
        // Clear the auto-fetched set when provider models change
        // This allows auto-fetching again if models were cleared
        setAutoFetchedProviders(new Set());
    }, [providerModels]);

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
            // Select the custom model
            if (onSelected) {
                onSelected({ provider: customModelDialog.provider, model: customModel });
            }
        }
        setCustomModelDialog({ open: false, provider: null, value: '' });
    };

    const handleTabChange = (_: React.SyntheticEvent, newValue: number) => {
        if (externalActiveTab === undefined) {
            setInternalCurrentTab(newValue);
        }

        // Get the target provider
        const targetProvider = (providers || []).filter(provider => provider.enabled)[newValue];
        if (!targetProvider) return;

        // Auto-fetch models if the provider has no models and hasn't been auto-fetched before
        const providerModelData = providerModels?.[targetProvider.name];
        const hasModels = providerModelData && (
            (providerModelData.models && providerModelData.models.length > 0) ||
            (providerModelData.star_models && providerModelData.star_models.length > 0) ||
            providerModelData.custom_model
        );

        if (!hasModels && !autoFetchedProviders.has(targetProvider.name) && onRefresh && !refreshingProviders.includes(targetProvider.name)) {
            // Mark as auto-fetched to avoid repeated requests
            setAutoFetchedProviders(prev => new Set([...prev, targetProvider.name]));
            // Trigger model fetch
            onRefresh(targetProvider);
        }

        // Auto-navigate to page containing selected model when switching tabs
        if (selectedProvider === targetProvider.name && selectedModel) {
            const modelTypeInfo = getModelTypeInfo(targetProvider, providerModels, customModels);
            const { isCustomModel, allModelsForSearch } = modelTypeInfo;

            // Only navigate to page for standard models, not custom models
            if (!isCustomModel(selectedModel)) {
                const standardModels = allModelsForSearch.filter(model => !isCustomModel(model));
                navigateToModelPage(selectedModel, targetProvider, gridLayout.modelsPerPage, setCurrentPage, () => standardModels);
            }
        }
    };

    const handleModelSelect = (provider: Provider, model: string) => {
        if (onSelected) {
            onSelected({ provider, model });
        }
    };

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
            const enabledProviders = (providers || []).filter(provider => provider.enabled);
            const targetProviderIndex = enabledProviders.findIndex(provider => provider.name === selectedProvider);

            // Auto-switch to the selected provider's tab
            if (targetProviderIndex !== -1) {
                if (externalActiveTab === undefined) {
                    setInternalCurrentTab(targetProviderIndex);
                }

                // Auto-navigate to selected model if also provided
                if (selectedModel) {
                    const targetProvider = enabledProviders[targetProviderIndex];
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
    }, [isInitialized, selectedProvider, selectedModel, providers, providerModels, externalActiveTab, customModels, gridLayout.modelsPerPage]);

    return (
        <Box sx={{ width: '100%' }}>
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
                <Tabs
                    value={currentTab}
                    onChange={handleTabChange}
                    aria-label="Provider selection tabs"
                    variant="scrollable"
                    scrollButtons="auto"
                    allowScrollButtonsMobile
                >
                    {(providers || []).filter(provider => provider.enabled).map((provider, index) => {
                        const modelTypeInfo = getModelTypeInfo(provider, providerModels, customModels);
                        const isProviderSelected = selectedProvider === provider.name;

                        return (
                            <Tab
                                key={provider.name}
                                label={
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <Typography variant="body1" fontWeight={600}>
                                            {provider.name}
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            ({modelTypeInfo.totalModelsCount})
                                        </Typography>
                                        {isProviderSelected && (
                                            <CheckCircle color="primary" sx={{ fontSize: 16 }} />
                                        )}
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

            {(providers || []).filter(provider => provider.enabled).map((provider, index) => {
                const modelTypeInfo = getModelTypeInfo(provider, providerModels, customModels);
                const { standardModelsForDisplay, isCustomModel } = modelTypeInfo;

                const isProviderSelected = selectedProvider === provider.name;
                const pagination = getPaginatedData(standardModelsForDisplay, provider.name);
                const isRefreshing = refreshingProviders.includes(provider.name);

                const backendCustomModel = providerModels?.[provider.name]?.custom_model;
                const localStorageCustomModels = customModels[provider.name] || [];

                return (
                    <TabPanel key={provider.name} value={currentTab} index={index}>
                        {/* Search and Pagination Controls */}
                        <Box sx={{ mb: 3 }}>
                            <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={2}>
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
                                        Custom Model
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
                                        {isRefreshing ? 'Fetching...' : 'Fetch Model List'}
                                    </Button>
                                </Stack>

                                {/* Pagination Controls */}
                                {pagination.totalPages > 1 && (
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
                                )}
                            </Stack>
                        </Box>

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
                                            />
                                        ))}
                                    </Box>
                                </Box>
                            )}

                            {/* All Models Section */}
                            <Box sx={{ minHeight: 200 }}>
                                <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>
                                    All Models ({pagination.totalItems})
                                </Typography>
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
        </Box>
    );
}