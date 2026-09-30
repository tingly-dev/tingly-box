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

// BotSettings is an alias for Settings from codegen.
//
// chat_id_lock is retained only for compatibility with existing generated
// Settings and persisted rows. Explicit DirectChat/Group access supersedes it;
// new UI and runtime authorization must not use this field.
export type BotSettings = Omit<Settings, 'chat_id'> & {
    chat_id_lock?: string;
    // Retained as an alias while codegen still emits it; new code should use
    // chat_id_lock. Removed once codegen drops chat_id.
    chat_id?: string;
	capabilities?: BotCapability[];
};

export type CapabilityName = 'notify' | 'remote_control';
export type AccessEffect = 'allow' | 'deny';
export interface BotCapability { bot_uuid: string; capability: CapabilityName; enabled: boolean; config?: Record<string, unknown>; }
export interface AccessPermission { capability: CapabilityName; action: string; effect: AccessEffect; }
export interface DirectChat { id: string; bot_uuid: string; platform: string; external_chat_id: string; peer_actor_id?: string; blocked: boolean; paired_at?: string; }
export interface DirectChatDetail { chat: DirectChat; permissions: AccessPermission[]; }
export interface GroupActor { actor: {id:string; external_actor_id:string; display_name?:string}; label?:string; permissions:AccessPermission[]; }
export interface BotGroup { id:string; bot_uuid:string; platform:string; external_group_id:string; name?:string; blocked:boolean; }
export interface BotGroupDetail { group:BotGroup; capabilities:Partial<Record<CapabilityName,AccessEffect>>; actors:GroupActor[]; }
export interface AuthorizationDecision { allowed:boolean; reason:string; failed_gate?:string; facts:Record<string,unknown>; }

// One concrete destination on the IM Notify work surface. Direct Chats and
// Groups are peer resources; the UI must not force users through a chat-kind
// mode picker before they can inspect or test them.
export interface NotifyTarget {
    id: string;
    kind: 'direct_chat' | 'group';
    external_id: string;
    name?: string;
    platform?: string;
    is_paired?: boolean;
    blocked: boolean;
    can_notify: boolean;
    can_reply: boolean;
}

export function capabilityEnabled(bot: BotSettings, name: CapabilityName): boolean {
    return bot.capabilities?.find((capability) => capability.capability === name)?.enabled === true;
}

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

// PLATFORM_DEFAULT_REQUIRE_PAIRING lists the token-DM platforms that default to
// TOFU pairing on. Mirrors bot.PlatformDefaultsRequirePairing on the backend.
// Shared by BotTable (decides whether to render the Pairing cell) and
// PairingCodePanel (decides whether to render at all) so the two stay in sync.
const PLATFORM_DEFAULT_REQUIRE_PAIRING: Record<string, boolean> = {
    telegram: true,
};

// isPairingRequired reports whether a bot enforces TOFU pairing — either via an
// explicit require_pairing flag, or the platform default. Used by BotTable and
// PairingCodePanel, and by BotChatsButton to tailor the empty-chats hint (a
// bot that requires pairing registers a chat only after the user pairs first).
export function isPairingRequired(bot?: {require_pairing?: boolean; platform?: string} | null): boolean {
    if (typeof bot?.require_pairing === 'boolean') {
        return bot.require_pairing;
    }
    return Boolean(PLATFORM_DEFAULT_REQUIRE_PAIRING[bot?.platform || '']);
}

// chatPermissionAllowed reports whether a Direct Chat has an explicit allow
// row for one capability action.
export function chatPermissionAllowed(chat: DirectChatDetail, capability: CapabilityName, action: string): boolean {
    return chat.permissions.some((permission) =>
        permission.capability === capability && permission.action === action && permission.effect === 'allow');
}

// remoteChatState is the ONE reading of a Direct Chat's Remote Control rows,
// shared by the Remote card's summary and both access dialogs so they can't
// disagree. Remote control needs both start (launch runs) and approve
// (answer permission/question prompts); one without the other silently
// breaks prompt replies, so it is its own state rather than "on".
export type RemoteChatState = 'on' | 'off' | 'startDenied' | 'approveDenied';
export function remoteChatState(chat: DirectChatDetail): RemoteChatState {
    const start = chatPermissionAllowed(chat, 'remote_control', 'remote_control.start');
    const approve = chatPermissionAllowed(chat, 'remote_control', 'remote_control.approve');
    if (start && approve) return 'on';
    if (!start && !approve) return 'off';
    return start ? 'approveDenied' : 'startDenied';
}

// chatCanControl: this Direct Chat can drive the bot right now.
export function chatCanControl(chat: DirectChatDetail): boolean {
    return !chat.chat.blocked
        && chatPermissionAllowed(chat, 'remote_control', 'access')
        && remoteChatState(chat) === 'on';
}

// groupCanControl: someone in this Group can drive the bot right now — Remote
// Control allowed on the group AND at least one actor granted both start and
// approve. Remote allowed on a group with no such actor still lets nobody in.
export function groupCanControl(detail: BotGroupDetail): boolean {
    const allowed = (actor: GroupActor, action: string) => actor.permissions.some((permission) =>
        permission.capability === 'remote_control' && permission.action === action && permission.effect === 'allow');
    return !detail.group.blocked
        && detail.capabilities.remote_control === 'allow'
        && detail.actors.some((actor) => allowed(actor, 'remote_control.start') && allowed(actor, 'remote_control.approve'));
}

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

// A chat a bot can reach, as returned by GET /api/v1/bots/:bot/chats. The
// chat_id is the channel-native conversation identifier the notify/interact
// API requires in its request body — surfacing it here (and in BotTable) is
// what makes those endpoints usable from the UI. Placeholder until codegen.
export interface BotChat {
    /** Stable internal DirectChat UUID used by control-plane mutations. */
    id: string;
    chat_id: string;
    platform?: string;
    is_paired?: boolean;
    is_whitelisted?: boolean;
    project_path?: string;
    updated_at?: string;
    blocked?: boolean;
    can_notify?: boolean;
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
export function countBotsByPlatform(
    bots: BotSettings[],
    isActive: (bot: BotSettings) => boolean = (bot) => Boolean(bot.enabled),
): Record<string, { active: number; total: number }> {
    const counts: Record<string, { active: number; total: number }> = {};
    for (const bot of bots) {
        if (!bot.platform) continue;
        const slot = counts[bot.platform] ?? (counts[bot.platform] = { active: 0, total: 0 });
        slot.total++;
        if (isActive(bot)) slot.active++;
    }
    return counts;
}
