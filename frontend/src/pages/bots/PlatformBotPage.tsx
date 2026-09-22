import { BotTable, BotConfigDialog, BotAccessDialog } from '@/components/bot';
import EmptyState from '@/components/EmptyState';
import { PageLayout } from '@/components/PageLayout';
import CollapsibleGuide from '@/components/remote-control/CollapsibleGuide';
import UnifiedCard from '@/components/UnifiedCard';
import { api, enrichBotsWithCapabilities } from '@/services/api';
import type { BotSettings } from '@/types/bot';
import { useBotToggle } from '@/hooks/useBotToggle';
import { useNotify } from '@/hooks/useNotify';
import { Add } from '@/components/icons';
import { Box, Button, CircularProgress } from '@mui/material';
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
	const [accessBot,setAccessBot]=useState<BotSettings|null>(null);

    // Add/Edit dialog state — the dialog itself is the shared BotConfigDialog.
    const [dialogOpen, setDialogOpen] = useState(false);
    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>('add');
    const [dialogEditUuid, setDialogEditUuid] = useState<string | null>(null);

    // Starts true (not false) so the very first render doesn't see an empty
    // `bots` array and briefly flash an empty state / auto-expand the guide
    // before the initial fetch has had a chance to resolve.
    const [botLoading, setBotLoading] = useState(true);

    // Toggle loading state
    const [restartingBotUuid, setRestartingBotUuid] = useState<string | null>(null);

    const notify = useNotify();

    // Notification adapter (message first, severity second) — shared with
    // BotConfigDialog's `notify` prop; rendered globally by NotificationProvider.
    const showNotification = useCallback((message: string, severity: 'success' | 'error' | 'info' | 'warning' = 'success') => {
        notify[severity](message);
    }, [notify]);

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
                setBots(await enrichBotsWithCapabilities(data.settings));
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

    // Toggle uses the shared useBotToggle hook (same op across all bot pages).
    const {toggle: handleBotToggle, isToggling} = useBotToggle({onDone: loadBotSettings});

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
        <PageLayout
            loading={false}
            title={t('bots.overview.title', {defaultValue: 'Bots'})}
            subtitle={t('bots.overview.pageSubtitle', {defaultValue: 'Connect and maintain the messaging accounts used by Remote Control and IM Notify.'})}
            rightAction={
                <Button variant="contained" startIcon={<Add/>} onClick={openAddDialog} size="small">
                    {t('bots.overview.connectBot', {defaultValue: 'Connect a bot'})}
                </Button>
            }
        >
            <UnifiedCard
                title={t('bots.overview.platformTitle', { defaultValue: '{{platform}} Bots', platform: platformName })}
                titleHeadingLevel={2}
                subtitle={t('remoteControl.bots.configuredCount', {
                    defaultValue: `${filteredBots.length} bot${filteredBots.length !== 1 ? 's' : ''} configured`,
                    count: filteredBots.length,
                })}
                size="full"
                sx={{ mb: 2 }}
            >
                {botLoading ? (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                        <CircularProgress />
                    </Box>
                ) : filteredBots.length === 0 ? (
                    <EmptyState
                        title={t('remoteControl.bots.emptyTitle', { defaultValue: 'No {{platform}} Bots Configured', platform: platformName })}
                        description={t('remoteControl.bots.emptyDescription', { defaultValue: 'Configure {{platform}} bots to enable remote-control chat integration.', platform: platformName })}
                        primaryAction={{
                            label: t('remoteControl.bots.addPlatformBot', { defaultValue: 'Add {{platform}} Bot', platform: platformName }),
                            onClick: openAddDialog,
                        }}
                    />
                ) : (
                    <BotTable
                        bots={filteredBots}
                        onEdit={(uuid) => openEditDialog(uuid)}
                        onDelete={(uuid) => handleDeleteBot(uuid)}
                        onBotToggle={(uuid, enabled) => handleBotToggle(uuid, enabled)}
                        onRestart={(uuid) => handleBotRestart(uuid)}
                        isToggling={isToggling}
                        isRestarting={(uuid) => restartingBotUuid === uuid}
						onManageAccess={(bot)=>setAccessBot(bot)}
                    />
                )}
            </UnifiedCard>
            {!botLoading && platformGuide && (
                <CollapsibleGuide
                    platformName={platformName}
                    platformGuide={platformGuide}
                    defaultExpanded={filteredBots.length === 0}
                />
            )}
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
            <BotAccessDialog open={Boolean(accessBot)} bot={accessBot} onClose={()=>setAccessBot(null)} onChanged={loadBotSettings}/>
        </PageLayout>
    );
};

export default PlatformBotPage;
