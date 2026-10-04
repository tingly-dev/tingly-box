import type { ImageAsset, PromptSnippet } from './assetTypes';
import { USE_MOCK, createWorkspaceCollection } from '../imageWorkspaceDb';

// The library, kept in the browser (imageWorkspaceDb.ts). A page-free module —
// the profile store and the playground both import it.
const assetStore = createWorkspaceCollection<ImageAsset>(
    'assets',
    USE_MOCK ? () => import('./mockAssets').then((module) => module.mockAssets) : null,
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
export const keepImage = (fields: Omit<ImageAsset, 'id' | 'createdAt'>): ImageAsset => {
    const existing = assetStore.get().find((asset) => asset.src === fields.src);
    if (existing) return existing;
    const asset: ImageAsset = { ...fields, id: newId('img'), createdAt: Date.now() };
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
