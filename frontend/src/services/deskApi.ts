// Desk API — a web front door onto local Claude Code (see
// .design/desk.md). Split out of services/api.ts following the
// same pattern as botApi.ts: a small, typed slice of the generated OpenAPI
// client, callers use try/catch (these throw on failure) rather than the
// {success,data} envelope most of api.ts follows.
import type {components} from '@/client';
import type {ApiClient} from './openapi';
import {
    errorMessage,
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
} from './openapi';

export type SessionInfo = components['schemas']['SessionInfo'];
export type MessageInfo = components['schemas']['MessageInfo'];
export type RecentFolder = components['schemas']['RecentFolder'];
export type SessionStatus = components['schemas']['SessionStatusResponse'];
export type QuotaSegment = components['schemas']['QuotaSegmentInfo'];
export type ModelChoice = components['schemas']['ModelsResponse'];
export type ModelTier = components['schemas']['ModelTierInfo'];

type ClientCall<T> = (client: ApiClient, headers: Record<string, string>) => Promise<{
    data?: T;
    error?: unknown;
    response: Response;
}>;

async function call<T>(fn: ClientCall<T>): Promise<T> {
    const client = await getClient();
    const headers = await getAuthHeaders();
    const {data, error, response} = await fn(client, headers);
    // Only `error` means failure. Several endpoints here (send/respond/
    // interrupt) reply 202/204 with no body, which openapi-fetch reports as
    // a successful `data: undefined` — treating that as a failure (as
    // botAccessCall's copy of this check does) would make every one of
    // those calls throw despite the request succeeding.
    if (error !== undefined) {
        throw new Error(errorMessage(error) || `request failed (${response.status})`);
    }
    return data as T;
}

export const listRecentFolders = (limit?: number): Promise<RecentFolder[]> =>
    call((client, headers) => client.GET('/api/v1/desk/folders/recent', {
        headers,
        params: {query: limit ? {limit} : {}},
    })).then((r) => r.folders);

export const listPermissionModes = (): Promise<string[]> =>
    call((client, headers) => client.GET('/api/v1/desk/permission-modes', {headers}))
        .then((r) => r.modes);

export const listSessions = (active?: boolean): Promise<SessionInfo[]> =>
    call((client, headers) => client.GET('/api/v1/desk/sessions', {
        headers,
        params: {query: active ? {active} : {}},
    })).then((r) => r.sessions);

export const getSession = (sessionId: string): Promise<SessionInfo> =>
    call((client, headers) => client.GET('/api/v1/desk/sessions/{session_id}', {
        headers,
        params: {path: {session_id: sessionId}},
    }));

export const getMessages = (sessionId: string): Promise<MessageInfo[]> =>
    call((client, headers) => client.GET('/api/v1/desk/sessions/{session_id}/messages', {
        headers,
        params: {path: {session_id: sessionId}},
    })).then((r) => r.messages);

export const createSession = (path: string, prompt: string, permissionMode?: string, profile?: string, model?: string): Promise<SessionInfo> =>
    call((client, headers) => client.POST('/api/v1/desk/sessions', {
        headers,
        body: {path, prompt, permission_mode: permissionMode || '', profile: profile || '', model: model || ''},
    }));

// listModels lists the model tiers a profile offers ('' is the main routing).
export const listModels = (profile: string): Promise<ModelChoice> =>
    call((client, headers) => client.GET('/api/v1/desk/models', {
        headers,
        params: {query: {profile}},
    }));

export const setModel = (sessionId: string, model: string): Promise<SessionInfo> =>
    call((client, headers) => client.PUT('/api/v1/desk/sessions/{session_id}/model', {
        headers,
        params: {path: {session_id: sessionId}},
        body: {model},
    }));

export const setProfile = (sessionId: string, profile: string): Promise<SessionInfo> =>
    call((client, headers) => client.PUT('/api/v1/desk/sessions/{session_id}/profile', {
        headers,
        params: {path: {session_id: sessionId}},
        body: {profile},
    }));

export const getStatus = (sessionId: string): Promise<SessionStatus> =>
    call((client, headers) => client.GET('/api/v1/desk/sessions/{session_id}/status', {
        headers,
        params: {path: {session_id: sessionId}},
    }));

export const sendMessage = (sessionId: string, text: string): Promise<void> =>
    call((client, headers) => client.POST('/api/v1/desk/sessions/{session_id}/messages', {
        headers,
        params: {path: {session_id: sessionId}},
        body: {text},
    })).then(() => undefined);

export const respond = (sessionId: string, requestId: string, approved: boolean, answer?: string): Promise<void> =>
    call((client, headers) => client.POST('/api/v1/desk/sessions/{session_id}/respond', {
        headers,
        params: {path: {session_id: sessionId}},
        body: {request_id: requestId, approved, answer: answer || ''},
    })).then(() => undefined);

export const setPermissionMode = (sessionId: string, mode: string): Promise<SessionInfo> =>
    call((client, headers) => client.PUT('/api/v1/desk/sessions/{session_id}/permission-mode', {
        headers,
        params: {path: {session_id: sessionId}},
        body: {mode},
    }));

export const interrupt = (sessionId: string): Promise<void> =>
    call((client, headers) => client.POST('/api/v1/desk/sessions/{session_id}/interrupt', {
        headers,
        params: {path: {session_id: sessionId}},
    })).then(() => undefined);

export const archive = (sessionId: string): Promise<SessionInfo> =>
    call((client, headers) => client.POST('/api/v1/desk/sessions/{session_id}/archive', {
        headers,
        params: {path: {session_id: sessionId}},
    }));

// handoff releases the session's resident process and returns the shell
// command that resumes it in a local terminal.
export const handoff = (sessionId: string): Promise<string> =>
    call((client, headers) => client.POST('/api/v1/desk/sessions/{session_id}/handoff', {
        headers,
        params: {path: {session_id: sessionId}},
    })).then((r) => r.command);
