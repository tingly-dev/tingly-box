import { useEffect, useState } from 'react';

// Local storage key for custom models
const CUSTOM_MODELS_STORAGE_KEY = 'tingly_custom_models';

// Helper functions to manage custom models in local storage
export const loadCustomModelsFromStorage = (): { [providerUuid: string]: string } => {
    try {
        const stored = localStorage.getItem(CUSTOM_MODELS_STORAGE_KEY);
        return stored ? JSON.parse(stored) : {};
    } catch (error) {
        console.error('Failed to load custom models from storage:', error);
        return {};
    }
};

export const saveCustomModelToStorage = (providerUuid: string, customModel: string | string[]) => {
    try {
        const customModels = loadCustomModelsFromStorage();
        if (typeof customModel === 'string') {
            // For backward compatibility, if saving a single string, check if it's already an array
            const existing = customModels[providerUuid];
            if (Array.isArray(existing)) {
                // Add to existing array if not duplicate
                if (!existing.includes(customModel)) {
                    customModels[providerUuid] = [...existing, customModel];
                }
            } else {
                // Convert to array format
                customModels[providerUuid] = existing ? [existing, customModel] : [customModel];
            }
        } else {
            // Save array directly
            customModels[providerUuid] = customModel;
        }
        localStorage.setItem(CUSTOM_MODELS_STORAGE_KEY, JSON.stringify(customModels));
        return true;
    } catch (error) {
        console.error('Failed to save custom model to storage:', error);
        return false;
    }
};

export const removeCustomModelFromStorage = (providerUuid: string) => {
    try {
        const customModels = loadCustomModelsFromStorage();
        delete customModels[providerUuid];
        localStorage.setItem(CUSTOM_MODELS_STORAGE_KEY, JSON.stringify(customModels));
        return true;
    } catch (error) {
        console.error('Failed to remove custom model from storage:', error);
        return false;
    }
};

// Custom hook to manage custom models
export const useCustomModels = () => {
    const [customModels, setCustomModels] = useState<{ [providerUuid: string]: string[] }>({});

    // Load custom models from local storage on hook mount
    useEffect(() => {
        const storedCustomModels = loadCustomModelsFromStorage();
        // Convert single string to array for backward compatibility
        const adaptedModels: { [providerUuid: string]: string[] } = {};
        Object.keys(storedCustomModels).forEach(providerUuid => {
            const value = storedCustomModels[providerUuid];
            // If it's a string (old format), convert to array
            adaptedModels[providerUuid] = Array.isArray(value) ? value : [value].filter(Boolean);
        });
        setCustomModels(adaptedModels);
    }, []);

    // Save custom model for a provider
    const saveCustomModel = (providerUuid: string, customModel: string) => {
        if (!customModel?.trim()) return false;

        const currentModels = customModels[providerUuid] || [];
        // Avoid duplicates
        if (currentModels.includes(customModel)) {
            return true; // Already exists
        }

        const newModels = [...currentModels, customModel];
        if (saveCustomModelToStorage(providerUuid, newModels)) {
            setCustomModels(prev => ({ ...prev, [providerUuid]: newModels }));
            return true;
        }
        return false;
    };

    // Remove custom model for a provider
    const removeCustomModel = (providerUuid: string, customModel: string) => {
        const currentModels = customModels[providerUuid] || [];
        const newModels = currentModels.filter(model => model !== customModel);

        if (newModels.length === 0) {
            // Remove the entire entry if no models left
            if (removeCustomModelFromStorage(providerUuid)) {
                setCustomModels(prev => {
                    const newModels = { ...prev };
                    delete newModels[providerUuid];
                    return newModels;
                });
                return true;
            }
        } else if (saveCustomModelToStorage(providerUuid, newModels)) {
            setCustomModels(prev => ({ ...prev, [providerUuid]: newModels }));
            return true;
        }
        return false;
    };

    // Get all custom models for a specific provider
    const getCustomModels = (providerUuid: string): string[] => {
        return customModels[providerUuid] || [];
    };

    // Get the first/custom model for backward compatibility
    const getCustomModel = (providerUuid: string): string | undefined => {
        const models = customModels[providerUuid];
        return models && models.length > 0 ? models[0] : undefined;
    };

    // Check if a model is a custom model for a provider
    const isCustomModel = (model: string, providerUuid: string): boolean => {
        return customModels[providerUuid]?.includes(model) || false;
    };

    return {
        customModels,
        saveCustomModel,
        removeCustomModel,
        getCustomModels,
        getCustomModel, // Keep for backward compatibility
        isCustomModel,
        loadCustomModelsFromStorage,
        saveCustomModelToStorage,
        removeCustomModelFromStorage
    };
};

// Export event name for custom model updates
export const CUSTOM_MODEL_UPDATE_EVENT = 'tingly_custom_model_update';

// Helper to dispatch custom model update event
export const dispatchCustomModelUpdate = (providerUuid: string, modelName: string) => {
    window.dispatchEvent(new CustomEvent(CUSTOM_MODEL_UPDATE_EVENT, {
        detail: { providerUuid, modelName }
    }));
};

// Helper to listen for custom model updates
export const listenForCustomModelUpdates = (callback: (providerUuid: string, modelName: string) => void) => {
    const handler = ((event: CustomEvent) => {
        callback(event.detail.providerUuid, event.detail.modelName);
    }) as EventListener;

    window.addEventListener(CUSTOM_MODEL_UPDATE_EVENT, handler);

    // Return cleanup function
    return () => {
        window.removeEventListener(CUSTOM_MODEL_UPDATE_EVENT, handler);
    };
};
