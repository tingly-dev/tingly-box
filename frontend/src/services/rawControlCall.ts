// Shared skeleton for control-plane endpoints that cannot go through
// openapi.ts's controlApi(): their routes have no declared error response,
// so openapi-fetch narrows `error` to `never` on the (only) success branch
// and the call sites must widen it to read one defensively at runtime (a
// non-2xx is still possible, e.g. a 401 from auth middleware). Each call
// site supplies its own error/exception handlers because the return
// contracts differ — the config apply/preview endpoints surface
// `{success:false, message}` (callers read `message`, not `error`), while
// oauthRefresh preserves the backend's raw error body under `data`.
import type { ApiClient } from './openapi';
import {
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
    unwrap,
} from './openapi';

export async function rawControlCall(
    request: (client: ApiClient, headers: Record<string, string>) => Promise<any>,
    onError: (err: unknown) => any,
    onException: (error: any) => any,
): Promise<any> {
    try {
        const client = await getClient();
        const headers = await getAuthHeaders();
        const response = await request(client, headers);
        // No declared error response narrows `error` to `never` on the
        // success branch; widen to read it defensively.
        const err = (response as {error?: unknown}).error;
        if (err) {
            return onError(err);
        }
        return unwrap(response);
    } catch (error: any) {
        return onException(error);
    }
}
