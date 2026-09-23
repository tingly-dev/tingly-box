import {api, enrichBotsWithCapabilities} from '@/services/api';
import type {BotSettings} from '@/types/bot';
import {useCallback, useState} from 'react';
import {useTranslation} from 'react-i18next';

// useBotList is the single owner of the "load the ImBot list" operation plus
// the restart / delete actions shared by the bot pages (PlatformBotPage,
// BotOverviewPage, PlatformRemoteAgentPage). It owns the bots state, the
// loading flag, the in-flight restart UUID, and the success/error toasts, then
// reloads so the caller's view stays in sync.
//
// Previously load/restart/delete were copy-pasted across the three pages —
// identical toast strings, identical refresh timing, three silent copies (see
// .design/ux-principles #3 — one word, one meaning). Toast texts stay in the
// hook since all three pages used the same ones; delivery goes through the
// caller's `notify` adapter so each page keeps its notification surface.
export interface UseBotListOptions {
    /** Notification adapter (message first, severity second) — typically the
     *  page's showNotification wrapper over useNotify. */
    notify: (message: string, severity?: 'success' | 'error' | 'info' | 'warning') => void;
    /** Re-show the loading spinner on every load, not just the initial one.
     *  The Bots pages did this; PlatformRemoteAgentPage only spins on first
     *  load (default). */
    spinnerOnRefresh?: boolean;
}

export function useBotList({notify, spinnerOnRefresh = false}: UseBotListOptions) {
    const {t} = useTranslation();
    // Starts true (not false) so the very first render doesn't see an empty
    // `bots` array and briefly flash an empty state before the initial fetch
    // has had a chance to resolve.
    const [bots, setBots] = useState<BotSettings[]>([]);
    const [loading, setLoading] = useState(true);
    const [restartingUuid, setRestartingUuid] = useState<string | null>(null);

    const load = useCallback(async () => {
        try {
            if (spinnerOnRefresh) setLoading(true);
            const data = await api.getImBotSettingsList();
            if (data?.success && Array.isArray(data.settings)) {
                setBots(await enrichBotsWithCapabilities(data.settings));
            } else if (data?.success === false) {
                notify(data.error || t('remoteControl.notify.loadFailed', {defaultValue: 'Failed to load bot settings'}), 'error');
            }
        } catch (err) {
            console.error('Failed to load bot settings:', err);
            notify(t('remoteControl.notify.loadFailed', {defaultValue: 'Failed to load bot settings'}), 'error');
        } finally {
            setLoading(false);
        }
    }, [notify, spinnerOnRefresh, t]);

    const restart = useCallback(async (uuid: string) => {
        setRestartingUuid(uuid);
        try {
            const result = await api.restartImBot(uuid);
            if (result?.success) {
                notify(t('remoteControl.notify.botRestarted', {defaultValue: 'Bot restarted'}), 'success');
                await load();
            } else {
                notify(t('remoteControl.notify.restartFailed', {defaultValue: 'Failed to restart bot: {{error}}', error: result?.error || 'Unknown error'}), 'error');
            }
        } catch (err) {
            console.error('Failed to restart bot:', err);
            notify(t('remoteControl.notify.restartFailedGeneric', {defaultValue: 'Failed to restart bot'}), 'error');
        } finally {
            setRestartingUuid(null);
        }
    }, [load, notify, t]);

    const remove = useCallback(async (uuid: string) => {
        try {
            const result = await api.deleteImBotSetting(uuid);
            if (result?.success) {
                notify(t('remoteControl.notify.botDeleted', {defaultValue: 'Bot deleted successfully'}), 'success');
                await load();
            } else {
                notify(t('remoteControl.notify.deleteFailed', {defaultValue: 'Failed to delete bot: {{error}}', error: result?.error}), 'error');
            }
        } catch (err) {
            notify(t('remoteControl.notify.deleteFailedGeneric', {defaultValue: 'Failed to delete bot'}), 'error');
        }
    }, [load, notify, t]);

    const isRestarting = useCallback((uuid: string) => restartingUuid === uuid, [restartingUuid]);

    return {bots, setBots, loading, load, restart, restartingUuid, isRestarting, remove};
}
