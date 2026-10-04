import type { ImageProfile } from './imageProfileTypes';
import { USE_MOCK, createWorkspaceCollection } from '../imageWorkspaceDb';

// Profiles, shared by the sidebar and the profile pages, kept in the browser
// (imageWorkspaceDb.ts). A page-free module on purpose — the sidebar imports
// it, and must not pull a page into the eager bundle (frontend/CLAUDE.md).
const store = createWorkspaceCollection<ImageProfile>(
    'profiles',
    USE_MOCK ? () => import('./mockProfiles').then((module) => module.mockProfiles()) : null,
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
