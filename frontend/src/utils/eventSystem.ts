/**
 * Generic event system for cross-component communication.
 * Eliminates duplication across useCustomModels, useNewModels, useRecentModels, useProviderModels.
 */
import { useEffect } from 'react';

export interface EventSystem<T = any> {
  eventName: string;
  dispatch: (data?: T) => void;
  listen: (callback: (data?: T) => void) => () => void;
}

/**
 * Create an event system for cross-component communication.
 *
 * @param eventName - The name of the custom event
 * @param crossTab - If true, events are also broadcast to other tabs via BroadcastChannel
 * @returns Event system with dispatch and listen methods
 */
export function createEventSystem<T = any>(eventName: string, crossTab = false): EventSystem<T> {
  // Suppress echo: BroadcastChannel delivers to all tabs including the sender;
  // tag each message so the originating tab can skip it.
  const tabId = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : Math.random().toString(36).slice(2);
  const channel = crossTab && typeof BroadcastChannel !== 'undefined'
    ? new BroadcastChannel(`tingly_event_${eventName}`)
    : null;

  return {
    eventName,

    dispatch: (data?: T) => {
      window.dispatchEvent(new CustomEvent(eventName, { detail: data }));
      channel?.postMessage({ tabId, data });
    },

    listen: (callback: (data?: T) => void): (() => void) => {
      const localHandler = (event: Event) => {
        callback((event as CustomEvent<T>).detail);
      };

      window.addEventListener(eventName, localHandler);

      let channelHandler: ((e: MessageEvent) => void) | undefined;
      if (channel) {
        channelHandler = (e: MessageEvent) => {
          if (e.data?.tabId === tabId) return;
          callback(e.data?.data);
        };
        channel.addEventListener('message', channelHandler);
      }

      return () => {
        window.removeEventListener(eventName, localHandler);
        if (channel && channelHandler) {
          channel.removeEventListener('message', channelHandler);
        }
      };
    },
  };
}

/**
 * React hook for listening to events.
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
