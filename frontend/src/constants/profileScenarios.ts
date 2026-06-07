/**
 * Scenarios that support named profiles.
 * Source of truth is the backend ScenarioDescriptor.SupportsProfiles field.
 * This list is used as a fallback during initial load before descriptors are fetched.
 */
export const PROFILE_SCENARIOS = [
    'claude_code',
] as const;

export type ProfileScenario = typeof PROFILE_SCENARIOS[number];
