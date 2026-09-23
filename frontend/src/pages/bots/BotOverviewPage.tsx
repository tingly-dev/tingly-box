import { BotTable, BotConfigDialog, PlatformPicker, BotAccessDialog } from '@/components/bot';
import EmptyState from '@/components/EmptyState';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import CollapsibleGuide from '@/components/remote-control/CollapsibleGuide';
import { BOT_PLATFORM_IDS, PLATFORM_BRAND_ICONS, platformDisplayName, usePlatformGuide } from '@/constants/platformGuides';
import { countBotsByPlatform } from '@/types/bot';
import type { BotSettings } from '@/types/bot';
import { useBotList } from '@/hooks/useBotList';
import { useBotToggle } from '@/hooks/useBotToggle';
import { useNotify } from '@/hooks/useNotify';
import { Add, ListAlt } from '@/components/icons';
import { Box, Button, CircularProgress } from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

// BotOverviewPage is the front door for the Bots section: every connected
// bot, across every platform, in one list. It's the only place a bot's
// credential (token / OAuth / QR session) gets typed, rotated, or deleted —
// Remote Control and IM Notify only mount purposes onto bots that already
// exist here. "All" is the default view; picking a platform (picker tile,
// ?platform=) both filters the list AND brings back that platform's setup
// guide, since a guide only makes sense once you've committed to one
// platform.
const BotOverviewPage = () => {
    const { t } = useTranslation();
    const [searchParams, setSearchParams] = useSearchParams();
    const selectedPlatform = searchParams.get('platform') || 'all';
    const guideConfig = usePlatformGuide(selectedPlatform === 'all' ? '' : selectedPlatform);

    const [dialogOpen, setDialogOpen] = useState(false);
    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>('add');
    const [dialogEditUuid, setDialogEditUuid] = useState<string | null>(null);
    const [dialogPlatformId, setDialogPlatformId] = useState('telegram');
    const [accessBot, setAccessBot] = useState<BotSettings | null>(null);

    const notify = useNotify();

    // Notification adapter (message first, severity second) — shared with
    // BotConfigDialog's `notify` prop; rendered globally by NotificationProvider.
    const showNotification = useCallback((message: string, severity: 'success' | 'error' | 'info' | 'warning' = 'success') => {
        notify[severity](message);
    }, [notify]);

    // Bot list + restart/delete via the shared useBotList hook (same ops
    // across all bot pages). `spinnerOnRefresh` keeps this page's behavior of
    // re-showing the loading spinner on every reload, not just the first.
    const {
        bots,
        loading: botLoading,
        load: loadBotSettings,
        restart: handleBotRestart,
        isRestarting,
        remove: handleDeleteBot,
    } = useBotList({notify: showNotification, spinnerOnRefresh: true});

    useEffect(() => {
        loadBotSettings();
    }, [loadBotSettings]);

    // Per-platform active/total, plus the 'all' aggregate — drives both the
    // picker tile subtitles and the card header count.
    const platformCounts = useMemo(() => countBotsByPlatform(bots), [bots]);

    const countLabel = (active: number, total: number): string | undefined =>
        total > 0 ? t('bots.activeCount', { defaultValue: 'active {{active}} / {{total}}', active, total }) : undefined;

    const pickerItems = useMemo(() => [
        {
            id: 'all',
            label: t('bots.overview.allPlatforms', { defaultValue: 'All' }),
            icon: <ListAlt sx={{fontSize: 20, color: 'text.disabled'}}/>,
            activeIcon: <ListAlt sx={{fontSize: 20, color: 'primary.main'}}/>,
            subtitle: countLabel(bots.filter(b => b.enabled).length, bots.length),
        },
        ...BOT_PLATFORM_IDS.map((id) => {
            const BrandIcon = PLATFORM_BRAND_ICONS[id];
            const c = platformCounts[id];
            return {
                id,
                label: platformDisplayName(id, t),
                icon: <BrandIcon size={20} grayscale/>,
                activeIcon: <BrandIcon size={20} grayscale={false}/>,
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

    // Toggle uses the shared useBotToggle hook (same op across all bot pages).
    const {toggle: handleBotToggle, isToggling} = useBotToggle({onDone: loadBotSettings});

    const platformName = selectedPlatform === 'all' ? '' : platformDisplayName(selectedPlatform, t);

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
            <PlatformPicker items={pickerItems} value={selectedPlatform} onChange={selectPlatform} />
            <UnifiedCard
                title={selectedPlatform === 'all'
                    ? t('bots.overview.allConnections', { defaultValue: 'All connections' })
                    : t('bots.overview.platformTitle', { defaultValue: '{{platform}} Bots', platform: platformName })}
                subtitle={t('bots.overview.subtitle', {
                    defaultValue: `${filteredBots.length} bot${filteredBots.length !== 1 ? 's' : ''} connected`,
                    count: filteredBots.length,
                })}
                size="full"
                sx={{ mb: 2 }}
                titleHeadingLevel={2}
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
                    <BotTable
                        bots={filteredBots}
                        onEdit={(uuid, platformId) => openEditDialog(uuid, platformId!)}
                        onDelete={(uuid) => handleDeleteBot(uuid)}
                        onBotToggle={(uuid, enabled) => handleBotToggle(uuid, enabled)}
                        onRestart={(uuid) => handleBotRestart(uuid)}
                        isToggling={isToggling}
                        isRestarting={isRestarting}
                        onManageAccess={setAccessBot}
                    />
                )}
            </UnifiedCard>
            {!botLoading && selectedPlatform !== 'all' && guideConfig?.guide && (
                <CollapsibleGuide
                    platformName={platformName}
                    platformGuide={guideConfig.guide}
                    defaultExpanded={filteredBots.length === 0}
                />
            )}
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
            <BotAccessDialog
                open={Boolean(accessBot)}
                bot={accessBot}
                onClose={() => setAccessBot(null)}
                onChanged={loadBotSettings}
            />
        </PageLayout>
    );
};

export default BotOverviewPage;
