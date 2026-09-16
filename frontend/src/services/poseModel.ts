// The pose estimator's files, as the gateway manages them: downloaded once on
// first use into the config directory, served same-origin from then on.
// See .design/pose-from-image.md §6.
import { getControlApiClient, getControlApiHeaders } from './openapi';
import { getApiBaseUrl } from '@/utils/protocol';

export interface PoseModelStatus {
    ready: boolean;
    baseUrl: string;      // absolute, trailing slash — what the estimator is pointed at
    totalBytes: number;
    files: { name: string; size: number; ready: boolean; error?: string }[];
}

const absolute = async (path: string): Promise<string> => `${await getApiBaseUrl()}${path}`;

export const getPoseModelStatus = async (): Promise<PoseModelStatus> => {
    const client = await getControlApiClient();
    const headers = await getControlApiHeaders();
    const { data, error } = await client.GET('/api/v1/pose/model', { headers });
    if (error || !data) throw new Error('pose model status unavailable');
    return {
        ready: data.ready === true,
        baseUrl: await absolute(data.base_url ?? '/pose-model/'),
        totalBytes: data.total_bytes ?? 0,
        files: (data.files ?? []).map((f) => ({ name: f.name ?? '', size: f.size ?? 0, ready: f.ready === true, error: f.error })),
    };
};

// Downloads whatever is missing. Slow the first time (about eighteen
// megabytes), instant after; the caller shows progress around it.
export const ensurePoseModel = async (): Promise<PoseModelStatus> => {
    const client = await getControlApiClient();
    const headers = await getControlApiHeaders();
    const { data, response } = await client.POST('/api/v1/pose/model/ensure', { headers });
    const body = data ?? (await response.clone().json().catch(() => null)) as typeof data;
    if (!body) throw new Error('pose model download failed');
    return {
        ready: body.ready === true,
        baseUrl: await absolute(body.base_url ?? '/pose-model/'),
        totalBytes: body.total_bytes ?? 0,
        files: (body.files ?? []).map((f) => ({ name: f.name ?? '', size: f.size ?? 0, ready: f.ready === true, error: f.error })),
    };
};
