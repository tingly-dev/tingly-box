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

// CLAUDE_CODE_AGENT is the base agent identifier stored in default_agent when
// @cc uses the main claude_code scenario. A profiled selection is stored as
// "claude_code:<profileId>" — mirrors the backend scenario naming.
export const CLAUDE_CODE_AGENT = 'claude_code';

// ccProfileIdFromDefaultAgent extracts the Claude Code profile ID from a bot's
// default_agent value. Returns '' for unset / "claude_code" / non-claude_code.
export function ccProfileIdFromDefaultAgent(defaultAgent?: string): string {
    const v = (defaultAgent || '').trim();
    if (!v.startsWith(CLAUDE_CODE_AGENT + ':')) return '';
    return v.slice(CLAUDE_CODE_AGENT.length + 1);
}

// defaultAgentForCCProfile builds the default_agent value for a profile
// selection ('' → the explicit base "claude_code", not an empty string — a
// concrete value reads clearly in raw settings/logs, per the "show the
// concrete value, not the alias" principle in .design/ux-principles.md).
export function defaultAgentForCCProfile(profileId: string): string {
    return profileId ? `${CLAUDE_CODE_AGENT}:${profileId}` : CLAUDE_CODE_AGENT;
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

// A route row in a bot's scenarios JSON: a real outbound scenario binding
// (e.g. "claude_code") rather than the "remote_agent" mount row. Read-only
// mirror of the Go struct in remote/binding — there is no write path for
// these from the frontend yet (see NotifyPage).
export interface NotifyRoute {
    name: string;
    chat_id?: string;
    events?: string[];
    enabled?: boolean;
}

// notifyRoutes extracts a bot's outbound scenario bindings (every scenarios
// row that isn't the remote_agent mount) from its raw scenarios JSON.
export function notifyRoutes(scenarios?: string): NotifyRoute[] {
    if (!scenarios || !scenarios.trim()) return [];
    try {
        const rows = JSON.parse(scenarios) as Array<NotifyRoute & { name?: string }>;
        if (!Array.isArray(rows)) return [];
        return rows.filter((r): r is NotifyRoute => !!r?.name && r.name !== REMOTE_AGENT_SCENARIO);
    } catch {
        return [];
    }
}

// isNotifyMounted reports whether the notify purpose is mounted on a bot.
// Mirrors the backend binding.OutboundScenarioMounted: mounted iff at least
// one non-remote_agent row exists and isn't explicitly disabled. Unlike
// remote_agent, notify fails CLOSED — no bindings means not mounted.
export function isNotifyMounted(scenarios?: string): boolean {
    return notifyRoutes(scenarios).some((r) => r.enabled !== false);
}

// countBotsByPlatform tallies active/total bots per platform — feeds the
// "active X / Y" subtitle on both the Bots page's picker (all platforms at
// once, from a list it already has loaded) and the Remote Control page's
// picker (fetched separately, since that page doesn't otherwise need the
// full bot list).
export function countBotsByPlatform(bots: BotSettings[]): Record<string, { active: number; total: number }> {
    const counts: Record<string, { active: number; total: number }> = {};
    for (const bot of bots) {
        if (!bot.platform) continue;
        const slot = counts[bot.platform] ?? (counts[bot.platform] = { active: 0, total: 0 });
        slot.total++;
        if (bot.enabled) slot.active++;
    }
    return counts;
}
