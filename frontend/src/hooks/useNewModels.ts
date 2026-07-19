import { useCallback, useEffect } from 'react';
import { useLocalStorage } from './useLocalStorage';
import { createEventSystem } from '../utils/eventSystem';

// Local storage key for new models
const NEW_MODELS_STORAGE_KEY = 'tingly_new_models';
const DEFAULT_NEW_MODELS = {};

// Type definition for new models diff
export interface NewModelsDiff {
    newModels: string[];
    removedModels?: string[];
    timestamp: string;
}

// Type for the entire storage structure
export type NewModelsData = { [providerUuid: string]: NewModelsDiff };

// Event system for new models updates
const newModelsEvent = createEventSystem<{ providerUuid: string; diff: NewModelsDiff | null }>(
    'tingly_new_models_update'
);

// Export event name for backward compatibility
export const NEW_MODELS_UPDATE_EVENT = newModelsEvent.eventName;

// Custom hook to manage new models
export const useNewModels = () => {
    const { data: newModels, version, saveData, removeKey, setData, refetch } =
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

    // Get new models diff for a specific provider
    const getNewModels = useCallback((providerUuid: string): NewModelsDiff | undefined => {
        return newModels[providerUuid];
    }, [newModels]);

    // Helper functions for backward compatibility
    const loadNewModelsFromStorage = useCallback(() => {
        const stored = localStorage.getItem(NEW_MODELS_STORAGE_KEY);
        return stored ? JSON.parse(stored) : {};
    }, []);

    const saveNewModelsToStorage = useCallback((providerUuid: string, diff: NewModelsDiff) => {
        return saveData(providerUuid, diff);
    }, [saveData]);

    const removeNewModelsFromStorage = useCallback((providerUuid: string) => {
        return removeKey(providerUuid);
    }, [removeKey]);

    // Helper to dispatch new models update event (backward compatibility)
    const dispatchNewModelsUpdate = (providerUuid: string, diff: NewModelsDiff | null) => {
        newModelsEvent.dispatch({ providerUuid, diff });
    };

    // Helper to listen for new models updates (backward compatibility)
    const listenForNewModelsUpdates = (callback: (providerUuid: string, diff: NewModelsDiff | null) => void) => {
        return newModelsEvent.listen((data) => {
            if (!data) return;
            const { providerUuid, diff } = data;
            callback(providerUuid, diff);
        });
    };

    return {
        newModels,
        version,
        refetch,
        detectAndStoreNewModels,
        getNewModels,
        clearNewModels,
        loadNewModelsFromStorage,
        saveNewModelsToStorage,
        removeNewModelsFromStorage,
        dispatchNewModelsUpdate,
        listenForNewModelsUpdates,
    };
};
