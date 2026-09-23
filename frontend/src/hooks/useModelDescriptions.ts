import { useEffect, useState, useCallback } from 'react';
import api from '../services/api';
import type { OpenAIModelsResponse, AnthropicModelsResponse, ModelDescriptionMap } from '../types/model';

// Module-level cache: the /v1/models description map is gateway-wide (the
// `providerUuid` param is accepted for signature compatibility but the
// endpoints are not provider-scoped), so repeat mounts of ModelsPanel reuse
// it instead of re-fetching on every open of the model-select dialog.
let cachedDescriptions: ModelDescriptionMap | null = null;

/**
 * Hook to fetch and cache model descriptions from /v1/models API
 */
export const useModelDescriptions = (providerUuid?: string) => {
  const [descriptions, setDescriptions] = useState<ModelDescriptionMap>(cachedDescriptions ?? {});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchDescriptions = useCallback(async (force = false) => {
    // Return cached data if available and not forcing refresh
    if (!force && cachedDescriptions !== null) {
      return cachedDescriptions;
    }

    setLoading(true);
    setError(null);

    try {
      // Try OpenAI format first
      const openaiResponse = await api.listOpenAIModels() as OpenAIModelsResponse;

      const map = toDescriptionMap(openaiResponse?.data)
        ?? toDescriptionMap((await api.listAnthropicModels() as AnthropicModelsResponse)?.data);

      if (map) {
        cachedDescriptions = map;
        setDescriptions(map);
        return map;
      }
    } catch (err) {
      console.error('Failed to fetch model descriptions:', err);
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }

    return cachedDescriptions ?? descriptions;
  }, [descriptions]);

  // Auto-fetch on mount if no cached data
  useEffect(() => {
    if (Object.keys(descriptions).length === 0) {
      fetchDescriptions();
    }
  }, []);

  const getDescription = useCallback((modelId: string): string | undefined => {
    return descriptions[modelId];
  }, [descriptions]);

  const refresh = useCallback(() => {
    return fetchDescriptions(true);
  }, [fetchDescriptions]);

  return {
    descriptions,
    loading,
    error,
    getDescription,
    refresh,
  };
};

// Normalize either gateway response shape into a {modelId: description} map;
// returns null when the payload is missing/empty or not the expected format
// (falls through to the other format, preserving the original
// OpenAI-then-Anthropic order; nothing is cached until one format yields
// entries, so a transient empty response is retried next mount).
function toDescriptionMap(data?: Array<{ id: string; description?: string }>): ModelDescriptionMap | null {
  if (!data || data.length === 0) {
    return null;
  }
  const map: ModelDescriptionMap = {};
  data.forEach((model) => {
    if (model.description) {
      map[model.id] = model.description;
    }
  });
  return map;
}
