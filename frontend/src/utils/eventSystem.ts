/**
 * Generic event system for cross-component communication.
 * Eliminates duplication across useCustomModels, useNewModels, useRecentModels, useProviderModels.
 */

export interface EventSystem<T = any> {
  eventName: string;
  dispatch: (data?: T) => void;
  listen: (callback: (data?: T) => void) => () => void;
}

/**
 * Create an event system for cross-component communication.
 *
 * @param eventName - The name of the custom event
 * @returns Event system with dispatch and listen methods
 *
 * @example
 * ```ts
 * const modelUpdateEvent = createEventSystem<{ uuid: string }>('model_update');
 *
 * // Dispatch event
 * modelUpdateEvent.dispatch({ uuid: 'abc-123' });
 *
 * // Listen to event
 * const unsubscribe = modelUpdateEvent.listen((data) => {
 *   console.log('Model updated:', data.uuid);
 * });
 *
 * // Cleanup
 * unsubscribe();
 * ```
 */
export function createEventSystem<T = any>(eventName: string): EventSystem<T> {
  return {
    eventName,

    /**
     * Dispatch an event to all listeners
     */
    dispatch: (data?: T) => {
      window.dispatchEvent(new CustomEvent(eventName, { detail: data }));
    },

    /**
     * Listen for events. Returns an unsubscribe function.
     */
    listen: (callback: (data?: T) => void): (() => void) => {
      const handler = (event: Event) => {
        const customEvent = event as CustomEvent<T>;
        callback(customEvent.detail);
      };

      window.addEventListener(eventName, handler);

      // Return unsubscribe function
      return () => {
        window.removeEventListener(eventName, handler);
      };
    },
  };
}

/**
 * React hook for listening to events.
 *
 * @param eventName - The name of the custom event
 * @param callback - The callback function to execute when event is dispatched
 * @param deps - Dependencies for the callback (like useEffect)
 */
export function useEvent<T = any>(
  eventName: string,
  callback: (data?: T) => void,
  deps: any[] = []
) {
  useEffect(() => {
    const handler = (event: Event) => {
      const customEvent = event as CustomEvent<T>;
      callback(customEvent.detail);
    };

    window.addEventListener(eventName, handler);

    return () => {
      window.removeEventListener(eventName, handler);
    };
  }, [eventName, ...deps]);
}
