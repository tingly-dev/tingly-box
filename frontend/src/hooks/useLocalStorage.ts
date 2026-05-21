/**
 * Generic localStorage hook for managing keyed data.
 * Eliminates duplication across useCustomModels, useNewModels, useRecentModels, useProviderModels.
 */
import { useEffect, useState, useCallback } from 'react';

export interface LocalStorageOptions<T> {
  serializer?: (value: T) => string;
  deserializer?: (value: string) => T;
}

export interface LocalStorageReturn<T> {
  data: T;
  version: number;
  loadData: () => T;
  saveData: (key: string, value: any) => boolean;
  removeKey: (key: string) => boolean;
  setData: (data: T) => void;
  refetch: () => void;
}

const defaultSerializer = JSON.stringify;
const defaultDeserializer = JSON.parse;

/**
 * Generic hook for managing localStorage with any data structure.
 *
 * @param storageKey - The localStorage key to use
 * @param defaultValue - Default value if storage is empty
 * @param options - Optional serializer/deserializer
 */
export function useLocalStorage<T extends Record<string, any>>(
  storageKey: string,
  defaultValue: T,
  options?: LocalStorageOptions<T>
): LocalStorageReturn<T> {
  const serializer = options?.serializer ?? defaultSerializer;
  const deserializer = options?.deserializer ?? defaultDeserializer;

  const [data, setData] = useState<T>(defaultValue);
  const [version, setVersion] = useState(0);

  // Load data from localStorage
  const loadData = useCallback((): T => {
    try {
      const stored = localStorage.getItem(storageKey);
      return stored ? deserializer(stored) : defaultValue;
    } catch (error) {
      console.error(`Failed to load ${storageKey} from storage:`, error);
      return defaultValue;
    }
  }, [storageKey, deserializer, defaultValue]);

  // Save a key-value pair to localStorage
  const saveData = useCallback((key: string, value: any): boolean => {
    try {
      const currentData = loadData();
      currentData[key] = value;
      localStorage.setItem(storageKey, serializer(currentData));
      return true;
    } catch (error) {
      console.error(`Failed to save ${key} to ${storageKey}:`, error);
      return false;
    }
  }, [storageKey, serializer, loadData]);

  // Remove a key from localStorage
  const removeKey = useCallback((key: string): boolean => {
    try {
      const currentData = loadData();
      delete currentData[key];
      localStorage.setItem(storageKey, serializer(currentData));
      return true;
    } catch (error) {
      console.error(`Failed to remove ${key} from ${storageKey}:`, error);
      return false;
    }
  }, [storageKey, serializer, loadData]);

  // Refetch data from storage
  const refetch = useCallback(() => {
    const loaded = loadData();
    setData(loaded);
    setVersion(prev => prev + 1);
  }, [loadData]);

  // Load data on mount
  useEffect(() => {
    refetch();
  }, [refetch]);

  // Sync with other tabs: when a different tab writes to the same key, reload
  useEffect(() => {
    const handleStorage = (e: StorageEvent) => {
      if (e.key === storageKey) {
        refetch();
      }
    };
    window.addEventListener('storage', handleStorage);
    return () => window.removeEventListener('storage', handleStorage);
  }, [storageKey, refetch]);

  return {
    data,
    version,
    loadData,
    saveData,
    removeKey,
    setData,
    refetch,
  };
}
