import {afterEach, describe, expect, it, vi} from 'vitest';

const controlApi = vi.hoisted(() => vi.fn());
vi.mock('./openapi', () => ({controlApi}));

import {profileApi} from './profileApi';

afterEach(() => controlApi.mockReset());

const route = {scenario: 'claude_code', quota: []};

describe('profileApi.getClaudeCodeRoute', () => {
    it('shares one request between callers asking for the same route at the same time', async () => {
        controlApi.mockResolvedValue(route);
        const [a, b] = await Promise.all([
            profileApi.getClaudeCodeRoute({profile: 'work', tier: 'sonnet', model: 'm'}),
            profileApi.getClaudeCodeRoute({profile: 'work', tier: 'sonnet', model: 'm'}),
        ]);
        expect(controlApi).toHaveBeenCalledTimes(1);
        expect(a).toEqual(route);
        expect(b).toBe(a);
    });

    it('asks again once the first request is done, and separately for another launch', async () => {
        controlApi.mockResolvedValue(route);
        await profileApi.getClaudeCodeRoute({profile: 'work', tier: 'sonnet'});
        await profileApi.getClaudeCodeRoute({profile: 'work', tier: 'sonnet'});
        expect(controlApi).toHaveBeenCalledTimes(2);
        await Promise.all([
            profileApi.getClaudeCodeRoute({profile: 'a', tier: ''}),
            profileApi.getClaudeCodeRoute({profile: 'b', tier: ''}),
        ]);
        expect(controlApi).toHaveBeenCalledTimes(4);
    });
});
