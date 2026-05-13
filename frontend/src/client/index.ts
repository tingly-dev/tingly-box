// Re-export commonly used types from schema
import type {paths, components, operations} from './schema';
export type {paths, components, operations};

// Helper types for commonly used schemas
// Temporary alias during v1-to-v2 probe migration
export type ProbeResponse = components['schemas']['ProbeV2Response'];
export type ErrorDetail = components['schemas']['ErrorDetail'];

// Provider types
export type ProviderResponse = components['schemas']['ProviderResponse'];
export type ProviderModelsResponse = components['schemas']['ProviderModelsResponse'];
export type ProviderModelInfo = components['schemas']['ProviderModelInfo'];
export type OAuthDetail = components['schemas']['OAuthDetail'];

// Usage types
export type UsageStatsResponse = components['schemas']['UsageStatsResponse'];
export type UsageWindow = components['schemas']['UsageWindow'];
export type UsageCost = components['schemas']['UsageCost'];
export type UsageAccount = components['schemas']['UsageAccount'];
export type UsageBreakdown = components['schemas']['UsageBreakdown'];
export type ProviderUsage = components['schemas']['ProviderUsage'];

// Bot/ImBot types
export type Settings = components['schemas']['Settings'];
export type SettingsResponse = components['schemas']['SettingsResponse'];
export type PlatformConfig = components['schemas']['PlatformConfig'];
export type FieldSpec = components['schemas']['FieldSpec'];
export type PlatformsResponse = components['schemas']['PlatformsResponse'];
export type DeleteResponse = components['schemas']['DeleteResponse'];

// Rule types
export type RuleResponse = components['schemas']['RuleResponse'];

// Skill types
export type Skill = components['schemas']['Skill'];
export type SkillLocation = components['schemas']['SkillLocation'];
export type GroupingStrategy = components['schemas']['GroupingStrategy'];
export type DiscoveryResult = components['schemas']['DiscoveryResult'];
export type ScanResult = components['schemas']['ScanResult'];
