
// Re-export provider-related types from codegen
import type {
    ProviderResponse,
    ProviderModelInfo,
    OAuthDetail,
} from '@/client';

// Virtual-model provider configuration. The backend stores this on
// Provider.vmodel_detail; codegen will eventually emit it as
// components['schemas']['VModelDetail'] but until /scripts regenerates the
// client we keep a hand-rolled placeholder.
// TODO(codegen): drop this once swagger emits VModelDetail.
export interface VModelDetail {
    models?: string[];
    latency_profile?: string;
}

// Provider extends the codegen ProviderResponse with two fields that are
// already on the wire but not yet in the OpenAPI spec:
//   - vmodel_detail: virtual-model registry config (only for auth_type=vmodel)
//   - source: "user" (default) or "builtin"
// TODO(codegen): drop the intersection once swagger emits these fields.
export type Provider = ProviderResponse & {
    vmodel_detail?: VModelDetail;
    source?: 'user' | 'builtin';
};

// Provider models data indexed by provider UUID
// Note: ProviderModelInfo from codegen is the new format
export type ProviderModelData = ProviderModelInfo;

export interface ProviderModelsDataByUuid {
    [providerUuid: string]: ProviderModelData;
}

// Re-export for consumers
export type { ProviderResponse, ProviderModelInfo, OAuthDetail };