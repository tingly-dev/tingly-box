import type { ProbeRequest, ProbeResult } from '@/types/probe';

const getUserAuthToken = (): string | null => localStorage.getItem('user_auth_token');

// runProbe posts to /api/v2/probe and normalizes transport/HTTP failures into
// the same envelope shape the backend returns, so callers only handle one type.
export async function runProbe(body: ProbeRequest): Promise<ProbeResult> {
    const token = getUserAuthToken();
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (token) headers['Authorization'] = `Bearer ${token}`;

    try {
        const response = await fetch('/api/v2/probe', {
            method: 'POST',
            headers,
            body: JSON.stringify(body),
        });
        if (!response.ok) {
            let message = `HTTP ${response.status}`;
            try {
                const e = await response.json();
                message = e.error?.message || message;
            } catch {
                /* ignore */
            }
            return { success: false, error: { message, type: 'http_error' } };
        }
        return await response.json();
    } catch (err: any) {
        return { success: false, error: { message: err?.message || 'Probe failed', type: 'client_error' } };
    }
}

// formatLatency renders a millisecond latency compactly: "850ms" / "1.8s".
export const formatLatency = (ms: number): string => (ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`);
