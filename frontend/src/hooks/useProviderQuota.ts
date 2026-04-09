import { useCallback, useEffect, useState } from 'react';
import type { ProviderQuota } from '@/types/quota';

interface ProviderQuotaData {
  [providerUuid: string]: ProviderQuota;
}

interface UseProviderQuotaOptions {
  /**
   * Whether to fetch quota on mount
   * @default true
   */
  fetchOnMount?: boolean;
}

/**
 * Helper function to fetch from API
 */
async function fetchUIAPI(url: string, options: RequestInit = {}): Promise<any> {
  const basePath = window.location.origin;
  const fullUrl = `${basePath}/api/v1${url}`;

  const token = localStorage.getItem('user_auth_token');

  const response = await fetch(fullUrl, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token && { Authorization: `Bearer ${token}` }),
      ...options.headers,
    },
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

/**
 * Hook for fetching and managing provider quota data.
 *
 * Uses batch API to fetch quota for multiple providers efficiently.
 */
export function useProviderQuota(providers: Array<{ uuid: string }>, options: UseProviderQuotaOptions = {}) {
  const { fetchOnMount = true } = options;

  const [quotaData, setQuotaData] = useState<ProviderQuotaData>({});
  const [refreshing, setRefreshing] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState<Map<string, string>>(new Map());

  // Batch fetch quota for multiple providers
  const batchFetchQuota = useCallback(async (providerUuids: string[]): Promise<void> => {
    if (providerUuids.length === 0) {
      return;
    }

    setLoading(true);
    try {
      console.log('[useProviderQuota] Batch fetching quota for providers:', providerUuids);
      const response = await fetchUIAPI('/provider-quota/batch', {
        method: 'POST',
        body: JSON.stringify({ provider_uuids: providerUuids }),
      });

      console.log('[useProviderQuota] Batch response:', response);

      if (response.data) {
        setQuotaData(prev => ({ ...prev, ...response.data }));
        // Clear any previous errors for these providers
        setErrors(prev => {
          const next = new Map(prev);
          for (const uuid of providerUuids) {
            next.delete(uuid);
          }
          return next;
        });
      }
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      console.error('[useProviderQuota] Batch fetch failed:', error);
      // Set error for all providers in the batch
      setErrors(prev => {
        const next = new Map(prev);
        for (const uuid of providerUuids) {
          next.set(uuid, errorMessage);
        }
        return next;
      });
    } finally {
      setLoading(false);
    }
  }, []);

  // Fetch quota for a single provider
  const fetchQuota = useCallback(async (providerUuid: string): Promise<ProviderQuota | null> => {
    try {
      console.log(`[useProviderQuota] Fetching quota for ${providerUuid}`);
      const response = await fetchUIAPI(`/provider-quota/${providerUuid}`);

      console.log(`[useProviderQuota] Response for ${providerUuid}:`, response);

      if (response && response.provider_uuid) {
        setQuotaData(prev => ({ ...prev, [providerUuid]: response }));
        // Clear any previous error for this provider
        setErrors(prev => {
          const next = new Map(prev);
          next.delete(providerUuid);
          return next;
        });
        return response;
      }

      return null;
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      console.error(`[useProviderQuota] Failed to fetch quota for ${providerUuid}:`, error);
      setErrors(prev => new Map(prev).set(providerUuid, errorMessage));
      return null;
    }
  }, []);

  // Refresh quota for a single provider
  const refreshQuota = useCallback(async (providerUuid: string): Promise<void> => {
    setRefreshing(prev => new Set(prev).add(providerUuid));
    try {
      await fetchUIAPI(`/provider-quota/${providerUuid}/refresh`, {
        method: 'POST',
      });
      await fetchQuota(providerUuid);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      console.error(`[useProviderQuota] Failed to refresh quota for ${providerUuid}:`, error);
      setErrors(prev => new Map(prev).set(providerUuid, errorMessage));
    } finally {
      setRefreshing(prev => {
        const next = new Set(prev);
        next.delete(providerUuid);
        return next;
      });
    }
  }, [fetchQuota]);

  // Fetch all quotas
  const fetchAllQuotas = useCallback(async () => {
    const uuids = providers.map(p => p.uuid);
    await batchFetchQuota(uuids);
  }, [providers, batchFetchQuota]);

  // Lazy load: fetch quotas when hook is initialized with providers
  useEffect(() => {
    if (!fetchOnMount || providers.length === 0) return;

    const providerUuids = providers.map(p => p.uuid);
    batchFetchQuota(providerUuids);
  }, [providers.length, fetchOnMount, batchFetchQuota]);

  return {
    quotaData,
    refreshing,
    loading,
    errors,
    fetchQuota,
    refreshQuota,
    fetchAllQuotas,
    batchFetchQuota,
  };
}
