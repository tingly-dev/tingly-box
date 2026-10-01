import EmptyState from '@/components/EmptyState';
import GuideAction from '@/components/GuideAction';
import { PageLayout } from '@/components/PageLayout';
import NotifyGuide from '@/components/notify/NotifyGuide';
import BotNotifyGroup from '@/components/notify/BotNotifyGroup';
import { useStableBotOrder } from '@/components/bot/useStableBotOrder';
import { api, enrichBotsWithCapabilities } from '@/services/api';
import type { BotSettings } from '@/types/bot';
import { capabilityEnabled } from '@/types/bot';
import { notify } from '@/utils/notify';
import { Stack } from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

// NotifyPage opens up the authenticated bot-interaction API: it teaches how to
// call it (top guide) and, per bot, shows the chats that bot can reach as an
// always-expanded graph — each Chat node pairs the concrete platform id with
// the stable internal target UUID required by /notify, with test actions inline
// (no extra click to reach the work surface). The bot's enabled switch is the
// on/off for whether it can be driven.
//
// This page is no longer about "which scenario routes point at this bot" (that
// was the old read-only framing, surfaced as a misleading "No routes" chip) —
// it answers the operator's actual question: "what can I send to, right now?"
// See .design/bot-interaction-api.md and ux-principles #1/#5/#11.
//
// Like Remote Control, it lists every bot on one page: platform is a property
// of a bot (its card's icon), not a filter to pick before seeing anything.
const NotifyPage = () => {
    const { t } = useTranslation();
    const [bots, setBots] = useState<BotSettings[]>([]);
    const [loading, setLoading] = useState(true);
    const [toggling, setToggling] = useState<string | null>(null);

    const loadBots = useCallback(async () => {
        try {
            setLoading(true);
            const data = await api.getImBotSettingsList();
            if (data?.success && Array.isArray(data.settings)) {
                setBots(await enrichBotsWithCapabilities(data.settings));
            }
        } catch (err) {
            console.error('Failed to load bot settings:', err);
            notify.error(err instanceof Error ? err.message : t('notify.loadFailed', {defaultValue: 'Failed to load Notify targets'}));
        } finally {
            setLoading(false);
        }
    }, [t]);

    useEffect(() => {
        loadBots();
    }, [loadBots]);

    // Notify is an explicit capability. Its lifecycle is reconciled by the
    // backend: enabling it starts the Bot, and disabling the last capability
    // stops the Bot.
    const handleToggle = useCallback(async (uuid: string, enabled: boolean) => {
        setToggling(uuid);
        try {
            await api.setBotCapability(uuid, 'notify', enabled);
            await loadBots();
        } catch (toggleError) {
            notify.error(toggleError instanceof Error ? toggleError.message : t('notify.toggleFailed', {defaultValue: 'Failed to update Notify'}));
        } finally {
            setToggling(null);
        }
    }, [loadBots]);

    const sortedBots = useStableBotOrder(bots, loading, (bot) =>
        Boolean(bot.enabled ?? true) && capabilityEnabled(bot, 'notify'));

    return (
        <PageLayout
            loading={loading}
            title={t('notify.title', {defaultValue: 'IM Notify'})}
            subtitle={t('notify.subtitle', {defaultValue: 'Authorize a target, send through the production path, and see whether delivery worked.'})}
            rightAction={(
                <GuideAction
                    label={t('notify.guide.action', { defaultValue: 'API guide' })}
                    title={t('notify.guide.title', { defaultValue: 'IM Notify API Guide' })}
                    description={t('notify.guide.description', {
                        defaultValue: 'Authentication, request examples, and target IDs',
                    })}
                >
                    <NotifyGuide />
                </GuideAction>
            )}
        >
            {bots.length === 0 ? (
                <EmptyState
                    title={t('notify.emptyTitle', { defaultValue: 'No bots connected yet' })}
                    description={t('notify.emptyDescription', { defaultValue: 'Connect a bot on the Bots page first, then come back here to send it notifications.' })}
                />
            ) : (
                <Stack spacing={1.5}>
                    {sortedBots.map((bot) => (
                        <BotNotifyGroup
                            key={bot.uuid}
                            bot={bot}
                            onToggle={handleToggle}
                            isToggling={toggling === bot.uuid}
                        />
                    ))}
                </Stack>
            )}
        </PageLayout>
    );
};

export default NotifyPage;
