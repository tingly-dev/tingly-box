import { describe, expect, it } from 'vitest';
import { readEventStream } from './useProjectEvents';

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
