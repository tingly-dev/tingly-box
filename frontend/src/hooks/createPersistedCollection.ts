/**
 * Factory for localStorage-backed model-collection hooks.
 * Encapsulates the shared skeleton of useCustomModels / useNewModels /
 * useRecentModels: a useLocalStorage store plus an event system that keeps
 * mounted instances in sync — dispatching from one instance (e.g. the dialog)
 * makes other mounted instances (e.g. ModelsPanel) refetch.
 *
 * Call at module scope (once per hook), like the event systems it wraps; the
 * returned hook owns the listen→refetch wiring and exposes the storage
 * primitives plus `notify` for the hook's own semantics to build on.
 */
import { useEffect } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { useLocalStorage } from './useLocalStorage';
import { createEventSystem } from '../utils/eventSystem';

export interface PersistedCollection<T extends Record<string, any>, P> {
  data: T;
  loadData: () => T;
  saveData: (key: string, value: any) => boolean;
  removeKey: (key: string) => boolean;
  setData: Dispatch<SetStateAction<T>>;
  refetch: () => void;
  /** Broadcast a change to other mounted instances of this collection. */
  notify: (payload: P) => void;
}

/**
 * @param storageKey - localStorage key backing the collection
 * @param eventName - custom event name used for cross-instance sync
 * @param defaultValue - default value if storage is empty
 * @param P - event payload type
 */
export function createPersistedCollection<T extends Record<string, any>, P = unknown>(
  storageKey: string,
  eventName: string,
  defaultValue: T
) {
  const event = createEventSystem<P>(eventName);

  return function usePersistedCollection(): PersistedCollection<T, P> {
    const { data, loadData, saveData, removeKey, setData, refetch } =
      useLocalStorage<T>(storageKey, defaultValue);

    // Reload when another mounted instance notifies about a change
    useEffect(() => {
      const cleanup = event.listen(() => {
        refetch();
      });
      return cleanup;
    }, [refetch]);

    return { data, loadData, saveData, removeKey, setData, refetch, notify: event.dispatch };
  };
}
