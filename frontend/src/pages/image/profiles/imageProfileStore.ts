import type { ImageProfile } from './imageProfileTypes';
import { USE_MOCK, createWorkspaceCollection } from '../imageWorkspaceDb';

// Profiles, shared by the sidebar and the profile pages, kept in the browser
// (imageWorkspaceDb.ts). A page-free module on purpose — the sidebar imports
// it, and must not pull a page into the eager bundle (frontend/CLAUDE.md).
// A reference is its `file`; `previewUrl` is only how the page shows it, and a
// blob: URL does not outlive the page — so it is rebuilt from the file on every
// load. Profiles saved by older builds carry a base64 copy of the same image:
// those are rewritten once without it (a new object, so the store saves it)
// and the rest are fixed in place (the loaded object is what the store
// believes it holds; replacing it would rewrite every profile on every load).
export const reviveProfile = (profile: ImageProfile): ImageProfile => {
    let migrate = false;
    profile.refs.forEach((ref) => {
        if (ref.file && ref.previewUrl.startsWith('data:')) migrate = true;
    });
    const refs = profile.refs.map((ref) => {
        if (!ref.file) return ref;
        const previewUrl = URL.createObjectURL(ref.file);
        if (migrate) return { ...ref, previewUrl };
        ref.previewUrl = previewUrl;
        return ref;
    });
    return migrate ? { ...profile, refs } : profile;
};

const store = createWorkspaceCollection<ImageProfile>(
    'profiles',
    USE_MOCK ? () => import('./mockProfiles').then((module) => module.mockProfiles()) : null,
    reviveProfile,
);

export const useImageProfiles = store.useItems;
export const useImageProfilesReady = store.useReady;

export const getImageProfile = (id: string) => store.get().find((profile) => profile.id === id);

export const createImageProfile = (fields: Omit<ImageProfile, 'id' | 'updatedAt'>): ImageProfile => {
    const profile: ImageProfile = {
        ...fields,
        id: Math.random().toString(36).slice(2, 10),
        updatedAt: Date.now(),
    };
    store.set([...store.get(), profile]);
    return profile;
};

export const updateImageProfile = (id: string, patch: Partial<Omit<ImageProfile, 'id'>>) => {
    store.set(store.get().map((profile) => (profile.id === id ? { ...profile, ...patch, updatedAt: Date.now() } : profile)));
};

export const removeImageProfile = (id: string) => {
    store.set(store.get().filter((profile) => profile.id !== id));
};
