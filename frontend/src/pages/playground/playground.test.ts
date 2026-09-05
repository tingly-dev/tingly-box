import { describe, expect, it } from 'vitest';
import { parsePlaygroundLink, playgroundDeepLink } from './playgroundLink';
import { DEFAULT_STATE, buildProbeRequest, runLabel, type PlaygroundState } from './playgroundState';
import { diffTopLevel } from './PayloadPanel';

const base = (over: Partial<PlaygroundState> = {}): PlaygroundState => ({
    ...DEFAULT_STATE,
    axes: { ...DEFAULT_STATE.axes },
    target: { kind: 'rule', ruleUuid: 'r1', scenario: 'claude_code' },
    ...over,
});

describe('playgroundLink', () => {
    it('round-trips a rule target with knobs', () => {
        const url = playgroundDeepLink({ targetType: 'rule', targetId: 'r1', scenario: 'claude_code:p1', axes: { stream: false, tool: true, thinking: 'high' }, message: 'hi there' });
        const parsed = parsePlaygroundLink(url.slice(url.indexOf('?')));
        expect(parsed.target).toEqual({ kind: 'rule', ruleUuid: 'r1', scenario: 'claude_code:p1' });
        expect(parsed.axes).toEqual({ stream: false, tool: true, thinking: 'high' });
        expect(parsed.message).toBe('hi there');
    });

    it('round-trips a provider target whose model contains slashes', () => {
        const url = playgroundDeepLink({ targetType: 'provider', targetId: 'p1', model: 'openai/gpt-5:free', axes: { direct: true, protocol: 'openai_responses' } });
        const parsed = parsePlaygroundLink(url.slice(url.indexOf('?')));
        expect(parsed.target).toEqual({ kind: 'provider', providerUuid: 'p1', model: 'openai/gpt-5:free' });
        expect(parsed.axes).toEqual({ direct: true, protocol: 'openai_responses' });
    });

    it('ignores garbage', () => {
        const parsed = parsePlaygroundLink('?target=nope&thinking=turbo&vision=x');
        expect(parsed.target).toBeNull();
        expect(parsed.axes).toEqual({});
    });
});

describe('buildProbeRequest', () => {
    it('returns null without a target', () => {
        expect(buildProbeRequest({ ...DEFAULT_STATE })).toBeNull();
    });

    it('sends only what the user set', () => {
        const req = buildProbeRequest(base())!;
        expect(req).toEqual({ target_type: 'rule', scenario: 'claude_code', rule_uuid: 'r1', stream: true, tool: false, thinking: 'none' });
    });

    it('carries overlay, conversation and hand edits through TB', () => {
        const req = buildProbeRequest(
            base({
                system: 'be brief',
                turns: [{ role: 'user', text: 'q' }],
                flags: { skip_usage: false },
                bodyOverrides: { temperature: 0.2, stop: null },
                headers: { 'X-Extra': '1' },
            }),
        )!;
        expect(req.system).toBe('be brief');
        expect(req.messages).toEqual([{ role: 'user', text: 'q' }]);
        expect(req.flags).toEqual({ skip_usage: false });
        expect(req.body_overrides).toEqual({ temperature: 0.2, stop: null });
        expect(req.headers).toEqual({ 'X-Extra': '1' });
    });

    it('sends the client identity only through TB', () => {
        expect(buildProbeRequest(base({ client: 'claude_code' }))!.client).toBe('claude_code');
        const direct = base({ target: { kind: 'provider', providerUuid: 'p1', model: 'm' }, axes: { ...DEFAULT_STATE.axes, direct: true }, client: 'claude_code' });
        expect(buildProbeRequest(direct)!.client).toBeUndefined();
    });

    it('sends routing only when a rule is pinned', () => {
        expect(buildProbeRequest(base())!.routing).toBeUndefined();
        expect(buildProbeRequest(base({ routing: 'pinned' }))!.routing).toBe('pinned');
    });

    it('drops the flag overlay on a direct provider probe', () => {
        const req = buildProbeRequest(
            base({ target: { kind: 'provider', providerUuid: 'p1', model: 'm' }, axes: { ...DEFAULT_STATE.axes, direct: true, protocol: 'openai_chat' }, flags: { skip_usage: true } }),
        )!;
        expect(req.target_type).toBe('provider');
        expect(req.direct).toBe(true);
        expect(req.protocol).toBe('openai_chat');
        expect(req.flags).toBeUndefined();
    });
});

describe('runLabel', () => {
    it('names what made the run different', () => {
        const label = runLabel(base({ flags: { a: 1, b: 2 }, axes: { ...DEFAULT_STATE.axes, tool: true }, turns: [{ role: 'user', text: 'x' }] }));
        expect(label).toBe('stream · tool · 2 flags · 1 turn');
    });
});

describe('diffTopLevel', () => {
    it('sets changed keys, deletes missing ones, keeps untouched overrides', () => {
        const displayed = { model: 'm', temperature: 0.5, stream: true };
        const edited = { model: 'm', temperature: 0.9, extra: { a: 1 } };
        expect(diffTopLevel(displayed, edited, { max_tokens: 5 })).toEqual({ max_tokens: 5, temperature: 0.9, stream: null, extra: { a: 1 } });
    });

    it('escapes dots for sjson paths', () => {
        expect(diffTopLevel({}, { 'a.b': 1 }, {})).toEqual({ 'a\\.b': 1 });
    });
});
