import BotAuthForm from '@/components/bot/BotAuthForm';
import BotPlatformSelector from '@/components/bot/BotPlatformSelector';
import { BotCard, useBotModelDialog } from '@/components/bot';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import { PageLayout } from '@/components/PageLayout';
import CollapsibleGuide from '@/components/remote-control/CollapsibleGuide';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import type { BotPlatformConfig, BotSettings } from '@/types/bot';
import type { Provider } from '@/types/provider';
import { Add } from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Modal,
    Snackbar,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface PlatformBotPageProps {
    platformId: string;
    platformName: string;
    platformGuide?: React.ReactNode;
}

const PlatformBotPage = ({ platformId, platformName, platformGuide }: PlatformBotPageProps) => {
    const { t } = useTranslation();

    // Bot settings state - filtered by platform
    const [bots, setBots] = useState<BotSettings[]>([]);

    // Bot platforms config state
    const [botPlatforms, setBotPlatforms] = useState<BotPlatformConfig[]>([]);
    const [currentPlatformConfig, setCurrentPlatformConfig] = useState<BotPlatformConfig | null>(null);

    // Bot form draft state for add/edit dialog
    const [botDialogMode, setBotDialogMode] = useState<'add' | 'edit'>('add');
    const [botEditUuid, setBotEditUuid] = useState<string | null>(null);
    const [botNameDraft, setBotNameDraft] = useState('');
    const [botPlatformDraft, setBotPlatformDraft] = useState(platformId);
    const [botAuthDraft, setBotAuthDraft] = useState<Record<string, string>>({});
    const [botProxyDraft, setBotProxyDraft] = useState('');
    const [botChatIdDraft, setBotChatIdDraft] = useState('');
    const [botAllowlistDraft, setBotAllowlistDraft] = useState('');

    // Starts true (not false) so the very first render doesn't see an empty
    // `bots` array and briefly flash an empty state / auto-expand the guide
    // before the initial fetch has had a chance to resolve.
    const [botLoading, setBotLoading] = useState(true);
    const [botSaving, setBotSaving] = useState(false);
    const [botPlatformsLoading, setBotPlatformsLoading] = useState(false);
    const [botTokenDialogOpen, setBotTokenDialogOpen] = useState(false);

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

    // Providers for model selection
    const [providers, setProviders] = useState<Provider[]>([]);
    const [selectedBot, setSelectedBot] = useState<BotSettings | null>(null);

    useEffect(() => {
        loadBotPlatforms();
        loadBotSettings();
        loadProviders();
    }, []);

    // Filter bots by platform. useMemo (not a derived-state effect) so this
    // is never one render behind `bots` - a lagging value here previously
    // caused CollapsibleGuide's defaultExpanded to lock in against a stale
    // (still-empty) count.
    const filteredBots = useMemo(
        () => bots.filter(b => b.platform === platformId),
        [bots, platformId]
    );

    // Load bot platforms configuration
    const loadBotPlatforms = useCallback(async () => {
        try {
            setBotPlatformsLoading(true);
            const data = await api.getImBotPlatforms();
            if (data?.success && data?.platforms) {
                setBotPlatforms(data.platforms);
                // Set current platform config
                const config = data.platforms.find(p => p.platform === platformId);
                if (config) {
                    setCurrentPlatformConfig(config);
                }
            }
        } catch (err) {
            console.error('Failed to load bot platforms:', err);
        } finally {
            setBotPlatformsLoading(false);
        }
    }, [platformId]);

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

    const loadProviders = useCallback(async () => {
        const data = await api.getProviders();
        if (data?.success && data?.data) {
            setProviders(data.data);
        }
    }, []);

    // Bot handlers
    const handleOpenBotTokenDialog = useCallback(async (editUuid?: string) => {
        if (editUuid) {
            // Edit mode
            const bot = bots.find(b => b.uuid === editUuid);
            if (bot) {
                setBotDialogMode('edit');
                setBotEditUuid(editUuid);
                setBotNameDraft(bot.name || '');
                setBotPlatformDraft(bot.platform || platformId);
                setBotAuthDraft(bot.auth ? { ...bot.auth } : {});
                setBotProxyDraft(bot.proxy_url || '');
                setBotChatIdDraft(bot.chat_id || '');
                setBotAllowlistDraft((bot.bash_allowlist || []).join('\n'));
                // Set platform config
                const config = botPlatforms.find(p => p.platform === bot.platform);
                if (config) {
                    setCurrentPlatformConfig(config);
                }
            }
        } else {
            // Add mode - pre-select the current platform
            setBotDialogMode('add');
            setBotEditUuid(null);
            setBotNameDraft('');
            setBotPlatformDraft(platformId);
            setBotAuthDraft({});
            setBotProxyDraft('');
            setBotChatIdDraft('');
            setBotAllowlistDraft('');
            // Set default platform config
            const config = botPlatforms.find(p => p.platform === platformId);
            if (config) {
                setCurrentPlatformConfig(config);
                // For QR auth: reuse an existing orphan bot (one that was created by a
                // previous QR binding but whose frontend session failed before cleanup)
                if (config.auth_type === 'qr') {
                    const orphan = bots.find(
                        b => b.platform === platformId && b.auth_type === 'qr' && !b.auth?.token
                    );
                    if (orphan?.uuid) {
                        setBotEditUuid(orphan.uuid);
                        showNotification(t('remoteControl.notify.unboundReuse', { defaultValue: 'Found an unbound bot, reusing it for QR binding' }), 'info');
                    }
                }
            }
        }
        setBotTokenDialogOpen(true);
    }, [bots, botPlatforms, platformId, showNotification, loadBotSettings, t]);

    const handleSaveBotToken = async () => {
        setBotSaving(true);

        try {
            const allowlist = botAllowlistDraft
                .split(/[\n,]+/)
                .map((entry) => entry.trim())
                .filter((entry) => entry.length > 0);

            // Get platform config to validate required fields
            const platformConfig = botPlatforms.find(p => p.platform === botPlatformDraft);
            if (!platformConfig) {
                showNotification(t('remoteControl.notify.unknownPlatform', { defaultValue: 'Unknown platform: {{platform}}', platform: botPlatformDraft }), 'error');
                return;
            }

            // For QR auth type, auth is handled by QR flow, no validation needed
            // For other auth types, validate required fields
            if (platformConfig.auth_type === 'qr') {
                // QR: bot must have been bound before saving
                if (!botEditUuid) {
                    showNotification(t('remoteControl.notify.qrBindRequired', { defaultValue: 'Please complete WeChat QR binding before saving' }), 'error');
                    return;
                }
            } else {
                const missingFields = platformConfig.fields
                    .filter(f => f.required && !botAuthDraft[f.key]?.trim())
                    .map(f => f.label);

                if (missingFields.length > 0) {
                    showNotification(t('remoteControl.notify.missingFields', { defaultValue: 'Missing required fields: {{fields}}', fields: missingFields.join(', ') }), 'error');
                    return;
                }
            }

            const data = {
                name: botNameDraft.trim() || `${botPlatformDraft} Bot`,
                platform: botPlatformDraft,
                auth_type: platformConfig.auth_type,
                auth: botAuthDraft,
                proxy_url: botProxyDraft.trim(),
                chat_id: botChatIdDraft.trim(),
                bash_allowlist: allowlist,
                enabled: true, // Enable the bot after saving
            };

            let result;
            if (botDialogMode === 'edit' && botEditUuid) {
                result = await api.updateImBotSetting(botEditUuid, data);
            } else {
                result = await api.createImBotSetting(data);
            }

            if (result?.success === false) {
                showNotification(result.error || t('remoteControl.notify.saveFailed', { defaultValue: 'Failed to save bot settings' }), 'error');
                return;
            }

            // Reload bots
            await loadBotSettings();

            showNotification(
                botDialogMode === 'edit'
                    ? t('remoteControl.notify.botUpdated', { defaultValue: 'Bot updated successfully.' })
                    : t('remoteControl.notify.botCreated', { defaultValue: 'Bot created successfully.' }),
                'success'
            );
            setBotTokenDialogOpen(false);
        } catch (err) {
            console.error('Failed to save bot settings:', err);
            showNotification(t('remoteControl.notify.saveFailed', { defaultValue: 'Failed to save bot settings' }), 'error');
        } finally {
            setBotSaving(false);
        }
    };

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

    const handleCWDChange = useCallback(async (botUuid: string, cwd: string) => {
        try {
            const result = await api.updateImBotSetting(botUuid, { default_cwd: cwd });
            if (result?.success) {
                // No notification needed for CWD change - it's a minor change
                await loadBotSettings();
            } else {
                showNotification(result?.error || t('remoteControl.notify.cwdUpdateFailed', { defaultValue: 'Failed to update working directory' }), 'error');
            }
        } catch (err) {
            showNotification(t('remoteControl.notify.cwdUpdateFailed', { defaultValue: 'Failed to update working directory' }), 'error');
        }
    }, [loadBotSettings, showNotification, t]);

    // SmartGuide dialog using the same pattern as RuleCard
    const handleBotModelUpdate = useCallback(async (uuid: string, provider: string, model: string) => {
        const response = await api.updateImBotSetting(uuid, {
            smartguide_provider: provider,
            smartguide_model: model,
        });

        if (response.success) {
            showNotification(t('remoteControl.notify.modelUpdated', { defaultValue: 'Bot model configuration updated' }), 'success');
            await loadBotSettings();
        } else {
            const message = response.error || t('remoteControl.notify.modelUpdateFailed', { defaultValue: 'Failed to update bot configuration' });
            showNotification(message, 'error');
            throw new Error(message);
        }
    }, [loadBotSettings, showNotification, t]);

    const {
        openDialog: openBotModelDialog,
        closeDialog: closeBotModelDialog,
        BotModelDialog,
        isOpen: BotModelDialogOpen,
    } = useBotModelDialog({
        bot: selectedBot,
        providers,
        onUpdate: handleBotModelUpdate,
        onClose: () => setSelectedBot(null),
    });

    const handleBotModelSelect = useCallback((botUuid: string) => {
        const bot = bots.find(b => b.uuid === botUuid);
        if (bot) {
            setSelectedBot(bot);
            openBotModelDialog();
        }
    }, [bots, openBotModelDialog]);

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
                        onClick={() => handleOpenBotTokenDialog()}
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
                        onAddApiKeyClick={() => handleOpenBotTokenDialog()}
                    />
                ) : (
                    filteredBots.map((bot) => (
                        <div key={bot.uuid}>
                            <BotCard
                                bot={bot}
                                providers={providers}
                                onEdit={() => handleOpenBotTokenDialog(bot.uuid)}
                                onDelete={() => handleDeleteBot(bot.uuid!)}
                                onBotToggle={() => handleBotToggle(bot.uuid!, !bot.enabled)}
                                onRestart={() => handleBotRestart(bot.uuid!)}
                                onModelClick={() => handleBotModelSelect(bot.uuid!)}
                                onCWDChange={(cwd) => handleCWDChange(bot.uuid!, cwd)}
                                isToggling={togglingBotUuid === bot.uuid}
                                isRestarting={restartingBotUuid === bot.uuid}
                            />
                        </div>
                    ))
                )}
            </UnifiedCard>

            {/* Bot Add/Edit Dialog */}
            <Modal open={botTokenDialogOpen} onClose={() => setBotTokenDialogOpen(false)}>
                <Box
                    sx={{
                        position: 'absolute',
                        top: '50%',
                        left: '50%',
                        transform: 'translate(-50%, -50%)',
                        width: 600,
                        maxWidth: '80vw',
                        maxHeight: '80vh',
                        bgcolor: 'background.paper',
                        boxShadow: 24,
                        borderRadius: 2,
                        display: 'flex',
                        flexDirection: 'column',
                        overflow: 'hidden',
                    }}
                >
                <Stack
                    sx={{
                        overflowY: 'auto',
                        p: 4,
                        gap: 2,
                        flex: 1,
                    }}
                >
                    <Typography variant="h6">
                        {botDialogMode === 'edit'
                            ? t('remoteControl.dialog.editTitle', { defaultValue: 'Edit Bot Configuration' })
                            : t('remoteControl.dialog.addTitle', { defaultValue: 'Add Bot Configuration' })}
                    </Typography>
                    <Stack spacing={2}>
                        <Stack spacing={1}>
                            <Typography variant="body2" color="text.secondary">
                                {t('remoteControl.dialog.platform', { defaultValue: 'Platform' })}
                            </Typography>
                            <BotPlatformSelector
                                value={botPlatformDraft}
                                onChange={(platform) => {
                                    setBotPlatformDraft(platform);
                                    // Clear auth draft when platform changes
                                    setBotAuthDraft({});
                                    // Update current platform config
                                    const config = botPlatforms.find(p => p.platform === platform);
                                    if (config) {
                                        setCurrentPlatformConfig(config);
                                    }
                                }}
                                platforms={botPlatforms}
                                loading={botPlatformsLoading}
                                disabled={botSaving || botDialogMode === 'add'}
                            />
                        </Stack>

                        {currentPlatformConfig && (
                            <BotAuthForm
                                platform={botPlatformDraft}
                                authType={currentPlatformConfig.auth_type}
                                fields={currentPlatformConfig.fields}
                                authData={botAuthDraft}
                                onChange={(key, value) => setBotAuthDraft(prev => ({ ...prev, [key]: value }))}
                                disabled={botSaving}
                                botUUID={botEditUuid ?? undefined}
                                botName={botNameDraft || `${botPlatformDraft} Bot`}
                                onBindingComplete={async (realUUID) => {
                                    // After QR scan: set the real UUID and reload credentials
                                    setBotEditUuid(realUUID);
                                    setBotDialogMode('edit');
                                    try {
                                        const data = await api.getImBotSetting(realUUID);
                                        if (data?.settings?.auth) {
                                            setBotAuthDraft(data.settings.auth);
                                        }
                                    } catch (err) {
                                        console.error('Failed to reload bot after binding:', err);
                                    }
                                    await loadBotSettings();
                                }}
                            />
                        )}

                        <TextField
                            label={t('remoteControl.dialog.alias', { defaultValue: 'Alias' })}
                            placeholder="My Bot"
                            value={botNameDraft}
                            onChange={(e) => setBotNameDraft(e.target.value)}
                            fullWidth
                            size="small"
                            helperText={t('remoteControl.dialog.aliasHelper', { defaultValue: 'Optional: a friendly name for this bot configuration.' })}
                            disabled={botSaving}
                        />

                        <TextField
                            label={t('remoteControl.dialog.proxyUrl', { defaultValue: 'Proxy URL' })}
                            placeholder="http://user:pass@host:port"
                            value={botProxyDraft}
                            onChange={(e) => setBotProxyDraft(e.target.value)}
                            fullWidth
                            size="small"
                            helperText={t('remoteControl.dialog.proxyUrlHelper', { defaultValue: 'Optional HTTP/HTTPS proxy for bot API requests.' })}
                            disabled={botSaving}
                        />

                        <TextField
                            label={t('remoteControl.dialog.chatIdLock', { defaultValue: 'Chat ID Lock' })}
                            placeholder="e.g. 123456789"
                            value={botChatIdDraft}
                            onChange={(e) => setBotChatIdDraft(e.target.value)}
                            fullWidth
                            size="small"
                            helperText={t('remoteControl.dialog.chatIdLockHelper', { defaultValue: 'Optional: when set, only this chat ID can use the bot.' })}
                            disabled={botSaving}
                        />

                        <TextField
                            label={t('remoteControl.dialog.bashAllowlist', { defaultValue: 'Bash Allowlist' })}
                            placeholder="cd\nls\npwd"
                            value={botAllowlistDraft}
                            onChange={(e) => setBotAllowlistDraft(e.target.value)}
                            fullWidth
                            multiline
                            minRows={3}
                            size="small"
                            helperText={t('remoteControl.dialog.bashAllowlistHelper', { defaultValue: 'Allowlisted /bash subcommands. Default: cd, ls, pwd.' })}
                            disabled={botSaving}
                        />
                    </Stack>

                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <Button
                            onClick={() => setBotTokenDialogOpen(false)}
                            color="inherit"
                            disabled={botSaving}
                        >
                            {t('remoteControl.dialog.cancel', { defaultValue: 'Cancel' })}
                        </Button>
                        <Button
                            variant="contained"
                            onClick={handleSaveBotToken}
                            disabled={botSaving || botLoading}
                        >
                            {botSaving
                                ? t('remoteControl.dialog.saving', { defaultValue: 'Saving...' })
                                : t('remoteControl.dialog.save', { defaultValue: 'Save Configuration' })}
                        </Button>
                    </Stack>
                </Stack>
                </Box>
            </Modal>

            {/* SmartGuide Selector Dialog */}
            <BotModelDialog open={BotModelDialogOpen} />

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
