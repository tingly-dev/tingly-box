// Re-export commonly used types from schema
export type { paths, components, operations } from './schema';

// Helper types for commonly used schemas
export type ProbeResponse = components['schemas']['ProbeResponse'];
export type ErrorDetail = components['schemas']['ErrorDetail'];

// Provider types
export type ProviderResponse = components['schemas']['ProviderResponse'];
export type ProviderModelsResponse = components['schemas']['ProviderModelsResponse'];
export type ProviderRequest = components['schemas']['ProviderRequest'];

// Rule types
export type RuleResponse = components['schemas']['RuleResponse'];
export type RuleRequest = components['schemas']['RuleRequest'];

// OAuth types
export type OAuthSessionStatus = components['schemas']['OAuthSessionStatus'];

// Usage types
export type UsageStatsResponse = components['schemas']['UsageStatsResponse'];
