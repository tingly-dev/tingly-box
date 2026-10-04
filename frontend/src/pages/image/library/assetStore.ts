import { useSyncExternalStore } from 'react';
import type { ImageAsset, PromptSnippet } from './assetTypes';
import { mockAssets, mockSnippets } from './mockAssets';

// PROTOTYPE store: module memory for the tab's lifetime. A page-free module —
// the profile store and the playground both import it.
let assets: ImageAsset[] = mockAssets;
let snippets: PromptSnippet[] = mockSnippets;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((listener) => listener());
const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
};
const newId = (prefix: string) => `${prefix}-${Math.random().toString(36).slice(2, 10)}`;

export const useAssets = (): ImageAsset[] => useSyncExternalStore(subscribe, () => assets, () => assets);
export const useSnippets = (): PromptSnippet[] => useSyncExternalStore(subscribe, () => snippets, () => snippets);

// Keeping the same picture twice is a no-op, not a duplicate.
export const keepImage = (fields: Omit<ImageAsset, 'id' | 'createdAt'>): ImageAsset => {
    const existing = assets.find((asset) => asset.src === fields.src);
    if (existing) return existing;
    const asset: ImageAsset = { ...fields, id: newId('img'), createdAt: Date.now() };
    assets = [asset, ...assets];
    emit();
    return asset;
};

export const renameAsset = (id: string, name: string) => {
    assets = assets.map((asset) => (asset.id === id ? { ...asset, name } : asset));
    emit();
};

export const removeAsset = (id: string) => {
    assets = assets.filter((asset) => asset.id !== id);
    emit();
};

export const saveSnippet = (fields: { id?: string; name: string; text: string }) => {
    if (fields.id) {
        snippets = snippets.map((item) => (item.id === fields.id ? { ...item, name: fields.name, text: fields.text } : item));
    } else {
        snippets = [{ id: newId('snip'), name: fields.name, text: fields.text, createdAt: Date.now() }, ...snippets];
    }
    emit();
};

export const removeSnippet = (id: string) => {
    snippets = snippets.filter((item) => item.id !== id);
    emit();
};
