// Multi-tenant API token + team control-plane API.
//
// Two contracts coexist here:
// - The plain token endpoints (list/create/delete/enable) follow the
//   repo-wide {success,data}/{success,error} convention and get a small
//   shared wrapper below.
// - Team endpoints don't follow that convention — callers (UseTeamPage.tsx,
//   SharingKeysDialog.tsx) read `result.error?.message`, an object. Keep
//   teamApiCall's contract rather than forcing it through controlApi()/
//   unwrap().
import {
    controlApi,
    errorMessage,
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
} from './openapi';

async function teamApiCall<T>(
    call: (client: Awaited<ReturnType<typeof getClient>>, headers: Record<string, string>) => Promise<{data?: T; error?: unknown; response: Response}>,
): Promise<{success: boolean; data?: T; error?: {message: string}}> {
    try {
        const client = await getClient();
        const headers = await getAuthHeaders();
        const {data, error, response} = await call(client, headers);
        if (data === undefined || error !== undefined) {
            return {success: false, error: {message: errorMessage(error) || `request failed (${response.status})`}};
        }
        return {success: true, data};
    } catch (error: any) {
        return {success: false, error: {message: error.message || 'Team API request failed'}};
    }
}

// Shared wrap for the plain token endpoints: pass the controlApi error
// through verbatim, otherwise normalize to {success, data}.
async function tokenWrap(call: (client: Awaited<ReturnType<typeof getClient>>, headers: Record<string, string>) => Promise<any>): Promise<any> {
    const data = await controlApi(call);
    if (data?.success === false) {
        return data;
    }
    return {success: true, data};
}

export const tokenApi = {
    // ============================================
    // API Token Management (Multi-Tenant)
    // ============================================

    // List all API tokens
    listAPITokens: async (params?: {
        user_id?: string;
        team_id?: string;
        enabled?: boolean;
        limit?: number;
        offset?: number;
    }): Promise<any> =>
        tokenWrap((client, headers) => client.GET('/api/v1/tokens', {
            headers,
            params: {query: params as any}
        })),

    // Create a new API token
    createAPIToken: async (data: {
        display_name: string;
        team_id?: string;
    }): Promise<any> =>
        tokenWrap((client, headers) => client.POST('/api/v1/tokens', {
            headers,
            body: data as any
        })),

    // Delete an API token
    deleteAPIToken: async (tokenId: string): Promise<any> =>
        tokenWrap((client, headers) => client.DELETE('/api/v1/tokens/{token_id}', {
            headers,
            params: {path: {token_id: tokenId}}
        })),

    // Enable an API token
    setAPITokenEnabled: async (tokenId: string, enabled: boolean): Promise<any> =>
        tokenWrap((client, headers) => client.PUT(enabled
            ? '/api/v1/tokens/{token_id}/enable'
            : '/api/v1/tokens/{token_id}/disable', {
            headers,
            params: {path: {token_id: tokenId}}
        })),

    moveAPITokenToTeam: async (tokenId: string, teamId: string): Promise<any> =>
        teamApiCall((client, headers) => client.PUT('/api/v1/tokens/{token_id}/team', {
            headers,
            params: {path: {token_id: tokenId}},
            body: {team_id: teamId},
        })),

    listTeams: async (): Promise<any> =>
        teamApiCall((client, headers) => client.GET('/api/v1/teams', {headers})),

    createTeam: async (data: {name: string}): Promise<any> =>
        teamApiCall((client, headers) => client.POST('/api/v1/teams', {headers, body: data})),

    updateTeam: async (teamId: string, data: {name: string; quota_visible?: boolean}): Promise<any> =>
        teamApiCall((client, headers) => client.PUT('/api/v1/teams/{team_id}', {
            headers,
            params: {path: {team_id: teamId}},
            body: data,
        })),

    setTeamEnabled: async (teamId: string, enabled: boolean): Promise<any> =>
        teamApiCall((client, headers) => enabled
            ? client.PUT('/api/v1/teams/{team_id}/enable', {headers, params: {path: {team_id: teamId}}})
            : client.PUT('/api/v1/teams/{team_id}/disable', {headers, params: {path: {team_id: teamId}}})),

    deleteTeam: async (teamId: string): Promise<any> =>
        teamApiCall((client, headers) => client.DELETE('/api/v1/teams/{team_id}', {
            headers,
            params: {path: {team_id: teamId}},
        })),
};