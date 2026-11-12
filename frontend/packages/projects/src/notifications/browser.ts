// The Web Notification API, behind guards: the constructor is missing in
// some webviews (and in tests), requesting needs a user gesture, and a
// denied permission throws in some browsers. Callers must work without it.

export type NotificationPermissionState = 'granted' | 'denied' | 'default';

/** `renotify` lets a newer notification with the same tag re-alert; this
 * TypeScript's DOM lib predates it. */
export type ShowOptions = NotificationOptions & { renotify?: boolean };

/** One shown notification: closeable, and clickable (the engine routes the
 * click to the place that answers it). */
export interface BrowserNotification {
    close: () => void;
    onclick: ((ev: Event) => void) | null;
}

/** Whether this browser can show notifications at all. */
export function notificationsSupported(): boolean {
    return typeof window !== 'undefined' && 'Notification' in window;
}

/** The browser's permission for this origin; `default` when never asked. */
export function notificationPermission(): NotificationPermissionState {
    if (!notificationsSupported()) return 'denied';
    return Notification.permission;
}

/** Asks the browser. Call from a click: most browsers refuse without one. */
export async function requestNotificationPermission(): Promise<NotificationPermissionState> {
    if (!notificationsSupported()) return 'denied';
    try {
        return await Notification.requestPermission();
    } catch {
        return notificationPermission();
    }
}

/**
 * Shows one system notification. Returns null when it cannot be shown
 * (unsupported, not granted, or the browser rejected the construction),
 * so the caller never learns the hard way.
 */
export function showBrowserNotification(title: string, options?: ShowOptions): BrowserNotification | null {
    if (!notificationsSupported() || Notification.permission !== 'granted') return null;
    try {
        return new Notification(title, options);
    } catch {
        return null;
    }
}
