
export interface Provider {
    uuid: string;
    name: string;
    enabled: boolean;
    api_base: string;
    api_style: string; // "openai" or "anthropic", defaults to "openai"
    token?: string;
}

export interface ProviderModelsData {
    [providerName: string]: {
        models: string[];
        star_models?: string[];
        last_updated?: string;
        custom_model?: string;
    };
}