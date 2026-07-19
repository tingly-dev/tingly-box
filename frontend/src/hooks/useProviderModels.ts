import { useEffect, useState, useCallback } from 'react';
import api from '../services/api';
import type { ProviderModelData, ProviderModelsDataByUuid } from '../types/provider';
import { useNewModels } from './useNewModels';
import { createEventSystem } from '../utils/eventSystem';
import { usePageVisibility } from './usePageVisibility';

// Event types for cross-tab model cache synchronization
type ModelCacheEvent =
  | { type: 'provider_models_update'; providerUuid: string; models: ProviderModelData | null }
  | { type: 'refresh_trigger'; providerUuid: string }
  | { type: 'cache_invalidated'; providerUuid: string; reason: 'ttl_expired' | 'manual_refresh' | 'provider_deleted' };

// Event system for provider models updates — crossTab:true propagates to other browser tabs
const modelCacheEvent = createEventSystem<ModelCacheEvent>('tingly_model_cache', true);

// Export event name for backward compatibility
export const MODEL_CACHE_EVENT = modelCacheEvent.eventName;
// Legacy event name (deprecated)
export const PROVIDER_MODELS_UPDATE_EVENT = 'tingly_provider_models_update';

// Custom hook to manage provider models
export const useProviderModels = () => {
    const [providerModels, setProviderModels] = useState<ProviderModelsDataByUuid>({});
    const [cacheMeta, setCacheMeta] = useState<{ [providerUuid: string]: { expiresAt: string; source: string } }>({});
    const [refreshingProviders, setRefreshingProviders] = useState<Set<string>>(new Set());
    const { detectAndStoreNewModels } = useNewModels();

    // Check if cached data is still valid based on server-provided expiresAt
    const isCacheValid = useCallback((providerUuid: string): boolean => {
        const meta = cacheMeta[providerUuid];
        if (!meta || !meta.expiresAt) return false;
        return new Date(meta.expiresAt) > new Date();
    }, [cacheMeta]);

    // Fetch models for a provider (uses server-side caching)
    const fetchModels = useCallback(async (providerUuid: string): Promise<ProviderModelData | null> => {
        // Return cached data if valid
        if (isCacheValid(providerUuid) && providerModels[providerUuid]) {
            return providerModels[providerUuid];
        }

        // Prevent duplicate requests
        if (refreshingProviders.has(providerUuid)) {
            return null;
        }

        setRefreshingProviders(prev => new Set(prev).add(providerUuid));

        try {
            const result = await api.getProviderModelsByUUID(providerUuid);

            if (result.success && result.data) {
                const data = result.data;

                // Update cache metadata from server response
                setCacheMeta(prev => ({
                    ...prev,
                    [providerUuid]: {
                        expiresAt: data.expiresAt || new Date(Date.now() + 60 * 60 * 1000).toISOString(),
                        source: data.source || 'unknown',
                    },
                }));

                setProviderModels(prev => ({
                    ...prev,
                    [providerUuid]: data
                }));

                modelCacheEvent.dispatch({
                    type: 'provider_models_update',
                    providerUuid,
                    models: data
                });

                return data;
            }
        } catch (error) {
            console.error(`Failed to fetch models for provider ${providerUuid}:`, error);
        } finally {
            setRefreshingProviders(prev => {
                const next = new Set(prev);
                next.delete(providerUuid);
                return next;
            });
        }

        return null;
    }, [providerModels, cacheMeta, refreshingProviders, isCacheValid]);

    // Refresh models for a provider (POST - force fetch from provider API)
    const refreshModels = useCallback(async (providerUuid: string): Promise<ProviderModelData | null> => {
        // Prevent duplicate requests
        if (refreshingProviders.has(providerUuid)) {
            return null;
        }

        setRefreshingProviders(prev => new Set(prev).add(providerUuid));

        // Broadcast refresh start to other tabs
        modelCacheEvent.dispatch({
            type: 'refresh_trigger',
            providerUuid,
        });

        try {
            // Store old models for diff detection
            const oldModels = providerModels[providerUuid]?.models || [];

            // Force refresh from provider API
            const result = await api.updateProviderModelsByUUID(providerUuid);

            if (result.success && result.data) {
                // Detect and store new models
                const newModelsList = result.data.models || [];
                detectAndStoreNewModels(providerUuid, oldModels, newModelsList);

                // Update cache metadata from server response
                setCacheMeta(prev => ({
                    ...prev,
                    [providerUuid]: {
                        expiresAt: result.data.expiresAt || new Date(Date.now() + 60 * 60 * 1000).toISOString(),
                        source: result.data.source || 'api',
                    },
                }));

                setProviderModels(prev => ({
                    ...prev,
                    [providerUuid]: result.data!
                }));
                modelCacheEvent.dispatch({
                    type: 'provider_models_update',
                    providerUuid,
                    models: result.data
                });
                return result.data;
            }
        } catch (error) {
            console.error(`Failed to refresh models for provider ${providerUuid}:`, error);
        } finally {
            setRefreshingProviders(prev => {
                const next = new Set(prev);
                next.delete(providerUuid);
                return next;
            });
        }

        return null;
    }, [refreshingProviders, providerModels, detectAndStoreNewModels]);

    // Update models for a provider (manual set, e.g., from websocket or external source)
    const setModels = useCallback((providerUuid: string, models: ProviderModelData) => {
        setProviderModels(prev => ({
            ...prev,
            [providerUuid]: models
        }));
        modelCacheEvent.dispatch({
            type: 'provider_models_update',
            providerUuid,
            models
        });
    }, []);

    // Remove models for a provider
    const removeModels = useCallback((providerUuid: string) => {
        setProviderModels(prev => {
            const next = { ...prev };
            delete next[providerUuid];
            return next;
        });
        setCacheMeta(prev => {
            const next = { ...prev };
            delete next[providerUuid];
            return next;
        });
        modelCacheEvent.dispatch({
            type: 'cache_invalidated',
            providerUuid,
            reason: 'provider_deleted'
        });
    }, []);

    // Refetch all providers that have cached data
    const refetchAll = useCallback(async () => {
        const promises = Object.keys(providerModels).map(uuid => refreshModels(uuid));
        await Promise.allSettled(promises);
    }, [providerModels, refreshModels]);

    // Check if a provider is currently refreshing
    const isRefreshing = useCallback((providerUuid: string): boolean => {
        return refreshingProviders.has(providerUuid);
    }, [refreshingProviders]);

    // Get models for a specific provider
    const getModels = useCallback((providerUuid: string): ProviderModelData | undefined => {
        return providerModels[providerUuid];
    }, [providerModels]);

    // Listen for cross-tab cache events
    useEffect(() => {
        const cleanup = modelCacheEvent.listen((event) => {
            if (!event) return;
            switch (event.type) {
                case 'provider_models_update': {
                    const updatedModels = event.models;
                    if (updatedModels) {
                        setProviderModels(prev => ({
                            ...prev,
                            [event.providerUuid]: updatedModels
                        }));
                    } else {
                        setProviderModels(prev => {
                            const next = { ...prev };
                            delete next[event.providerUuid];
                            return next;
                        });
                    }
                    break;
                }

                case 'refresh_trigger':
                    // Another tab is refreshing, show loading state
                    setRefreshingProviders(prev => new Set(prev).add(event.providerUuid));
                    break;

                case 'cache_invalidated':
                    // Clear local cache for this provider
                    setProviderModels(prev => {
                        const next = { ...prev };
                        delete next[event.providerUuid];
                        return next;
                    });
                    setCacheMeta(prev => {
                        const next = { ...prev };
                        delete next[event.providerUuid];
                        return next;
                    });
                    break;
            }
        });
        return cleanup;
    }, []);

    // Bust in-memory cache when tab regains focus after ≥30s so the next
    // fetchModels call re-fetches from the server instead of serving stale data.
    usePageVisibility(useCallback(() => {
        setProviderModels(prev => Object.keys(prev).length === 0 ? prev : {});
        setCacheMeta(prev => Object.keys(prev).length === 0 ? prev : {});
    }, []));

    return {
        providerModels,
        refreshingProviders: Array.from(refreshingProviders),
        fetchModels,
        refreshModels,
        setModels,
        removeModels,
        refetchAll,
        isRefreshing,
        getModels,
    };
};
