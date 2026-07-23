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

// REMOTE_AGENT_SCENARIO is the mount name for the remote-agent purpose
// (control Claude Code / SmartGuide from chat). Mirrors the backend constant.
export const REMOTE_AGENT_SCENARIO = 'remote_agent';

// isRemoteAgentMounted reports whether the remote_agent purpose is mounted on a
// bot, from its raw scenarios JSON. Mirrors the backend binding.ScenarioMounted:
// an absent binding counts as mounted (legacy default on); an explicit
// enabled:false turns it off; malformed JSON is treated as mounted.
export function isRemoteAgentMounted(scenarios?: string): boolean {
    if (!scenarios || !scenarios.trim()) return true;
    try {
        const rows = JSON.parse(scenarios) as Array<{ name?: string; enabled?: boolean }>;
        const row = Array.isArray(rows) ? rows.find((r) => r?.name === REMOTE_AGENT_SCENARIO) : undefined;
        if (!row) return true;
        return row.enabled !== false;
    } catch {
        return true;
    }
}
