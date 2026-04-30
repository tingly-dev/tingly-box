import TinglyService from "@/bindings";

export interface OnboardingTokenCandidate {
    value: string;
    preview: string;
    source: string; // bearer | x-api-key | env:NAME | json:api_key | key_prefix
}

export interface OnboardingExtractResult {
    success: boolean;
    urls: string[];
    tokens: OnboardingTokenCandidate[];
    error?: string;
}

const getUserAuthToken = (): string | null => {
    return localStorage.getItem('user_auth_token');
};

const getAuthBearer = async (): Promise<string | null> => {
    let token = getUserAuthToken();
    if (!token && import.meta.env.VITE_PKG_MODE === "gui") {
        const svc = TinglyService;
        if (svc) {
            try {
                const guiToken = await svc.GetUserAuthToken();
                if (guiToken) token = guiToken;
            } catch (err) {
                console.error('Failed to get GUI token for onboarding extract:', err);
            }
        }
    }
    return token;
};

export async function extractOnboardingCandidates(input: string): Promise<OnboardingExtractResult> {
    const headers: Record<string, string> = {
        'Content-Type': 'application/json',
    };
    const token = await getAuthBearer();
    if (token) headers['Authorization'] = `Bearer ${token}`;

    try {
        const resp = await fetch('/api/v1/onboarding/extract', {
            method: 'POST',
            headers,
            body: JSON.stringify({input}),
        });
        const body = await resp.json();
        if (!body?.success) {
            return {
                success: false,
                urls: [],
                tokens: [],
                error: body?.error?.message || 'Extraction failed',
            };
        }
        return {
            success: true,
            urls: body?.data?.urls ?? [],
            tokens: body?.data?.tokens ?? [],
        };
    } catch (err) {
        return {
            success: false,
            urls: [],
            tokens: [],
            error: (err as Error).message,
        };
    }
}
