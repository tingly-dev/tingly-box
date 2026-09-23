import { useCallback } from 'react';
import { createPersistedCollection } from './createPersistedCollection';

// Local storage key for custom models
const CUSTOM_MODELS_STORAGE_KEY = 'tingly_custom_models';

// Type for custom models data (supports both old string format and new array format)
type CustomModelsData = { [providerUuid: string]: string | string[] };
const DEFAULT_CUSTOM_MODELS = {};

// Storage + cross-instance event sync are shared with the other model
// collection hooks (see createPersistedCollection). Dispatching from one hook
// instance (e.g. the dialog) makes other mounted instances (e.g. ModelsPanel)
// refetch.
const useCustomModelsStorage = createPersistedCollection<
    CustomModelsData,
    { providerUuid: string; modelName: string }
>(CUSTOM_MODELS_STORAGE_KEY, 'tingly_custom_model_update', DEFAULT_CUSTOM_MODELS);

// Helper to convert storage data to array format
const toArrayFormat = (value: string | string[]): string[] => {
    return Array.isArray(value) ? value : [value].filter(Boolean);
};

// Custom hook to manage custom models
export const useCustomModels = () => {
    const { data, loadData, removeKey, refetch, notify } = useCustomModelsStorage();

    // Convert storage data to normalized array format
    const customModels: { [providerUuid: string]: string[] } = useCallback(() => {
        const adapted: { [providerUuid: string]: string[] } = {};
        Object.keys(data).forEach(providerUuid => {
            adapted[providerUuid] = toArrayFormat(data[providerUuid]);
        });
        return adapted;
    }, [data])();

    // Helper function to save with backward compatibility
    const saveCustomModelToStorage = useCallback((
        providerUuid: string,
        customModel: string | string[]
    ): boolean => {
        try {
            const currentData = loadData();
            if (typeof customModel === 'string') {
                // For backward compatibility
                const existing = currentData[providerUuid];
                if (Array.isArray(existing)) {
                    if (!existing.includes(customModel)) {
                        currentData[providerUuid] = [...existing, customModel];
                    }
                } else {
                    currentData[providerUuid] = existing ? [existing, customModel] : [customModel];
                }
            } else {
                currentData[providerUuid] = customModel;
            }
            localStorage.setItem(CUSTOM_MODELS_STORAGE_KEY, JSON.stringify(currentData));
            return true;
        } catch (error) {
            console.error('Failed to save custom model to storage:', error);
            return false;
        }
    }, [loadData]);

    // Save custom model for a provider
    const saveCustomModel = useCallback((providerUuid: string, customModel: string) => {
        if (!customModel?.trim()) return false;

        const currentModels = customModels[providerUuid] || [];
        // Avoid duplicates
        if (currentModels.includes(customModel)) {
            return true; // Already exists
        }

        const newModels = [...currentModels, customModel];
        if (saveCustomModelToStorage(providerUuid, newModels)) {
            refetch();
            notify({ providerUuid, modelName: customModel });
            return true;
        }
        return false;
    }, [customModels, saveCustomModelToStorage, refetch, notify]);

    // Remove custom model for a provider
    const removeCustomModel = useCallback((providerUuid: string, customModel: string) => {
        const currentModels = customModels[providerUuid] || [];
        const newModels = currentModels.filter(model => model !== customModel);

        if (newModels.length === 0) {
            // Remove the entire entry if no models left
            if (removeKey(providerUuid)) {
                refetch();
                notify({ providerUuid, modelName: customModel });
                return true;
            }
        } else if (saveCustomModelToStorage(providerUuid, newModels)) {
            refetch();
            notify({ providerUuid, modelName: customModel });
            return true;
        }
        return false;
    }, [customModels, saveCustomModelToStorage, removeKey, refetch, notify]);

    // Update custom model for a provider (atomically replace old value with new value)
    const updateCustomModel = useCallback((providerUuid: string, oldValue: string, newValue: string) => {
        if (!newValue?.trim()) return false;

        const currentModels = customModels[providerUuid] || [];

        // Remove old value and add new value in one operation
        const newModels = currentModels.filter(model => model !== oldValue);

        // Avoid duplicates (in case newValue already exists)
        if (!newModels.includes(newValue)) {
            newModels.push(newValue);
        }

        // Save to storage
        if (saveCustomModelToStorage(providerUuid, newModels.length > 0 ? newModels : [])) {
            refetch();
            notify({ providerUuid, modelName: newValue });
            return true;
        }

        return false;
    }, [customModels, saveCustomModelToStorage, refetch, notify]);

    return {
        customModels,
        saveCustomModel,
        removeCustomModel,
        updateCustomModel,
    };
};
