import { BotCard, BotConfigDialog, PlatformSideNav } from '@/components/bot';
import EmptyState from '@/components/EmptyState';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import CollapsibleGuide from '@/components/remote-control/CollapsibleGuide';
import { Telegram, Feishu, Lark, DingTalk, Weixin, WeCom, QQ, Discord, Slack } from '@/components/BrandIcons';
import { BOT_PLATFORM_IDS, platformDisplayName, usePlatformGuide } from '@/constants/platformGuides';
import { api } from '@/services/api';
import type { BotSettings } from '@/types/bot';
import { Add, ListAlt } from '@/components/icons';
import { Alert, Box, Button, CircularProgress, Snackbar } from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

// Plain lookup (not the usePlatformGuide hook) so it's safe to call from
// inside a .map() when building the side nav's item list.
const PLATFORM_BRAND_ICONS: Record<string, React.ComponentType<{ size?: number }>> = {
    telegram: Telegram,
    feishu: Feishu,
    lark: Lark,
    dingtalk: DingTalk,
    weixin: Weixin,
    wecom: WeCom,
    qq: QQ,
    discord: Discord,
    slack: Slack,
};

// BotOverviewPage is the front door for the Bots section: every connected
// bot, across every platform, in one list. It's the only place a bot's
// credential (token / OAuth / QR session) gets typed, rotated, or deleted —
// Remote and Notify only mount purposes onto bots that already exist here.
// "All" is the default view; picking a platform (left nav, ?platform=) both
// filters the list AND brings back that platform's setup guide, since a
// guide only makes sense once you've committed to one platform.
const BotOverviewPage = () => {
    const { t } = useTranslation();
    const [searchParams, setSearchParams] = useSearchParams();
    const selectedPlatform = searchParams.get('platform') || 'all';
    const guideConfig = usePlatformGuide(selectedPlatform === 'all' ? '' : selectedPlatform);

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

    // Per-platform active/total, plus the 'all' aggregate — drives both the
    // side nav subtitles and the card header count.
    const platformCounts = useMemo(() => {
        const counts: Record<string, { active: number; total: number }> = {};
        for (const bot of bots) {
            if (!bot.platform) continue;
            const slot = counts[bot.platform] ?? (counts[bot.platform] = { active: 0, total: 0 });
            slot.total++;
            if (bot.enabled) slot.active++;
        }
        return counts;
    }, [bots]);

    const countLabel = (active: number, total: number): string | undefined =>
        total > 0 ? `active ${active} / ${total}` : undefined;

    const sideNavItems = useMemo(() => [
        {
            id: 'all',
            label: t('bots.overview.allPlatforms', { defaultValue: 'All' }),
            icon: <ListAlt sx={{ fontSize: 20 }} />,
            subtitle: countLabel(bots.filter(b => b.enabled).length, bots.length),
        },
        ...BOT_PLATFORM_IDS.map((id) => {
            const BrandIcon = PLATFORM_BRAND_ICONS[id];
            const c = platformCounts[id];
            return {
                id,
                label: platformDisplayName(id, t),
                icon: <BrandIcon size={20} />,
                subtitle: c ? countLabel(c.active, c.total) : undefined,
            };
        }),
    ], [t, bots, platformCounts]);

    const selectPlatform = useCallback((id: string) => {
        const next = new URLSearchParams(searchParams);
        if (id === 'all') next.delete('platform');
        else next.set('platform', id);
        setSearchParams(next);
    }, [searchParams, setSearchParams]);

    const filteredBots = useMemo(
        () => selectedPlatform === 'all' ? bots : bots.filter(b => b.platform === selectedPlatform),
        [bots, selectedPlatform]
    );

    const openAddDialog = useCallback(() => {
        setDialogMode('add');
        setDialogEditUuid(null);
        if (selectedPlatform !== 'all') setDialogPlatformId(selectedPlatform);
        setDialogOpen(true);
    }, [selectedPlatform]);

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

    const platformName = selectedPlatform === 'all' ? '' : platformDisplayName(selectedPlatform, t);

    return (
        <PageLayout loading={false}>
            <Box sx={{ display: 'flex', gap: 2, alignItems: 'flex-start', flexDirection: { xs: 'column', md: 'row' } }}>
                <PlatformSideNav items={sideNavItems} value={selectedPlatform} onChange={selectPlatform} />
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    {!botLoading && selectedPlatform !== 'all' && guideConfig?.guide && (
                        <CollapsibleGuide
                            platformName={platformName}
                            platformGuide={guideConfig.guide}
                            defaultExpanded={filteredBots.length === 0}
                        />
                    )}
                    <UnifiedCard
                        title={selectedPlatform === 'all'
                            ? t('bots.overview.title', { defaultValue: 'Bots' })
                            : t('bots.overview.platformTitle', { defaultValue: '{{platform}} Bots', platform: platformName })}
                        subtitle={t('bots.overview.subtitle', {
                            defaultValue: `${filteredBots.length} bot${filteredBots.length !== 1 ? 's' : ''} connected`,
                            count: filteredBots.length,
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
                        ) : filteredBots.length === 0 ? (
                            <EmptyState
                                title={t('bots.overview.emptyTitle', { defaultValue: 'No bots connected yet' })}
                                description={t('bots.overview.emptyDescription', { defaultValue: 'Connect a bot to drive Claude Code from chat (Remote) or deliver notifications (Notify).' })}
                                primaryAction={{
                                    label: t('bots.overview.connectBot', { defaultValue: 'Connect a bot' }),
                                    onClick: openAddDialog,
                                }}
                            />
                        ) : (
                            filteredBots.map((bot) => (
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
                </Box>
            </Box>
            {/* Shared add/edit dialog for the bot resource. Locked to the
                selected platform when browsing one; unlocked under "All" so
                the user picks a platform in the dialog itself. */}
            <BotConfigDialog
                open={dialogOpen}
                mode={dialogMode}
                editUuid={dialogEditUuid}
                platformId={dialogPlatformId}
                lockPlatform={selectedPlatform !== 'all'}
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
