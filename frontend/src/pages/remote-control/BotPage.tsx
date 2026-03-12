import BotAuthForm from '@/components/bot/BotAuthForm';
import BotPlatformSelector from '@/components/bot/BotPlatformSelector';
import { BotCard, useBotModelDialog } from '@/components/bot';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import { PageLayout } from '@/components/PageLayout';
import PlatformGuide from '@/components/remote-control/PlatformGuide';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import type { BotPlatformConfig, BotSettings } from '@/types/bot';
import type { Provider } from '@/types/provider';
import { Add } from '@mui/icons-material';
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
import { useCallback, useEffect, useState } from 'react';

const BotPage = () => {
    // Bot settings state
    const [bots, setBots] = useState<BotSettings[]>([]);

    // Bot platforms config state
    const [botPlatforms, setBotPlatforms] = useState<BotPlatformConfig[]>([]);
    const [currentPlatformConfig, setCurrentPlatformConfig] = useState<BotPlatformConfig | null>(null);

    // Bot form draft state for add/edit dialog
    const [botDialogMode, setBotDialogMode] = useState<'add' | 'edit'>('add');
    const [botEditUuid, setBotEditUuid] = useState<string | null>(null);
    const [botNameDraft, setBotNameDraft] = useState('');
    const [botPlatformDraft, setBotPlatformDraft] = useState('telegram');
    const [botAuthDraft, setBotAuthDraft] = useState<Record<string, string>>({});
    const [botProxyDraft, setBotProxyDraft] = useState('');
    const [botChatIdDraft, setBotChatIdDraft] = useState('');
    const [botAllowlistDraft, setBotAllowlistDraft] = useState('');

    const [botLoading, setBotLoading] = useState(false);
    const [botSaving, setBotSaving] = useState(false);
    const [botPlatformsLoading, setBotPlatformsLoading] = useState(false);
    const [botTokenDialogOpen, setBotTokenDialogOpen] = useState(false);
    const [guideExpanded, setGuideExpanded] = useState<string | false>(false);

    // Toggle loading state
    const [togglingBotUuid, setTogglingBotUuid] = useState<string | null>(null);

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

    // Load bot platforms configuration
    const loadBotPlatforms = useCallback(async () => {
        try {
            setBotPlatformsLoading(true);
            const data = await api.getImBotPlatforms();
            if (data?.success && data?.platforms) {
                setBotPlatforms(data.platforms);
            }
        } catch (err) {
            console.error('Failed to load bot platforms:', err);
        } finally {
            setBotPlatformsLoading(false);
        }
    }, []);

    const loadBotSettings = useCallback(async () => {
        try {
            setBotLoading(true);
            const data = await api.getImBotSettingsList();
            if (data?.success && Array.isArray(data.settings)) {
                setBots(data.settings);
            } else if (data?.success === false) {
                showNotification(data.error || 'Failed to load bot settings', 'error');
            }
        } catch (err) {
            console.error('Failed to load bot settings:', err);
            showNotification('Failed to load bot settings', 'error');
        } finally {
            setBotLoading(false);
        }
    }, []);

    const loadProviders = useCallback(async () => {
        const data = await api.getProviders();
        if (data?.success && data?.data) {
            setProviders(data.data);
        }
    }, []);

    // Update current platform config when platform draft changes
    useEffect(() => {
        if (botPlatformDraft && botPlatforms.length > 0) {
            const config = botPlatforms.find(p => p.platform === botPlatformDraft);
            if (config) {
                setCurrentPlatformConfig(config);
            }
        }
    }, [botPlatformDraft, botPlatforms]);

    // Bot handlers
    const handleOpenBotTokenDialog = useCallback((editUuid?: string) => {
        if (editUuid) {
            // Edit mode
            const bot = bots.find(b => b.uuid === editUuid);
            if (bot) {
                setBotDialogMode('edit');
                setBotEditUuid(editUuid);
                setBotNameDraft(bot.name || '');
                setBotPlatformDraft(bot.platform || 'telegram');
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
            // Add mode
            setBotDialogMode('add');
            setBotEditUuid(null);
            setBotNameDraft('');
            setBotPlatformDraft('telegram');
            setBotAuthDraft({});
            setBotProxyDraft('');
            setBotChatIdDraft('');
            setBotAllowlistDraft('');
            // Set default platform config
            const config = botPlatforms.find(p => p.platform === 'telegram');
            if (config) {
                setCurrentPlatformConfig(config);
            }
        }
        setBotTokenDialogOpen(true);
    }, [bots, botPlatforms]);

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
                showNotification(`Unknown platform: ${botPlatformDraft}`, 'error');
                return;
            }

            // Validate required auth fields
            const missingFields = platformConfig.fields
                .filter(f => f.required && !botAuthDraft[f.key]?.trim())
                .map(f => f.label);

            if (missingFields.length > 0) {
                showNotification(`Missing required fields: ${missingFields.join(', ')}`, 'error');
                return;
            }

            const data = {
                name: botNameDraft.trim(),
                platform: botPlatformDraft,
                auth_type: platformConfig.auth_type,
                auth: botAuthDraft,
                proxy_url: botProxyDraft.trim(),
                chat_id: botChatIdDraft.trim(),
                bash_allowlist: allowlist,
                enabled: true,
            };

            let result;
            if (botDialogMode === 'edit' && botEditUuid) {
                result = await api.updateImBotSetting(botEditUuid, data);
            } else {
                result = await api.createImBotSetting(data);
            }

            if (result?.success === false) {
                showNotification(result.error || 'Failed to save bot settings', 'error');
                return;
            }

            // Reload bots
            await loadBotSettings();

            showNotification(`Bot ${botDialogMode === 'edit' ? 'updated' : 'created'} successfully.`, 'success');
            setBotTokenDialogOpen(false);
        } catch (err) {
            console.error('Failed to save bot settings:', err);
            showNotification('Failed to save bot settings', 'error');
        } finally {
            setBotSaving(false);
        }
    };

    const handleBotToggle = useCallback(async (uuid: string, enabled: boolean) => {
        setTogglingBotUuid(uuid);
        try {
            const result = await api.toggleImBotSetting(uuid);
            if (result?.success) {
                showNotification(enabled ? 'Bot enabled' : 'Bot disabled', 'success');
                await loadBotSettings();
            } else {
                showNotification(`Failed to toggle bot: ${result?.error || 'Unknown error'}`, 'error');
            }
        } catch (err) {
            console.error('Failed to toggle bot:', err);
            showNotification('Failed to toggle bot', 'error');
        } finally {
            setTogglingBotUuid(null);
        }
    }, [loadBotSettings, showNotification]);

    const handleDeleteBot = useCallback(async (uuid: string) => {
        try {
            const result = await api.deleteImBotSetting(uuid);
            if (result?.success) {
                showNotification('Bot deleted successfully', 'success');
                await loadBotSettings();
            } else {
                showNotification(`Failed to delete bot: ${result?.error}`, 'error');
            }
        } catch (err) {
            showNotification('Failed to delete bot', 'error');
        }
    }, [loadBotSettings, showNotification]);

    const handleCWDChange = useCallback(async (botUuid: string, cwd: string) => {
        try {
            const result = await api.updateImbotSetting(botUuid, { default_cwd: cwd });
            if (result?.success) {
                // No notification needed for CWD change - it's a minor change
                await loadBotSettings();
            } else {
                showNotification(result?.error || 'Failed to update working directory', 'error');
            }
        } catch (err) {
            showNotification('Failed to update working directory', 'error');
        }
    }, [loadBotSettings]);

    // SmartGuide dialog using the same pattern as RuleCard
    const handleBotModelUpdate = useCallback(async (uuid: string, provider: string, model: string) => {
        const response = await api.updateImbotSetting(uuid, {
            smartguide_provider: provider,
            smartguide_model: model,
        });

        if (response.success) {
            showNotification('Bot model configuration updated', 'success');
            await loadBotSettings();
        } else {
            showNotification(response.error || 'Failed to update bot configuration', 'error');
            throw new Error(response.error || 'Failed to update bot configuration');
        }
    }, [loadBotSettings, showNotification]);

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
            {/* Preview Notice Card */}
            <UnifiedCard
                title="Preview Version"
                size="full"
                sx={{ mb: 2 }}
            >
                <Alert severity="info" sx={{ mb: 2 }}>
                    <Typography variant="body2">
                        This feature is currently in <strong>preview</strong>. It is designed to work with{' '}
                        <strong>Claude Code</strong> installed on your local machine with your config.
                    </Typography>
                </Alert>
                <Typography variant="body2" color="text.secondary">
                    The <strong>Remote Control</strong> Bot enables you to interact with <strong>Claude
                    Code</strong> through instant messaging platforms
                    like Telegram.
                </Typography>
                <Typography variant="body2" color="text.secondary">
                    Make sure you have <strong>Claude Code CLI</strong> installed and configured before using this
                    feature.
                </Typography>
                <Typography variant="body2" color="text.secondary">
                    <strong>Once you enable a bot, the remote control is started with corresponding IM, and vice
                        versa.</strong>
                </Typography>
            </UnifiedCard>

            <UnifiedCard
                title="Bots"
                subtitle={`${bots.length} bot${bots.length !== 1 ? 's' : ''} configured`}
                size="full"
                sx={{ mb: 2 }}
                rightAction={
                    <Button
                        variant="contained"
                        startIcon={<Add />}
                        onClick={() => handleOpenBotTokenDialog()}
                        size="small"
                    >
                        Add Bot
                    </Button>
                }
            >
                {botLoading ? (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                        <CircularProgress />
                    </Box>
                ) : bots.length === 0 ? (
                    <EmptyStateGuide
                        title="No Bots Configured"
                        description="Configure bots to enable remote-control chat integration."
                        showOAuthButton={false}
                        showHeroIcon={false}
                        primaryButtonLabel="Add Bot"
                        onAddApiKeyClick={() => handleOpenBotTokenDialog()}
                    />
                ) : (
                    bots.map((bot) => (
                        <div key={bot.uuid}>
                            <BotCard
                                bot={bot}
                                providers={providers}
                                onEdit={() => handleOpenBotTokenDialog(bot.uuid)}
                                onDelete={() => handleDeleteBot(bot.uuid!)}
                                onBotToggle={() => handleBotToggle(bot.uuid!, !bot.enabled)}
                                onModelClick={() => handleBotModelSelect(bot.uuid!)}
                                onCWDChange={(cwd) => handleCWDChange(bot.uuid!, cwd)}
                                isToggling={togglingBotUuid === bot.uuid}
                            />
                        </div>
                    ))
                )}
            </UnifiedCard>


            {/* Platform Guide Card */}
            <UnifiedCard
                title="Platform Configuration Guide"
                subtitle="How to configure different IM platforms"
                sx={{ mb: 2 }}
                size="full"
            >
                <PlatformGuide
                    expanded={guideExpanded}
                    onChange={(panel: string) => (_event: React.SyntheticEvent, isExpanded: boolean) => {
                        setGuideExpanded(isExpanded ? panel : false);
                    }}
                />
            </UnifiedCard>

            {/* Bot Add/Edit Dialog */}
            <Modal open={botTokenDialogOpen} onClose={() => setBotTokenDialogOpen(false)}>
                <Stack
                    sx={{
                        position: 'absolute',
                        top: '50%',
                        left: '50%',
                        transform: 'translate(-50%, -50%)',
                        width: 600,
                        maxWidth: '80vw',
                        maxHeight: '80vh',
                        overflowY: 'auto',
                        bgcolor: 'background.paper',
                        boxShadow: 24,
                        p: 4,
                        borderRadius: 2,
                        gap: 2,
                    }}
                >
                    <Typography
                        variant="h6">{botDialogMode === 'edit' ? 'Edit Bot Configuration' : 'Add Bot Configuration'}</Typography>
                    <Stack spacing={2}>
                        <Stack spacing={1}>
                            <Typography variant="body2" color="text.secondary">
                                Platform
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
                                disabled={botSaving}
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
                            />
                        )}

                        <TextField
                            label="Alias"
                            placeholder="My Bot"
                            value={botNameDraft}
                            onChange={(e) => setBotNameDraft(e.target.value)}
                            fullWidth
                            size="small"
                            helperText="Optional: a friendly name for this bot configuration."
                            disabled={botSaving}
                        />

                        <TextField
                            label="Proxy URL"
                            placeholder="http://user:pass@host:port"
                            value={botProxyDraft}
                            onChange={(e) => setBotProxyDraft(e.target.value)}
                            fullWidth
                            size="small"
                            helperText="Optional HTTP/HTTPS proxy for bot API requests."
                            disabled={botSaving}
                        />

                        <TextField
                            label="Chat ID Lock"
                            placeholder="e.g. 123456789"
                            value={botChatIdDraft}
                            onChange={(e) => setBotChatIdDraft(e.target.value)}
                            fullWidth
                            size="small"
                            helperText="Optional: when set, only this chat ID can use the bot."
                            disabled={botSaving}
                        />

                        <TextField
                            label="Bash Allowlist"
                            placeholder="cd\nls\npwd"
                            value={botAllowlistDraft}
                            onChange={(e) => setBotAllowlistDraft(e.target.value)}
                            fullWidth
                            multiline
                            minRows={3}
                            size="small"
                            helperText="Allowlisted /bash subcommands. Default: cd, ls, pwd."
                            disabled={botSaving}
                        />
                    </Stack>

                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <Button
                            onClick={() => setBotTokenDialogOpen(false)}
                            color="inherit"
                            disabled={botSaving}
                        >
                            Cancel
                        </Button>
                        <Button
                            variant="contained"
                            onClick={handleSaveBotToken}
                            disabled={botSaving || botLoading}
                        >
                            {botSaving ? 'Saving...' : 'Save Configuration'}
                        </Button>
                    </Stack>
                </Stack>
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

export default BotPage;
