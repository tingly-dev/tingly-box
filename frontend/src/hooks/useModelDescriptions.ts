import { useEffect, useState, useCallback } from 'react';
import api from '../services/api';
import type { OpenAIModelsResponse, AnthropicModelsResponse, ModelDescriptionMap } from '../types/model';

/**
 * Hook to fetch and cache model descriptions from /v1/models API
 */
export const useModelDescriptions = (providerUuid?: string) => {
  const [descriptions, setDescriptions] = useState<ModelDescriptionMap>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchDescriptions = useCallback(async (force = false) => {
    // Return cached data if available and not forcing refresh
    if (!force && Object.keys(descriptions).length > 0) {
      return descriptions;
    }

    setLoading(true);
    setError(null);

    try {
      // Try OpenAI format first
      const openaiResponse = await api.listOpenAIModels() as OpenAIModelsResponse;

      if (openaiResponse?.data) {
        const map: ModelDescriptionMap = {};
        openaiResponse.data.forEach((model) => {
          if (model.description) {
            map[model.id] = model.description;
          }
        });
        setDescriptions(map);
        return map;
      }

      // Fallback to Anthropic format
      const anthropicResponse = await api.listAnthropicModels() as AnthropicModelsResponse;

      if (anthropicResponse?.data) {
        const map: ModelDescriptionMap = {};
        anthropicResponse.data.forEach((model) => {
          if (model.description) {
            map[model.id] = model.description;
          }
        });
        setDescriptions(map);
        return map;
      }
    } catch (err) {
      console.error('Failed to fetch model descriptions:', err);
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }

    return descriptions;
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
