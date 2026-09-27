// Where the assets are kept: the image assets API (internal/imageasset). The
// functions below are the whole contract the UI relies on; they map the
// API's records onto the assets model and tell every open view when
// something changed.
//
// Failures come back as `null` (or `false`), never as exceptions: every
// caller already has a "could not save" message for that.

import {
    imageAssetsApi,
    type AddedReferenceImageDto,
    type PromptPieceDto,
    type ReferenceImageDto,
} from '@/services/imageAssetsApi';
import { fileToDataUrl } from '../components/imageFiles';
import type { ImageInput, AssetImage, PieceInput, PieceKind, PromptPiece } from './model';

// Every open view (the Assets page, the playground's menu and picker) hears
// about a write made by any other, so a save shows up everywhere at once.
const listeners = new Set<() => void>();
const changed = () => listeners.forEach((listener) => listener());
export const subscribeAssets = (listener: () => void): (() => void) => {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
};

const ok = (result: any): boolean => Boolean(result?.success);

// --- Pieces ------------------------------------------------------------------

const toPiece = (dto: PromptPieceDto): PromptPiece => ({
    id: dto.id,
    kind: dto.kind as PieceKind,
    title: dto.title,
    text: dto.text,
    tags: dto.tags ?? [],
    ...(dto.source_id ? { sourceId: dto.source_id } : {}),
    createdAt: dto.created_at,
    updatedAt: dto.updated_at,
});

/** Most recently edited first. Empty when the list cannot be read. */
export const listPieces = async (): Promise<PromptPiece[]> => {
    const result = await imageAssetsApi.listPieces();
    return ok(result) ? (result.pieces ?? []).map(toPiece) : [];
};

/**
 * Creates pieces, or updates the ones whose `id` is set, in one transaction:
 * all of them land or none does.
 */
export const savePieces = async (inputs: PieceInput[]): Promise<PromptPiece[] | null> => {
    const result = await imageAssetsApi.savePieces(inputs.map((input) => ({
        ...(input.id ? { id: input.id } : {}),
        kind: input.kind,
        title: input.title ?? '',
        text: input.text,
        tags: input.tags ?? [],
        ...(input.sourceId ? { source_id: input.sourceId } : {}),
    })));
    if (!ok(result)) return null;
    changed();
    return (result.pieces ?? []).map(toPiece);
};

export const deletePiece = async (id: string): Promise<boolean> => {
    const deleted = ok(await imageAssetsApi.deletePiece(id));
    if (deleted) changed();
    return deleted;
};

// --- Images ------------------------------------------------------------------

// Image bytes by id. A reference's bytes never change under its id, so each
// is fetched once per page load. The promise is cached, so concurrent readers
// share one request; a failed fetch is dropped so the next read retries.
const contents = new Map<string, Promise<string | null>>();
const readContent = (id: string): Promise<string | null> => {
    let pending = contents.get(id);
    if (!pending) {
        pending = imageAssetsApi.referenceContent(id)
            .then((blob) => (blob instanceof Blob ? fileToDataUrl(blob) : null))
            .catch(() => null)
            .then((src) => {
                if (!src) contents.delete(id);
                return src;
            });
        contents.set(id, pending);
    }
    return pending;
};

const toImage = (dto: ReferenceImageDto, src: string): AssetImage => ({
    id: dto.id,
    name: dto.name,
    src,
    ...(dto.width && dto.height ? { width: dto.width, height: dto.height } : {}),
    bytes: dto.bytes,
    createdAt: dto.created_at,
});

/** Newest first, with their bytes. An image whose bytes cannot be read is left out. */
export const listImages = async (): Promise<AssetImage[]> => {
    const result = await imageAssetsApi.listReferences();
    if (!ok(result)) return [];
    const dtos: ReferenceImageDto[] = result.references ?? [];
    const srcs = await Promise.all(dtos.map((dto) => readContent(dto.id)));
    return dtos.flatMap((dto, index) => (srcs[index] ? [toImage(dto, srcs[index] as string)] : []));
};

export interface AddedImage {
    image: AssetImage;
    // The same image was already kept; nothing new was stored.
    existing: boolean;
}

// The server takes this many images per request.
const IMAGES_PER_REQUEST = 10;

/**
 * Keeps images, in order. An image already kept comes back marked
 * `existing`. Sent in batches the server accepts; `null` if a batch failed
 * (the batches before it are kept).
 */
export const addImages = async (inputs: ImageInput[]): Promise<AddedImage[] | null> => {
    const added: AddedImage[] = [];
    let failed = false;
    for (let start = 0; start < inputs.length && !failed; start += IMAGES_PER_REQUEST) {
        const batch = inputs.slice(start, start + IMAGES_PER_REQUEST);
        const result = await imageAssetsApi.addReferences(batch.map((input) => ({ name: input.name, data_url: input.src })));
        if (!ok(result)) {
            failed = true;
            break;
        }
        (result.references ?? []).forEach((dto: AddedReferenceImageDto, index: number) => {
            // The bytes just sent are the bytes stored: no need to fetch them back.
            if (!contents.has(dto.id)) contents.set(dto.id, Promise.resolve(batch[index].src));
            added.push({ image: toImage(dto, batch[index].src), existing: dto.existing });
        });
    }
    if (added.some((item) => !item.existing)) changed();
    return failed ? null : added;
};

export const renameImage = async (image: AssetImage, name: string): Promise<boolean> => {
    const renamed = ok(await imageAssetsApi.renameReference(image.id, name.trim() || image.name));
    if (renamed) changed();
    return renamed;
};

export const deleteImage = async (id: string): Promise<boolean> => {
    const deleted = ok(await imageAssetsApi.deleteReference(id));
    if (deleted) {
        contents.delete(id);
        changed();
    }
    return deleted;
};
