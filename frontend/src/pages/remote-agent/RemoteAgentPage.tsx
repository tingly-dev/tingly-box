import { BotConfigDialog, RemoteAgentBotCard } from '@/components/bot';
import CCProfileDialog from '@/components/bot/CCProfileDialog';
import EmptyState from '@/components/EmptyState';
import { PageLayout } from '@/components/PageLayout';
import { api } from '@/services/api';
import { BOT_PLATFORM_IDS } from '@/constants/platformGuides';
import { useProfileContext } from '@/contexts/ProfileContext';
import { useBotList } from '@/hooks/useBotList';
import { useStableBotOrder } from '@/components/bot/useStableBotOrder';
import type { BotSettings } from '@/types/bot';
import { capabilityEnabled, defaultAgentForCCProfile } from '@/types/bot';
import { Add } from '@/components/icons';
import { Box, Button, CircularProgress } from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useNotify } from '@/hooks/useNotify';
import { useTranslation } from 'react-i18next';

// RemoteAgentPage is the Remote Control PURPOSE page: every bot, one card
// each, answering who can use Claude Code through it and under which profile. It used to be
// paginated by platform (a row of platform tiles above a per-platform list),
// but platform is a property of a bot, not a question the user is asking —
// most setups have one or two bots, and the tiles mostly advertised empty
// platforms. Platform is now the icon on each card and a choice inside the
// Connect dialog (which also carries that platform's setup guide).
const RemoteAgentPage = () => {
    const { t } = useTranslation();

    // The SHARED bot-resource dialog, opened in place — no bouncing to the
    // Bots section. mode 'add' from the Add button / empty state; mode 'edit'
    // from a card's edit action.
    const [dialogOpen, setDialogOpen] = useState(false);
    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>('add');
    const [dialogEditUuid, setDialogEditUuid] = useState<string | null>(null);
    const openAddDialog = useCallback(() => {
        setDialogMode('add');
        setDialogEditUuid(null);
        setDialogOpen(true);
    }, []);
    const openEditDialog = useCallback((uuid: string) => {
        setDialogMode('edit');
        setDialogEditUuid(uuid);
        setDialogOpen(true);
    }, []);

    const [togglingBotUuid, setTogglingBotUuid] = useState<string | null>(null);

    const notify = useNotify();

    // Notification adapter (message first, severity second) — shared with
    // BotConfigDialog's `notify` prop; rendered globally by NotificationProvider.
    const showNotification = useCallback((message: string, severity: 'success' | 'error' | 'info' | 'warning' = 'success') => {
        notify[severity](message);
    }, [notify]);

    // Bot list + restart/delete via the shared useBotList hook (same ops
    // across the bot pages). No `spinnerOnRefresh` here: this page only spins
    // on the first load, keeping the list rendered during refreshes.
    const {
        bots,
        loading,
        load: loadBots,
        restart: handleBotRestart,
        restartingUuid: restartingBotUuid,
        remove: handleDeleteBot,
    } = useBotList({notify: showNotification});

    const sortedBots = useStableBotOrder(bots, loading, (bot) =>
        Boolean(bot.enabled ?? true) && capabilityEnabled(bot, 'remote_control'));

    useEffect(() => {
        loadBots();
    }, [loadBots]);

    // Toggle the explicit Remote Control capability.
    const handleMountToggle = useCallback(async (bot: BotSettings, enabled: boolean) => {
        if (!bot.uuid) return;
        setTogglingBotUuid(bot.uuid);
        try {
            // Capability lifecycle is reconciled server-side: enabling Remote
            // starts the Bot; disabling the last capability turns it off.
            const result = await api.setBotCapability(bot.uuid, 'remote_control', enabled);
            if (result?.capability) {
                showNotification(
                    enabled
                        ? t('remoteControl.notify.remoteAgentOn', { defaultValue: 'Remote Control enabled' })
                        : t('remoteControl.notify.remoteAgentOff', { defaultValue: 'Remote Control disabled' }),
                    'success'
                );
                await loadBots();
            } else {
                showNotification(result?.reason || t('remoteControl.notify.toggleFailedGeneric', { defaultValue: 'Failed to toggle bot' }), 'error');
            }
        } catch (err) {
            console.error('Failed to toggle Remote Control capability:', err);
            showNotification(
                err instanceof Error && err.message
                    ? err.message
                    : t('remoteControl.notify.toggleFailedGeneric', { defaultValue: 'Failed to toggle Remote Control' }),
                'error',
            );
        } finally {
            setTogglingBotUuid(null);
        }
    }, [loadBots, showNotification, t]);

    // Claude Code profiles — the selected one is what a chat's first session
    // runs under.
    const { getProfiles: getScenarioProfiles } = useProfileContext();
    const ccProfiles = getScenarioProfiles('claude_code');
    const [profileDialogBot, setProfileDialogBot] = useState<BotSettings | null>(null);

    const handleCCProfileSelect = useCallback(async (uuid: string, profileId: string) => {
        const response = await api.updateImBotSetting(uuid, {
            default_agent: defaultAgentForCCProfile(profileId),
        });
        if (response?.success) {
            showNotification(t('remoteAgent.notify.ccProfileUpdated', { defaultValue: 'Claude Code profile updated' }), 'success');
            await loadBots();
        } else {
            const message = response?.error || t('remoteAgent.notify.ccProfileUpdateFailed', { defaultValue: 'Failed to update Claude Code profile' });
            showNotification(message, 'error');
            throw new Error(message);
        }
    }, [loadBots, showNotification, t]);

    return (
        <PageLayout
            loading={false}
            title={t('remoteAgent.pageTitle', {defaultValue: 'Remote Control'})}
            subtitle={t('remoteAgent.pageSubtitle', {defaultValue: 'Who can use Claude Code from each bot\'s chats, and the profile it runs under.'})}
            rightAction={
                <Button variant="contained" startIcon={<Add/>} onClick={openAddDialog} size="small">
                    {t('remoteControl.bots.addBot', {defaultValue: 'Connect a bot'})}
                </Button>
            }
        >
            {loading ? (
                <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                    <CircularProgress />
                </Box>
            ) : bots.length === 0 ? (
                <EmptyState
                    title={t('remoteAgent.emptyTitle', { defaultValue: 'No bots yet' })}
                    description={t('remoteAgent.emptyDescription', { defaultValue: 'Connect a Telegram, Feishu, DingTalk or other chat bot, then use Claude Code from that chat.' })}
                    primaryAction={{
                        label: t('remoteControl.bots.addBot', { defaultValue: 'Connect a bot' }),
                        onClick: openAddDialog,
                    }}
                />
            ) : (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                    {sortedBots.map((bot) => (
                        <RemoteAgentBotCard
                            key={bot.uuid}
                            bot={bot}
                            onMountToggle={(enabled) => handleMountToggle(bot, enabled)}
                            ccProfiles={ccProfiles}
                            onCCProfileClick={() => setProfileDialogBot(bot)}
                            onEdit={() => openEditDialog(bot.uuid!)}
                            onRestart={() => handleBotRestart(bot.uuid!)}
                            onDelete={() => handleDeleteBot(bot.uuid!)}
                            isToggling={togglingBotUuid === bot.uuid}
                            isRestarting={restartingBotUuid === bot.uuid}
                        />
                    ))}
                </Box>
            )}
            {/* Shared bot-resource dialog: add/edit a bot without leaving this page */}
            <BotConfigDialog
                open={dialogOpen}
                mode={dialogMode}
                editUuid={dialogEditUuid}
                platformId={BOT_PLATFORM_IDS[0]}
                lockPlatform={false}
                bots={bots}
                onClose={() => setDialogOpen(false)}
                onSaved={loadBots}
                notify={showNotification}
            />
            <CCProfileDialog
                open={Boolean(profileDialogBot)}
                bot={profileDialogBot}
                profiles={ccProfiles}
                onSelect={handleCCProfileSelect}
                onClose={() => setProfileDialogBot(null)}
            />
        </PageLayout>
    );
};

export default RemoteAgentPage;
