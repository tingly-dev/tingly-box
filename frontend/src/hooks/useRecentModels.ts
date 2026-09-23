import { useCallback } from 'react';
import { useLocalStorage } from './useLocalStorage';
import { createPersistedCollection } from './createPersistedCollection';

// Local storage key for recent models
const RECENT_MODELS_STORAGE_KEY = 'tingly_recent_models';
const LAST_PROVIDER_STORAGE_KEY = 'tingly_last_provider';
const MAX_RECENT_MODELS = 3;
const DEFAULT_RECENT_MODELS = {};
const DEFAULT_LAST_PROVIDER_DATA: Record<string, string> = {};

// Type for recent models data
type RecentModelsData = { [providerUuid: string]: string[] };

// Storage + cross-instance event sync are shared with the other model
// collection hooks (see createPersistedCollection). Dispatching from one hook
// instance makes other mounted instances refetch. The last-provider value is
// a plain localStorage write (no cross-instance sync needed).
const useRecentModelsStorage = createPersistedCollection<RecentModelsData, { providerUuid: string; modelName: string }>(
    RECENT_MODELS_STORAGE_KEY,
    'tingly_recent_models_update',
    DEFAULT_RECENT_MODELS
);

// Custom hook to manage recent models
export const useRecentModels = () => {
    const { data: recentModels, saveData, setData, loadData, notify } = useRecentModelsStorage();
    const { data: lastProvider, saveData: saveLastProvider } =
        useLocalStorage<Record<string, string>>(LAST_PROVIDER_STORAGE_KEY, DEFAULT_LAST_PROVIDER_DATA);

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
            notify({ providerUuid, modelName: model });
        }

        // Also update last used provider
        saveLastProvider('default', providerUuid);
    }, [recentModels, saveData, setData, saveLastProvider, loadData, notify]);

    return {
        recentModels,
        addRecentModel,
        lastProvider: lastProvider['default'] || '',
    };
};
