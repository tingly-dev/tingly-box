import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import NavigateBeforeIcon from '@mui/icons-material/NavigateBefore';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import RefreshIcon from '@mui/icons-material/Refresh';
import SearchIcon from '@mui/icons-material/Search';
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
import React, { useCallback } from 'react';
import type { Provider } from '../../types/provider';
import { getModelTypeInfo } from '../../utils/modelUtils';
import { useCustomModels } from '../../hooks/useCustomModels';
import { useProviderModels } from '../../hooks/useProviderModels';
import { usePagination } from '../../hooks/usePagination';
import { useModelSelectContext } from '../../contexts/ModelSelectContext';
import CustomModelCard from './CustomModelCard';
import ModelCard from './ModelCard';

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
    const { providerModels, refreshingProviders, refreshModels } = useProviderModels();
    const { isModelProbing } = useModelSelectContext();

    const isProviderSelected = selectedProvider === provider.uuid;
    const isRefreshing = refreshingProviders.includes(provider.uuid);
    const backendCustomModel = providerModels?.[provider.uuid]?.custom_model;

    // Get custom models for this provider
    const providerCustomModels = customModels[provider.uuid] || [];

    // Get model type info
    const modelTypeInfo = getModelTypeInfo(provider, providerModels, customModels);
    const { standardModelsForDisplay, isCustomModel } = modelTypeInfo;

    // Pagination and search
    const { searchTerms, handleSearchChange, getPaginatedData, setCurrentPage } = usePagination(
        [provider.uuid],
        modelsPerPage
    );

    const pagination = getPaginatedData(standardModelsForDisplay, provider.uuid);

    const handlePageChange = useCallback((newPage: number) => {
        setCurrentPage(prev => ({ ...prev, [provider.uuid]: newPage }));
    }, [provider.uuid, setCurrentPage]);

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
                                />
                            ))}
                        </Box>
                    </Box>
                )}

                {/* All Models Section */}
                <Box sx={{ minHeight: 200 }}>
                    <Box sx={{ display: 'grid', gridTemplateColumns: `repeat(${columns}, 1fr)`, gap: 0.8 }}>
                        {/* Custom models from local storage */}
                        {providerCustomModels.map((customModel, index) => (
                            <CustomModelCard
                                key={`localStorage-custom-model-${index}`}
                                model={customModel}
                                provider={provider}
                                isSelected={isProviderSelected && selectedModel === customModel}
                                onEdit={() => onCustomModelEdit(provider, customModel)}
                                onDelete={() => onCustomModelDelete(provider, customModel)}
                                onSelect={() => onModelSelect(provider, customModel)}
                                variant="localStorage"
                                loading={provider.auth_type === 'oauth' && isModelProbing(`${provider.uuid}-${customModel}`)}
                            />
                        ))}

                        {/* Persisted custom model card (from backend) */}
                        {backendCustomModel && providerCustomModels.length === 0 && (
                            <CustomModelCard
                                key="persisted-custom-model"
                                model={backendCustomModel}
                                provider={provider}
                                isSelected={isProviderSelected && selectedModel === backendCustomModel}
                                onEdit={() => onCustomModelEdit(provider, backendCustomModel)}
                                onDelete={() => onCustomModelDelete(provider, backendCustomModel)}
                                onSelect={() => onModelSelect(provider, backendCustomModel)}
                                variant="backend"
                                loading={provider.auth_type === 'oauth' && isModelProbing(`${provider.uuid}-${backendCustomModel}`)}
                            />
                        )}

                        {/* Currently selected custom model card (not persisted) */}
                        {isProviderSelected && selectedModel && isCustomModel(selectedModel) &&
                            !providerCustomModels.includes(selectedModel) &&
                            selectedModel !== backendCustomModel && (
                                <CustomModelCard
                                    key="selected-custom-model"
                                    model={selectedModel}
                                    provider={provider}
                                    isSelected={true}
                                    onEdit={() => onCustomModelEdit(provider, selectedModel)}
                                    onDelete={() => onCustomModelDelete(provider, selectedModel)}
                                    onSelect={() => onModelSelect(provider, selectedModel)}
                                    variant="selected"
                                    loading={provider.auth_type === 'oauth' && isModelProbing(`${provider.uuid}-${selectedModel}`)}
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
                                    onClick={() => onModelSelect(provider, model)}
                                    variant="standard"
                                    loading={provider.auth_type === 'oauth' && isModelProbing(`${provider.uuid}-${model}`)}
                                />
                            );
                        })}
                    </Box>

                    {/* Empty state */}
                    {pagination.totalItems === 0 &&
                        providerCustomModels.length === 0 &&
                        !backendCustomModel &&
                        !(isProviderSelected && selectedModel && isCustomModel(selectedModel)) && (
                            <Box sx={{ textAlign: 'center', py: 4 }}>
                                <Typography variant="body2" color="text.secondary">
                                    No models found matching "{searchTerms[provider.uuid] || ''}"
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
