import {api} from '@/services/api';
import {notify} from '@/utils/notify';
import {useCallback, useState} from 'react';
import {useTranslation} from 'react-i18next';

// useBotToggle is the single owner of the "toggle an ImBot on/off" operation
// (POST /api/v1/imbot-settings/:uuid/toggle). It owns the in-flight UUID, the
// API call, and the success/error toasts, then invokes onDone so the caller can
// refresh its own view.
//
// Previously this block was copy-pasted across PlatformBotPage, BotOverviewPage,
// and NotifyPage — each with subtly different toast strings and two different
// notification systems (the `notify` util vs each page's Snackbar). One hook
// retires all three copies and the drift (see .design/ux-principles #3 — one
// word, one meaning).
export interface UseBotToggleOptions {
    /** Invoked with the toggled bot's UUID after a successful toggle, so the
     *  caller can refresh or patch its own view. */
    onDone?: (uuid: string) => void | Promise<void>;
}

export interface UseBotToggleResult {
    /** Toggle the bot. `enabled` is the state the bot is moving TO (i.e.
     *  `!bot.enabled`), so the toast reports the outcome rather than the state
     *  that just ended. */
    toggle: (uuid: string, enabled: boolean) => Promise<void>;
    /** The UUID currently being toggled, or null. */
    togglingUuid: string | null;
    /** Convenience predicate for a specific bot row. */
    isToggling: (uuid: string) => boolean;
}

export function useBotToggle({onDone}: UseBotToggleOptions = {}): UseBotToggleResult {
    const {t} = useTranslation();
    const [togglingUuid, setTogglingUuid] = useState<string | null>(null);

    const toggle = useCallback(async (uuid: string, enabled: boolean) => {
        setTogglingUuid(uuid);
        try {
            const result = await api.toggleImBotSetting(uuid);
            if (result?.success) {
                notify.success(
                    enabled
                        ? t('bots.toggle.enabled', {defaultValue: 'Bot enabled'})
                        : t('bots.toggle.disabled', {defaultValue: 'Bot disabled'}),
                );
                await onDone?.(uuid);
            } else {
                notify.error(t('bots.toggle.failed', {
                    defaultValue: 'Failed to toggle bot: {{error}}',
                    error: result?.error || 'Unknown error',
                }));
            }
        } catch (err) {
            console.error('Failed to toggle bot:', err);
            notify.error(t('bots.toggle.failedGeneric', {defaultValue: 'Failed to toggle bot'}));
        } finally {
            setTogglingUuid(null);
        }
    }, [onDone, t]);

    const isToggling = useCallback((uuid: string) => togglingUuid === uuid, [togglingUuid]);

    return {toggle, togglingUuid, isToggling};
}
