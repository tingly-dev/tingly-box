import { useSyncExternalStore } from 'react';
import type { ImageEntity } from './entityTypes';
import { MOCK_ENTITIES } from './mockEntities';

// PROTOTYPE store: module memory, shared by the library page and the
// playground for the lifetime of the tab. Replaced by the API once the
// backend exists — the hook's shape is what the pages depend on.
let entities: ImageEntity[] = MOCK_ENTITIES;
const listeners = new Set<() => void>();

const emit = () => listeners.forEach((listener) => listener());

const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
};

export const useEntities = (): ImageEntity[] =>
    useSyncExternalStore(subscribe, () => entities, () => entities);

export const saveEntity = (entity: ImageEntity) => {
    const exists = entities.some((item) => item.id === entity.id);
    entities = exists
        ? entities.map((item) => (item.id === entity.id ? entity : item))
        : [...entities, entity];
    emit();
};

export const removeEntity = (id: string) => {
    entities = entities.filter((item) => item.id !== id);
    emit();
};

export const newEntityId = () => `ent-${Math.random().toString(36).slice(2, 10)}`;
