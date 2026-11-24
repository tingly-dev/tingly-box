// The notifications preference as one status the controls render: the user's
// choice (on unless turned off) crossed with what the browser permits.
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNotify } from '../hooks/useNotify';
import { notificationsPreferred, setNotificationsEnabled, syncNotifications } from './engine';
import {
    notificationPermission,
    notificationsSecure,
    notificationsSupported,
    requestNotificationPermission,
    sendTestNotification,
} from './browser';

/**
 * - `on`: preferred and permitted; pings while the tab is hidden.
 * - `off`: the user turned it off.
 * - `ask`: preferred, but the browser has not been asked yet (a click does that).
 * - `blocked`: the browser refuses; only its own site settings can change it.
 * - `insecure`: the page is not a secure context (plain http on a LAN address); the browser withholds the API.
 * - `unsupported`: this browser cannot show notifications.
 */
export type NotificationStatus = 'on' | 'off' | 'ask' | 'blocked' | 'insecure' | 'unsupported';

export function notificationStatus(): NotificationStatus {
    if (!notificationsSecure()) return 'insecure';
    if (!notificationsSupported()) return 'unsupported';
    const permission = notificationPermission();
    if (permission === 'denied') return notificationsPreferred() ? 'blocked' : 'off';
    if (!notificationsPreferred()) return 'off';
    return permission === 'granted' ? 'on' : 'ask';
}

export interface Notifications {
    status: NotificationStatus;
    /** The browser's own dialog is open. */
    asking: boolean;
    /** Turns it on, asking the browser first when it has not granted yet. Call from a click. */
    turnOn: () => Promise<void>;
    turnOff: () => void;
    /** A test notification is on its way (the browser may be asking first). */
    testing: boolean;
    /** Sends one notification now and says how it went. Call from a click. */
    sendTest: () => Promise<void>;
}

export function useNotifications(): Notifications {
    const [status, setStatus] = useState(notificationStatus);
    const [asking, setAsking] = useState(false);
    const [testing, setTesting] = useState(false);
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const refresh = useCallback(() => setStatus(notificationStatus()), []);

    // The permission can change outside the page (the browser's site settings):
    // follow it, so unblocking there turns the feature on without a reload.
    useEffect(() => {
        if (!notificationsSupported() || !navigator.permissions?.query) return;
        let alive = true;
        let watched: PermissionStatus | undefined;
        const onChange = () => {
            syncNotifications();
            refresh();
        };
        navigator.permissions.query({ name: 'notifications' }).then((s) => {
            if (!alive) return;
            watched = s;
            s.addEventListener('change', onChange);
        }, () => undefined);
        return () => {
            alive = false;
            watched?.removeEventListener('change', onChange);
        };
    }, [refresh]);

    const turnOn = useCallback(async () => {
        if (notificationPermission() !== 'granted') {
            setAsking(true);
            await requestNotificationPermission();
            setAsking(false);
        }
        // Denied or dismissed: keep the preference on, so the status says "blocked" or "ask" rather than silently off.
        setNotificationsEnabled(true);
        refresh();
    }, [refresh]);

    const turnOff = useCallback(() => {
        setNotificationsEnabled(false);
        refresh();
    }, [refresh]);

    const sendTest = useCallback(async () => {
        setTesting(true);
        const result = await sendTestNotification(t('notifyControl.testTitle'), t('notifyControl.testBody'));
        setTesting(false);
        // The test may have just been granted the permission: start watching if that is what the user wants.
        syncNotifications();
        refresh();
        notify(t(`notifyControl.test_${result}`), result === 'sent' ? 'success' : 'warning');
    }, [notify, refresh, t]);

    return { status, asking, turnOn, turnOff, testing, sendTest };
}
