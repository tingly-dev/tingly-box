import type { Provider } from '../types/provider';

export interface CustomModelsData {
    [providerName: string]: string[];
}

export interface ProviderModelsData {
    [providerName: string]: {
        models?: string[];
        star_models?: string[];
        custom_model?: string;
    };
}

export interface ModelData {
    models?: string[];
    star_models?: string[];
    custom_model?: string;
}

export interface ModelTypeInfo {
    totalModelsCount: number;
    isCustomModel: (model: string) => boolean;
    allModelsForSearch: string[];
    standardModelsForDisplay: string[];
}

export function getModelTypeInfo(
    provider: Provider,
    providerModels: ProviderModelsData | undefined,
    customModels: CustomModelsData
): ModelTypeInfo {
    const models = providerModels?.[provider.name]?.models || [];
    const starModels = providerModels?.[provider.name]?.star_models || [];
    const backendCustomModel = providerModels?.[provider.name]?.custom_model;
    const localStorageCustomModels = customModels[provider.name] || [];

    // Calculate total unique models count
    const uniqueModels = new Set(models);
    starModels.forEach(model => uniqueModels.add(model));
    if (backendCustomModel) uniqueModels.add(backendCustomModel);
    localStorageCustomModels.forEach(model => uniqueModels.add(model));

    // Combine all models for searching
    const allModelsForSearch = [...models];
    starModels.forEach(model => {
        if (!allModelsForSearch.includes(model)) {
            allModelsForSearch.push(model);
        }
    });
    if (backendCustomModel && !allModelsForSearch.includes(backendCustomModel)) {
        allModelsForSearch.push(backendCustomModel);
    }
    localStorageCustomModels.forEach(model => {
        if (!allModelsForSearch.includes(model)) {
            allModelsForSearch.push(model);
        }
    });

    // Get standard models for display (excluding custom models)
    const standardModelsForDisplay = allModelsForSearch.filter(model => {
        if (model === backendCustomModel) return false;
        if (localStorageCustomModels.includes(model)) return false;
        if (starModels.includes(model)) return false;
        return true;
    });

    const isCustomModel = (model: string) => {
        return !models.includes(model) &&
            !starModels.includes(model) &&
            model !== '' &&
            model !== backendCustomModel &&
            !localStorageCustomModels.includes(model);
    };

    return {
        totalModelsCount: uniqueModels.size,
        isCustomModel: isCustomModel,
        allModelsForSearch,
        standardModelsForDisplay
    };
}

export function filterModels(models: string[], searchTerm: string): string[] {
    if (!searchTerm) return models;
    return models.filter(model =>
        model.toLowerCase().includes(searchTerm.toLowerCase())
    );
}

export function navigateToModelPage(
    selectedModel: string,
    provider: Provider,
    modelsPerPage: number,
    setCurrentPage: React.Dispatch<React.SetStateAction<{ [key: string]: number }>>,
    getStandardModels: () => string[]
): void {
    const standardModels = getStandardModels();
    const modelIndex = standardModels.indexOf(selectedModel);

    if (modelIndex !== -1) {
        const targetPage = Math.floor(modelIndex / modelsPerPage) + 1;
        setCurrentPage(prev => ({ ...prev, [provider.name]: targetPage }));
    }
}