import { useEffect, useState, useCallback } from 'react';
import api from '../services/api';
import type { ProviderModelData, ProviderModelsDataByUuid } from '../types/provider';

// Export event name for provider models updates
export const PROVIDER_MODELS_UPDATE_EVENT = 'tingly_provider_models_update';

// Helper to dispatch provider models update event
export const dispatchProviderModelsUpdate = (providerUuid: string, models: ProviderModelData | null) => {
    window.dispatchEvent(new CustomEvent(PROVIDER_MODELS_UPDATE_EVENT, {
        detail: { providerUuid, models }
    }));
};

// Helper to listen for provider models updates
export const listenForProviderModelsUpdates = (callback: (providerUuid: string, models: ProviderModelData | null) => void) => {
    const handler = ((event: CustomEvent) => {
        callback(event.detail.providerUuid, event.detail.models);
    }) as EventListener;

    window.addEventListener(PROVIDER_MODELS_UPDATE_EVENT, handler);

    return () => {
        window.removeEventListener(PROVIDER_MODELS_UPDATE_EVENT, handler);
    };
};

// Custom hook to manage provider models
export const useProviderModels = () => {
    const [providerModels, setProviderModels] = useState<ProviderModelsDataByUuid>({});
    const [refreshingProviders, setRefreshingProviders] = useState<Set<string>>(new Set());
    const [version, setVersion] = useState(0);

    // Fetch models for a provider (only if not already cached)
    const fetchModels = useCallback(async (providerUuid: string): Promise<ProviderModelData | null> => {
        // Return cached data if available
        if (providerModels[providerUuid]) {
            return providerModels[providerUuid];
        }

        return refreshModels(providerUuid);
    }, [providerModels]);

    // Refresh models for a provider (force fetch from API)
    const refreshModels = useCallback(async (providerUuid: string): Promise<ProviderModelData | null> => {
        // Prevent duplicate requests
        if (refreshingProviders.has(providerUuid)) {
            return null;
        }

        setRefreshingProviders(prev => new Set(prev).add(providerUuid));

        try {
            const result = await api.getProviderModelsByUUID(providerUuid);

            if (result.success && result.data) {
                setProviderModels(prev => ({
                    ...prev,
                    [providerUuid]: result.data!
                }));
                setVersion(prev => prev + 1);
                dispatchProviderModelsUpdate(providerUuid, result.data);
                return result.data;
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
    }, [refreshingProviders]);

    // Update models for a provider (manual set, e.g., from websocket or external source)
    const setModels = useCallback((providerUuid: string, models: ProviderModelData) => {
        setProviderModels(prev => ({
            ...prev,
            [providerUuid]: models
        }));
        setVersion(prev => prev + 1);
        dispatchProviderModelsUpdate(providerUuid, models);
    }, []);

    // Remove models for a provider
    const removeModels = useCallback((providerUuid: string) => {
        setProviderModels(prev => {
            const next = { ...prev };
            delete next[providerUuid];
            return next;
        });
        setVersion(prev => prev + 1);
        dispatchProviderModelsUpdate(providerUuid, null);
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

    // Listen for updates from other components
    useEffect(() => {
        const cleanup = listenForProviderModelsUpdates((providerUuid, models) => {
            if (models) {
                setProviderModels(prev => ({
                    ...prev,
                    [providerUuid]: models
                }));
            } else {
                setProviderModels(prev => {
                    const next = { ...prev };
                    delete next[providerUuid];
                    return next;
                });
            }
            setVersion(prev => prev + 1);
        });
        return cleanup;
    }, []);

    return {
        providerModels,
        refreshingProviders: Array.from(refreshingProviders),
        version,
        fetchModels,
        refreshModels,
        setModels,
        removeModels,
        refetchAll,
        isRefreshing,
        getModels,
    };
};
