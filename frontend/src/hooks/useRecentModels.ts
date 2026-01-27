import { useEffect, useState, useCallback } from 'react';

// Local storage key for recent models
const RECENT_MODELS_STORAGE_KEY = 'tingly_recent_models';
const MAX_RECENT_MODELS = 3;

// Export event name for recent models updates
export const RECENT_MODELS_UPDATE_EVENT = 'tingly_recent_models_update';

// Helper functions to manage recent models in local storage
export const loadRecentModelsFromStorage = (): { [providerUuid: string]: string[] } => {
    try {
        const stored = localStorage.getItem(RECENT_MODELS_STORAGE_KEY);
        return stored ? JSON.parse(stored) : {};
    } catch (error) {
        console.error('Failed to load recent models from storage:', error);
        return {};
    }
};

export const saveRecentModelsToStorage = (providerUuid: string, models: string[]) => {
    try {
        const recentModels = loadRecentModelsFromStorage();
        recentModels[providerUuid] = models;
        localStorage.setItem(RECENT_MODELS_STORAGE_KEY, JSON.stringify(recentModels));
        return true;
    } catch (error) {
        console.error('Failed to save recent models to storage:', error);
        return false;
    }
};

export const removeRecentModelsFromStorage = (providerUuid: string) => {
    try {
        const recentModels = loadRecentModelsFromStorage();
        delete recentModels[providerUuid];
        localStorage.setItem(RECENT_MODELS_STORAGE_KEY, JSON.stringify(recentModels));
        return true;
    } catch (error) {
        console.error('Failed to remove recent models from storage:', error);
        return false;
    }
};

// Custom hook to manage recent models
export const useRecentModels = () => {
    const [recentModels, setRecentModels] = useState<{ [providerUuid: string]: string[] }>({});
    const [version, setVersion] = useState(0);

    // Function to load recent models from storage and update state
    const refetch = useCallback(() => {
        const storedRecentModels = loadRecentModelsFromStorage();
        setRecentModels(storedRecentModels);
        setVersion(prev => prev + 1);
    }, []);

    // Load recent models from local storage on hook mount
    useEffect(() => {
        refetch();
    }, [refetch]);

    // Listen for recent models updates from other components and reload
    useEffect(() => {
        const cleanup = listenForRecentModelsUpdates(() => {
            refetch();
        });
        return cleanup;
    }, [refetch]);

    // Add a model to recent list (prepend, keep max 3, remove duplicates)
    const addRecentModel = useCallback((providerUuid: string, model: string) => {
        if (!model?.trim()) return;

        const currentModels = recentModels[providerUuid] || [];
        // Remove duplicate if exists
        const filtered = currentModels.filter(m => m !== model);
        // Prepend new model
        const newModels = [model, ...filtered].slice(0, MAX_RECENT_MODELS);

        if (saveRecentModelsToStorage(providerUuid, newModels)) {
            setRecentModels(prev => ({ ...prev, [providerUuid]: newModels }));
            dispatchRecentModelsUpdate(providerUuid, model);
        }
    }, [recentModels]);

    // Get recent models for a specific provider
    const getRecentModels = useCallback((providerUuid: string): string[] => {
        return recentModels[providerUuid] || [];
    }, [recentModels]);

    // Clear recent models for a specific provider
    const clearRecentModels = useCallback((providerUuid: string) => {
        if (removeRecentModelsFromStorage(providerUuid)) {
            setRecentModels(prev => {
                const newModels = { ...prev };
                delete newModels[providerUuid];
                return newModels;
            });
            dispatchRecentModelsUpdate(providerUuid, '');
        }
    }, []);

    return {
        recentModels,
        version,
        refetch,
        addRecentModel,
        getRecentModels,
        clearRecentModels,
        loadRecentModelsFromStorage,
        saveRecentModelsToStorage,
        removeRecentModelsFromStorage,
    };
};

// Helper to dispatch recent models update event
export const dispatchRecentModelsUpdate = (providerUuid: string, modelName: string) => {
    window.dispatchEvent(new CustomEvent(RECENT_MODELS_UPDATE_EVENT, {
        detail: { providerUuid, modelName }
    }));
};

// Helper to listen for recent models updates
export const listenForRecentModelsUpdates = (callback: (providerUuid: string, modelName: string) => void) => {
    const handler = ((event: CustomEvent) => {
        callback(event.detail.providerUuid, event.detail.modelName);
    }) as EventListener;

    window.addEventListener(RECENT_MODELS_UPDATE_EVENT, handler);

    // Return cleanup function
    return () => {
        window.removeEventListener(RECENT_MODELS_UPDATE_EVENT, handler);
    };
};
