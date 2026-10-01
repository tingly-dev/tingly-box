// The image archive (what the gateway saved under ~/.tingly-box/image) and the
// focus workbenches built on it — /api/v1/imagegen/{images,workbenches}.
// See .design/image-workbench.md.
import type { components } from '@/client';
import { getApiBaseUrl } from '@/utils/protocol';
import { controlApi, getControlApiHeaders } from './openapi';

export type ArchivedImage = components['schemas']['ImageGenImage'];
export type Workbench = components['schemas']['ImageGenWorkbench'];
export type WorkbenchItem = components['schemas']['ImageGenWorkbenchItem'];

export const imageArchiveApi = {
    listImages: async (params: { limit?: number; before?: string } = {}): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imagegen/images', { headers, params: { query: params } })),

    importImage: async (dataUrl: string, name: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/imagegen/images', {
            headers,
            body: { data: dataUrl, name },
        })),

    deleteImage: async (id: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v1/imagegen/images/{id}', {
            headers,
            params: { path: { id } },
        })),

    // The pixels, as a Blob. <img src> cannot carry the bearer token, so the
    // page fetches the bytes and shows them through an object or data URL.
    fetchImageBlob: async (id: string): Promise<Blob> => {
        const [base, headers] = await Promise.all([getApiBaseUrl(), getControlApiHeaders()]);
        const response = await fetch(`${base}/api/v1/imagegen/images/${encodeURIComponent(id)}/file`, { headers });
        if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
        return response.blob();
    },

    listWorkbenches: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imagegen/workbenches', { headers })),

    getWorkbench: async (id: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imagegen/workbenches/{id}', {
            headers,
            params: { path: { id } },
        })),

    createWorkbench: async (body: { name: string; description: string; root_image_id: string }): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/imagegen/workbenches', { headers, body })),

    updateWorkbench: async (id: string, body: { name?: string; description?: string }): Promise<any> =>
        controlApi((client, headers) => client.PUT('/api/v1/imagegen/workbenches/{id}', {
            headers,
            params: { path: { id } },
            body,
        })),

    deleteWorkbench: async (id: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v1/imagegen/workbenches/{id}', {
            headers,
            params: { path: { id } },
        })),

    addWorkbenchItems: async (id: string, items: { image_id: string; parent_id: string }[]): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/imagegen/workbenches/{id}/items', {
            headers,
            params: { path: { id } },
            body: { items },
        })),

    removeWorkbenchItem: async (id: string, imageId: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v1/imagegen/workbenches/{id}/items/{image_id}', {
            headers,
            params: { path: { id, image_id: imageId } },
        })),
};
