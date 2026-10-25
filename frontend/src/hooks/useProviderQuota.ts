import { useCallback, useEffect, useRef, useState } from 'react';
import { useNotify } from '@/hooks/useNotify';
import { fetchUIAPI } from '@/services/api';
import type { ProviderQuota } from '@/types/quota';

/** How a reward spend ended; `error` is a failure before the server could say. */
export interface RedeemOutcome {
  status: 'spent' | 'rejected' | 'unconfirmed' | 'not_found' | 'busy' | 'unsupported' | 'error';
  message?: string;
}

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

/** 404 means the provider has no quota to show — not an error worth raising. */
function isMissingQuota(error: unknown): boolean {
  return (error as { status?: number })?.status === 404;
}

/**
 * Hook for fetching and managing provider quota data.
 *
 * Uses batch API to fetch quota for multiple providers efficiently.
 */
export function useProviderQuota(providers: Array<{ uuid: string; name?: string }>, options: UseProviderQuotaOptions = {}) {
  const { fetchOnMount = true } = options;

  const [quotaData, setQuotaData] = useState<ProviderQuotaData>({});
  const [refreshing, setRefreshing] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState<Map<string, string>>(new Map());
  const notify = useNotify();

  // Names are read through a ref so the fetch callbacks keep a stable identity:
  // batchFetchQuota is a dependency of the mount effect, and re-creating it on
  // every render would re-fetch on every render.
  const namesRef = useRef<Map<string, string>>(new Map());
  useEffect(() => {
    namesRef.current = new Map(providers.map(p => [p.uuid, p.name || p.uuid]));
  }, [providers]);
  const providerName = useCallback(
    (uuid: string) => namesRef.current.get(uuid) || uuid,
    [],
  );

  // Batch fetch quota for multiple providers
  const batchFetchQuota = useCallback(async (providerUuids: string[]): Promise<void> => {
    if (providerUuids.length === 0) {
      return;
    }

    setLoading(true);
    try {
      const response = await fetchUIAPI('/provider-quota/batch', {
        method: 'POST',
        body: JSON.stringify({ provider_uuids: providerUuids }),
      });

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
      // One notification for the batch, not one per provider: the whole call
      // failed, so per-provider toasts would repeat a single fact N times.
      notify.error(`Failed to load quota: ${errorMessage}`);
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
  }, [notify]);

  // Fetch quota for a single provider
  const fetchQuota = useCallback(async (providerUuid: string): Promise<ProviderQuota | null> => {
    try {
      const response = await fetchUIAPI(`/provider-quota/${providerUuid}`);

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
      if (isMissingQuota(error)) {
        return null;
      }
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
      // A refused upstream still answers 200: an unreadable provider comes
      // back as a usage record carrying last_error. The error is not toasted —
      // it's visible via Details on the quota row — so the refresh stays quiet.
      await fetchQuota(providerUuid);
    } catch (error) {
      if (isMissingQuota(error)) {
        return;
      }
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      console.error(`[useProviderQuota] Failed to refresh quota for ${providerUuid}:`, error);
      notify.error(`Failed to refresh ${providerName(providerUuid)} quota: ${errorMessage}`);
      setErrors(prev => new Map(prev).set(providerUuid, errorMessage));
    } finally {
      setRefreshing(prev => {
        const next = new Set(prev);
        next.delete(providerUuid);
        return next;
      });
    }
  }, [fetchQuota, notify, providerName]);

  // Spend one reward (a reset card). Irreversible and sent once by the server,
  // so this never retries; the usage it answers with replaces the old reading,
  // which is how the card disappears and the windows refill on screen.
  const redeemReward = useCallback(async (providerUuid: string, rewardId: string): Promise<RedeemOutcome> => {
    const apply = (answer: { usage?: ProviderQuota } | undefined) => {
      if (answer?.usage?.provider_uuid) {
        setQuotaData(prev => ({ ...prev, [providerUuid]: answer.usage as ProviderQuota }));
      }
    };
    try {
      const answer = await fetchUIAPI(`/provider-quota/${providerUuid}/rewards/${encodeURIComponent(rewardId)}/use`, { method: 'POST' });
      apply(answer);
      return { status: answer.status, message: answer.message };
    } catch (error) {
      const body = (error as { body?: { status?: RedeemOutcome['status']; message?: string; usage?: ProviderQuota } })?.body;
      if (body?.status) {
        apply(body);
        return { status: body.status, message: body.message };
      }
      return { status: 'error', message: error instanceof Error ? error.message : undefined };
    }
  }, []);

  // Fetch all quotas
  const fetchAllQuotas = useCallback(async () => {
    const uuids = providers.map(p => p.uuid);
    await batchFetchQuota(uuids);
  }, [providers, batchFetchQuota]);

  // Refresh every provider upstream in one call, then re-read them in one batch.
  // Every provider counts as refreshing meanwhile, so each row shows it.
  const refreshAllQuotas = useCallback(async () => {
    const uuids = providers.map(p => p.uuid);
    if (uuids.length === 0) return;
    setRefreshing(new Set(uuids));
    try {
      await fetchUIAPI('/provider-quota/refresh', { method: 'POST' });
      await batchFetchQuota(uuids);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      console.error('[useProviderQuota] Refresh all failed:', error);
      notify.error(`Failed to refresh quota: ${errorMessage}`);
    } finally {
      setRefreshing(new Set());
    }
  }, [providers, batchFetchQuota, notify]);

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
    redeemReward,
    fetchAllQuotas,
    refreshAllQuotas,
    batchFetchQuota,
  };
}
