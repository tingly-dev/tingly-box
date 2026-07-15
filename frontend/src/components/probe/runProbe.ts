import type { ProbeRequest, ProbeResult } from '@/types/probe';
import { controlApi } from '@/services/openapi';

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

// formatLatency renders a millisecond latency compactly: "850ms" / "1.8s".
export const formatLatency = (ms: number): string => (ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`);
