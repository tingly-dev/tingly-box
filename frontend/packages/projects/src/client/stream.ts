// Chat streaming. openapi-fetch cannot read a text/event-stream body, so the
// two streaming endpoints are called with fetch directly; the event shape is
// still the generated StreamEvent type.
import { ApiError, apiFetch, type StreamEvent } from './index';

export interface StreamOptions {
    signal?: AbortSignal;
    onEvent: (ev: StreamEvent) => void;
}

async function stream(path: string, body: unknown, { signal, onEvent }: StreamOptions): Promise<void> {
    const res = await apiFetch(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
        body: JSON.stringify(body),
        signal,
    });
    if (!res.ok || !res.body) {
        let msg = `${res.status} ${res.statusText}`;
        try {
            msg = ((await res.json()) as { error?: string }).error ?? msg;
        } catch {
            // keep the status line
        }
        throw new ApiError(msg, res.status);
    }
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        let idx: number;
        while ((idx = buf.indexOf('\n\n')) >= 0) {
            const frame = buf.slice(0, idx);
            buf = buf.slice(idx + 2);
            for (const line of frame.split('\n')) {
                if (line.startsWith('data: ')) {
                    onEvent(JSON.parse(line.slice(6)) as StreamEvent);
                }
            }
        }
    }
}

export function sendMessage(conversationId: string, content: string, model: string | undefined, opts: StreamOptions) {
    return stream(`/api/v1/conversations/${conversationId}/messages`, { content, model: model || undefined }, opts);
}

export function regenerate(conversationId: string, model: string | undefined, opts: StreamOptions) {
    return stream(`/api/v1/conversations/${conversationId}/regenerate`, { model: model || undefined }, opts);
}
