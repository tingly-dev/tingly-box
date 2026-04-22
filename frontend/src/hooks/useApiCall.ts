/**
 * Shared hook for API calls with loading/error states.
 * Standardizes try/catch patterns across 25+ files.
 */
import { useState, useCallback, type Dispatch, type SetStateAction } from 'react';

export interface ApiCallResult<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
  execute: (apiFn: () => Promise<{ success?: boolean; data?: T; error?: string }>) => Promise<void>;
  setData: Dispatch<SetStateAction<T | null>>;
  setError: Dispatch<SetStateAction<string | null>>;
  clearError: () => void;
}

export function useApiCall<T = any>(): ApiCallResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const execute = useCallback(async (
    apiFn: () => Promise<{ success?: boolean; data?: T; error?: string }>
  ) => {
    setLoading(true);
    setError(null);

    try {
      const result = await apiFn();

      if (result?.success === false) {
        setError(result.error || 'Request failed');
        return;
      }

      if (result.data !== undefined) {
        setData(result.data);
      }
    } catch (err: any) {
      setError(err?.message || 'An unexpected error occurred');
    } finally {
      setLoading(false);
    }
  }, []);

  const clearError = useCallback(() => {
    setError(null);
  }, []);

  return {
    data,
    loading,
    error,
    execute,
    setData,
    setError,
    clearError,
  };
}
