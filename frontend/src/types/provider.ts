
export interface Provider {
    uuid: string;
    name: string;
    enabled: boolean;
    api_base: string;
    api_style: "openai" | "anthropic"; // "openai" or "anthropic", defaults to "openai"
    token?: string;
    auth_type?: "api_key" | "oauth"; // "api_key" or "oauth"
    oauth_detail?: OAuthDetail;
}

export interface OAuthDetail {
    access_token: string;
    provider_type: string; // anthropic, google, etc.
    user_id: string;
    refresh_token?: string;
    expires_at?: string;
}

export interface ProviderModelData {
    uuid: string;
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