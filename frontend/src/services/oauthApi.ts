// OAuth control-plane API: authorize/status/cancel session management and
// token refresh.
import {rawControlCall} from './rawControlCall';
import {controlApi} from './openapi';

export const oauthApi = {
    // ============================================
    // OAuth API
    // ============================================

    // Initiate OAuth authorization flow
    oauthAuthorize: async (data: {
        provider: string;
        proxy_url?: string;
        redirect?: string;
        state?: string;
        // When set, re-authenticate this existing provider in place (preserves
        // its UUID and all rule/service references) instead of creating a new one.
        provider_uuid?: string;
    }): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/oauth/authorize', {
            headers,
            body: data as any
        })),

    // Get OAuth session status
    oauthStatus: async (session_id: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/oauth/status', {
            headers,
            params: {query: {session_id}}
        })),

    // Cancel an in-progress OAuth session
    oauthCancel: async (data: { session_id: string }): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/oauth/cancel', {
            headers,
            body: data
        })),

    // Refresh OAuth token. Deliberately not controlApi(): on a non-2xx the
    // backend's real error body ({success:false, error:"..."}) is preserved
    // under `data` (not just its extracted message) so callers
    // (CredentialPage.tsx) can read response.data?.error and decide whether
    // to guide the user to reauthorize.
    oauthRefresh: async (data: { provider_uuid: string }): Promise<any> =>
        rawControlCall(
            (client, headers) => client.POST('/api/v1/oauth/refresh', {headers, body: data}),
            (err) => ({success: false, error: 'Request failed', data: err}),
            (error: any) => ({success: false, error: error.message}),
        ),
};
