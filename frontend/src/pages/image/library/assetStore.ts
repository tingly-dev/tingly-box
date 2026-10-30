import type { ImageAsset, PromptSnippet } from './assetTypes';
import { fetchBlob } from '@tingly/vision';
import { USE_MOCK, createWorkspaceCollection } from '../imageWorkspaceDb';
import { base64DataUrlToBlob } from '../components/imageGenSession';

// The library, kept in the browser (imageWorkspaceDb.ts). A page-free module —
// the profile store and the playground both import it.
// An asset is its `blob`; `src` is how a page shows it, and a blob: URL does not
// outlive the page, so it is rebuilt on every load. Assets kept by older builds
// hold a base64 data URL: those are rewritten once as a blob (a new object, so
// the store saves it), the rest are fixed in place so a load rewrites nothing.
export const reviveAsset = (asset: ImageAsset): ImageAsset => {
    if (asset.blob) {
        asset.src = URL.createObjectURL(asset.blob);
        return asset;
    }
    const blob = base64DataUrlToBlob(asset.src);
    return blob ? { ...asset, blob, src: URL.createObjectURL(blob) } : asset;
};

const assetStore = createWorkspaceCollection<ImageAsset>(
    'assets',
    USE_MOCK ? () => import('./mockAssets').then((module) => module.mockAssets) : null,
    reviveAsset,
);
const snippetStore = createWorkspaceCollection<PromptSnippet>(
    'snippets',
    USE_MOCK ? () => import('./mockAssets').then((module) => module.mockSnippets) : null,
);
const newId = (prefix: string) => `${prefix}-${Math.random().toString(36).slice(2, 10)}`;

export const useAssets = assetStore.useItems;
export const useSnippets = snippetStore.useItems;
export const useLibraryReady = (): boolean => {
    // Both hooks every render — `a() && b()` would skip one and break hook order.
    const assetsReady = assetStore.useReady();
    const snippetsReady = snippetStore.useReady();
    return assetsReady && snippetsReady;
};

// Keeping the same picture twice is a no-op, not a duplicate.
const sameBytes = async (a: Blob, b: Blob): Promise<boolean> => {
    if (a.size !== b.size || a.type !== b.type) return false;
    const [left, right] = await Promise.all([a.arrayBuffer(), b.arrayBuffer()]);
    const x = new Uint8Array(left);
    const y = new Uint8Array(right);
    return x.every((byte, index) => byte === y[index]);
};

// What the picture is, not which URL it was reached by: a result restored after
// a reload has a new blob: URL but the same bytes.
const findKept = async (src: string, blob: Blob | null): Promise<ImageAsset | undefined> => {
    const bySrc = assetStore.get().find((asset) => asset.src === src);
    if (bySrc || !blob) return bySrc;
    for (const asset of assetStore.get()) {
        if (asset.blob && await sameBytes(asset.blob, blob)) return asset;
    }
    return undefined;
};

const keeping = new Map<string, Promise<ImageAsset>>();

// One keep per picture at a time: a double click is one asset, not two.
export const keepImage = (fields: Omit<ImageAsset, 'id' | 'createdAt' | 'blob'>): Promise<ImageAsset> => {
    const running = keeping.get(fields.src);
    if (running) return running;
    const started = keepImageNow(fields).finally(() => keeping.delete(fields.src));
    keeping.set(fields.src, started);
    return started;
};

const keepImageNow = async (fields: Omit<ImageAsset, 'id' | 'createdAt' | 'blob'>): Promise<ImageAsset> => {
    // A link that will not hand over its bytes is kept as the link, as before.
    const blob = await fetchBlob(fields.src).catch(() => null);
    const existing = await findKept(fields.src, blob);
    if (existing) return existing;
    const asset: ImageAsset = {
        ...fields,
        id: newId('img'),
        createdAt: Date.now(),
        ...(blob ? { blob, src: fields.src.startsWith('blob:') ? fields.src : URL.createObjectURL(blob) } : {}),
    };
    assetStore.set([asset, ...assetStore.get()]);
    return asset;
};

export const renameAsset = (id: string, name: string) => {
    assetStore.set(assetStore.get().map((asset) => (asset.id === id ? { ...asset, name } : asset)));
};

export const removeAsset = (id: string) => {
    assetStore.set(assetStore.get().filter((asset) => asset.id !== id));
};

export const saveSnippet = (fields: { id?: string; name: string; text: string }) => {
    const snippets = snippetStore.get();
    snippetStore.set(fields.id
        ? snippets.map((item) => (item.id === fields.id ? { ...item, name: fields.name, text: fields.text } : item))
        : [{ id: newId('snip'), name: fields.name, text: fields.text, createdAt: Date.now() }, ...snippets]);
};

export const removeSnippet = (id: string) => {
    snippetStore.set(snippetStore.get().filter((item) => item.id !== id));
};
