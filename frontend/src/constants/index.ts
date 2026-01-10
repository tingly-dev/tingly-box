// Application constants
export const DEFAULT_RULE = "tingly";
export const DEFAULT_RULE_UUID = "tingly";

// API Styles
export const API_STYLES = {
  OPENAI: 'openai',
  ANTHROPIC: 'anthropic',
} as const;

// Notification types
export const NOTIFICATION_TYPES = {
  SUCCESS: 'success',
  ERROR: 'error',
  WARNING: 'warning',
  INFO: 'info',
} as const;

// Export feature flags
export * from './featureFlags';