import { BotCard, BotConfigDialog } from '@/components/bot';
import EmptyState from '@/components/EmptyState';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import type { BotSettings } from '@/types/bot';
import { Add } from '@/components/icons';
import { Alert, Box, Button, CircularProgress, Snackbar } from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

// BotOverviewPage is the front door for the Bots section: every connected
// bot, across every platform, in one list. It's the only place a bot's
// credential (token / OAuth / QR session) gets typed, rotated, or deleted —
// Remote and Notify only mount purposes onto bots that already exist here.
// Platform is just a column, not a nav split: unlike the per-platform pages
// it supersedes in the nav, this page has no platform filter.
const BotOverviewPage = () => {
    const { t } = useTranslation();
    const [searchParams, setSearchParams] = useSearchParams();

    const [bots, setBots] = useState<BotSettings[]>([]);

    const [dialogOpen, setDialogOpen] = useState(false);
    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>('add');
    const [dialogEditUuid, setDialogEditUuid] = useState<string | null>(null);
    const [dialogPlatformId, setDialogPlatformId] = useState('telegram');

    const [botLoading, setBotLoading] = useState(true);
    const [togglingBotUuid, setTogglingBotUuid] = useState<string | null>(null);
    const [restartingBotUuid, setRestartingBotUuid] = useState<string | null>(null);

    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error' | 'info' | 'warning';
    }>({ open: false, message: '', severity: 'success' });

    const showNotification = useCallback((message: string, severity: 'success' | 'error' | 'info' | 'warning' = 'success') => {
        setSnackbar({ open: true, message, severity });
    }, []);

    const loadBotSettings = useCallback(async () => {
        try {
            setBotLoading(true);
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
            setBotLoading(false);
        }
    }, [showNotification, t]);

    useEffect(() => {
        loadBotSettings();
    }, [loadBotSettings]);

    const openAddDialog = useCallback(() => {
        setDialogMode('add');
        setDialogEditUuid(null);
        setDialogOpen(true);
    }, []);

    const openEditDialog = useCallback((uuid: string, platformId: string) => {
        setDialogMode('edit');
        setDialogEditUuid(uuid);
        setDialogPlatformId(platformId);
        setDialogOpen(true);
    }, []);

    // ?add=1 deep link opens the create dialog, same convention as the
    // per-platform pages this replaces in the nav.
    useEffect(() => {
        if (searchParams.get('add') === '1' && !dialogOpen) {
            openAddDialog();
            const next = new URLSearchParams(searchParams);
            next.delete('add');
            setSearchParams(next, { replace: true });
        }
    }, [searchParams, setSearchParams, dialogOpen, openAddDialog]);

    const handleBotToggle = useCallback(async (uuid: string, enabled: boolean) => {
        setTogglingBotUuid(uuid);
        try {
            const result = await api.toggleImBotSetting(uuid);
            if (result?.success) {
                showNotification(
                    enabled
                        ? t('remoteControl.notify.botEnabled', { defaultValue: 'Bot enabled' })
                        : t('remoteControl.notify.botDisabled', { defaultValue: 'Bot disabled' }),
                    'success'
                );
                await loadBotSettings();
            } else {
                showNotification(t('remoteControl.notify.toggleFailed', { defaultValue: 'Failed to toggle bot: {{error}}', error: result?.error || 'Unknown error' }), 'error');
            }
        } catch (err) {
            console.error('Failed to toggle bot:', err);
            showNotification(t('remoteControl.notify.toggleFailedGeneric', { defaultValue: 'Failed to toggle bot' }), 'error');
        } finally {
            setTogglingBotUuid(null);
        }
    }, [loadBotSettings, showNotification, t]);

    const handleBotRestart = useCallback(async (uuid: string) => {
        setRestartingBotUuid(uuid);
        try {
            const result = await api.restartImBot(uuid);
            if (result?.success) {
                showNotification(t('remoteControl.notify.botRestarted', { defaultValue: 'Bot restarted' }), 'success');
                await loadBotSettings();
            } else {
                showNotification(t('remoteControl.notify.restartFailed', { defaultValue: 'Failed to restart bot: {{error}}', error: result?.error || 'Unknown error' }), 'error');
            }
        } catch (err) {
            console.error('Failed to restart bot:', err);
            showNotification(t('remoteControl.notify.restartFailedGeneric', { defaultValue: 'Failed to restart bot' }), 'error');
        } finally {
            setRestartingBotUuid(null);
        }
    }, [loadBotSettings, showNotification, t]);

    const handleDeleteBot = useCallback(async (uuid: string) => {
        try {
            const result = await api.deleteImBotSetting(uuid);
            if (result?.success) {
                showNotification(t('remoteControl.notify.botDeleted', { defaultValue: 'Bot deleted successfully' }), 'success');
                await loadBotSettings();
            } else {
                showNotification(t('remoteControl.notify.deleteFailed', { defaultValue: 'Failed to delete bot: {{error}}', error: result?.error }), 'error');
            }
        } catch (err) {
            showNotification(t('remoteControl.notify.deleteFailedGeneric', { defaultValue: 'Failed to delete bot' }), 'error');
        }
    }, [loadBotSettings, showNotification, t]);

    return (
        <PageLayout loading={false}>
            <UnifiedCard
                title={t('bots.overview.title', { defaultValue: 'Bots' })}
                subtitle={t('bots.overview.subtitle', {
                    defaultValue: `${bots.length} bot${bots.length !== 1 ? 's' : ''} connected, across every platform`,
                    count: bots.length,
                })}
                size="full"
                sx={{ mb: 2 }}
                titleHeadingLevel={1}
                rightAction={
                    <Button
                        variant="contained"
                        startIcon={<Add />}
                        onClick={openAddDialog}
                        size="small"
                    >
                        {t('bots.overview.connectBot', { defaultValue: 'Connect a bot' })}
                    </Button>
                }
            >
                {botLoading ? (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                        <CircularProgress />
                    </Box>
                ) : bots.length === 0 ? (
                    <EmptyState
                        title={t('bots.overview.emptyTitle', { defaultValue: 'No bots connected yet' })}
                        description={t('bots.overview.emptyDescription', { defaultValue: 'Connect a bot to drive Claude Code from chat (Remote) or deliver notifications (Notify).' })}
                        primaryAction={{
                            label: t('bots.overview.connectBot', { defaultValue: 'Connect a bot' }),
                            onClick: openAddDialog,
                        }}
                    />
                ) : (
                    bots.map((bot) => (
                        <div key={bot.uuid}>
                            <BotCard
                                bot={bot}
                                onEdit={() => openEditDialog(bot.uuid!, bot.platform!)}
                                onDelete={() => handleDeleteBot(bot.uuid!)}
                                onBotToggle={() => handleBotToggle(bot.uuid!, !bot.enabled)}
                                onRestart={() => handleBotRestart(bot.uuid!)}
                                isToggling={togglingBotUuid === bot.uuid}
                                isRestarting={restartingBotUuid === bot.uuid}
                            />
                        </div>
                    ))
                )}
            </UnifiedCard>
            {/* Shared add/edit dialog for the bot resource. lockPlatform=false:
                this page has no fixed platform, so the user picks one here. */}
            <BotConfigDialog
                open={dialogOpen}
                mode={dialogMode}
                editUuid={dialogEditUuid}
                platformId={dialogPlatformId}
                lockPlatform={false}
                bots={bots}
                onClose={() => setDialogOpen(false)}
                onSaved={loadBotSettings}
                notify={showNotification}
            />
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

export default BotOverviewPage;
