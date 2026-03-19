import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import NavigateBeforeIcon from '@mui/icons-material/NavigateBefore';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import BuildIcon from '@mui/icons-material/Build';
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
import api from '@/services/api';

const TOOL_SUPPORT_STORAGE_KEY = 'tingly_tool_support_by_provider';

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
    const { isModelProbing, refreshTrigger, showSnackbar } = useModelSelectContext();
    const { recentModels } = useRecentModels();
    const { newModels, clearNewModels } = useNewModels();
    const [toolSupportByModel, setToolSupportByModel] = useState<Record<string, boolean>>({});
    const [toolProbing, setToolProbing] = useState(false);

    const loadToolSupportForProvider = useCallback((providerUUID: string): Record<string, boolean> => {
        try {
            const raw = localStorage.getItem(TOOL_SUPPORT_STORAGE_KEY);
            if (!raw) return {};
            const parsed = JSON.parse(raw) as Record<string, string[]>;
            const models = Array.isArray(parsed?.[providerUUID]) ? parsed[providerUUID] : [];
            return models.reduce<Record<string, boolean>>((acc, model) => {
                if (typeof model === 'string' && model.trim() !== '') acc[model] = true;
                return acc;
            }, {});
        } catch {
            return {};
        }
    }, []);

    const persistToolSupportForProvider = useCallback((providerUUID: string, model: string) => {
        if (!model) return;
        try {
            const raw = localStorage.getItem(TOOL_SUPPORT_STORAGE_KEY);
            const parsed = raw ? JSON.parse(raw) as Record<string, string[]> : {};
            const current = Array.isArray(parsed[providerUUID]) ? parsed[providerUUID] : [];
            if (!current.includes(model)) {
                parsed[providerUUID] = [...current, model];
                localStorage.setItem(TOOL_SUPPORT_STORAGE_KEY, JSON.stringify(parsed));
            }
        } catch {
            // ignore storage failures
        }
    }, []);

    // Re-fetch provider models when refresh trigger changes (e.g., after custom model deletion)
    useEffect(() => {
        if (refreshTrigger > 0) {
            fetchModels(provider.uuid);
        }
    }, [refreshTrigger, provider.uuid, fetchModels]);

    useEffect(() => {
        setToolSupportByModel(loadToolSupportForProvider(provider.uuid));
    }, [provider.uuid, loadToolSupportForProvider]);

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

    const handleProbeToolSupport = useCallback(async () => {
        if (!selectedModel || toolProbing) return;
        setToolProbing(true);
        try {
            const result = await api.probeModelCapability(provider.uuid, selectedModel, true);
            const supported = result?.data?.tool_parser_endpoint?.available;
            const reason = result?.data?.tool_parser_endpoint?.error_message;
            if (supported) {
                setToolSupportByModel(prev => ({ ...prev, [selectedModel]: true }));
                persistToolSupportForProvider(provider.uuid, selectedModel);
                showSnackbar(`Tool parser supported for ${selectedModel}`, 'success');
            } else {
                showSnackbar(
                    reason
                        ? `Tool parser not supported for ${selectedModel}: ${reason}`
                        : `Tool parser not supported for ${selectedModel}`,
                    'error'
                );
            }
        } catch {
            showSnackbar(`Tool parser probe failed for ${selectedModel}`, 'error');
        } finally {
            setToolProbing(false);
        }
    }, [provider.uuid, selectedModel, toolProbing, showSnackbar, persistToolSupportForProvider]);

    const toolSupportSet = useMemo(() => toolSupportByModel, [toolSupportByModel]);

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

    return (
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
                        <Button
                            variant="outlined"
                            startIcon={toolProbing ? <CircularProgress size={16} /> : <BuildIcon />}
                            onClick={handleProbeToolSupport}
                            disabled={!selectedModel || toolProbing}
                            sx={{ height: 40, minWidth: 140 }}
                        >
                            {toolProbing ? 'Probing...' : 'Probe Tool Parser'}
                        </Button>
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
                                    loading={provider.auth_type === 'oauth' && isModelProbing(`${provider.uuid}-${starModel}`)}
                                    showToolSupport={!!toolSupportSet[starModel]}
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
                                        loading={provider.auth_type === 'oauth' && isModelProbing(`${provider.uuid}-${model}`)}
                                        showToolSupport={!!toolSupportSet[model]}
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
                                        loading={provider.auth_type === 'oauth' && isModelProbing(`${provider.uuid}-${model}`)}
                                        showToolSupport={!!toolSupportSet[model]}
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
    );
}

export default ModelsPanel;
