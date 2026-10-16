// Typed API client generated from ../openapi.json (`pnpm gen:api`). Do not
// hand-edit schema.d.ts; regenerate it with `task codegen`.
import createClient from 'openapi-fetch';
import type { components, paths } from './schema';

export type { components, paths };
type S = components['schemas'];

export type Project = S['Project'];
export type ProjectSummary = S['ProjectSummary'];
export type ProjectDetail = S['ProjectDetailResponse'];
export type ThreadCounts = S['ThreadCounts'];
export type ProjectMessage = S['ProjectMessage'];
export type Thread = S['Thread'];
export type ThreadEvent = S['ThreadEvent'];
export type ThreadDetail = S['ThreadDetailResponse'];
export type Diff = S['Diff'];
export type MemoryFile = S['MemoryFile'];
export type LibraryFile = S['LibraryFile'];
export type Usage = S['UsageResponse'];
export type Conversation = S['Conversation'];
export type ConversationDetail = S['ConversationDetailResponse'];
export type Message = S['Message'];
export type StreamEvent = S['StreamEvent'];
export type Settings = S['SettingsResponse'];
export type ConnectionDraft = S['ConnectionDraft'];
export type Model = S['Model'];
export type TestResponse = S['TestResponse'];

export type ProjectRuntime = S['ProjectRuntimeResponse'];

export type Workdir = S['Workdir'];
export type SessionDetail = S['SessionDetailResponse'];
export type BackgroundTask = S['BackgroundTask'];

let hostFetch: typeof fetch = (input, init) => globalThis.fetch(input, init);

/**
 * Sets the fetch every API call goes through, so an embedding host can add
 * its authentication. Undefined restores the browser's fetch.
 */
export function setApiFetch(f: typeof fetch | undefined) {
    hostFetch = f ?? ((input, init) => globalThis.fetch(input, init));
}

/** fetch through the host's fetch, for calls the typed client cannot make (streams, downloads). */
export function apiFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    return hostFetch(input, init);
}

export const api = createClient<paths>({ baseUrl: '', fetch: (req) => hostFetch(req) });

/** Error carrying the server's `{"error": "..."}` message. */
export class ApiError extends Error {
    status: number;
    body: unknown;
    constructor(message: string, status: number, body?: unknown) {
        super(message);
        this.status = status;
        this.body = body;
    }
}

/**
 * Unwraps an openapi-fetch result: returns `data`, or throws ApiError with
 * the server's message.
 */
export async function unwrap<T>(p: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
    const { data, error, response } = await p;
    if (!response.ok || data === undefined) {
        const msg = (error as { error?: string } | undefined)?.error ?? `${response.status} ${response.statusText}`;
        throw new ApiError(msg, response.status, error);
    }
    return data;
}
