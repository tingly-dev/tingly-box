import { useEffect, useState } from 'react';

// Local storage key for custom models
const CUSTOM_MODELS_STORAGE_KEY = 'tingly_custom_models';

// Helper functions to manage custom models in local storage
export const loadCustomModelsFromStorage = (): { [providerName: string]: string } => {
    try {
        const stored = localStorage.getItem(CUSTOM_MODELS_STORAGE_KEY);
        return stored ? JSON.parse(stored) : {};
    } catch (error) {
        console.error('Failed to load custom models from storage:', error);
        return {};
    }
};

export const saveCustomModelToStorage = (providerName: string, customModel: string | string[]) => {
    try {
        const customModels = loadCustomModelsFromStorage();
        if (typeof customModel === 'string') {
            // For backward compatibility, if saving a single string, check if it's already an array
            const existing = customModels[providerName];
            if (Array.isArray(existing)) {
                // Add to existing array if not duplicate
                if (!existing.includes(customModel)) {
                    customModels[providerName] = [...existing, customModel];
                }
            } else {
                // Convert to array format
                customModels[providerName] = existing ? [existing, customModel] : [customModel];
            }
        } else {
            // Save array directly
            customModels[providerName] = customModel;
        }
        localStorage.setItem(CUSTOM_MODELS_STORAGE_KEY, JSON.stringify(customModels));
        return true;
    } catch (error) {
        console.error('Failed to save custom model to storage:', error);
        return false;
    }
};

export const removeCustomModelFromStorage = (providerName: string) => {
    try {
        const customModels = loadCustomModelsFromStorage();
        delete customModels[providerName];
        localStorage.setItem(CUSTOM_MODELS_STORAGE_KEY, JSON.stringify(customModels));
        return true;
    } catch (error) {
        console.error('Failed to remove custom model from storage:', error);
        return false;
    }
};

// Custom hook to manage custom models
export const useCustomModels = () => {
    const [customModels, setCustomModels] = useState<{ [providerName: string]: string[] }>({});

    // Load custom models from local storage on hook mount
    useEffect(() => {
        const storedCustomModels = loadCustomModelsFromStorage();
        // Convert single string to array for backward compatibility
        const adaptedModels: { [providerName: string]: string[] } = {};
        Object.keys(storedCustomModels).forEach(providerName => {
            const value = storedCustomModels[providerName];
            // If it's a string (old format), convert to array
            adaptedModels[providerName] = Array.isArray(value) ? value : [value].filter(Boolean);
        });
        setCustomModels(adaptedModels);
    }, []);

    // Save custom model for a provider
    const saveCustomModel = (providerName: string, customModel: string) => {
        if (!customModel?.trim()) return false;

        const currentModels = customModels[providerName] || [];
        // Avoid duplicates
        if (currentModels.includes(customModel)) {
            return true; // Already exists
        }

        const newModels = [...currentModels, customModel];
        if (saveCustomModelToStorage(providerName, newModels)) {
            setCustomModels(prev => ({ ...prev, [providerName]: newModels }));
            return true;
        }
        return false;
    };

    // Remove custom model for a provider
    const removeCustomModel = (providerName: string, customModel: string) => {
        const currentModels = customModels[providerName] || [];
        const newModels = currentModels.filter(model => model !== customModel);

        if (newModels.length === 0) {
            // Remove the entire entry if no models left
            if (removeCustomModelFromStorage(providerName)) {
                setCustomModels(prev => {
                    const newModels = { ...prev };
                    delete newModels[providerName];
                    return newModels;
                });
                return true;
            }
        } else if (saveCustomModelToStorage(providerName, newModels)) {
            setCustomModels(prev => ({ ...prev, [providerName]: newModels }));
            return true;
        }
        return false;
    };

    // Get all custom models for a specific provider
    const getCustomModels = (providerName: string): string[] => {
        return customModels[providerName] || [];
    };

    // Get the first/custom model for backward compatibility
    const getCustomModel = (providerName: string): string | undefined => {
        const models = customModels[providerName];
        return models && models.length > 0 ? models[0] : undefined;
    };

    // Check if a model is a custom model for a provider
    const isCustomModel = (model: string, providerName: string): boolean => {
        return customModels[providerName]?.includes(model) || false;
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
export const dispatchCustomModelUpdate = (providerName: string, modelName: string) => {
    window.dispatchEvent(new CustomEvent(CUSTOM_MODEL_UPDATE_EVENT, {
        detail: { providerName, modelName }
    }));
};

// Helper to listen for custom model updates
export const listenForCustomModelUpdates = (callback: (providerName: string, modelName: string) => void) => {
    const handler = ((event: CustomEvent) => {
        callback(event.detail.providerName, event.detail.modelName);
    }) as EventListener;

    window.addEventListener(CUSTOM_MODEL_UPDATE_EVENT, handler);

    // Return cleanup function
    return () => {
        window.removeEventListener(CUSTOM_MODEL_UPDATE_EVENT, handler);
    };
};