// Bot platform authentication types

// Re-export bot-related types from codegen
import type {
    Settings,
    SettingsResponse,
    PlatformConfig,
    FieldSpec,
    PlatformsResponse,
    DeleteResponse,
} from '@/client';

// Re-export Skill-related types from codegen
import type {
    Skill,
    SkillLocation,
    GroupingStrategy,
} from '@/client';

// BotSettings is an alias for Settings from codegen
export type BotSettings = Settings;

// BotPlatformConfig is an alias for PlatformConfig from codegen
export type BotPlatformConfig = PlatformConfig;

// Re-export for consumers
export type {
    Settings,
    SettingsResponse,
    PlatformConfig,
    FieldSpec,
    PlatformsResponse,
    DeleteResponse,
    Skill,
    SkillLocation,
    GroupingStrategy,
};

// Category display labels
export const CategoryLabels: Record<string, string> = {
    im: 'IM Platforms',
    enterprise: 'Enterprise',
    business: 'Business',
};

// Auth type display labels
export const AuthTypeLabels: Record<string, string> = {
    token: 'Token',
    oauth: 'OAuth',
    qr: 'QR Code',
    basic: 'Basic Auth',
};

// Helper to mask secret values for display
export function maskSecret(value: string, visible = false): string {
    if (!value) return '-';
    if (visible) return value;
    if (value.length <= 8) return '*'.repeat(value.length);
    return value.substring(0, 4) + '*'.repeat(Math.min(8, value.length - 4)) + value.substring(value.length - 4);
}

// Helper to get display name for auth field value
export function getAuthDisplayValue(settings: BotSettings, config: BotPlatformConfig): string {
    if (!settings.auth || Object.keys(settings.auth).length === 0) {
        return '-';
    }

    // For token auth, show masked token
    if (config.auth_type === 'token') {
        const token = settings.auth['token'];
        return token ? maskSecret(token) : '-';
    }

    // For OAuth, show clientId
    if (config.auth_type === 'oauth') {
        const clientId = settings.auth['clientId'];
        return clientId ? maskSecret(clientId) : '-';
    }

    return 'Configured';
}
