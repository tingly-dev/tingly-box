import { useSyncExternalStore } from 'react';
import type { ImageProfile } from './imageProfileTypes';
import { mockProfiles } from './mockProfiles';

// PROTOTYPE store: module memory, shared by the sidebar and the profile pages
// for the lifetime of the tab. A page-free module on purpose — the sidebar
// imports it, and must not pull a page into the eager bundle
// (frontend/CLAUDE.md).
let profiles: ImageProfile[] = mockProfiles();
const listeners = new Set<() => void>();

const emit = () => listeners.forEach((listener) => listener());

const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
};

export const useImageProfiles = (): ImageProfile[] =>
    useSyncExternalStore(subscribe, () => profiles, () => profiles);

export const getImageProfile = (id: string) => profiles.find((profile) => profile.id === id);

export const createImageProfile = (fields: Omit<ImageProfile, 'id' | 'updatedAt'>): ImageProfile => {
    const profile: ImageProfile = {
        ...fields,
        id: Math.random().toString(36).slice(2, 10),
        updatedAt: Date.now(),
    };
    profiles = [...profiles, profile];
    emit();
    return profile;
};

export const updateImageProfile = (id: string, patch: Partial<Omit<ImageProfile, 'id'>>) => {
    profiles = profiles.map((profile) => (profile.id === id ? { ...profile, ...patch, updatedAt: Date.now() } : profile));
    emit();
};

export const removeImageProfile = (id: string) => {
    profiles = profiles.filter((profile) => profile.id !== id);
    emit();
};
