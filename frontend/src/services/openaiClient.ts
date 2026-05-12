import OpenAI from 'openai';
import { getApiBaseUrl } from '@/utils/protocol';
import { api } from '@/services/api';

/**
 * Build an OpenAI SDK client targeting a tingly scenario passthrough endpoint.
 * The model token is always sourced from /api/v1/token — the backend manages
 * (and auto-generates) it. dangerouslyAllowBrowser is intentional since calls
 * go through our own gateway, not directly to a provider.
 */
export const getOpenAIClient = async (scenario: string): Promise<OpenAI> => {
    const base = await getApiBaseUrl();
    const result = await api.getToken();
    const apiKey = result?.token ?? '';
    return new OpenAI({
        baseURL: `${base}/tingly/${scenario}/v1`,
        apiKey,
        dangerouslyAllowBrowser: true,
    });
};
