import { BotConfigDialog, RemoteAgentBotCard, useBotModelDialog } from '@/components/bot';
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
import type { Provider } from '@/types/provider';
import { Add } from '@/components/icons';
import { Box, Button, CircularProgress } from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useNotify } from '@/hooks/useNotify';
import { useTranslation } from 'react-i18next';

// RemoteAgentPage is the Remote Control PURPOSE page: every bot, one card
// each, answering who can control it and where @tb / @cc go. It used to be
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

    const [providers, setProviders] = useState<Provider[]>([]);
    const [togglingBotUuid, setTogglingBotUuid] = useState<string | null>(null);
    const [selectedBot, setSelectedBot] = useState<BotSettings | null>(null);

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

    const loadProviders = useCallback(async () => {
        const data = await api.getProviders();
        if (data?.success && data?.data) {
            setProviders(data.data);
        }
    }, []);

    useEffect(() => {
        loadBots();
        loadProviders();
    }, [loadBots, loadProviders]);

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

    const handleBotModelUpdate = useCallback(async (uuid: string, provider: string, model: string) => {
        const response = await api.updateImBotSetting(uuid, {
            smartguide_provider: provider,
            smartguide_model: model,
        });
        if (response.success) {
            showNotification(t('remoteControl.notify.modelUpdated', { defaultValue: 'Bot model configuration updated' }), 'success');
            await loadBots();
        } else {
            const message = response.error || t('remoteControl.notify.modelUpdateFailed', { defaultValue: 'Failed to update bot configuration' });
            showNotification(message, 'error');
            throw new Error(message);
        }
    }, [loadBots, showNotification, t]);

    const {
        openDialog: openBotModelDialog,
        BotModelDialog,
        isOpen: botModelDialogOpen,
    } = useBotModelDialog({
        bot: selectedBot,
        providers,
        onUpdate: handleBotModelUpdate,
        onClose: () => setSelectedBot(null),
    });

    const handleModelClick = useCallback((bot: BotSettings) => {
        setSelectedBot(bot);
        openBotModelDialog();
    }, [openBotModelDialog]);

    // Claude Code profiles for the @cc branch — the selected profile decides
    // which claude_code scenario remote @cc executions route through.
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

    const handleTogglePersistentSession = useCallback(async (uuid: string, enabled: boolean) => {
        const response = await api.updateImBotSetting(uuid, {
            persistent_session: enabled,
        });
        if (response?.success) {
            await loadBots();
        } else {
            const message = response?.error || t('remoteAgent.notify.persistentSessionUpdateFailed', { defaultValue: 'Failed to update persistent-session setting' });
            showNotification(message, 'error');
            throw new Error(message);
        }
    }, [loadBots, showNotification, t]);

    return (
        <PageLayout
            loading={false}
            title={t('remoteAgent.pageTitle', {defaultValue: 'Remote Control'})}
            subtitle={t('remoteAgent.pageSubtitle', {defaultValue: 'Who can control each bot, and where @tb and @cc messages go.'})}
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
                    description={t('remoteAgent.emptyDescription', { defaultValue: 'Connect a Telegram, Feishu, DingTalk or other chat bot, then drive Claude Code and SmartGuide from that chat.' })}
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
                            providers={providers}
                            onMountToggle={(enabled) => handleMountToggle(bot, enabled)}
                            onModelClick={() => handleModelClick(bot)}
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
            <BotModelDialog open={botModelDialogOpen} />
            <CCProfileDialog
                open={Boolean(profileDialogBot)}
                bot={profileDialogBot}
                profiles={ccProfiles}
                onSelect={handleCCProfileSelect}
                onTogglePersistentSession={handleTogglePersistentSession}
                onClose={() => setProfileDialogBot(null)}
            />
        </PageLayout>
    );
};

export default RemoteAgentPage;
