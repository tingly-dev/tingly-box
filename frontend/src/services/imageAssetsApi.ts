// Image assets API (internal/imageasset): kept prompt pieces and reference
// images. Thin calls only; pages/image/assets/store.ts maps them onto the
// library's model.
import type { components } from '@/client';
import { controlApi } from './openapi';

export type PromptPieceDto = components['schemas']['PromptPiece'];
export type PromptPieceInputDto = components['schemas']['PromptPieceInput'];
export type ReferenceImageDto = components['schemas']['ReferenceImage'];
export type AddedReferenceImageDto = components['schemas']['AddedReferenceImage'];

export const imageAssetsApi = {
    listPieces: (): Promise<any> => controlApi((client, headers) => client.GET('/api/v1/image-assets/pieces', { headers })),
    savePieces: (pieces: PromptPieceInputDto[]): Promise<any> => controlApi((client, headers) => client.POST('/api/v1/image-assets/pieces', {
        headers,
        body: { pieces },
    })),
    deletePiece: (id: string): Promise<any> => controlApi((client, headers) => client.DELETE('/api/v1/image-assets/pieces/{id}', {
        headers,
        params: { path: { id } },
    })),
    listReferences: (): Promise<any> => controlApi((client, headers) => client.GET('/api/v1/image-assets/references', { headers })),
    addReferences: (references: Array<{ name: string; data_url: string }>): Promise<any> => controlApi((client, headers) => client.POST('/api/v1/image-assets/references', {
        headers,
        body: { references },
    })),
    renameReference: (id: string, name: string): Promise<any> => controlApi((client, headers) => client.PUT('/api/v1/image-assets/references/{id}', {
        headers,
        params: { path: { id } },
        body: { name },
    })),
    deleteReference: (id: string): Promise<any> => controlApi((client, headers) => client.DELETE('/api/v1/image-assets/references/{id}', {
        headers,
        params: { path: { id } },
    })),
    // The image's bytes, as a Blob (or `{success: false}` on failure).
    referenceContent: (id: string): Promise<Blob | { success: false; error: string }> => controlApi((client, headers) => client.GET('/api/v1/image-assets/references/{id}/content', {
        headers,
        params: { path: { id } },
        parseAs: 'blob',
    })),
};
