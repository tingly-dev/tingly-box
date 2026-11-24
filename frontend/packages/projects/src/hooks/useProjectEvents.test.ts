import { renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { readEventStream, useProjectEvents } from './useProjectEvents';

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }));
vi.mock('../client', async (actual) => ({ ...(await actual<typeof import('../client')>()), apiFetch }));

function streamOf(chunks: string[]): Response {
    const enc = new TextEncoder();
    return new Response(new ReadableStream({
        start(c) {
            for (const ch of chunks) c.enqueue(enc.encode(ch));
            c.close();
        },
    }));
}

describe('readEventStream', () => {
    it('yields data frames split across chunks and skips comments', async () => {
        const got: string[] = [];
        await readEventStream(streamOf([': connected\n\n', 'data: {"a":', '1}\n\ndata: {"b":2}\n\n: ping\n\n']), (d) => got.push(d));
        expect(got).toEqual(['{"a":1}', '{"b":2}']);
    });

    it('rejects a refused stream', async () => {
        await expect(readEventStream(new Response('no', { status: 401 }), () => {})).rejects.toThrow('401');
    });
});

describe('useProjectEvents', () => {
    it('forwards events to the latest callback on the existing connection', async () => {
        const first = vi.fn();
        const second = vi.fn();
        let send = (_event: string) => {};
        const stream = new ReadableStream<Uint8Array>({
            start(controller) {
                send = (event) => controller.enqueue(new TextEncoder().encode(`data: ${event}\n\n`));
            },
        });
        apiFetch.mockResolvedValueOnce(new Response(stream));
        const view = renderHook(({ onEvent }) => useProjectEvents('p1', onEvent), { initialProps: { onEvent: first } });
        const event = { type: 'thread', project_id: 'p1', data: {} };

        send(JSON.stringify(event));
        await waitFor(() => expect(first).toHaveBeenCalledExactlyOnceWith(event));
        view.rerender({ onEvent: second });
        send(JSON.stringify(event));
        await waitFor(() => expect(second).toHaveBeenCalledExactlyOnceWith(event));

        expect(first).toHaveBeenCalledTimes(1);
        expect(apiFetch).toHaveBeenCalledTimes(1);
        const signal = apiFetch.mock.calls[0][1].signal as AbortSignal;
        view.unmount();
        expect(signal.aborted).toBe(true);
    });
});
