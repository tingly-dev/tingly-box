import { host } from '@/host';
import type { paths } from '@/client';
import { getApiBaseUrl } from '@/utils/protocol';
import createClient from 'openapi-fetch';

export type ApiClient = ReturnType<typeof createClient<paths>>;

let clientPromise: Promise<ApiClient> | null = null;

export const getControlApiClient = async (): Promise<ApiClient> => {
    if (!clientPromise) {
        clientPromise = getApiBaseUrl().then((baseUrl) => createClient<paths>({ baseUrl }));
    }
    return clientPromise;
};

export const resetControlApiClient = (): void => {
    clientPromise = null;
};

export const getControlApiHeaders = async (): Promise<Record<string, string>> => {
    const token = localStorage.getItem('user_auth_token');
    if (token) {
        return { Authorization: `Bearer ${token}` };
    }

    try {
        const shellToken = await host.shellAuthToken();
        if (shellToken) {
            return { Authorization: `Bearer ${shellToken}` };
        }
    } catch (error) {
        console.error('Failed to get GUI token:', error);
    }

    return {};
};

export const errorMessage = (error: unknown): string => {
    if (error instanceof Error) {
        return error.message;
    }
    if (typeof error === 'object' && error !== null) {
        const value = error as { error?: unknown; message?: unknown };
        if (typeof value.error === 'string' && value.error) return value.error;
        if (value.error && typeof value.error === 'object') {
            const nested = (value.error as { message?: unknown }).message;
            if (typeof nested === 'string' && nested) return nested;
        }
        return typeof value.message === 'string' && value.message ? value.message : 'Request failed';
    }
    if (typeof error === 'string' && error) {
        return error;
    }
    return 'Request failed';
};

// unwrap normalizes a raw openapi-fetch response for call sites that manage
// their own client/headers/try-catch (unlike controlApi, which owns all of
// that): non-2xx leaves `data` undefined and puts the parsed error body on
// `error`, so returning `data` raw would crash `result.success` readers and
// swallow the backend's message. Lives here so the message-extraction ladder
// (errorMessage) exists exactly once.
export const unwrap = (response: { data?: any; error?: any }): any => {
    if (response.data !== undefined) return response.data;
    return {success: false, error: errorMessage(response.error)};
};

export const controlApi = async (
    request: (client: ApiClient, headers: Record<string, string>) => Promise<any>,
): Promise<any> => {
    try {
        const [client, headers] = await Promise.all([
            getControlApiClient(),
            getControlApiHeaders(),
        ]);
        const response = await request(client, headers);
        return unwrap(response);
    } catch (error) {
        return { success: false, error: errorMessage(error) };
    }
};
