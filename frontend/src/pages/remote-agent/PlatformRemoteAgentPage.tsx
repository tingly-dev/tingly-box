import { BotConfigDialog, RemoteAgentBotCard, useBotModelDialog } from '@/components/bot';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import { PageLayout } from '@/components/PageLayout';
import CollapsibleGuide from '@/components/remote-control/CollapsibleGuide';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { usePlatformGuide } from '@/constants/platformGuides';
import type { BotSettings } from '@/types/bot';
import type { Provider } from '@/types/provider';
import { Add } from '@/components/icons';
import { Alert, Box, Button, CircularProgress, Snackbar } from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface PlatformRemoteAgentPageProps {
    platformId: string;
    platformName: string;
}

// PlatformRemoteAgentPage is the PURPOSE page, deliberately mirroring the
// Bots section's per-platform structure: same pagination, different content.
// A Bots page manages this platform's bot connections; this page manages the
// same bots' Remote Agent configuration — mount switch, SmartGuide model,
// and agent behavior (chat lock, bash allowlist).
const PlatformRemoteAgentPage = ({ platformId, platformName }: PlatformRemoteAgentPageProps) => {
    const { t } = useTranslation();
    const guideConfig = usePlatformGuide(platformId);

    // The SHARED bot-resource dialog, opened in place — no bouncing to the
    // Bots section. mode 'add' from the Add button / empty state; mode 'edit'
    // from a card's edit action while the Bots nav section is hidden.
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

    const [bots, setBots] = useState<BotSettings[]>([]);
    const [providers, setProviders] = useState<Provider[]>([]);
    const [loading, setLoading] = useState(true);
    const [togglingBotUuid, setTogglingBotUuid] = useState<string | null>(null);
    const [restartingBotUuid, setRestartingBotUuid] = useState<string | null>(null);
    const [selectedBot, setSelectedBot] = useState<BotSettings | null>(null);

    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error' | 'info' | 'warning';
    }>({ open: false, message: '', severity: 'success' });

    const showNotification = useCallback((message: string, severity: 'success' | 'error' | 'info' | 'warning' = 'success') => {
        setSnackbar({ open: true, message, severity });
    }, []);

    // Same platform filter as the Bots page — the two sections paginate
    // identically and differ only in what they show for each bot.
    const filteredBots = useMemo(
        () => bots.filter(b => b.platform === platformId),
        [bots, platformId]
    );

    const loadBots = useCallback(async () => {
        try {
            const data = await api.getImBotSettingsList();
            if (data?.success && Array.isArray(data.settings)) {
                setBots(data.settings);
            } else if (data?.success === false) {
                showNotification(data.error || t('remoteControl.notify.loadFailed', { defaultValue: 'Failed to load bot settings' }), 'error');
            }
        } catch (err) {
            console.error('Failed to load bot settings:', err);
            showNotification(t('remoteControl.notify.loadFailed', { defaultValue: 'Failed to load bot settings' }), 'error');
        } finally {
            setLoading(false);
        }
    }, [showNotification, t]);

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

    // Toggle the remote_agent mount. Turning it on cascades the bot's Enabled
    // flag server-side, so one flip lights the bot up.
    const handleMountToggle = useCallback(async (uuid: string, mounted: boolean) => {
        setTogglingBotUuid(uuid);
        try {
            const result = await api.updateImBotSetting(uuid, { remote_agent: mounted });
            if (result?.success) {
                showNotification(
                    mounted
                        ? t('remoteControl.notify.remoteAgentOn', { defaultValue: 'Remote Agent mounted' })
                        : t('remoteControl.notify.remoteAgentOff', { defaultValue: 'Remote Agent unmounted' }),
                    'success'
                );
                await loadBots();
            } else {
                showNotification(result?.error || t('remoteControl.notify.toggleFailedGeneric', { defaultValue: 'Failed to toggle bot' }), 'error');
            }
        } catch (err) {
            console.error('Failed to toggle remote_agent mount:', err);
            showNotification(t('remoteControl.notify.toggleFailedGeneric', { defaultValue: 'Failed to toggle bot' }), 'error');
        } finally {
            setTogglingBotUuid(null);
        }
    }, [loadBots, showNotification, t]);

    const handleBotRestart = useCallback(async (uuid: string) => {
        setRestartingBotUuid(uuid);
        try {
            const result = await api.restartImBot(uuid);
            if (result?.success) {
                showNotification(t('remoteControl.notify.botRestarted', { defaultValue: 'Bot restarted' }), 'success');
                await loadBots();
            } else {
                showNotification(t('remoteControl.notify.restartFailed', { defaultValue: 'Failed to restart bot: {{error}}', error: result?.error || 'Unknown error' }), 'error');
            }
        } catch (err) {
            console.error('Failed to restart bot:', err);
            showNotification(t('remoteControl.notify.restartFailedGeneric', { defaultValue: 'Failed to restart bot' }), 'error');
        } finally {
            setRestartingBotUuid(null);
        }
    }, [loadBots, showNotification, t]);

    const handleDeleteBot = useCallback(async (uuid: string) => {
        try {
            const result = await api.deleteImBotSetting(uuid);
            if (result?.success) {
                showNotification(t('remoteControl.notify.botDeleted', { defaultValue: 'Bot deleted successfully' }), 'success');
                await loadBots();
            } else {
                showNotification(t('remoteControl.notify.deleteFailed', { defaultValue: 'Failed to delete bot: {{error}}', error: result?.error }), 'error');
            }
        } catch (err) {
            showNotification(t('remoteControl.notify.deleteFailedGeneric', { defaultValue: 'Failed to delete bot' }), 'error');
        }
    }, [loadBots, showNotification, t]);

    const handleAgentSettingsSave = useCallback(async (uuid: string, settings: { chat_id: string; bash_allowlist: string[] }) => {
        const result = await api.updateImBotSetting(uuid, settings);
        if (result?.success) {
            showNotification(t('remoteAgent.notify.agentSettingsSaved', { defaultValue: 'Agent settings saved' }), 'success');
            await loadBots();
        } else {
            showNotification(result?.error || t('remoteControl.notify.saveFailed', { defaultValue: 'Failed to save bot settings' }), 'error');
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

    return (
        <PageLayout loading={false}>
            {/* Same platform setup guide as the Bots page — mirrored sections
                share the education. Gated on !loading so defaultExpanded locks
                in against the real bot count. */}
            {!loading && guideConfig?.guide && (
                <CollapsibleGuide
                    platformName={platformName}
                    platformGuide={guideConfig.guide}
                    defaultExpanded={filteredBots.length === 0}
                />
            )}
            <UnifiedCard
                title={t('remoteAgent.title', { defaultValue: '{{platform}} Remote Agent', platform: platformName })}
                subtitle={t('remoteAgent.subtitle', { defaultValue: 'Mount {{platform}} bots to drive Claude Code / SmartGuide from chat, and configure how the agent behaves. Bot connections are managed on the Bots pages.', platform: platformName })}
                size="full"
                sx={{ mb: 2 }}
                rightAction={
                    <Button
                        variant="contained"
                        startIcon={<Add />}
                        onClick={openAddDialog}
                        size="small"
                    >
                        {t('remoteControl.bots.addBot', { defaultValue: 'Add Bot' })}
                    </Button>
                }
            >
                {loading ? (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                        <CircularProgress />
                    </Box>
                ) : filteredBots.length === 0 ? (
                    <EmptyStateGuide
                        title={t('remoteAgent.emptyTitle', { defaultValue: 'No {{platform}} Bots Yet', platform: platformName })}
                        description={t('remoteAgent.emptyDescription', { defaultValue: 'Remote Agent runs on top of a bot. Create a {{platform}} bot connection first, then mount it here.', platform: platformName })}
                        showHeroIcon={false}
                        primaryButtonLabel={t('remoteControl.bots.addPlatformBot', { defaultValue: 'Add {{platform}} Bot', platform: platformName })}
                        onAddApiKeyClick={openAddDialog}
                    />
                ) : (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                        {filteredBots.map((bot) => (
                            <RemoteAgentBotCard
                                key={bot.uuid}
                                bot={bot}
                                providers={providers}
                                onMountToggle={(mounted) => handleMountToggle(bot.uuid!, mounted)}
                                onModelClick={() => handleModelClick(bot)}
                                onAgentSettingsSave={(settings) => handleAgentSettingsSave(bot.uuid!, settings)}
                                onEdit={() => openEditDialog(bot.uuid!)}
                                onRestart={() => handleBotRestart(bot.uuid!)}
                                onDelete={() => handleDeleteBot(bot.uuid!)}
                                isToggling={togglingBotUuid === bot.uuid}
                                isRestarting={restartingBotUuid === bot.uuid}
                            />
                        ))}
                    </Box>
                )}
            </UnifiedCard>
            {/* Shared bot-resource dialog: add/edit a bot without leaving this page */}
            <BotConfigDialog
                open={dialogOpen}
                mode={dialogMode}
                editUuid={dialogEditUuid}
                platformId={platformId}
                bots={bots}
                onClose={() => setDialogOpen(false)}
                onSaved={loadBots}
                notify={showNotification}
            />
            <BotModelDialog open={botModelDialogOpen} />
            <Snackbar
                open={snackbar.open}
                autoHideDuration={snackbar.severity === 'error' ? null : 4000}
                onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            >
                <Alert
                    onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
                    severity={snackbar.severity}
                    sx={{ width: '100%' }}
                >
                    {snackbar.message}
                </Alert>
            </Snackbar>
        </PageLayout>
    );
};

export default PlatformRemoteAgentPage;
