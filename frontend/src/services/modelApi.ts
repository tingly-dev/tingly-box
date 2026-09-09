// Model gateway API — the OpenAI/Anthropic-compatible passthrough endpoints
// (/openai/v1/*, /anthropic/v1/*) plus the OpenAI SDK client builder for
// scenarios that want the real SDK (e.g. the ImageGen playground, which
// targets /tingly/{scenario}/v1).
//
// Distinct from services/api.ts's control-plane API (/api/v1/*, /api/v2/*):
// these hit the model gateway and authenticate with the model token, not the
// user token, so they don't go through controlApi() or the generated
// control-plane client. getOpenAIClient still needs the current model token,
// which the backend manages behind /api/v1/token — read straight from
// openapi.ts's generated client (not api.ts) to avoid a circular import.
import TinglyService from '@/bindings';
import {getApiBaseUrl} from '@/utils/protocol';
import OpenAI from 'openai';
import {controlApi} from './openapi';

// Get model token for OpenAI/Anthropic API from localStorage
const getModelToken = (): string | null => {
    return localStorage.getItem('model_token');
};

export const setModelToken = (token: string): void => {
    localStorage.setItem('model_token', token);
};

export const removeModelToken = (): void => {
    localStorage.removeItem('model_token');
};

// Fetch helper for model API endpoints (OpenAI/Anthropic compatible).
async function modelAPI(path: string, options: RequestInit = {}): Promise<any> {
    let token = getModelToken();

    // Try to get model token from GUI if available
    if (!token && import.meta.env.VITE_PKG_MODE === "gui") {
        const svc = TinglyService;
        if (svc) {
            try {
                const guiToken = await svc.GetUserAuthToken();
                if (guiToken) {
                    token = guiToken;
                }
            } catch (err) {
                console.error('Failed to get GUI token for modelAPI:', err);
            }
        }
    }

    const headers: Record<string, string> = {
        'Content-Type': 'application/json',
        ...options.headers as Record<string, string>,
    };
    if (token) headers['Authorization'] = `Bearer ${token}`;

    try {
        // Root-relative fetch() resolves against the page's own origin,
        // which is wrong in GUI/Wails mode (the webview's origin isn't the
        // backend's) — go through getApiBaseUrl() like the rest of the app.
        const base = await getApiBaseUrl();
        const response = await fetch(`${base}${path}`, {headers, ...options});
        return await response.json();
    } catch (error) {
        return {success: false, error: (error as Error).message};
    }
}

export const openAIChatCompletions = (data: any): Promise<any> => modelAPI('/openai/v1/chat/completions', {
    method: 'POST',
    body: JSON.stringify(data),
});
export const anthropicMessages = (data: any): Promise<any> => modelAPI('/anthropic/v1/messages', {
    method: 'POST',
    body: JSON.stringify(data),
});
export const listOpenAIModels = (): Promise<any> => modelAPI('/openai/v1/models');
export const listAnthropicModels = (): Promise<any> => modelAPI('/anthropic/v1/models');

/**
 * Build an OpenAI SDK client targeting a tingly scenario passthrough endpoint.
 * The model token is always sourced from /api/v1/token — the backend manages
 * (and auto-generates) it. dangerouslyAllowBrowser is intentional since calls
 * go through our own gateway, not directly to a provider.
 */
export const getOpenAIClient = async (scenario: string): Promise<OpenAI> => {
    const base = await getApiBaseUrl();
    const result = await controlApi((client, headers) => client.GET('/api/v1/token', {headers}));
    const apiKey = result?.token ?? '';
    return new OpenAI({
        baseURL: `${base}/tingly/${scenario}/v1`,
        apiKey,
        dangerouslyAllowBrowser: true,
        // Retry and fallback across providers is the gateway's job (its own
        // SDK clients run with retries off). The browser SDK would otherwise
        // silently re-send a request that came back 5xx/429 or timed out two
        // more times, so a failed image run sat as a spinner for three upstream
        // round-trips before the error reached the page.
        maxRetries: 0,
    });
};
