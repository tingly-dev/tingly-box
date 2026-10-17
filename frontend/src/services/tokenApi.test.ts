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
});
