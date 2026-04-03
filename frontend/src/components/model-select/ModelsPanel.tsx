import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import NavigateBeforeIcon from '@mui/icons-material/NavigateBefore';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import RefreshIcon from '@mui/icons-material/Refresh';
import SearchIcon from '@mui/icons-material/Search';
import BugReportIcon from '@mui/icons-material/BugReport';
import {
    Box,
    Button,
    CircularProgress,
    IconButton,
    InputAdornment,
    Stack,
    TextField,
    Typography,
    Divider,
} from '@mui/material';
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import type { Provider } from '@/types/provider';
import { getModelTypeInfo } from '@/utils/modelUtils';
import { useCustomModels } from '@/hooks/useCustomModels';
import { useProviderModels } from '@/hooks/useProviderModels';
import { usePagination } from '@/hooks/usePagination';
import { useModelSelectContext } from '@/contexts/ModelSelectContext';
import { useRecentModels } from '@/hooks/useRecentModels';
import { useNewModels } from '@/hooks/useNewModels';
import CustomModelCard from './CustomModelCard';
import ModelCard from './ModelCard';
import RecentModelsSection from './RecentModelsSection';
import NewModelsSection from './NewModelsSection';
import { QuotaBar } from './QuotaBar';

async function fetchUIAPI(url: string, options: RequestInit = {}): Promise<any> {
    const basePath = window.location.origin;
    const fullUrl = `${basePath}/api/v1${url}`;

    const token = localStorage.getItem('user_auth_token');

    const response = await fetch(fullUrl, {
        ...options,
        headers: {
            'Content-Type': 'application/json',
            ...(token && { Authorization: `Bearer ${token}` }),
            ...options.headers,
        },
    });

    if (!response.ok) {
        throw new Error(`API error: ${response.status}`);
    }

    return response.json();
}

export interface ModelsPanelProps {
    provider: Provider;
    selectedProvider?: string;
    selectedModel?: string;
    columns: number;
    modelsPerPage: number;
    onModelSelect: (provider: Provider, model: string) => void;
    onCustomModelEdit: (provider: Provider, value?: string) => void;
    onCustomModelDelete: (provider: Provider, customModel: string) => void;
    onTest?: (model: string) => void;
    testing?: boolean;
}

export function ModelsPanel({
    provider,
    selectedProvider,
    selectedModel,
    columns,
    modelsPerPage,
    onModelSelect,
    onCustomModelEdit,
    onCustomModelDelete,
    onTest,
    testing = false,
}: ModelsPanelProps) {
    const { customModels } = useCustomModels();
    const { providerModels, refreshingProviders, refreshModels, fetchModels } = useProviderModels();
    const { refreshTrigger } = useModelSelectContext();
    const { recentModels } = useRecentModels();
    const { newModels, clearNewModels } = useNewModels();

    // Quota refresh state
    const [isRefreshingQuota, setIsRefreshingQuota] = useState(false);

    // Get quota data for this provider
    const providerQuota = useMemo(() => {
        return providerModels?.[provider.uuid]?.quota;
    }, [providerModels, provider.uuid]);

    // Prepare quota prop for ModelCard
    const quotaProp = useMemo(() => {
        return providerQuota;  // Pass full quota object, QuotaBar will handle breakdowns
    }, [providerQuota]);

    // Re-fetch provider models when refresh trigger changes (e.g., after custom model deletion)
    useEffect(() => {
        if (refreshTrigger > 0) {
            fetchModels(provider.uuid);
        }
    }, [refreshTrigger, provider.uuid, fetchModels]);

    const isProviderSelected = selectedProvider === provider.uuid;
    const isRefreshing = refreshingProviders.includes(provider.uuid);
    const backendCustomModel = providerModels?.[provider.uuid]?.custom_model;

    // Get custom models for this provider
    const providerCustomModels = customModels[provider.uuid] || [];

    // Get model type info
    const modelTypeInfo = getModelTypeInfo(provider, providerModels, customModels);
    const { standardModelsForDisplay, isCustomModel } = modelTypeInfo;

    // Consolidate all custom models from different sources with proper deduplication
    // Sources: localStorage (providerCustomModels), backend (backendCustomModel), selected model
    const customModelsSet = new Set<string>();

    // Add from localStorage
    providerCustomModels.forEach(model => customModelsSet.add(model));

    // Add from backend if not already present (only when no localStorage models exist)
    if (backendCustomModel && providerCustomModels.length === 0) {
        customModelsSet.add(backendCustomModel);
    }

    // Add currently selected model if it's a custom model not in any other source
    if (isProviderSelected && selectedModel && isCustomModel(selectedModel) &&
        !providerCustomModels.includes(selectedModel) &&
        selectedModel !== backendCustomModel) {
        customModelsSet.add(selectedModel);
    }

    // Combine all models for unified pagination: custom models first, then standard models
    const allModels = [
        ...Array.from(customModelsSet).map(model => ({ model, type: 'custom' as const })),
        ...standardModelsForDisplay.map(model => ({ model, type: 'standard' as const })),
    ];

    // Pagination and search
    const { searchTerms, handleSearchChange, getPaginatedData, setCurrentPage } = usePagination(
        [provider.uuid],
        modelsPerPage
    );

    // Filter by search term
    const searchTerm = searchTerms[provider.uuid] || '';
    const filteredModels = searchTerm
        ? allModels.filter(({ model }) => model.toLowerCase().includes(searchTerm.toLowerCase()))
        : allModels;

    const pagination = getPaginatedData(filteredModels.map(m => m.model), provider.uuid);
    const paginatedItems = filteredModels.slice(
        (pagination.currentPage - 1) * modelsPerPage,
        pagination.currentPage * modelsPerPage
    );

    const handlePageChange = useCallback((newPage: number) => {
        setCurrentPage(prev => ({ ...prev, [provider.uuid]: newPage }));
    }, [provider.uuid, setCurrentPage]);

    // Dev mode test handler: Simulate removing some models to test "new models" feature
    const handleDevTestRemoveModels = useCallback(async () => {
        const currentModels = providerModels[provider.uuid]?.models || [];
        if (currentModels.length < 2) {
            alert('Not enough models to test with');
            return;
        }

        // Randomly select 2-3 models to mark as "new" for testing
        const shuffled = [...currentModels].sort(() => Math.random() - 0.5);
        const modelsToMarkAsNew = shuffled.slice(0, Math.min(3, shuffled.length));

        // Manually set them in new_models storage for testing
        const storage = localStorage.getItem('tingly_new_models');
        const data = storage ? JSON.parse(storage) : {};
        data[provider.uuid] = {
            newModels: modelsToMarkAsNew,
            timestamp: new Date().toISOString(),
        };
        localStorage.setItem('tingly_new_models', JSON.stringify(data));

        // Force reload of new models
        window.dispatchEvent(new CustomEvent('tingly_new_models_update', {
            detail: { providerUuid: provider.uuid, diff: data[provider.uuid] }
        }));
    }, [provider.uuid, providerModels]);

    // Refresh quota for this provider
    const refreshQuota = useCallback(async (providerUuid: string) => {
        setIsRefreshingQuota(true);
        try {
            await fetchUIAPI(`/provider-quota/${providerUuid}/refresh`, {
                method: 'POST',
            });
            // Refresh provider models to get updated quota
            await fetchModels(providerUuid);
        } catch (error) {
            console.error('Failed to refresh quota:', error);
        } finally {
            setIsRefreshingQuota(false);
        }
    }, [fetchModels]);

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            {/* Scrollable content area */}
            <Box sx={{ flex: 1, overflowY: 'auto', p: 2 }}>
                <Stack spacing={2}>
                {/* Controls */}
                <Stack direction="row" justifyContent="space-between" alignItems="center">
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
                            sx={{ width: 200 }}
                        />
                        <Button
                            variant="outlined"
                            startIcon={<AddCircleOutlineIcon />}
                            onClick={() => onCustomModelEdit(provider)}
                            sx={{ height: 40, minWidth: 100 }}
                        >
                            Customize
                        </Button>
                        <Button
                            variant="outlined"
                            startIcon={isRefreshing ? <CircularProgress size={16} /> : <RefreshIcon />}
                            onClick={() => refreshModels(provider.uuid)}
                            disabled={isRefreshing}
                            sx={{ height: 40, minWidth: 100 }}
                        >
                            {isRefreshing ? 'Fetching...' : 'Refresh'}
                        </Button>
                        {import.meta.env.DEV && (
                            <Button
                                variant="outlined"
                                color="warning"
                                startIcon={<BugReportIcon />}
                                onClick={handleDevTestRemoveModels}
                                sx={{ height: 40, minWidth: 100 }}
                                title="Dev: Randomly mark some models as 'new' for testing"
                            >
                                Test New
                            </Button>
                        )}
                        {onTest && (
                            <Button
                                variant="outlined"
                                startIcon={testing ? <CircularProgress size={16} /> : <PlayArrowIcon />}
                                onClick={() => selectedModel && onTest(selectedModel)}
                                disabled={!selectedModel || testing}
                                sx={{ height: 40, minWidth: 80 }}
                            >
                                {testing ? 'Testing...' : 'Test'}
                            </Button>
                        )}
                    </Stack>
                    <Typography variant="caption" color="text.secondary">
                        {pagination.totalItems} models
                    </Typography>
                </Stack>

                {/* New Models Section */}
                {newModels[provider.uuid]?.newModels && newModels[provider.uuid].newModels.length > 0 && (
                    <NewModelsSection
                        providerUuid={provider.uuid}
                        newModels={newModels[provider.uuid].newModels}
                        selectedModel={isProviderSelected ? selectedModel : undefined}
                        onModelSelect={(model) => onModelSelect(provider, model)}
                        onDismiss={() => clearNewModels(provider.uuid)}
                        columns={columns}
                    />
                )}

                {/* Recent Models Section */}
                {recentModels[provider.uuid]?.length > 0 && (
                    <RecentModelsSection
                        providerUuid={provider.uuid}
                        recentModels={recentModels[provider.uuid]}
                        selectedModel={isProviderSelected ? selectedModel : undefined}
                        onModelSelect={(model) => onModelSelect(provider, model)}
                        columns={columns}
                    />
                )}

                {/* Star Models Section */}
                {providerModels?.[provider.uuid]?.star_models && providerModels[provider.uuid].star_models!.length > 0 && (
                    <Box>
                        <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>
                            Starred Models
                        </Typography>
                        <Box sx={{ display: 'grid', gridTemplateColumns: `repeat(${columns}, 1fr)`, gap: 0.8 }}>
                            {providerModels[provider.uuid].star_models!.map((starModel) => (
                                <ModelCard
                                    key={starModel}
                                    model={starModel}
                                    isSelected={isProviderSelected && selectedModel === starModel}
                                    onClick={() => onModelSelect(provider, starModel)}
                                    variant="starred"
                                />
                            ))}
                        </Box>
                    </Box>
                )}

                {/* All Models Section */}
                <Box sx={{ minHeight: 200 }}>
                    <Box sx={{ display: 'grid', gridTemplateColumns: `repeat(${columns}, 1fr)`, gap: 0.8 }}>
                        {paginatedItems.map(({ model, type }) => {
                            const isModelSelected = isProviderSelected && selectedModel === model;

                            if (type === 'custom') {
                                // Determine variant based on custom model source
                                let variant: 'localStorage' | 'backend' | 'selected' = 'localStorage';
                                if (model === backendCustomModel && providerCustomModels.length === 0) {
                                    variant = 'backend';
                                } else if (isProviderSelected && model === selectedModel &&
                                    !providerCustomModels.includes(model) && model !== backendCustomModel) {
                                    variant = 'selected';
                                }

                                return (
                                    <CustomModelCard
                                        key={model}
                                        model={model}
                                        provider={provider}
                                        isSelected={isModelSelected}
                                        onEdit={() => onCustomModelEdit(provider, model)}
                                        onDelete={() => onCustomModelDelete(provider, model)}
                                        onSelect={() => onModelSelect(provider, model)}
                                        variant={variant}
                                    />
                                );
                            } else {
                                return (
                                    <ModelCard
                                        key={model}
                                        model={model}
                                        isSelected={isModelSelected}
                                        onClick={() => onModelSelect(provider, model)}
                                        variant="standard"
                                    />
                                );
                            }
                        })}
                    </Box>

                    {/* Empty state */}
                    {paginatedItems.length === 0 && (
                        <Box sx={{ textAlign: 'center', py: 4 }}>
                            <Typography variant="body2" color="text.secondary">
                                No models found matching "{searchTerm}"
                            </Typography>
                        </Box>
                    )}
                </Box>

                {/* Pagination Controls */}
                {pagination.totalPages > 1 && (
                    <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                        <Stack direction="row" alignItems="center" spacing={1}>
                            <IconButton
                                size="small"
                                disabled={pagination.currentPage === 1}
                                onClick={() => handlePageChange(pagination.currentPage - 1)}
                            >
                                <NavigateBeforeIcon />
                            </IconButton>
                            <Typography variant="body2" sx={{ minWidth: 60, textAlign: 'center' }}>
                                {pagination.currentPage} / {pagination.totalPages}
                            </Typography>
                            <IconButton
                                size="small"
                                disabled={pagination.currentPage === pagination.totalPages}
                                onClick={() => handlePageChange(pagination.currentPage + 1)}
                            >
                                <NavigateNextIcon />
                            </IconButton>
                        </Stack>
                    </Box>
                )}
            </Stack>
            </Box>

            {/* Provider Quota Bars - fixed at the bottom */}
            {quotaProp && quotaProp.primary && (
                <Box sx={{ p: 2, pt: 1, borderTop: '1px solid', borderColor: 'divider' }}>
                    <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.5 }}>
                        <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
                            Provider Quota
                        </Typography>
                        <IconButton
                            size="small"
                            onClick={() => refreshQuota(provider.uuid)}
                            disabled={isRefreshingQuota}
                            sx={{
                                p: 0.5,
                                color: 'text.primary',
                                '&:hover': {
                                    bgcolor: 'action.hover',
                                },
                            }}
                            title="Refresh quota"
                        >
                            <RefreshIcon
                                sx={{
                                    fontSize: 16,
                                    ...(isRefreshingQuota && {
                                        '@keyframes spin': {
                                            '0%': { transform: 'rotate(0deg)' },
                                            '100%': { transform: 'rotate(360deg)' },
                                        },
                                        animation: 'spin 1s linear infinite',
                                    }),
                                }}
                            />
                        </IconButton>
                    </Stack>
                    <Stack spacing={1.5}>
                        {/* Primary quota */}
                        <Box>
                            <Typography variant="caption" sx={{ mb: 0.5, display: 'block', color: '#64748b' }}>
                                {quotaProp.primary.label}
                            </Typography>
                            <QuotaBar quota={quotaProp} windowIndex={0} />
                        </Box>

                        {/* Secondary quota */}
                        {quotaProp.secondary && (
                            <Box>
                                <Typography variant="caption" sx={{ mb: 0.5, display: 'block', color: '#64748b' }}>
                                    {quotaProp.secondary.label}
                                </Typography>
                                <QuotaBar quota={quotaProp} windowIndex={1} />
                            </Box>
                        )}

                        {/* Tertiary quota */}
                        {quotaProp.tertiary && (
                            <Box>
                                <Typography variant="caption" sx={{ mb: 0.5, display: 'block', color: '#64748b' }}>
                                    {quotaProp.tertiary.label}
                                </Typography>
                                <QuotaBar quota={quotaProp} windowIndex={2} />
                            </Box>
                        )}
                    </Stack>
                </Box>
            )}
        </Box>
    );
}

export default ModelsPanel;
