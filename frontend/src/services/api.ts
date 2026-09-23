// API service layer for communicating with the backend.
//
// `api` is an aggregate: the per-domain methods live in sibling modules
// (systemApi/rulesApi/... — same pattern as botApi.ts/modelApi.ts) and are
// spread in below, so every existing `api.<method>` call site keeps working.
// Only the pieces that are genuinely cross-domain (client init, the raw
// fetchUIAPI helper, user-auth-token management) remain defined here.
import * as botApi from './botApi';
import * as modelApi from './modelApi';
import {getApiBaseUrl} from '../utils/protocol';
import {
    controlApi,
    getControlApiClient as getClient,
    resetControlApiClient as resetClient,
} from './openapi';
import {systemApi} from './systemApi';
import {rulesApi} from './rulesApi';
import {scenarioApi} from './scenarioApi';
import {profileApi} from './profileApi';
import {guardrailsApi} from './guardrailsApi';
import {usageApi} from './usageApi';
import {oauthApi} from './oauthApi';
import {configApplyApi} from './configApplyApi';
import {skillApi} from './skillApi';
import {imbotApi} from './imbotApi';
import {tokenApi} from './tokenApi';

// IM bot interaction (capabilities/chats/groups/permissions + notify/interact/
// wait) lives in its own module — see botApi.ts for why it follows a
// different contract than the rest of this file.
export {enrichBotsWithCapabilities} from './botApi';

// Get user auth token for UI and control API from localStorage
const getUserAuthToken = (): string | null => {
    return localStorage.getItem('user_auth_token');
};

// Raw fetch helper for tingly-box's own control-plane API (`/api/v1/...`),
// authenticated with the browser's `user_auth_token`. Resolves the base URL
// through getApiBaseUrl() rather than window.location.origin so it also
// works in GUI/Wails mode, where the frontend's own origin does not
// necessarily match the backend's port.
export async function fetchUIAPI(url: string, options: RequestInit = {}): Promise<any> {
    const base = await getApiBaseUrl();
    const fullUrl = `${base}/api/v1${url}`;

    const token = getUserAuthToken();

    const response = await fetch(fullUrl, {
        ...options,
        headers: {
            'Content-Type': 'application/json',
            ...(token && {Authorization: `Bearer ${token}`}),
            ...options.headers,
        },
    });

    if (!response.ok) {
        // The status rides along so callers can tell "this provider has no
        // quota" (404) from a real failure, and stay quiet about the former.
        throw Object.assign(new Error(`API error: ${response.status}`), {status: response.status});
    }

    return response.json();
}

export const api = {
    // Initialize API client
    initialize: async (): Promise<void> => {
        await getClient();
    },

    ...systemApi,
    ...rulesApi,
    ...scenarioApi,
    ...profileApi,
    ...guardrailsApi,
    ...usageApi,
    ...oauthApi,
    ...configApplyApi,
    ...skillApi,
    ...imbotApi,
    ...tokenApi,

    // Model gateway API (OpenAI/Anthropic-compatible) — see modelApi.ts.
    listOpenAIModels: modelApi.listOpenAIModels,
    listAnthropicModels: modelApi.listAnthropicModels,

    // IM bot capabilities/chats/groups/permissions + notify/interact/wait —
    // see botApi.ts.
    listBotCapabilities: botApi.listBotCapabilities,
    setBotCapability: botApi.setBotCapability,
    listBotDirectChats: botApi.listBotDirectChats,
    setBotDirectChatBlocked: botApi.setBotDirectChatBlocked,
    deleteBotDirectChat: botApi.deleteBotDirectChat,
    setBotDirectChatPermission: botApi.setBotDirectChatPermission,
    setBotDirectChatPermissions: botApi.setBotDirectChatPermissions,
    listBotGroups: botApi.listBotGroups,
    getBotGroup: botApi.getBotGroup,
    setBotGroupBlocked: botApi.setBotGroupBlocked,
    setBotGroupCapability: botApi.setBotGroupCapability,
    addBotGroupActor: botApi.addBotGroupActor,
    listBotChats: botApi.listBotChats,
    notifyBot: botApi.notifyBot,
    interactBot: botApi.interactBot,
    waitBotInteract: botApi.waitBotInteract,

    // User Token Management APIs
    // Get current user token (masked)
    getUserAuthTokenInfo: async (): Promise<{
        success: boolean;
        data?: { token: string; is_default: boolean };
        error?: string
    }> => controlApi((client, headers) => client.GET('/api/v1/auth/token', {headers})),

    // Reset user token to a new secure random value
    resetUserToken: async (): Promise<{ success: boolean; data?: { token: string }; error?: string }> => {
        const result = await controlApi((client, headers) => client.POST('/api/v1/auth/token/reset', {headers}));
        if (result?.success && result.data?.token) {
            localStorage.setItem('user_auth_token', result.data.token);
            resetClient();
        }
        return result;
    },

    // Reset model token to a new secure random value
    resetModelToken: async (): Promise<{ success: boolean; data?: { token: string }; error?: string }> =>
        controlApi((client, headers) => client.POST('/api/v1/auth/model-token/reset', {headers})),
};

export default api;
