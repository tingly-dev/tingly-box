/**
 * Global notification store — a framework-agnostic singleton that any code
 * (components, hooks, contexts, plain modules) can push notifications into.
 *
 * Rendering is handled by NotificationProvider (see src/contexts/NotificationContext.tsx),
 * which subscribes to this store and renders a stacked, antd-style toast list.
 */

export type NotifySeverity = 'success' | 'error' | 'warning' | 'info';

export interface NotifyOptions {
  /** Optional bold heading shown above the message. */
  title?: string;
  /** Auto-dismiss delay in ms. 0 (or negative) keeps the toast until dismissed. */
  duration?: number;
  /** Stable id — pushing again with the same key updates the existing toast. */
  key?: string;
}

export interface NotifyItem {
  id: string;
  severity: NotifySeverity;
  message: string;
  title?: string;
  duration: number;
  createdAt: number;
}

const MAX_VISIBLE = 5;

const DEFAULT_DURATION: Record<NotifySeverity, number> = {
  success: 3500,
  info: 3500,
  warning: 5000,
  error: 6000,
};

let items: NotifyItem[] = [];
let counter = 0;
const listeners = new Set<() => void>();

function emit(): void {
  for (const listener of listeners) listener();
}

export function subscribeNotify(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getNotifyItems(): NotifyItem[] {
  return items;
}

export function pushNotify(
  severity: NotifySeverity,
  message: string,
  options?: NotifyOptions,
): string {
  const id = options?.key ?? `notify-${Date.now()}-${counter++}`;
  const item: NotifyItem = {
    id,
    severity,
    message,
    title: options?.title,
    duration: options?.duration ?? DEFAULT_DURATION[severity],
    createdAt: Date.now(),
  };
  const existingIndex = items.findIndex((i) => i.id === id);
  if (existingIndex >= 0) {
    items = items.map((i, idx) => (idx === existingIndex ? item : i));
  } else {
    items = [...items, item].slice(-MAX_VISIBLE);
  }
  emit();
  return id;
}

export function dismissNotify(id: string): void {
  const next = items.filter((i) => i.id !== id);
  if (next.length !== items.length) {
    items = next;
    emit();
  }
}

export function clearNotify(): void {
  if (items.length > 0) {
    items = [];
    emit();
  }
}

export const notify = {
  show: (severity: NotifySeverity, message: string, options?: NotifyOptions) =>
    pushNotify(severity, message, options),
  success: (message: string, options?: NotifyOptions) => pushNotify('success', message, options),
  error: (message: string, options?: NotifyOptions) => pushNotify('error', message, options),
  warning: (message: string, options?: NotifyOptions) => pushNotify('warning', message, options),
  info: (message: string, options?: NotifyOptions) => pushNotify('info', message, options),
  dismiss: dismissNotify,
  clear: clearNotify,
};
