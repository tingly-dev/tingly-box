// Re-export commonly used types from schema
import type {paths, components, operations} from './schema';
export type {paths, components, operations};

// Helper types for commonly used schemas
export type ProbeResponse = components ['schemas']['ProbeResponse'];
export type ErrorDetail = components['schemas']['ErrorDetail'];

// Provider types
export type ProviderResponse = components['schemas']['ProviderResponse'];
export type ProviderModelsResponse = components['schemas']['ProviderModelsResponse'];

// Rule types
export type RuleResponse = components['schemas']['RuleResponse'];

// Usage types
export type UsageStatsResponse = components['schemas']['UsageStatsResponse'];
