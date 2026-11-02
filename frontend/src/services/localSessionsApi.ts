import type { components } from '@/client';
import { controlApi } from './openapi';

export type SessionEntry = components['schemas']['LocalSessionEntry'];
export type SessionListing = components['schemas']['LocalSessionListing'];
export type SessionPage = components['schemas']['LocalSessionPage'];
export type SessionEvent = components['schemas']['LocalSessionEvent'];
export type SessionBlock = components['schemas']['LocalSessionBlock'];
export type SessionStatus = components['schemas']['LocalSessionStatus'];

async function checked<T>(request: Parameters<typeof controlApi>[0]): Promise<T> {
    const result = await controlApi(request);
    if (!result?.success || !result.data) throw new Error(result?.error || 'Unable to load local sessions.');
    return result.data as T;
}

export const localSessionsApi = {
    status: (signal?: AbortSignal) => checked<SessionStatus>((client, headers) =>
        client.GET('/api/v2/local-sessions/status', { headers, signal })),
    projects: (cursor = '', signal?: AbortSignal) => checked<SessionListing>((client, headers) =>
        client.GET('/api/v2/local-sessions/projects', { headers, signal, params: { query: { cursor } } })),
    sessions: (project: string, cursor = '', signal?: AbortSignal) => checked<SessionListing>((client, headers) =>
        client.GET('/api/v2/local-sessions/projects/{project}/sessions', { headers, signal, params: { path: { project }, query: { cursor } } })),
    preview: (session: string, cursor = '', direction = 'backward', signal?: AbortSignal) => checked<SessionPage>((client, headers) =>
        client.GET('/api/v2/local-sessions/sessions/{session}/events', { headers, signal, params: { path: { session }, query: { cursor, direction } } })),
};
