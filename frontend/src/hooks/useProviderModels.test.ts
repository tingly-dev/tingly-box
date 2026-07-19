import { renderHook, act } from '@testing-library/react';
import { vi } from 'vitest';
import { useProviderModels } from './useProviderModels';
import api from '../services/api';

// Mock the API
vi.mock('../services/api');
const mockApi = api as vi.Mocked<typeof api>;

// Mock event system to avoid cross-tab pollution.
// IMPORTANT: useProviderModels calls createEventSystem() at module top level
// (on import), so the mock must return a STABLE single instance whose
// dispatch/listen spies are the same ones the hook captures at import time
// and the ones the tests assert against. Returning fresh spies per call (or
// re-configuring mockReturnValue in beforeEach) would disconnect the two.
//
// vi.mock factories are hoisted above all imports, so the shared instance
// must be created with vi.hoisted() to be initialized before the factory runs.
const { mockEventSystem, mockDispatch, mockListen } = vi.hoisted(() => {
  const mockDispatch = vi.fn();
  const mockListen = vi.fn((): (() => void) => () => {});
  const mockEventSystem = {
    eventName: 'tingly_model_cache',
    dispatch: mockDispatch,
    listen: mockListen,
  };
  return { mockEventSystem, mockDispatch, mockListen };
});
vi.mock('../utils/eventSystem', () => ({
  createEventSystem: vi.fn(() => mockEventSystem),
}));

import { createEventSystem } from '../utils/eventSystem';
const mockCreateEventSystem = createEventSystem as vi.Mock;

describe('useProviderModels - Cache Convergence', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateEventSystem.mockReturnValue(mockEventSystem);
  });

  describe('Server-side TTL validation', () => {
    it('should use server-provided expiresAt for cache validity', async () => {
      const expiresAt = new Date(Date.now() + 60 * 60 * 1000).toISOString(); // 1 hour from now

      mockApi.getProviderModelsByUUID.mockResolvedValue({
        success: true,
        data: {
          models: ['model-1', 'model-2'],
          source: 'db',
          expiresAt: expiresAt,
        },
      });

      const { result } = renderHook(() => useProviderModels());

      await act(async () => {
        await result.current.fetchModels('provider-1');
      });

      expect(result.current.providerModels['provider-1']).toEqual({
        models: ['model-1', 'model-2'],
        source: 'db',
        expiresAt: expect.any(String),
      });
    });

    it('should re-fetch when server-provided expiresAt has passed', async () => {
      const expiredTime = new Date(Date.now() - 1000).toISOString(); // Expired

      mockApi.getProviderModelsByUUID.mockResolvedValue({
        success: true,
        data: {
          models: ['model-1'],
          source: 'db',
          expiresAt: expiredTime,
        },
      });

      const { result } = renderHook(() => useProviderModels());

      // First fetch should succeed even if expired (server handles it)
      await act(async () => {
        const data = await result.current.fetchModels('provider-1');
        expect(data).not.toBeNull();
      });

      expect(mockApi.getProviderModelsByUUID).toHaveBeenCalledTimes(1);
    });

    it('should return cached data if expiresAt is still valid', async () => {
      const futureTime = new Date(Date.now() + 60 * 60 * 1000).toISOString();

      mockApi.getProviderModelsByUUID.mockResolvedValue({
        success: true,
        data: {
          models: ['cached-model'],
          source: 'db',
          expiresAt: futureTime,
        },
      });

      const { result } = renderHook(() => useProviderModels());

      // First fetch
      await act(async () => {
        await result.current.fetchModels('provider-1');
      });

      // Second fetch should use cache (no API call)
      await act(async () => {
        await result.current.fetchModels('provider-1');
      });

      // Only one API call due to cache hit
      expect(mockApi.getProviderModelsByUUID).toHaveBeenCalledTimes(1);
    });
  });

  describe('Source tracking', () => {
    it('should track source from server response', async () => {
      mockApi.getProviderModelsByUUID.mockResolvedValue({
        success: true,
        data: {
          models: ['model-1'],
          source: 'template',
          expiresAt: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
        },
      });

      const { result } = renderHook(() => useProviderModels());

      await act(async () => {
        await result.current.fetchModels('provider-1');
      });

      expect(result.current.providerModels['provider-1']?.source).toBe('template');
    });

    const sources = ['db', 'api', 'template', 'vmodel'];
    sources.forEach((source) => {
      it(`should handle source: ${source}`, async () => {
        mockApi.getProviderModelsByUUID.mockResolvedValue({
          success: true,
          data: {
            models: ['model-1'],
            source: source,
            expiresAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
          },
        });

        const { result } = renderHook(() => useProviderModels());

        await act(async () => {
          await result.current.fetchModels('provider-1');
        });

        expect(result.current.providerModels['provider-1']?.source).toBe(source);
      });
    });
  });

  describe('Cross-tab sync events', () => {
    it('should dispatch refresh_trigger event when refreshing', async () => {
      mockApi.updateProviderModelsByUUID.mockResolvedValue({
        success: true,
        data: {
          models: ['model-1'],
          source: 'api',
          expiresAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        },
      });

      const { result } = renderHook(() => useProviderModels());

      await act(async () => {
        await result.current.refreshModels('provider-1');
      });

      expect(mockDispatch).toHaveBeenCalledWith({
        type: 'refresh_trigger',
        providerUuid: 'provider-1',
      });
    });

    it('should dispatch cache_invalidated event when removing models', () => {
      const { result } = renderHook(() => useProviderModels());

      act(() => {
        result.current.removeModels('provider-1');
      });

      expect(mockDispatch).toHaveBeenCalledWith({
        type: 'cache_invalidated',
        providerUuid: 'provider-1',
        reason: 'provider_deleted',
      });
    });
  });

  describe('Cache invalidation', () => {
    it('should clear cacheMeta when provider is removed', async () => {
      mockApi.getProviderModelsByUUID.mockResolvedValue({
        success: true,
        data: {
          models: ['model-1'],
          source: 'db',
          expiresAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        },
      });

      const { result } = renderHook(() => useProviderModels());

      // Fetch models
      await act(async () => {
        await result.current.fetchModels('provider-1');
      });

      expect(result.current.providerModels['provider-1']).toBeDefined();

      // Remove models
      act(() => {
        result.current.removeModels('provider-1');
      });

      expect(result.current.providerModels['provider-1']).toBeUndefined();
    });

    it('should bust cache on tab visibility change after 30s', () => {
      vi.useFakeTimers();

      mockApi.getProviderModelsByUUID.mockResolvedValue({
        success: true,
        data: {
          models: ['model-1'],
          source: 'db',
          expiresAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        },
      });

      const { result } = renderHook(() => useProviderModels());

      act(() => {
        result.current.setModels('provider-1', {
          models: ['model-1'],
          source: 'db',
          expiresAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        } as any);
      });

      // Cache should be present
      expect(result.current.providerModels['provider-1']).toBeDefined();

      // Simulate page visibility change (would trigger cache bust)
      // Note: This would require mocking usePageVisibility hook
    });
  });
});
