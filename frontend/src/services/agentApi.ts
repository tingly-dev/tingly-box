// Managed agent control plane (/api/v1/agent/*) — typed over the generated
// OpenAPI client. Every call resolves to a discriminated result so pages can
// branch without try/catch; the backend's {error:{message}} body is
// surfaced verbatim (it carries the validation / conflict reason).
import type {components} from '@/client';
import {getControlApiClient, getControlApiHeaders} from './openapi';

type Schemas = components['schemas'];
export type AgentSource = Schemas['Source'];
export type AgentEnvironment = Schemas['Environment'];
export type AgentWorkspace = Schemas['Workspace'];
export type AgentSession = Schemas['Session'];
export type AgentEvent = Schemas['Event'];
export type AgentDiff = Schemas['Diff'];
export type SessionDetail = Schemas['SessionDetail'];
export type SessionListItem = Schemas['SessionListItem'];
export type SourceRequest = Schemas['SourceRequest'];
export type EnvironmentRequest = Schemas['EnvironmentRequest'];
export type CreateSessionRequest = Schemas['CreateSessionRequest'];

export type SessionStatus =
    | 'queued' | 'running' | 'waiting_input' | 'idle' | 'done' | 'failed' | 'archived';

export const isActiveStatus = (s: string | undefined): boolean =>
    s === 'queued' || s === 'running' || s === 'waiting_input' || s === 'idle';

export type AgentResult<T> = {ok: true; data: T} | {ok: false; error: string; status: number};

const messageOf = (error: unknown, status: number): string => {
    if (typeof error === 'object' && error !== null) {
        const e = error as {error?: {message?: string} | string; message?: string};
        if (typeof e.error === 'object' && e.error?.message) return e.error.message;
        if (typeof e.error === 'string') return e.error;
        if (e.message) return e.message;
    }
    return `request failed (${status})`;
};

async function call<T>(
    fn: (client: Awaited<ReturnType<typeof getControlApiClient>>, headers: Record<string, string>) =>
        Promise<{data?: T; error?: unknown; response: Response}>,
): Promise<AgentResult<T>> {
    try {
        const [client, headers] = await Promise.all([getControlApiClient(), getControlApiHeaders()]);
        const {data, error, response} = await fn(client, headers);
        if (error !== undefined || !response.ok) {
            return {ok: false, error: messageOf(error, response.status), status: response.status};
        }
        return {ok: true, data: data as T};
    } catch (e: any) {
        return {ok: false, error: e?.message || 'network error', status: 0};
    }
}

export const agentApi = {
    // ---- sources
    listSources: () =>
        call<{sources: AgentSource[]}>((c, headers) => c.GET('/api/v1/agent/sources', {headers})),
    createSource: (body: SourceRequest) =>
        call<AgentSource>((c, headers) => c.POST('/api/v1/agent/sources', {headers, body})),
    updateSource: (id: string, body: SourceRequest) =>
        call<AgentSource>((c, headers) => c.PUT('/api/v1/agent/sources/{source_id}', {
            headers, params: {path: {source_id: id}}, body,
        })),
    deleteSource: (id: string) =>
        call<unknown>((c, headers) => c.DELETE('/api/v1/agent/sources/{source_id}', {
            headers, params: {path: {source_id: id}},
        })),

    // ---- environments
    listEnvironments: () =>
        call<{environments: AgentEnvironment[]; supported_runtimes: string[]}>(
            (c, headers) => c.GET('/api/v1/agent/environments', {headers})),
    createEnvironment: (body: EnvironmentRequest) =>
        call<AgentEnvironment>((c, headers) => c.POST('/api/v1/agent/environments', {headers, body})),
    updateEnvironment: (id: string, body: EnvironmentRequest) =>
        call<AgentEnvironment>((c, headers) => c.PUT('/api/v1/agent/environments/{environment_id}', {
            headers, params: {path: {environment_id: id}}, body,
        })),
    deleteEnvironment: (id: string) =>
        call<unknown>((c, headers) => c.DELETE('/api/v1/agent/environments/{environment_id}', {
            headers, params: {path: {environment_id: id}},
        })),

    // ---- sessions
    listSessions: (query: {active?: boolean; workspace_id?: string; limit?: number} = {}) =>
        call<{sessions: SessionListItem[]}>((c, headers) => c.GET('/api/v1/agent/sessions', {
            headers, params: {query},
        })),
    createSession: (body: CreateSessionRequest) =>
        call<SessionDetail>((c, headers) => c.POST('/api/v1/agent/sessions', {headers, body})),
    getSession: (id: string) =>
        call<SessionDetail>((c, headers) => c.GET('/api/v1/agent/sessions/{session_id}', {
            headers, params: {path: {session_id: id}},
        })),
    listEvents: (id: string, after = 0, limit = 0) =>
        call<{events: AgentEvent[]; next: number}>((c, headers) => c.GET('/api/v1/agent/sessions/{session_id}/events', {
            headers, params: {path: {session_id: id}, query: {after, limit}},
        })),
    sendMessage: (id: string, text: string) =>
        call<unknown>((c, headers) => c.POST('/api/v1/agent/sessions/{session_id}/messages', {
            headers, params: {path: {session_id: id}}, body: {text},
        })),
    respond: (id: string, body: {request_id: string; approved: boolean; answer?: string}) =>
        call<unknown>((c, headers) => c.POST('/api/v1/agent/sessions/{session_id}/respond', {
            headers, params: {path: {session_id: id}}, body: {answer: '', ...body},
        })),
    interrupt: (id: string) =>
        call<unknown>((c, headers) => c.POST('/api/v1/agent/sessions/{session_id}/interrupt', {
            headers, params: {path: {session_id: id}},
        })),
    archive: (id: string) =>
        call<SessionDetail>((c, headers) => c.POST('/api/v1/agent/sessions/{session_id}/archive', {
            headers, params: {path: {session_id: id}},
        })),
    diff: (id: string) =>
        call<AgentDiff>((c, headers) => c.GET('/api/v1/agent/sessions/{session_id}/diff', {
            headers, params: {path: {session_id: id}},
        })),
    push: (id: string) =>
        call<SessionDetail>((c, headers) => c.POST('/api/v1/agent/sessions/{session_id}/push', {
            headers, params: {path: {session_id: id}},
        })),
};
