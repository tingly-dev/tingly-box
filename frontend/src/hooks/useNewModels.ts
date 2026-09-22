import { useCallback, useEffect } from 'react';
import { useLocalStorage } from './useLocalStorage';
import { createEventSystem } from '../utils/eventSystem';

// Local storage key for new models
const NEW_MODELS_STORAGE_KEY = 'tingly_new_models';
const DEFAULT_NEW_MODELS = {};

// Type definition for new models diff
interface NewModelsDiff {
    newModels: string[];
    removedModels?: string[];
    timestamp: string;
}

// Type for the entire storage structure
type NewModelsData = { [providerUuid: string]: NewModelsDiff };

// Event system for new models updates — dispatching from one hook instance
// makes other mounted instances refetch.
const newModelsEvent = createEventSystem<{ providerUuid: string; diff: NewModelsDiff | null }>(
    'tingly_new_models_update'
);

// Custom hook to manage new models
export const useNewModels = () => {
    const { data: newModels, saveData, removeKey, setData, refetch } =
        useLocalStorage<NewModelsData>(NEW_MODELS_STORAGE_KEY, DEFAULT_NEW_MODELS);

    // Listen for new models updates from other components and reload
    useEffect(() => {
        const cleanup = newModelsEvent.listen(() => {
            refetch();
        });
        return cleanup;
    }, [refetch]);

    // Clear new models for a specific provider
    const clearNewModels = useCallback((providerUuid: string) => {
        if (removeKey(providerUuid)) {
            setData(prev => {
                const newModelsData = { ...prev };
                delete newModelsData[providerUuid];
                return newModelsData;
            });
            newModelsEvent.dispatch({ providerUuid, diff: null });
        }
    }, [removeKey, setData]);

    // Detect and store new models after a refresh
    const detectAndStoreNewModels = useCallback((
        providerUuid: string,
        oldModels: string[],
        newModelsList: string[]
    ) => {
        if (!oldModels || oldModels.length === 0) {
            // First time loading, don't treat all as new
            return;
        }

        const oldSet = new Set(oldModels);
        const newSet = new Set(newModelsList);

        // Find newly added models
        const addedModels = newModelsList.filter(m => !oldSet.has(m));

        // Get existing new models for this provider
        const existingDiff = newModels[providerUuid];
        const existingNewModels = existingDiff?.newModels || [];

        // Filter out existing new models that no longer exist in the current model list
        const stillExistingNewModels = existingNewModels.filter(m => newSet.has(m));

        // Merge existing new models (that still exist) with newly detected ones (avoid duplicates)
        const mergedNewModels = Array.from(new Set([...stillExistingNewModels, ...addedModels]));

        // Only update if there are new models to show
        if (mergedNewModels.length > 0) {
            const diff: NewModelsDiff = {
                newModels: mergedNewModels,
                timestamp: existingDiff?.timestamp || new Date().toISOString(),
            };

            if (saveData(providerUuid, diff)) {
                setData(prev => ({ ...prev, [providerUuid]: diff }));
                newModelsEvent.dispatch({ providerUuid, diff });
            }
        } else {
            // No new models left (all were removed), clear the entry
            clearNewModels(providerUuid);
        }
    }, [newModels, clearNewModels, saveData, setData]);

    return {
        newModels,
        detectAndStoreNewModels,
        clearNewModels,
    };
};
