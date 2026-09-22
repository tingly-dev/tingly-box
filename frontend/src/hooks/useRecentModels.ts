import { useCallback, useEffect } from 'react';
import { useLocalStorage } from './useLocalStorage';
import { createEventSystem } from '../utils/eventSystem';

// Local storage key for recent models
const RECENT_MODELS_STORAGE_KEY = 'tingly_recent_models';
const LAST_PROVIDER_STORAGE_KEY = 'tingly_last_provider';
const MAX_RECENT_MODELS = 3;
const DEFAULT_RECENT_MODELS = {};
const DEFAULT_LAST_PROVIDER_DATA: Record<string, string> = {};

// Type for recent models data
type RecentModelsData = { [providerUuid: string]: string[] };

// Event system for recent models updates — dispatching from one hook instance
// makes other mounted instances refetch.
const recentModelsEvent = createEventSystem<{ providerUuid: string; modelName: string }>(
    'tingly_recent_models_update'
);

// Custom hook to manage recent models
export const useRecentModels = () => {
    const { data: recentModels, saveData, setData, refetch, loadData } =
        useLocalStorage<RecentModelsData>(RECENT_MODELS_STORAGE_KEY, DEFAULT_RECENT_MODELS);
    const { data: lastProvider, saveData: saveLastProvider } =
        useLocalStorage<Record<string, string>>(LAST_PROVIDER_STORAGE_KEY, DEFAULT_LAST_PROVIDER_DATA);

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
            const currentData = loadData();
            setData({ ...currentData, [providerUuid]: newModels });
            recentModelsEvent.dispatch({ providerUuid, modelName: model });
        }

        // Also update last used provider
        saveLastProvider('default', providerUuid);
    }, [recentModels, saveData, setData, saveLastProvider, loadData]);

    return {
        recentModels,
        addRecentModel,
        lastProvider: lastProvider['default'] || '',
    };
};
