// IM bot interaction API — access control (capabilities/chats/groups/
// permissions) plus the general notify/interact/wait surface described in
// .design/bot-interaction-api.md.
//
// Split out of services/api.ts because these calls follow a different
// contract than the rest of the control-plane API:
//   - Access-control mutations (list/set capability, chat, group, permission)
//     THROW on failure — callers (BotAccessDialog.tsx) rely on try/catch, not
//     a {success,error} envelope, so they stay on that contract here too.
//   - notify / interact / wait / listBotChats RESOLVE with an `error` field
//     instead of throwing — callers (NotifyTestDialog, useChatProbe,
//     BotChatsButton) branch on `result.error`.
// Both still go through the generated OpenAPI client (getControlApiClient /
// getControlApiHeaders) for base URL + auth header resolution, instead of
// hand-rolled fetch() calls.
import type {BotCapability, BotChat, BotGroup, BotGroupDetail, BotSettings, DirectChatDetail} from '@/types/bot';
import type {ApiClient} from './openapi';
import {
    errorMessage,
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
} from './openapi';

type ClientCall<T> = (client: ApiClient, headers: Record<string, string>) => Promise<{
    data?: T;
    error?: unknown;
    response: Response;
}>;

// Throws on failure so existing callers (BotAccessDialog.tsx's try/catch)
// keep working unchanged.
async function botAccessCall<T>(call: ClientCall<T>): Promise<T> {
    const client = await getClient();
    const headers = await getAuthHeaders();
    const {data, error, response} = await call(client, headers);
    if (data === undefined || error !== undefined) {
        throw new Error(errorMessage(error) || `request failed (${response.status})`);
    }
    return data;
}

// ========== Bot capabilities ==========
//
// The functions below cast their generated-client result to this module's
// app-facing types (types/bot.ts). The OpenAPI schema types `capability` as
// a bare string (no enum in the Go source), while the app's own types narrow
// it to the two real values the backend ever sends — same reasoning as
// enrichBotsWithCapabilities above. Consumers (BotAccessDialog.tsx,
// RemoteAgentBotCard.tsx, BotNotifyGroup.tsx, RemoteAgentPage.tsx)
// already code against types/bot.ts; this keeps that contract stable.

export const listBotCapabilities = (botUUID: string): Promise<{capabilities: BotCapability[]; bot_running: boolean}> =>
    botAccessCall((client, headers) => client.GET('/api/v1/bots/{bot}/capabilities', {
        headers,
        params: {path: {bot: botUUID}},
    })) as Promise<{capabilities: BotCapability[]; bot_running: boolean}>;

export const setBotCapability = (botUUID: string, capability: 'notify' | 'remote_control', enabled: boolean): Promise<{capability: BotCapability; bot_running: boolean; reason?: string}> =>
    botAccessCall((client, headers) => client.PUT('/api/v1/bots/{bot}/capabilities/{capability}', {
        headers,
        params: {path: {bot: botUUID, capability}},
        body: {enabled, config: {}},
    })) as Promise<{capability: BotCapability; bot_running: boolean; reason?: string}>;

// Capability records are exposed separately from the generated bot settings
// model. Keep the join in one place so every bot surface gets the same
// per-bot failure fallback.
export async function enrichBotsWithCapabilities(bots: BotSettings[]): Promise<BotSettings[]> {
    return Promise.all(bots.map(async (bot) => {
        try {
            const result = await listBotCapabilities(bot.uuid!);
            // The generated CapabilityListResponse types `capability` as a
            // bare string (OpenAPI has no enum for it); the app's own
            // BotCapability narrows it to the two real values the backend
            // ever sends. Safe to assert — this is the runtime contract, not
            // a guess.
            return {...bot, capabilities: (result.capabilities || []) as BotCapability[]};
        } catch {
            return {...bot, capabilities: []};
        }
    }));
}

// ========== Direct chats ==========

export const listBotDirectChats = (botUUID: string): Promise<{chats: DirectChatDetail[]}> =>
    botAccessCall((client, headers) => client.GET('/api/v1/bots/{bot}/chats', {
        headers,
        params: {path: {bot: botUUID}},
    })) as Promise<{chats: DirectChatDetail[]}>;

export const setBotDirectChatBlocked = (botUUID: string, chatID: string, blocked: boolean) =>
    botAccessCall((client, headers) => client.PUT('/api/v1/bots/{bot}/chats/{chat}/blocked', {
        headers,
        params: {path: {bot: botUUID, chat: chatID}},
        body: {blocked},
    }));

export const deleteBotDirectChat = (botUUID: string, chatID: string) =>
    botAccessCall((client, headers) => client.DELETE('/api/v1/bots/{bot}/chats/{chat}', {
        headers,
        params: {path: {bot: botUUID, chat: chatID}},
    }));

export const setBotDirectChatPermission = (botUUID: string, chatID: string, capability: string, action: string, effect: 'allow' | 'deny') =>
    botAccessCall((client, headers) => client.PUT('/api/v1/bots/{bot}/chats/{chat}/permissions/{capability}/{action}', {
        headers,
        params: {path: {bot: botUUID, chat: chatID, capability, action}},
        body: {effect},
    }));

export const setBotDirectChatPermissions = (botUUID: string, chatID: string, permissions: Array<{capability: string; action: string; effect: 'allow' | 'deny'}>) =>
    botAccessCall((client, headers) => client.PUT('/api/v1/bots/{bot}/chats/{chat}/permissions', {
        headers,
        params: {path: {bot: botUUID, chat: chatID}},
        body: {permissions},
    }));

// ========== Groups ==========

export const listBotGroups = (botUUID: string): Promise<{groups: BotGroup[]}> =>
    botAccessCall((client, headers) => client.GET('/api/v1/bots/{bot}/groups', {
        headers,
        params: {path: {bot: botUUID}},
    })) as Promise<{groups: BotGroup[]}>;

export const getBotGroup = (botUUID: string, groupID: string): Promise<BotGroupDetail> =>
    botAccessCall((client, headers) => client.GET('/api/v1/bots/{bot}/groups/{group}', {
        headers,
        params: {path: {bot: botUUID, group: groupID}},
    })) as unknown as Promise<BotGroupDetail>;

export const setBotGroupBlocked = (botUUID: string, groupID: string, blocked: boolean) =>
    botAccessCall((client, headers) => client.PUT('/api/v1/bots/{bot}/groups/{group}/blocked', {
        headers,
        params: {path: {bot: botUUID, group: groupID}},
        body: {blocked},
    }));

export const setBotGroupCapability = (botUUID: string, groupID: string, capability: string, effect: 'allow' | 'deny') =>
    botAccessCall((client, headers) => client.PUT('/api/v1/bots/{bot}/groups/{group}/capabilities/{capability}', {
        headers,
        params: {path: {bot: botUUID, group: groupID, capability}},
        body: {effect},
    }));

export const addBotGroupActor = (botUUID: string, groupID: string, actorID: string, externalActorID: string, displayName?: string) =>
    botAccessCall((client, headers) => client.PUT('/api/v1/bots/{bot}/groups/{group}/actors/{actor}', {
        headers,
        params: {path: {bot: botUUID, group: groupID, actor: actorID}},
        body: {external_actor_id: externalActorID, display_name: displayName, label: 'Controller'},
    }));

// ========== General bot interaction (notify / interact / wait) ==========
// See .design/bot-interaction-api.md. Distinct from the access-control calls
// above: these resolve with `{..., error}` rather than throwing, matching
// the long-poll/probe callers (useChatProbe.ts, NotifyTestDialog.tsx).

// List the chats a bot can reach (GET /api/v1/bots/:bot/chats — the same
// route listBotDirectChats() above calls) reshaped into the flatter
// BotChat projection BotChatsButton renders. The backend response has no
// running flag on this route (that only exists on the unused notify.ListChats
// handler); `running: true` mirrors prior behavior for a successful fetch.
export const listBotChats = async (botUUID: string): Promise<{chats?: BotChat[]; running?: boolean; error?: string}> => {
    try {
        const client = await getClient();
        const headers = await getAuthHeaders();
        const response = await client.GET('/api/v1/bots/{bot}/chats', {
            headers,
            params: {path: {bot: botUUID}},
        });
        // This route has no declared error response, so openapi-fetch's
        // generated type narrows `error` to `never` on the (only) success
        // branch; widen to read it defensively.
        const err = (response as {error?: unknown}).error;
        if (err) {
            return {error: errorMessage(err)};
        }
        return {
            chats: (response.data?.chats || []).map((item) => ({
                chat_id: item.chat.external_chat_id,
                id: item.chat.id,
                platform: item.chat.platform,
                is_paired: Boolean(item.chat.peer_actor_id),
                blocked: item.chat.blocked,
                can_notify: (item.permissions || []).some((permission) =>
                    permission.capability === 'notify' &&
                    permission.action === 'notify.receive' &&
                    permission.effect === 'allow'),
            })),
            running: true,
        };
    } catch (error: any) {
        return {error: error.message};
    }
};

// Send a one-way notification to a running bot's chat
// (POST /api/v1/bots/:bot/notify).
export const notifyBot = async (
    botUUID: string,
    body: {target: {kind: 'direct_chat' | 'group'; id: string}; title?: string; body: string; level?: string},
): Promise<{ok?: boolean; error?: string}> => {
    try {
        const client = await getClient();
        const headers = await getAuthHeaders();
        const response = await client.POST('/api/v1/bots/{bot}/notify', {
            headers,
            params: {path: {bot: botUUID}},
            body,
        });
        if (response.error) {
            return {error: errorMessage(response.error)};
        }
        return {ok: response.data?.ok};
    } catch (error: any) {
        return {error: error.message};
    }
};

// Start an interactive prompt on a running bot's chat
// (POST /api/v1/bots/:bot/interact). Returns request_id + wait_url +
// expires_at, or {error}. The route replies 202 on success; openapi-fetch
// routes any 2xx into `data` regardless of the exact declared status.
export const interactBot = async (
    botUUID: string,
    body: {
        target: {kind: 'direct_chat' | 'group'; id: string};
        kind: 'confirm' | 'choose' | 'ask';
        title: string;
        body?: string;
        options?: Array<{value: string; label: string; style?: string}>;
        timeout_seconds?: number;
    },
): Promise<{request_id?: string; wait_url?: string; expires_at?: string; error?: string}> => {
    try {
        const client = await getClient();
        const headers = await getAuthHeaders();
        const response = await client.POST('/api/v1/bots/{bot}/interact', {
            headers,
            params: {path: {bot: botUUID}},
            body,
        });
        if (response.error) {
            return {error: errorMessage(response.error)};
        }
        return {
            request_id: response.data?.request_id,
            wait_url: response.data?.wait_url,
            expires_at: response.data?.expires_at,
        };
    } catch (error: any) {
        return {error: error.message};
    }
};

// Local shape for the 410 (timeout/error) body: only the 200 response has a
// swagger response model (BotInteractWaitResponse), so the WithErrorResponses
// 410 entry is untyped even though the handler (respondResult in handler.go)
// sends the same {status, decision, reason} shape for both statuses.
interface BotInteractWaitBody {
    status?: string;
    decision?: Record<string, unknown>;
    reason?: string;
}

// Long-poll for the reply to an interactive prompt
// (GET /api/v1/bots/:bot/interact/:request_id?timeout=Ns). Returns a
// normalized status:
//   'answered' | 'cancelled' (200, carries decision)
//   'timeout' | 'error'     (410, carries decision/reason)
//   'pending'               (504 — caller retries)
//   'expired'               (404)
//   'unavailable'           (503)
// Transport failures fold into {error} (mirrors runProbe.ts).
export const waitBotInteract = async (
    botUUID: string,
    requestID: string,
    timeoutMs = 45000,
): Promise<{status?: string; decision?: Record<string, unknown>; reason?: string; error?: string}> => {
    try {
        const client = await getClient();
        const headers = await getAuthHeaders();
        const response = await client.GET('/api/v1/bots/{bot}/interact/{request_id}', {
            headers,
            params: {
                path: {bot: botUUID, request_id: requestID},
                query: {timeout: `${Math.floor(timeoutMs / 1000)}s`},
            },
        });
        const status = response.response.status;
        if (status === 504) return {status: 'pending'};
        if (status === 404) return {status: 'expired'};
        if (status === 503) return {status: 'unavailable'};
        // 200 (answered/cancelled) and 410 (timeout/error) both carry a
        // {status, decision, reason} body — only the HTTP status differs.
        const body = (response.data ?? response.error) as BotInteractWaitBody | undefined;
        if (body?.status) {
            return {status: body.status, decision: body.decision, reason: body.reason};
        }
        return {error: errorMessage(response.error) || `wait failed (${status})`};
    } catch (error: any) {
        return {error: error.message};
    }
};
