import { useEffect, useRef } from 'react';
import { apiFetch, type ThreadCounts } from '../client';

export interface ProjectEvent {
    type: 'message_delta' | 'thread' | 'thread_event' | 'project' | 'coordinator';
    project_id: string;
    data: unknown;
    /** On a thread event: the project's thread counts after the change. */
    threads?: ThreadCounts;
}

const RETRY_MS = 2000;

/**
 * Reads a server-sent event stream and hands each `data:` payload to
 * onData. Resolves when the stream ends; rejects when it cannot be opened.
 */
export async function readEventStream(res: Response, onData: (data: string) => void): Promise<void> {
    if (!res.ok || !res.body) throw new Error(`${res.status} ${res.statusText}`);
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    for (;;) {
        const { done, value } = await reader.read();
        if (done) return;
        buf += decoder.decode(value, { stream: true });
        let idx: number;
        while ((idx = buf.indexOf('\n\n')) >= 0) {
            const frame = buf.slice(0, idx);
            buf = buf.slice(idx + 2);
            for (const line of frame.split('\n')) {
                if (line.startsWith('data: ')) onData(line.slice(6));
            }
        }
    }
}

/**
 * Subscribes to a project's live event stream, reconnecting when it drops.
 * onOpen runs on every reconnect so the page can re-fetch anything it missed
 * while offline. The stream is read with fetch rather than EventSource so it
 * goes through the host's authentication like every other request.
 */
export function useProjectEvents(projectId: string, onEvent: (ev: ProjectEvent) => void, onOpen?: () => void) {
    useEventStream(projectId ? `/api/v1/projects/${projectId}/events` : '', onEvent, onOpen);
}

/**
 * Every project's live events in one stream (the sidebar's and the folder
 * page's); streamed coordinator text is left out. Each event carries its
 * `project_id`.
 */
export function useAllProjectEvents(onEvent: (ev: ProjectEvent) => void, onOpen?: () => void, enabled = true) {
    useEventStream(enabled ? '/api/v1/projects/events' : '', onEvent, onOpen);
}

/**
 * Opens a live event stream outside React, reconnecting when it drops, and
 * returns the function that closes it. onOpen runs on every reconnect.
 */
export function openEventStream(url: string, onEvent: (ev: ProjectEvent) => void, onOpen?: () => void): () => void {
    const abort = new AbortController();
    const { signal } = abort;
    void (async () => {
        let first = true;
        while (!signal.aborted) {
            try {
                const res = await apiFetch(url, {
                    headers: { Accept: 'text/event-stream' },
                    signal,
                });
                if (res.ok && !first) onOpen?.();
                first = false;
                await readEventStream(res, (data) => {
                    try {
                        onEvent(JSON.parse(data) as ProjectEvent);
                    } catch {
                        // ignore malformed frames
                    }
                });
            } catch {
                // offline or refused: retry below
            }
            if (signal.aborted) return;
            await new Promise((r) => setTimeout(r, RETRY_MS));
        }
    })();
    return () => abort.abort();
}

function useEventStream(url: string, onEvent: (ev: ProjectEvent) => void, onOpen?: () => void) {
    const handler = useRef(onEvent);
    const opened = useRef(onOpen);
    handler.current = onEvent;
    opened.current = onOpen;

    useEffect(() => {
        if (!url) return;
        return openEventStream(url, (ev) => handler.current(ev), () => opened.current?.());
    }, [url]);
}
