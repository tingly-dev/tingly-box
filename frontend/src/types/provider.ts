
export interface Provider {
    uuid: string;
    name: string;
    enabled: boolean;
    api_base: string;
    api_style: "openai" | "anthropic"; // "openai" or "anthropic", defaults to "openai"
    token?: string;
}

export interface ProviderModelData {
    models: string[];
    star_models?: string[];
    last_updated?: string;
    custom_model?: string;
}

// Provider models data indexed by provider name (legacy)
export interface ProviderModelsData {
    [providerName: string]: ProviderModelData;
}

// Provider models data indexed by provider UUID (new)
export interface ProviderModelsDataByUuid {
    [providerUuid: string]: ProviderModelData;
}