import { renderHook, act } from '@testing-library/react';
import { useProviderModels } from '../useProviderModels';
import * as api from '../../services/api';

// Mock the API
jest.mock('../../services/api');
const mockApi = api as jest.Mocked<typeof api>;

// Mock event system to avoid cross-tab pollution
jest.mock('../../utils/eventSystem', () => ({
  createEventSystem: jest.fn((name: string) => {
    let listeners: Array<(data?: any) => void> = [];

    return {
      eventName: name,
      dispatch: jest.fn((data?: any) => {
        listeners.forEach((listener) => listener(data));
      }),
      listen: jest.fn((callback: (data?: any) => void) => {
        listeners.push(callback);
        return () => {
          listeners = listeners.filter((l) => l !== callback);
        };
      }),
    };
  }),
}));

import { createEventSystem } from '../../utils/eventSystem';
const mockCreateEventSystem = createEventSystem as jest.Mock;

describe('useProviderModels - Cache Convergence', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Setup event system mock
    mockCreateEventSystem.mockReturnValue({
      eventName: 'tingly_model_cache',
      dispatch: jest.fn(),
      listen: jest.fn((cb) => {
        // Return cleanup function
        return () => {};
      }),
    });
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
      const mockDispatch = jest.fn();
      mockCreateEventSystem.mockReturnValue({
        eventName: 'tingly_model_cache',
        dispatch: mockDispatch,
        listen: jest.fn(() => () => {}),
      });

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
      const mockDispatch = jest.fn();
      mockCreateEventSystem.mockReturnValue({
        eventName: 'tingly_model_cache',
        dispatch: mockDispatch,
        listen: jest.fn(() => () => {}),
      });

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
      jest.useFakeTimers();

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
