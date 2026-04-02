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
        // Proceed directly with selection without validation
        if (onSelected) {
            onSelected({ provider, model });
        }
        // Track recent model
        addRecentModel(provider.uuid, model);
    }, [onSelected, addRecentModel]);

    return { handleModelSelect };
}
