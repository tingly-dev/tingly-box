import { useEffect, useState, useCallback } from 'react';
import api from '../services/api';
import type { ProviderModelData, ProviderModelsDataByUuid } from '../types/provider';
import { useNewModels } from './useNewModels';
import { useLocalStorage } from './useLocalStorage';
import { createEventSystem } from '../utils/eventSystem';
import { usePageVisibility } from './usePageVisibility';

// Local storage key for refresh timestamps
const REFRESH_TIMESTAMPS_KEY = 'tingly_provider_refresh_timestamps';
const AUTO_REFRESH_INTERVAL = 24 * 60 * 60 * 1000; // 24 hours in milliseconds
const DEFAULT_TIMESTAMPS = {};

// Type for refresh timestamps
type RefreshTimestamps = { [providerUuid: string]: string };

// Event system for provider models updates — crossTab:true propagates to other browser tabs
const providerModelsEvent = createEventSystem<{ providerUuid: string; models: ProviderModelData | null }>(
    'tingly_provider_models_update',
    true
);

// Export event name for backward compatibility
export const PROVIDER_MODELS_UPDATE_EVENT = providerModelsEvent.eventName;

// Helper functions to manage refresh timestamps (now using useLocalStorage internally)
export const loadRefreshTimestamps = (): RefreshTimestamps => {
    try {
        const stored = localStorage.getItem(REFRESH_TIMESTAMPS_KEY);
        return stored ? JSON.parse(stored) : {};
    } catch (error) {
        console.error('Failed to load refresh timestamps:', error);
        return {};
    }
};

export const saveRefreshTimestamp = (providerUuid: string, timestamp: string): boolean => {
    try {
        const timestamps = loadRefreshTimestamps();
        timestamps[providerUuid] = timestamp;
        localStorage.setItem(REFRESH_TIMESTAMPS_KEY, JSON.stringify(timestamps));
        return true;
    } catch (error) {
        console.error('Failed to save refresh timestamp:', error);
        return false;
    }
};

export const shouldAutoRefresh = (providerUuid: string): boolean => {
    const timestamps = loadRefreshTimestamps();
    const lastRefresh = timestamps[providerUuid];

    if (!lastRefresh) {
        return true; // Never refreshed, should auto-refresh
    }

    const now = Date.now();
    const lastRefreshTime = new Date(lastRefresh).getTime();
    return now - lastRefreshTime >= AUTO_REFRESH_INTERVAL;
};

// Helper to dispatch provider models update event (backward compatibility)
export const dispatchProviderModelsUpdate = (providerUuid: string, models: ProviderModelData | null) => {
    providerModelsEvent.dispatch({ providerUuid, models });
};

// Helper to listen for provider models updates (backward compatibility)
export const listenForProviderModelsUpdates = (
    callback: (providerUuid: string, models: ProviderModelData | null) => void
) => {
    return providerModelsEvent.listen(({ providerUuid, models }) => {
        callback(providerUuid, models);
    });
};

// Custom hook to manage provider models
export const useProviderModels = () => {
    const [providerModels, setProviderModels] = useState<ProviderModelsDataByUuid>({});
    const [refreshingProviders, setRefreshingProviders] = useState<Set<string>>(new Set());
    const { detectAndStoreNewModels } = useNewModels();

    // Fetch models for a provider (GET - cached data, auto-refresh if empty or 24h passed)
    const fetchModels = useCallback(async (providerUuid: string): Promise<ProviderModelData | null> => {
        // Return cached data if available
        if (providerModels[providerUuid]) {
            return providerModels[providerUuid];
        }

        // Prevent duplicate requests
        if (refreshingProviders.has(providerUuid)) {
            return null;
        }

        setRefreshingProviders(prev => new Set(prev).add(providerUuid));

        try {
            // Check if we should auto-refresh (24h passed or never refreshed)
            const needAutoRefresh = shouldAutoRefresh(providerUuid);

            // If auto-refresh is needed, fetch from provider API directly
            if (needAutoRefresh) {
                const oldModels = providerModels[providerUuid]?.models || [];
                const refreshResult = await api.updateProviderModelsByUUID(providerUuid);

                if (refreshResult.success && refreshResult.data) {
                    // Detect and store new models
                    const newModelsList = refreshResult.data.models || [];
                    detectAndStoreNewModels(providerUuid, oldModels, newModelsList);

                    // Update refresh timestamp
                    saveRefreshTimestamp(providerUuid, new Date().toISOString());

                    setProviderModels(prev => ({
                        ...prev,
                        [providerUuid]: refreshResult.data!
                    }));
                    providerModelsEvent.dispatch({ providerUuid, models: refreshResult.data });
                    return refreshResult.data;
                }
            }

            // Try GET for cached data
            const result = await api.getProviderModelsByUUID(providerUuid);

            if (result.success && result.data) {
                // If GET returns empty list, auto-refresh from provider API
                if (!result.data.models || result.data.models.length === 0) {
                    const oldModels = providerModels[providerUuid]?.models || [];
                    const refreshResult = await api.updateProviderModelsByUUID(providerUuid);
                    if (refreshResult.success && refreshResult.data) {
                        // Detect and store new models
                        const newModelsList = refreshResult.data.models || [];
                        detectAndStoreNewModels(providerUuid, oldModels, newModelsList);

                        // Update refresh timestamp
                        saveRefreshTimestamp(providerUuid, new Date().toISOString());

                        setProviderModels(prev => ({
                            ...prev,
                            [providerUuid]: refreshResult.data!
                        }));
                        providerModelsEvent.dispatch({ providerUuid, models: refreshResult.data });
                        return refreshResult.data;
                    }
                } else {
                    setProviderModels(prev => ({
                        ...prev,
                        [providerUuid]: result.data!
                    }));
                    providerModelsEvent.dispatch({ providerUuid, models: result.data });
                    return result.data;
                }
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
    }, [providerModels, refreshingProviders, detectAndStoreNewModels]);

    // Refresh models for a provider (POST - force fetch from provider API)
    const refreshModels = useCallback(async (providerUuid: string): Promise<ProviderModelData | null> => {
        // Prevent duplicate requests
        if (refreshingProviders.has(providerUuid)) {
            return null;
        }

        setRefreshingProviders(prev => new Set(prev).add(providerUuid));

        try {
            // Store old models for diff detection
            const oldModels = providerModels[providerUuid]?.models || [];

            // Force refresh from provider API
            const result = await api.updateProviderModelsByUUID(providerUuid);

            if (result.success && result.data) {
                // Detect and store new models
                const newModelsList = result.data.models || [];
                detectAndStoreNewModels(providerUuid, oldModels, newModelsList);

                // Update refresh timestamp
                saveRefreshTimestamp(providerUuid, new Date().toISOString());

                setProviderModels(prev => ({
                    ...prev,
                    [providerUuid]: result.data!
                }));
                providerModelsEvent.dispatch({ providerUuid, models: result.data });
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
        providerModelsEvent.dispatch({ providerUuid, models });
    }, []);

    // Remove models for a provider
    const removeModels = useCallback((providerUuid: string) => {
        setProviderModels(prev => {
            const next = { ...prev };
            delete next[providerUuid];
            return next;
        });
        providerModelsEvent.dispatch({ providerUuid, models: null });
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

    // Listen for updates from other components (and other tabs via BroadcastChannel)
    useEffect(() => {
        const cleanup = providerModelsEvent.listen(({ providerUuid, models }) => {
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
        });
        return cleanup;
    }, []);

    // Bust in-memory cache when tab regains focus after ≥30s so the next
    // fetchModels call re-fetches from the server instead of serving stale data.
    usePageVisibility(useCallback(() => {
        setProviderModels(prev => Object.keys(prev).length === 0 ? prev : {});
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
