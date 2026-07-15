import { controlApi } from './openapi';

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

export async function extractOnboardingCandidates(input: string): Promise<OnboardingExtractResult> {
    const body = await controlApi((client, headers) => client.POST('/api/v1/onboarding/extract', {
        headers,
        body: {input},
    }));
    if (!body?.success) {
        return {
            success: false,
            urls: [],
            tokens: [],
            error: body?.error?.message || body?.error || 'Extraction failed',
        };
    }
    return {
        success: true,
        urls: body?.data?.urls ?? [],
        tokens: body?.data?.tokens ?? [],
    };
}
