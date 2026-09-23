import { BotTable, BotConfigDialog, BotAccessDialog } from '@/components/bot';
import EmptyState from '@/components/EmptyState';
import { PageLayout } from '@/components/PageLayout';
import CollapsibleGuide from '@/components/remote-control/CollapsibleGuide';
import UnifiedCard from '@/components/UnifiedCard';
import type { BotSettings } from '@/types/bot';
import { useBotList } from '@/hooks/useBotList';
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

	const [accessBot,setAccessBot]=useState<BotSettings|null>(null);

    // Add/Edit dialog state — the dialog itself is the shared BotConfigDialog.
    const [dialogOpen, setDialogOpen] = useState(false);
    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>('add');
    const [dialogEditUuid, setDialogEditUuid] = useState<string | null>(null);

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

    // Filter bots by platform. useMemo (not a derived-state effect) so this
    // is never one render behind `bots` - a lagging value here previously
    // caused CollapsibleGuide's defaultExpanded to lock in against a stale
    // (still-empty) count.
    const filteredBots = useMemo(
        () => bots.filter(b => b.platform === platformId),
        [bots, platformId]
    );

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
                        isRestarting={isRestarting}
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
