import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { resetControlApiClient } from './openapi';
import { tokenApi } from './tokenApi';

vi.mock('@/utils/protocol', () => ({ getApiBaseUrl: async () => 'https://example.test' }));

describe('token API', () => {
    beforeEach(() => {
        resetControlApiClient();
        localStorage.setItem('user_auth_token', 'synthetic-test-token');
    });

    afterEach(() => {
        vi.unstubAllGlobals();
        localStorage.clear();
    });

    // Enable / disable / delete answer 204 No Content; that is a success.
    it.each([
        ['enable', () => tokenApi.setAPITokenEnabled('tb-share-x', true)],
        ['disable', () => tokenApi.setAPITokenEnabled('tb-share-x', false)],
        ['delete', () => tokenApi.deleteAPIToken('tb-share-x')],
    ])('treats a 204 %s as success', async (_name, call) => {
        vi.stubGlobal('fetch', vi.fn(async () => new Response(null, { status: 204 })));
        expect(await call()).toEqual({ success: true, data: null });
    });

    it('still reports a failed request', async () => {
        vi.stubGlobal('fetch', vi.fn(async () => Response.json({ error: 'token not found' }, { status: 404 })));
        expect(await tokenApi.deleteAPIToken('tb-share-x')).toEqual({ success: false, error: 'token not found' });
    });

    it.each([null, 0, 1000000])('sends an explicit quota of %s', async (limit) => {
        const fetchMock = vi.fn(async (request: Request) => {
            expect(request.url).toBe('https://example.test/api/v1/tokens/tb-share-x/quota');
            expect(request.method).toBe('PUT');
            expect(await request.json()).toEqual({weekly_token_limit: limit});
            return Response.json({weekly_token_limit: limit, weekly_tokens_used: 123});
        });
        vi.stubGlobal('fetch', fetchMock);
        expect(await tokenApi.setAPITokenQuota('tb-share-x', limit)).toEqual({
            success: true, data: {weekly_token_limit: limit, weekly_tokens_used: 123},
        });
        expect(fetchMock).toHaveBeenCalledOnce();
    });
});
