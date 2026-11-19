import type { ProbeRequest, ProbeResult } from '@/types/probe';
import { controlApi } from '@/services/openapi';

// Generated curl data from POST /api/v2/probe/curl (backend probe.CurlData).
// Secrets arrive as placeholder env vars ($TB_API_KEY / $UPSTREAM_API_KEY).
export interface ProbeCurlData {
    command: string;
    method: string;
    url: string;
    headers: Record<string, string>;
    body: string;
    key_env_var: string;
}

export interface ProbeCurlResult {
    success: boolean;
    error?: { message: string; type: string };
    data?: ProbeCurlData;
}

// runProbe posts to /api/v2/probe and normalizes transport/HTTP failures into
// the same envelope shape the backend returns, so callers only handle one type.
export async function runProbe(body: ProbeRequest): Promise<ProbeResult> {
    const response = await controlApi((client, headers) => client.POST('/api/v2/probe', {
            headers,
            body,
        }));
    if (!response?.success) {
        return {
            success: false,
            error: {
                message: response?.error?.message || response?.error || 'Probe failed',
                type: response?.error?.type || 'client_error',
            },
        };
    }
    return response as ProbeResult;
}

// buildProbeCurl constructs the curl equivalent of a probe request without
// executing it. Same envelope normalization as runProbe.
export async function buildProbeCurl(body: ProbeRequest): Promise<ProbeCurlResult> {
    const response = await controlApi((client, headers) => client.POST('/api/v2/probe/curl', {
        headers,
        body,
    }));
    if (!response?.success) {
        return {
            success: false,
            error: {
                message: response?.error?.message || response?.error || 'Failed to build curl',
                type: response?.error?.type || 'client_error',
            },
        };
    }
    return response as ProbeCurlResult;
}

export { formatLatency } from '@/utils/formatLatency';
