import { useCallback } from 'react';
import { api } from '../services/api';
import type { Provider } from '../types/provider';
import { useModelSelectContext } from '../contexts/ModelSelectContext';
import { useRecentModels } from './useRecentModels';

export interface ModelSelectionHandlerProps {
    onSelected?: (option: { provider: Provider; model: string }) => void;
}

export function useModelSelection({ onSelected }: ModelSelectionHandlerProps) {
    const { addProbingModel, removeProbingModel, isModelProbing, showSnackbar } = useModelSelectContext();
    const { addRecentModel } = useRecentModels();

    const handleModelSelect = useCallback(async (provider: Provider, model: string) => {
        // Check if provider is oauth type
        if (provider.auth_type === 'oauth') {
            const modelKey = `${provider.uuid}-${model}`;
            // Check if already probing
            if (isModelProbing(modelKey)) {
                return;
            }

            // Add to probing set
            addProbingModel(modelKey);

            try {
                // Probe model availability
                const result = await api.probeModel(provider.uuid, model);

                // Remove from probing set
                removeProbingModel(modelKey);

                // Check if probe was successful
                if (result?.success === false || result?.error) {
                    showSnackbar(
                        `Model "${model}" is not available: ${result.error?.message || 'Unknown error'}`,
                        'error'
                    );
                    return; // Don't proceed with selection
                }

                // Success - proceed with selection
                if (onSelected) {
                    onSelected({ provider, model });
                }
                // Track recent model
                addRecentModel(provider.uuid, model);
            } catch (error: any) {
                // Remove from probing set
                removeProbingModel(modelKey);

                showSnackbar(
                    `Model "${model}" is not available: ${error || 'Network error'}`,
                    'error'
                );
            }
        } else {
            // Non-oauth provider - proceed directly
            if (onSelected) {
                onSelected({ provider, model });
            }
            // Track recent model
            addRecentModel(provider.uuid, model);
        }
    }, [onSelected, addProbingModel, removeProbingModel, isModelProbing, showSnackbar, addRecentModel]);

    return { handleModelSelect };
}
