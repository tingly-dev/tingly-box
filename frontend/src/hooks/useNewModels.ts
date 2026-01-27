import { useEffect, useState, useCallback } from 'react';

// Local storage key for new models
const NEW_MODELS_STORAGE_KEY = 'tingly_new_models';

// Export event name for new models updates
export const NEW_MODELS_UPDATE_EVENT = 'tingly_new_models_update';

// Type definition for new models diff
export interface NewModelsDiff {
    newModels: string[];
    removedModels?: string[];
    timestamp: string;
}

// Type for the entire storage structure
export type NewModelsData = { [providerUuid: string]: NewModelsDiff };

// Helper functions to manage new models in local storage
export const loadNewModelsFromStorage = (): NewModelsData => {
    try {
        const stored = localStorage.getItem(NEW_MODELS_STORAGE_KEY);
        return stored ? JSON.parse(stored) : {};
    } catch (error) {
        console.error('Failed to load new models from storage:', error);
        return {};
    }
};

export const saveNewModelsToStorage = (providerUuid: string, diff: NewModelsDiff) => {
    try {
        const newModels = loadNewModelsFromStorage();
        newModels[providerUuid] = diff;
        localStorage.setItem(NEW_MODELS_STORAGE_KEY, JSON.stringify(newModels));
        return true;
    } catch (error) {
        console.error('Failed to save new models to storage:', error);
        return false;
    }
};

export const removeNewModelsFromStorage = (providerUuid: string) => {
    try {
        const newModels = loadNewModelsFromStorage();
        delete newModels[providerUuid];
        localStorage.setItem(NEW_MODELS_STORAGE_KEY, JSON.stringify(newModels));
        return true;
    } catch (error) {
        console.error('Failed to remove new models from storage:', error);
        return false;
    }
};

// Custom hook to manage new models
export const useNewModels = () => {
    const [newModels, setNewModels] = useState<NewModelsData>({});
    const [version, setVersion] = useState(0);

    // Function to load new models from storage and update state
    const refetch = useCallback(() => {
        const storedNewModels = loadNewModelsFromStorage();
        setNewModels(storedNewModels);
        setVersion(prev => prev + 1);
    }, []);

    // Load new models from local storage on hook mount
    useEffect(() => {
        refetch();
    }, [refetch]);

    // Listen for new models updates from other components and reload
    useEffect(() => {
        const cleanup = listenForNewModelsUpdates(() => {
            refetch();
        });
        return cleanup;
    }, [refetch]);

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

        // Merge existing new models with newly detected ones (avoid duplicates)
        const mergedNewModels = Array.from(new Set([...existingNewModels, ...addedModels]));

        // Only update if there are new models to show
        if (mergedNewModels.length > 0) {
            const diff: NewModelsDiff = {
                newModels: mergedNewModels,
                timestamp: existingDiff?.timestamp || new Date().toISOString(),
            };

            if (saveNewModelsToStorage(providerUuid, diff)) {
                setNewModels(prev => ({ ...prev, [providerUuid]: diff }));
                dispatchNewModelsUpdate(providerUuid, diff);
            }
        }
        // Don't auto-clear - let users dismiss explicitly via close button
    }, [newModels]);

    // Get new models diff for a specific provider
    const getNewModels = useCallback((providerUuid: string): NewModelsDiff | undefined => {
        return newModels[providerUuid];
    }, [newModels]);

    // Clear new models for a specific provider
    const clearNewModels = useCallback((providerUuid: string) => {
        if (removeNewModelsFromStorage(providerUuid)) {
            setNewModels(prev => {
                const newModelsData = { ...prev };
                delete newModelsData[providerUuid];
                return newModelsData;
            });
            dispatchNewModelsUpdate(providerUuid, null);
        }
    }, []);

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
    };
};

// Helper to dispatch new models update event
export const dispatchNewModelsUpdate = (providerUuid: string, diff: NewModelsDiff | null) => {
    window.dispatchEvent(new CustomEvent(NEW_MODELS_UPDATE_EVENT, {
        detail: { providerUuid, diff }
    }));
};

// Helper to listen for new models updates
export const listenForNewModelsUpdates = (callback: (providerUuid: string, diff: NewModelsDiff | null) => void) => {
    const handler = ((event: CustomEvent) => {
        callback(event.detail.providerUuid, event.detail.diff);
    }) as EventListener;

    window.addEventListener(NEW_MODELS_UPDATE_EVENT, handler);

    // Return cleanup function
    return () => {
        window.removeEventListener(NEW_MODELS_UPDATE_EVENT, handler);
    };
};
