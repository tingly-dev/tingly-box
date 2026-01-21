import { CheckCircle } from '@mui/icons-material';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import NavigateBeforeIcon from '@mui/icons-material/NavigateBefore';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
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
    TextField,
    Typography,
} from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import { dispatchCustomModelUpdate, listenForCustomModelUpdates, useCustomModels } from '../hooks/useCustomModels';
import { useGridLayout } from '../hooks/useGridLayout';
import { usePagination } from '../hooks/usePagination';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import { api } from '../services/api';
import { getModelTypeInfo, navigateToModelPage } from '../utils/modelUtils';
import { ApiStyleBadge } from "./ApiStyleBadge";
import { AuthTypeBadge } from "./AuthTypeBadge";
import CustomModelCard from './CustomModelCard';
import ModelCard from './ModelCard';
import { TabPanel } from './TabPanel';

export interface ProviderSelectTabOption {
    provider: Provider;
    model?: string;
}

interface ProviderSelectTabProps {
    providers: Provider[];
    providerModels?: ProviderModelsDataByUuid;
    selectedProvider?: string; // This is now UUID
    selectedModel?: string;
    activeTab?: number;
    onSelected?: (option: ProviderSelectTabOption) => void;
    onProviderChange?: (provider: Provider) => void; // Called when switching to a provider tab
    onRefresh?: (provider: Provider) => void;
    onCustomModelSave?: (provider: Provider, customModel: string) => void;
    refreshingProviders?: string[]; // These are UUIDs
    // Single provider mode props
    singleProvider?: Provider | null; // If provided, only show this provider
    onTest?: (model: string) => void; // Callback for Test button
    testing?: boolean; // Whether a test is in progress
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
    singleProvider,
    onTest,
    testing = false,
}: ProviderSelectTabProps) {
    const [internalCurrentTab, setInternalCurrentTab] = useState(0);
    const [isInitialized, setIsInitialized] = useState(false);
    const { customModels, saveCustomModel, removeCustomModel, updateCustomModel } = useCustomModels();
    const gridLayout = useGridLayout();

    // In single provider mode, use only that provider
    const displayProviders = singleProvider ? [singleProvider] : providers;
    const isSingleProviderMode = singleProvider !== null && singleProvider !== undefined;

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
        () => displayProviders.filter(provider => provider.enabled),
        [displayProviders]
    );

    // Group and sort providers by auth_type
    const groupedProviders = React.useMemo(() => {
        const groups: { [key: string]: Provider[] } = {};
        const authTypeOrder = ['oauth', 'api_key', 'bearer_token', 'basic_auth'];

        enabledProviders.forEach(provider => {
            const authType = provider.auth_type || 'api key';
            if (!groups[authType]) {
                groups[authType] = [];
            }
            groups[authType].push(provider);
        });

        // Sort providers within each group by name
        Object.keys(groups).forEach(authType => {
            groups[authType].sort((a, b) => a.name.localeCompare(b.name));
        });

        // Sort groups by predefined order, then by auth_type name for unknown types
        const sortedGroups: Array<{ authType: string; providers: Provider[] }> = [];
        authTypeOrder.forEach(authType => {
            if (groups[authType]) {
                sortedGroups.push({ authType, providers: groups[authType] });
                delete groups[authType];
            }
        });

        // Add remaining groups
        Object.keys(groups).sort().forEach(authType => {
            sortedGroups.push({ authType, providers: groups[authType] });
        });

        return sortedGroups;
    }, [enabledProviders]);

    // Flatten grouped providers for tab indexing
    const flattenedProviders = React.useMemo(
        () => groupedProviders.flatMap(group => group.providers),
        [groupedProviders]
    );

    // Pagination and search
    const { searchTerms, setCurrentPage, handleSearchChange, handlePageChange, getPaginatedData } =
        usePagination(
            flattenedProviders.map(p => p.name),
            gridLayout.modelsPerPage
        );

    // Use external activeTab if provided, otherwise use internal state
    const currentTab = externalActiveTab !== undefined ? externalActiveTab : internalCurrentTab;

    const [customModelDialog, setCustomModelDialog] = useState<{
        open: boolean;
        provider: Provider | null;
        value: string;
        originalValue?: string; // Track original value when editing
    }>({
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
            if (customModelDialog.originalValue) {
                // Editing: use updateCustomModel to atomically replace old value with new value
                updateCustomModel(customModelDialog.provider.uuid, customModelDialog.originalValue, customModel);
                dispatchCustomModelUpdate(customModelDialog.provider.uuid, customModel);
            } else {
                // Adding new: use saveCustomModel
                if (saveCustomModel(customModelDialog.provider.uuid, customModel)) {
                    dispatchCustomModelUpdate(customModelDialog.provider.uuid, customModel);
                }
            }

            // Then save to persistence through parent component
            if (onCustomModelSave) {
                onCustomModelSave(customModelDialog.provider, customModel);
            }
        }
        setCustomModelDialog({ open: false, provider: null, value: '', originalValue: undefined });
    };

    const handleTabChange = (_: React.SyntheticEvent, newValue: number) => {
        if (externalActiveTab === undefined) {
            setInternalCurrentTab(newValue);
        }

        // Get the target provider from flattened list
        const targetProvider = flattenedProviders[newValue];
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
        removeCustomModel(provider.uuid, customModel);
        dispatchCustomModelUpdate(provider.uuid, customModel);
    };

    const handleCustomModelEdit = (provider: Provider, currentValue?: string) => {
        setCustomModelDialog({
            open: true,
            provider,
            value: currentValue || '',
            originalValue: currentValue // Set originalValue to track if we're editing
        });
    };

    const handleCustomModelCancel = () => {
        setCustomModelDialog({ open: false, provider: null, value: '', originalValue: undefined });
    };

    // Auto-switch to selected provider tab and navigate to selected model on component mount (only once)
    React.useEffect(() => {
        if (!isInitialized && selectedProvider) {
            const targetProviderIndex = flattenedProviders.findIndex(provider => provider.uuid === selectedProvider);

            // Auto-switch to the selected provider's tab
            if (targetProviderIndex !== -1) {
                if (externalActiveTab === undefined) {
                    setInternalCurrentTab(targetProviderIndex);
                }

                // Fetch models for the selected provider on initial load
                const targetProvider = flattenedProviders[targetProviderIndex];
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
        // Note: We intentionally exclude providerModels from dependencies to avoid re-triggering
        // when models are fetched after user manually switches tabs
    }, [isInitialized, selectedProvider, selectedModel, flattenedProviders, externalActiveTab, customModels, gridLayout.modelsPerPage, onProviderChange]);

    return (
        <Box sx={{ display: 'flex', flexDirection: 'row', height: '100%', width: '100%' }}>
            {/* Left Sidebar - Vertical Tabs */}
            <Box sx={{
                width: 300,
                borderRight: 1,
                borderColor: 'divider',
                display: 'flex',
                flexDirection: 'column',
                bgcolor: 'background.paper',
            }}>
                {/* Header */}
                <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                        Credentials ({displayProviders.length})
                    </Typography>
                </Box>

                {/* Vertical Navigation with Auth Type Grouping */}
                <Box
                    sx={{
                        flex: 1,
                        overflowY: 'auto',
                        '&::-webkit-scrollbar': {
                            width: 6,
                        },
                        '&::-webkit-scrollbar-thumb': {
                            bgcolor: 'divider',
                            borderRadius: 3,
                        },
                    }}
                >
                    {groupedProviders.map((group, groupIndex) => {
                        // Calculate starting index for this group
                        const groupStartIndex = groupedProviders
                            .slice(0, groupIndex)
                            .reduce((sum, g) => sum + g.providers.length, 0);

                        return (
                            <Box key={`group-${group.authType}`}>
                                {/* Auth Type Header */}
                                <Box
                                    sx={{
                                        px: 2,
                                        py: 1,
                                        borderBottom: 1,
                                        borderColor: 'divider',
                                        bgcolor: 'action.hover',
                                        position: 'sticky',
                                        top: 0,
                                        zIndex: 1,
                                    }}
                                >
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <AuthTypeBadge authType={group.authType} />
                                        <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                                            ({group.providers.length})
                                        </Typography>
                                    </Stack>
                                </Box>

                                {/* Provider Items */}
                                {group.providers.map((provider, providerIndex) => {
                                    const globalIndex = groupStartIndex + providerIndex;
                                    const isProviderSelected = selectedProvider === provider.uuid;
                                    const isSelectedTab = currentTab === globalIndex;

                                    return (
                                        <Box
                                            key={provider.uuid}
                                            onClick={() => handleTabChange(null as unknown as React.SyntheticEvent, globalIndex)}
                                            sx={{
                                                px: 2,
                                                py: 1.5,
                                                cursor: 'pointer',
                                                bgcolor: isSelectedTab ? 'action.selected' : 'transparent',
                                                borderLeft: 3,
                                                borderLeftColor: isSelectedTab ? 'primary.main' : 'transparent',
                                                '&:hover': {
                                                    bgcolor: isSelectedTab ? 'action.selected' : 'action.hover',
                                                },
                                                transition: 'all 0.2s',
                                            }}
                                        >
                                            <Stack direction="row" alignItems="center" spacing={1} sx={{ width: '100%', justifyContent: 'space-between' }}>
                                                <Stack direction="row" alignItems="center" spacing={1} sx={{ flex: 1, minWidth: 0 }}>
                                                    <Typography
                                                        variant="body2"
                                                        fontWeight={isSelectedTab ? 600 : 400}
                                                        color={isSelectedTab ? 'primary.main' : 'text.primary'}
                                                        noWrap
                                                    >
                                                        {provider.name}
                                                    </Typography>
                                                    {isProviderSelected && (
                                                        <CheckCircle color="primary" sx={{ fontSize: 14, flexShrink: 0 }} />
                                                    )}
                                                </Stack>
                                                {provider.api_style && (
                                                    <ApiStyleBadge compact={true} apiStyle={provider.api_style} sx={{ flexShrink: 0, width: "100px" }} />
                                                )}
                                            </Stack>
                                        </Box>
                                    );
                                })}
                            </Box>
                        );
                    })}
                </Box>
            </Box>

            {/* Right Panel - Tab Content */}
            <Box sx={{ flex: 1, overflowY: 'auto', p: 2 }}>
                {flattenedProviders.map((provider, index) => {
                const modelTypeInfo = getModelTypeInfo(provider, providerModels, customModels);
                const { standardModelsForDisplay, isCustomModel } = modelTypeInfo;

                const isProviderSelected = selectedProvider === provider.uuid; // Compare UUIDs
                const pagination = getPaginatedData(standardModelsForDisplay, provider.uuid);
                const isRefreshing = refreshingProviders.includes(provider.uuid); // Use UUID

                const backendCustomModel = providerModels?.[provider.uuid]?.custom_model;

                return (
                    <TabPanel key={provider.uuid} value={currentTab} index={index}> {/* Use UUID as key */}


                        {/* Models Display */}
                        <Stack spacing={2}>
                            {/* Star Models Section */}
                            {providerModels?.[provider.uuid]?.star_models && providerModels[provider.uuid].star_models!.length > 0 && (
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
                                        {providerModels[provider.uuid].star_models!.map((starModel) => (
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
                                            value={searchTerms[provider.uuid] || ''}
                                            onChange={(e) => handleSearchChange(provider.uuid, e.target.value)}
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
                                                minWidth: 110,
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
                                                minWidth: 110,
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
                                        {onTest && (
                                            <Button
                                                variant="outlined"
                                                startIcon={testing ? <CircularProgress size={16} /> : <PlayArrowIcon />}
                                                onClick={() => selectedModel && onTest(selectedModel)}
                                                disabled={!selectedModel || testing}
                                                sx={{
                                                    height: 40,
                                                    minWidth: 110,
                                                    borderColor: !selectedModel || testing ? 'grey.300' : 'primary.main',
                                                    color: !selectedModel || testing ? 'grey.500' : 'primary.main',
                                                    '&:hover': (!selectedModel || testing) ? {} : {
                                                        backgroundColor: 'primary.50',
                                                        borderColor: 'primary.dark',
                                                    },
                                                    '&:disabled': {
                                                        borderColor: 'grey.300',
                                                        color: 'grey.500',
                                                    }
                                                }}
                                            >
                                                {testing ? 'Testing...' : 'Test'}
                                            </Button>
                                        )}
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
                                    {customModels[provider.uuid]?.map((customModel, index) => (
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
                                        (!customModels[provider.uuid] || customModels[provider.uuid].length === 0) && (
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
                                        (!customModels[provider.uuid] || !customModels[provider.uuid].includes(selectedModel)) &&
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
                                    (!customModels[provider.uuid] || customModels[provider.uuid].length === 0) &&
                                    !backendCustomModel &&
                                    !(isProviderSelected && selectedModel && isCustomModel(selectedModel)) && (
                                        <Box sx={{ textAlign: 'center', py: 4 }}>
                                            <Typography variant="body2" color="text.secondary">
                                                No models found matching "{searchTerms[provider.uuid] || ''}"
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
                                            onClick={() => handlePageChange(provider.uuid, pagination.currentPage - 1)}
                                        >
                                            <NavigateBeforeIcon />
                                        </IconButton>
                                        <Typography variant="body2" sx={{ minWidth: 60, textAlign: 'center' }}>
                                            {pagination.currentPage} / {pagination.totalPages}
                                        </Typography>
                                        <IconButton
                                            size="small"
                                            disabled={pagination.currentPage === pagination.totalPages}
                                            onClick={() => handlePageChange(provider.uuid, pagination.currentPage + 1)}
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
            </Box>

            {/* Custom Model Dialog */}
            <Dialog
                open={customModelDialog.open}
                onClose={handleCustomModelCancel}
                maxWidth="sm"
                fullWidth
            >
                <DialogTitle>
                    {customModelDialog.originalValue ? 'Edit Custom Model' : 'Add Custom Model'}
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
                        {customModelDialog.originalValue ? 'Update' : 'Add'}
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