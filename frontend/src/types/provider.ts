
// Re-export provider-related types from codegen
import type {
    ProviderResponse,
    ProviderModelInfo,
    OAuthDetail,
} from '@/client';

// Type aliases for convenience
// Provider is an alias for ProviderResponse from codegen
export type Provider = ProviderResponse;

// Provider models data indexed by provider UUID
// Note: ProviderModelInfo from codegen is the new format
export type ProviderModelData = ProviderModelInfo;

export interface ProviderModelsDataByUuid {
    [providerUuid: string]: ProviderModelData;
}

// Legacy index types (deprecated, use ProviderModelsDataByUuid)
// @deprecated Use ProviderModelsDataByUuid instead
export type ProviderModelsData = Record<string, ProviderModelData>;

// Re-export for consumers
export type { ProviderResponse, ProviderModelInfo, OAuthDetail };