/**
 * Feature Flags
 *
 * Central place to control feature availability across the application.
 * Set to false to disable features, true to enable.
 */

// Feature flag keys as constants
export const FEATURE_FLAGS = {
  CLAUDE_CODE_MODE_SWITCH: 'CLAUDE_CODE_MODE_SWITCH',
  // Add more flags below as needed
  // EXAMPLE_NEW_FEATURE: 'EXAMPLE_NEW_FEATURE',
} as const;

/**
 * Feature flag values and their enabled/disabled state
 */
export const FLAGS = {
  [FEATURE_FLAGS.CLAUDE_CODE_MODE_SWITCH]: true,
  // [FEATURE_FLAGS.EXAMPLE_NEW_FEATURE]: false,
} as const;

/**
 * Type-safe feature flag key type
 */
export type FeatureFlagKey = keyof typeof FEATURE_FLAGS;

/**
 * Type-safe feature flag value type
 */
export type FeatureFlagValue = typeof FEATURE_FLAGS[FeatureFlagKey];

/**
 * Check if a feature flag is enabled
 */
export function isFeatureEnabled(key: FeatureFlagValue): boolean {
  return FLAGS[key] === true;
}

/**
 * Get all enabled feature flags
 */
export function getEnabledFeatures(): FeatureFlagValue[] {
  return Object.entries(FLAGS)
    .filter(([_, value]) => value === true)
    .map(([key]) => key as FeatureFlagValue);
}
