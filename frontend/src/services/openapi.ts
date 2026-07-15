import TinglyService from '@/bindings';
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

    if (import.meta.env.VITE_PKG_MODE === 'gui' && TinglyService) {
        try {
            const guiToken = await TinglyService.GetUserAuthToken();
            if (guiToken) {
                return { Authorization: `Bearer ${guiToken}` };
            }
        } catch (error) {
            console.error('Failed to get GUI token:', error);
        }
    }

    return {};
};

const errorMessage = (error: unknown): string => {
    if (error instanceof Error) {
        return error.message;
    }
    if (typeof error === 'object' && error !== null) {
        const value = error as { error?: string; message?: string };
        return value.error || value.message || 'Request failed';
    }
    return 'Request failed';
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
        if (response.error) {
            return { success: false, error: errorMessage(response.error) };
        }
        return response.data;
    } catch (error) {
        return { success: false, error: errorMessage(error) };
    }
};
