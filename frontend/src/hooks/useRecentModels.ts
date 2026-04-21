import { useCallback, useEffect } from 'react';
import { useLocalStorage } from './useLocalStorage';
import { createEventSystem } from '../utils/eventSystem';

// Local storage key for recent models
const RECENT_MODELS_STORAGE_KEY = 'tingly_recent_models';
const MAX_RECENT_MODELS = 3;
const DEFAULT_RECENT_MODELS = {};

// Type for recent models data
export type RecentModelsData = { [providerUuid: string]: string[] };

// Event system for recent models updates
const recentModelsEvent = createEventSystem<{ providerUuid: string; modelName: string }>(
    'tingly_recent_models_update'
);

// Export event name for backward compatibility
export const RECENT_MODELS_UPDATE_EVENT = recentModelsEvent.eventName;

// Custom hook to manage recent models
export const useRecentModels = () => {
    const { data: recentModels, version, saveData, removeKey, setData, refetch } =
        useLocalStorage<RecentModelsData>(RECENT_MODELS_STORAGE_KEY, DEFAULT_RECENT_MODELS);

    // Listen for recent models updates from other components and reload
    useEffect(() => {
        const cleanup = recentModelsEvent.listen(() => {
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

        if (saveData(providerUuid, newModels)) {
            setData(prev => ({ ...prev, [providerUuid]: newModels }));
            recentModelsEvent.dispatch({ providerUuid, modelName: model });
        }
    }, [recentModels, saveData, setData]);

    // Get recent models for a specific provider
    const getRecentModels = useCallback((providerUuid: string): string[] => {
        return recentModels[providerUuid] || [];
    }, [recentModels]);

    // Clear recent models for a specific provider
    const clearRecentModels = useCallback((providerUuid: string) => {
        if (removeKey(providerUuid)) {
            setData(prev => {
                const newModels = { ...prev };
                delete newModels[providerUuid];
                return newModels;
            });
            recentModelsEvent.dispatch({ providerUuid, modelName: '' });
        }
    }, [removeKey, setData]);

    // Helper functions for backward compatibility
    const loadRecentModelsFromStorage = useCallback(() => {
        const stored = localStorage.getItem(RECENT_MODELS_STORAGE_KEY);
        return stored ? JSON.parse(stored) : {};
    }, []);

    const saveRecentModelsToStorage = useCallback((providerUuid: string, models: string[]) => {
        return saveData(providerUuid, models);
    }, [saveData]);

    const removeRecentModelsFromStorage = useCallback((providerUuid: string) => {
        return removeKey(providerUuid);
    }, [removeKey]);

    // Helper to dispatch recent models update event (backward compatibility)
    const dispatchRecentModelsUpdate = (providerUuid: string, modelName: string) => {
        recentModelsEvent.dispatch({ providerUuid, modelName });
    };

    // Helper to listen for recent models updates (backward compatibility)
    const listenForRecentModelsUpdates = (callback: (providerUuid: string, modelName: string) => void) => {
        return recentModelsEvent.listen(({ providerUuid, modelName }) => {
            callback(providerUuid, modelName);
        });
    };

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
        dispatchRecentModelsUpdate,
        listenForRecentModelsUpdates,
    };
};
