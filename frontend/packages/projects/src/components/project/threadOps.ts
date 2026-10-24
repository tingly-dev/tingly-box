// What a thread page does to its thread. Every thread is addressed the same
// way, by id under /threads (.design/projects-sessions.md): a project's worker
// thread, a project's main thread and a folder thread differ only in what the
// page offers, never in the route.
import { api, unwrap, type Attachment } from '../../client';
import type { SendMode } from '../Composer';

export interface ThreadOps {
    /** mode is only set when the user chose how to send while a turn runs; the default joins the turn. */
    send: (content: string, attachments?: Attachment[], mode?: SendMode) => Promise<unknown>;
    /** Takes back a message held for after the running turn. */
    unqueue: (id: string) => Promise<unknown>;
    respond: (requestId: string, allow: boolean, remember: boolean, answer?: string, answers?: Record<string, string>) => Promise<unknown>;
    stop: () => Promise<unknown>;
    /** Archive (resolve) the thread; it stays readable and can be reopened. */
    finish: () => Promise<unknown>;
    reopen: () => Promise<unknown>;
    /** Release a folder thread to a terminal: the command that resumes it. */
    handoff: () => Promise<string>;
    /** When a question with a suggested answer is answered without the user. */
    setAutoAnswer: (mode: '' | 'off' | 'delay' | 'now') => Promise<unknown>;
}

export function threadOps(id: string): ThreadOps {
    const path = { params: { path: { id } } };
    return {
        send: (content, attachments, mode) => unwrap(api.POST('/api/v1/threads/{id}/messages', { ...path, body: { content, attachments: attachments?.map((a) => a.path), mode } })),
        unqueue: (qid) => unwrap(api.DELETE('/api/v1/threads/{id}/queue/{qid}', { params: { path: { id, qid } } })),
        respond: (request_id, allow, remember, answer, answers) => unwrap(api.POST('/api/v1/threads/{id}/respond', {
            ...path, body: { request_id, allow, scope: remember ? 'thread' : 'once', answer: answer || undefined, answers: answers && Object.keys(answers).length ? answers : undefined },
        })),
        stop: () => unwrap(api.POST('/api/v1/threads/{id}/stop', path)),
        finish: () => unwrap(api.POST('/api/v1/threads/{id}/archive', path)),
        reopen: () => unwrap(api.POST('/api/v1/threads/{id}/reopen', path)),
        handoff: async () => (await unwrap(api.POST('/api/v1/threads/{id}/handoff', path))).command,
        setAutoAnswer: (auto_answer) => unwrap(api.PUT('/api/v1/threads/{id}/auto-answer', { ...path, body: { auto_answer } })),
    };
}
