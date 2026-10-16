// What a thread page does to its thread, over either set of routes: a
// project's thread, or a folder session (a thread with no project). The
// panel works with one of these so it never branches on the URL itself.
import { api, unwrap } from '../../client';

export interface ThreadOps {
    send: (content: string) => Promise<unknown>;
    respond: (requestId: string, allow: boolean, remember: boolean, answer?: string) => Promise<unknown>;
    stop: () => Promise<unknown>;
    /** Resolve a thread; archive a session. */
    finish: () => Promise<unknown>;
    reopen: () => Promise<unknown>;
    /** Release a session to a terminal: the command that resumes it. Threads have none. */
    handoff: () => Promise<string>;
}

export function projectThreadOps(projectId: string, tid: string): ThreadOps {
    const path = { params: { path: { id: projectId, tid } } };
    return {
        send: (content) => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/messages', { ...path, body: { content } })),
        respond: (request_id, allow, remember, answer) => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/respond', {
            ...path, body: { request_id, allow, scope: remember ? 'thread' : 'once', answer: answer || undefined },
        })),
        stop: () => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/stop', path)),
        finish: () => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/resolve', path)),
        reopen: () => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/reopen', path)),
        handoff: () => Promise.reject(new Error('a project thread is continued in its worktree')),
    };
}

export function sessionOps(sid: string): ThreadOps {
    const path = { params: { path: { sid } } };
    return {
        send: (content) => unwrap(api.POST('/api/v1/projects/sessions/{sid}/messages', { ...path, body: { content } })),
        respond: (request_id, allow, remember, answer) => unwrap(api.POST('/api/v1/projects/sessions/{sid}/respond', {
            ...path, body: { request_id, allow, scope: remember ? 'thread' : 'once', answer: answer || undefined },
        })),
        stop: () => unwrap(api.POST('/api/v1/projects/sessions/{sid}/stop', path)),
        finish: () => unwrap(api.POST('/api/v1/projects/sessions/{sid}/archive', path)),
        reopen: () => unwrap(api.POST('/api/v1/projects/sessions/{sid}/reopen', path)),
        handoff: async () => (await unwrap(api.POST('/api/v1/projects/sessions/{sid}/handoff', path))).command,
    };
}
