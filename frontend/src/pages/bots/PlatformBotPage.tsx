import { BotCard, BotConfigDialog } from '@/components/bot';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import { PageLayout } from '@/components/PageLayout';
import CollapsibleGuide from '@/components/remote-control/CollapsibleGuide';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import type { BotSettings } from '@/types/bot';
import { Add } from '@/components/icons';
import { Alert, Box, Button, CircularProgress, Snackbar } from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

interface PlatformBotPageProps {
    platformId: string;
    platformName: string;
    platformGuide?: React.ReactNode;
}

const PlatformBotPage = ({ platformId, platformName, platformGuide }: PlatformBotPageProps) => {
    const { t } = useTranslation();
    const [searchParams, setSearchParams] = useSearchParams();

    // Bot settings state - filtered by platform
    const [bots, setBots] = useState<BotSettings[]>([]);

    // Add/Edit dialog state — the dialog itself is the shared BotConfigDialog.
    const [dialogOpen, setDialogOpen] = useState(false);
    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>('add');
    const [dialogEditUuid, setDialogEditUuid] = useState<string | null>(null);

    // Starts true (not false) so the very first render doesn't see an empty
    // `bots` array and briefly flash an empty state / auto-expand the guide
    // before the initial fetch has had a chance to resolve.
    const [botLoading, setBotLoading] = useState(true);

    // Toggle loading state
    const [togglingBotUuid, setTogglingBotUuid] = useState<string | null>(null);
    const [restartingBotUuid, setRestartingBotUuid] = useState<string | null>(null);

    // Snackbar notification state
    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error' | 'info' | 'warning';
    }>({ open: false, message: '', severity: 'success' });

    // Notification helper - errors require manual dismissal, others auto-hide
    const showNotification = useCallback((message: string, severity: 'success' | 'error' | 'info' | 'warning' = 'success') => {
        setSnackbar({ open: true, message, severity });
    }, []);

    // Filter bots by platform. useMemo (not a derived-state effect) so this
    // is never one render behind `bots` - a lagging value here previously
    // caused CollapsibleGuide's defaultExpanded to lock in against a stale
    // (still-empty) count.
    const filteredBots = useMemo(
        () => bots.filter(b => b.platform === platformId),
        [bots, platformId]
    );

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

    const openEditDialog = useCallback((uuid: string) => {
        setDialogMode('edit');
        setDialogEditUuid(uuid);
        setDialogOpen(true);
    }, []);

    // ?add=1 (deep link) opens the create dialog, then strips the param so
    // refresh/back doesn't re-open it.
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
            {/* Platform-specific Guide with Preview Notice. Gated on
                !botLoading so CollapsibleGuide only mounts once the real bot
                count is known - its default-expanded state is fixed at
                mount and would otherwise lock in based on the empty initial
                array. */}
            {!botLoading && platformGuide && (
                <CollapsibleGuide
                    platformName={platformName}
                    platformGuide={platformGuide}
                    defaultExpanded={filteredBots.length === 0}
                />
            )}
            <UnifiedCard
                title={t('remoteControl.bots.title', { defaultValue: '{{platform}} Bots', platform: platformName })}
                subtitle={t('remoteControl.bots.configuredCount', {
                    defaultValue: `${filteredBots.length} bot${filteredBots.length !== 1 ? 's' : ''} configured`,
                    count: filteredBots.length,
                })}
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
                {botLoading ? (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                        <CircularProgress />
                    </Box>
                ) : filteredBots.length === 0 ? (
                    <EmptyStateGuide
                        title={t('remoteControl.bots.emptyTitle', { defaultValue: 'No {{platform}} Bots Configured', platform: platformName })}
                        description={t('remoteControl.bots.emptyDescription', { defaultValue: 'Configure {{platform}} bots to enable remote-control chat integration.', platform: platformName })}
                        showHeroIcon={false}
                        primaryButtonLabel={t('remoteControl.bots.addPlatformBot', { defaultValue: 'Add {{platform}} Bot', platform: platformName })}
                        onAddApiKeyClick={openAddDialog}
                    />
                ) : (
                    filteredBots.map((bot) => (
                        <div key={bot.uuid}>
                            <BotCard
                                bot={bot}
                                onEdit={() => openEditDialog(bot.uuid!)}
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
            {/* Shared add/edit dialog for the bot resource */}
            <BotConfigDialog
                open={dialogOpen}
                mode={dialogMode}
                editUuid={dialogEditUuid}
                platformId={platformId}
                bots={bots}
                onClose={() => setDialogOpen(false)}
                onSaved={loadBotSettings}
                notify={showNotification}
            />
            {/* Snackbar for notifications */}
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

export default PlatformBotPage;
