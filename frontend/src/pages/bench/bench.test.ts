import { describe, expect, it } from 'vitest';
import { DEFAULT_STATE, buildProbeRequest, runLabel, type BenchState } from './benchState';

const base = (over: Partial<BenchState> = {}): BenchState => ({
    ...DEFAULT_STATE,
    axes: { ...DEFAULT_STATE.axes },
    target: { providerUuid: 'p1', model: 'm' },
    ...over,
});

describe('buildProbeRequest', () => {
    it('returns nothing without a target', () => {
        expect(buildProbeRequest({ ...DEFAULT_STATE }).request).toBeNull();
    });

    it('fixture mode sends the knobs and the message', () => {
        const { request } = buildProbeRequest(base({ message: 'hello', axes: { ...DEFAULT_STATE.axes, tool: true } }));
        expect(request).toEqual({ target_type: 'provider', provider_uuid: 'p1', model: 'm', direct: false, stream: true, tool: true, thinking: 'none', message: 'hello' });
    });

    it('raw mode sends the request on its own protocol and drops the fixture knobs', () => {
        const { request } = buildProbeRequest(
            base({
                axes: { ...DEFAULT_STATE.axes, tool: true, thinking: 'high', protocol: 'openai_chat' },
                message: 'ignored',
                raw: { protocol: 'openai_responses', body: '{"input":[{"role":"user","content":"hi"}]}' },
                flags: { skip_usage: false },
                headers: { 'X-Extra': '1' },
            }),
        );
        expect(request).toEqual({
            target_type: 'provider', provider_uuid: 'p1', model: 'm', direct: false, stream: true,
            request: { input: [{ role: 'user', content: 'hi' }] }, request_protocol: 'openai_responses', protocol: 'openai_responses',
            flags: { skip_usage: false }, headers: { 'X-Extra': '1' },
        });
    });

    it('raw mode with a body that does not parse yields an error instead of a request', () => {
        const built = buildProbeRequest(base({ raw: { protocol: 'anthropic_v1', body: '{oops' } }));
        expect(built.request).toBeNull();
        expect(built.error).toBeTruthy();
        expect(buildProbeRequest(base({ raw: { protocol: 'anthropic_v1', body: '[1]' } })).error).toContain('object');
    });

    it('drops the flag overlay on a direct provider probe', () => {
        const { request } = buildProbeRequest(
            base({ axes: { ...DEFAULT_STATE.axes, direct: true, protocol: 'openai_chat' }, flags: { skip_usage: true } }),
        );
        expect(request!.direct).toBe(true);
        expect(request!.flags).toBeUndefined();
    });
});

describe('runLabel', () => {
    it('names what made the run different', () => {
        expect(runLabel(base({ flags: { a: 1, b: 2 }, axes: { ...DEFAULT_STATE.axes, tool: true } }))).toBe('stream · tool · 2 flags');
        expect(runLabel(base({ raw: { protocol: 'anthropic_v1', body: '{}' }, headers: { X: '1' } }))).toBe('stream · raw anthropic_v1 · 1 header');
    });
});
