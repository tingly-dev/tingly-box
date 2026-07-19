import { useCallback, useEffect } from 'react';
import { useLocalStorage } from './useLocalStorage';
import { createEventSystem } from '../utils/eventSystem';

// Local storage key for custom models
const CUSTOM_MODELS_STORAGE_KEY = 'tingly_custom_models';

// Type for custom models data (supports both old string format and new array format)
type CustomModelsData = { [providerUuid: string]: string | string[] };
const DEFAULT_CUSTOM_MODELS = {};

// Event system for custom model updates
const customModelEvent = createEventSystem<{ providerUuid: string; modelName: string }>(
    'tingly_custom_model_update'
);

// Export event name for backward compatibility
export const CUSTOM_MODEL_UPDATE_EVENT = customModelEvent.eventName;

// Helper to convert storage data to array format
const toArrayFormat = (value: string | string[]): string[] => {
    return Array.isArray(value) ? value : [value].filter(Boolean);
};

// Custom hook to manage custom models
export const useCustomModels = () => {
    const { data, version, saveData, removeKey, loadData, refetch } =
        useLocalStorage<CustomModelsData>(CUSTOM_MODELS_STORAGE_KEY, DEFAULT_CUSTOM_MODELS);

    // Convert storage data to normalized array format
    const customModels: { [providerUuid: string]: string[] } = useCallback(() => {
        const adapted: { [providerUuid: string]: string[] } = {};
        Object.keys(data).forEach(providerUuid => {
            adapted[providerUuid] = toArrayFormat(data[providerUuid]);
        });
        return adapted;
    }, [data])();

    // Listen for custom model updates from other components and reload
    useEffect(() => {
        const cleanup = customModelEvent.listen(() => {
            refetch();
        });
        return cleanup;
    }, [refetch]);

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
            customModelEvent.dispatch({ providerUuid, modelName: customModel });
            return true;
        }
        return false;
    }, [customModels, saveCustomModelToStorage, refetch]);

    // Remove custom model for a provider
    const removeCustomModel = useCallback((providerUuid: string, customModel: string) => {
        const currentModels = customModels[providerUuid] || [];
        const newModels = currentModels.filter(model => model !== customModel);

        if (newModels.length === 0) {
            // Remove the entire entry if no models left
            if (removeKey(providerUuid)) {
                refetch();
                customModelEvent.dispatch({ providerUuid, modelName: customModel });
                return true;
            }
        } else if (saveCustomModelToStorage(providerUuid, newModels)) {
            refetch();
            customModelEvent.dispatch({ providerUuid, modelName: customModel });
            return true;
        }
        return false;
    }, [customModels, saveCustomModelToStorage, removeKey, refetch]);

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
            customModelEvent.dispatch({ providerUuid, modelName: newValue });
            return true;
        }

        return false;
    }, [customModels, saveCustomModelToStorage, refetch]);

    // Get all custom models for a specific provider
    const getCustomModels = useCallback((providerUuid: string): string[] => {
        return customModels[providerUuid] || [];
    }, [customModels]);

    // Get the first/custom model for backward compatibility
    const getCustomModel = useCallback((providerUuid: string): string | undefined => {
        const models = customModels[providerUuid];
        return models && models.length > 0 ? models[0] : undefined;
    }, [customModels]);

    // Check if a model is a custom model for a provider
    const isCustomModel = useCallback((model: string, providerUuid: string): boolean => {
        return customModels[providerUuid]?.includes(model) || false;
    }, [customModels]);

    // Helper functions for backward compatibility
    const loadCustomModelsFromStorage = useCallback((): CustomModelsData => {
        return loadData();
    }, [loadData]);

    const removeCustomModelFromStorage = useCallback((providerUuid: string): boolean => {
        return removeKey(providerUuid);
    }, [removeKey]);

    // Helper to dispatch custom model update event (backward compatibility)
    const dispatchCustomModelUpdate = (providerUuid: string, modelName: string) => {
        customModelEvent.dispatch({ providerUuid, modelName });
    };

    // Helper to listen for custom model updates (backward compatibility)
    const listenForCustomModelUpdates = (callback: (providerUuid: string, modelName: string) => void) => {
        return customModelEvent.listen((data) => {
            if (!data) return;
            const { providerUuid, modelName } = data;
            callback(providerUuid, modelName);
        });
    };

    return {
        customModels,
        version,
        refetch,
        saveCustomModel,
        removeCustomModel,
        updateCustomModel,
        getCustomModels,
        getCustomModel, // Keep for backward compatibility
        isCustomModel,
        loadCustomModelsFromStorage,
        saveCustomModelToStorage,
        removeCustomModelFromStorage,
        dispatchCustomModelUpdate,
        listenForCustomModelUpdates,
    };
};
