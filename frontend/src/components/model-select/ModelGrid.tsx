import { Box, Grid, Typography } from '@mui/material';
import React, { useMemo } from 'react';
import type { Provider } from '../../types/provider';
import { getModelTypeInfo } from '../../utils/modelUtils';
import { useCustomModels } from '../../hooks/useCustomModels';
import { useModelSelectContext } from '../../contexts/ModelSelectContext';
import CustomModelCard from './CustomModelCard.tsx';
import ModelCard from './ModelCard.tsx';

export interface ModelGridProps {
    provider: Provider;
    providerModels?: { [uuid: string]: { star_models?: string[]; custom_model?: string } };
    selectedProvider?: string;
    selectedModel?: string;
    onModelSelect: (provider: Provider, model: string) => void;
    onCustomModelEdit: (provider: Provider, value?: string) => void;
    onCustomModelDelete: (provider: Provider, customModel: string) => void;
    columns: number;
    searchTerms: { [uuid: string]: string };
    paginatedModels: {
        items: string[];
        currentPage: number;
        totalPages: number;
        totalItems: number;
    };
}

export function ModelGrid({
    provider,
    providerModels,
    selectedProvider,
    selectedModel,
    onModelSelect,
    onCustomModelEdit,
    onCustomModelDelete,
    columns,
    searchTerms,
    paginatedModels,
}: ModelGridProps) {
    const { customModels } = useCustomModels();
    const { isModelProbing } = useModelSelectContext();

    // Memoize model type info to avoid unnecessary recalculations
    const modelTypeInfo = useMemo(
        () => getModelTypeInfo(provider, providerModels, customModels),
        [provider, providerModels, customModels]
    );
    const { standardModelsForDisplay, isCustomModel } = modelTypeInfo;

    // Memoize computed values
    const isProviderSelected = useMemo(
        () => selectedProvider === provider.uuid,
        [selectedProvider, provider.uuid]
    );

    const backendCustomModel = useMemo(
        () => providerModels?.[provider.uuid]?.custom_model,
        [providerModels, provider.uuid]
    );

    const searchTerm = searchTerms[provider.uuid] || '';

    const providerCustomModels = useMemo(
        () => customModels[provider.uuid] || [],
        [customModels, provider.uuid]
    );

    return (
        <>
            {/* Star Models Section */}
            {providerModels?.[provider.uuid]?.star_models && providerModels[provider.uuid].star_models!.length > 0 && (
                <Box>
                    <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>
                        Starred Models
                    </Typography>
                    <Box
                        sx={{
                            display: 'grid',
                            gridTemplateColumns: `repeat(${columns}, 1fr)`,
                            gap: 0.8,
                        }}
                    >
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
                <Box
                    sx={{
                        display: 'grid',
                        gridTemplateColumns: `repeat(${columns}, 1fr)`,
                        gap: 0.8,
                    }}
                >
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
                    {backendCustomModel &&
                        providerCustomModels.length === 0 && (
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
                    {paginatedModels.items.map((model) => {
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
                {paginatedModels.totalItems === 0 &&
                    providerCustomModels.length === 0 &&
                    !backendCustomModel &&
                    !(isProviderSelected && selectedModel && isCustomModel(selectedModel)) && (
                        <Box sx={{ textAlign: 'center', py: 4 }}>
                            <Typography variant="body2" color="text.secondary">
                                No models found matching "{searchTerm}"
                            </Typography>
                        </Box>
                    )}
            </Box>
        </>
    );
}

// Memoize the component to prevent unnecessary re-renders
const MemoizedModelGrid = React.memo(ModelGrid);
export default MemoizedModelGrid;
export { ModelGrid };
