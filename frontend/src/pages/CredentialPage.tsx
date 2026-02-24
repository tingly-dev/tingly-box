import { Add, Edit, ExpandMore, SmartToy, VpnKey } from '@mui/icons-material';
import {
    Alert,
    Button,
    Chip,
    CircularProgress,
    Menu,
    MenuItem,
    Modal,
    Snackbar,
    Stack,
    Tab,
    Tabs,
    TextField,
    Typography,
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { PageLayout } from '@/components/PageLayout';
import ProviderFormDialog from '@/components/ProviderFormDialog.tsx';
import { type EnhancedProviderFormData } from '@/components/ProviderFormDialog.tsx';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '../services/api';
import ApiKeyTable from '@/components/ApiKeyTable.tsx';
import OAuthTable from '@/components/OAuthTable.tsx';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import OAuthDialog from '@/components/OAuthDialog.tsx';
import OAuthDetailDialog from '@/components/OAuthDetailDialog.tsx';
import BotPlatformSelector from '@/components/bot/BotPlatformSelector';
import BotAuthForm from '@/components/bot/BotAuthForm';
import BotTable from '@/components/bot/BotTable';
import { BotPlatformConfig, BotSettings } from '@/types/bot';

type ProviderFormData = EnhancedProviderFormData;

interface OAuthEditFormData {
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic';
    enabled: boolean;
    proxyUrl?: string;
}

type CredentialTab = 'api-keys' | 'oauth' | 'bot-token';

const CredentialPage = () => {
    const navigate = useNavigate();
    const [searchParams, setSearchParams] = useSearchParams();
    const [providers, setProviders] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error';
    }>({ open: false, message: '', severity: 'success' });

    // Tab state
    const [activeTab, setActiveTab] = useState<CredentialTab>('api-keys');

    // Add button menu state
    const [addMenuAnchorEl, setAddMenuAnchorEl] = useState<HTMLElement | null>(null);

    // API Key Dialog state
    const [apiKeyDialogOpen, setApiKeyDialogOpen] = useState(false);
    const [apiKeyDialogMode, setApiKeyDialogMode] = useState<'add' | 'edit'>('add');
    const [providerFormData, setProviderFormData] = useState<ProviderFormData>({
        uuid: undefined,
        name: '',
        apiBase: '',
        apiStyle: undefined,
        token: '',
        enabled: true,
    });

    // OAuth Dialog state
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);
    const [oauthDetailProvider, setOAuthDetailProvider] = useState<any | null>(null);
    const [oauthDetailDialogOpen, setOAuthDetailDialogOpen] = useState(false);

    // Bot settings state - V2 multi-bot support
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
    const [botNotice, setBotNotice] = useState<string | null>(null);
    const [botError, setBotError] = useState<string | null>(null);
    const [botTokenDialogOpen, setBotTokenDialogOpen] = useState(false);

    // URL param handling for auto-opening dialogs
    useEffect(() => {
        const dialog = searchParams.get('dialog');
        const style = searchParams.get('style') as 'openai' | 'anthropic' | null;
        const tab = searchParams.get('tab');

        // Set tab from URL
        if (tab === 'oauth') {
            setActiveTab('oauth');
        } else if (tab === 'bot-token') {
            setActiveTab('bot-token');
        } else if (tab === 'api-keys' || tab === null) {
            setActiveTab('api-keys');
        }

        // Handle dialog auto-open from URL
        if (dialog === 'add') {
            // Clear URL params
            setSearchParams({});

            if (style === 'oauth' || tab === 'oauth') {
                // Open OAuth dialog
                setOAuthDialogOpen(true);
            } else {
                // Open API Key dialog
                const apiStyle = style === 'openai' || style === 'anthropic' ? style : undefined;
                setApiKeyDialogMode('add');
                setProviderFormData({
                    uuid: undefined,
                    name: '',
                    apiBase: '',
                    apiStyle: apiStyle,
                    token: '',
                    enabled: true,
                    noKeyRequired: false,
                    proxyUrl: '',
                } as any);
                setApiKeyDialogOpen(true);
            }
        }
    }, [searchParams, setSearchParams]);

    useEffect(() => {
        loadProviders();
        loadBotPlatforms();
    }, []);

    // Load bot platforms configuration
    const loadBotPlatforms = async () => {
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
    };

    useEffect(() => {
        const loadBotSettings = async () => {
            try {
                setBotLoading(true);
                const data = await api.getImBotSettingsList();
                if (data?.success && Array.isArray(data.settings)) {
                    setBots(data.settings);
                } else if (data?.success === false) {
                    setBotError(data.error || 'Failed to load bot settings');
                }
            } catch (err) {
                console.error('Failed to load bot settings:', err);
                setBotError('Failed to load bot settings');
            } finally {
                setBotLoading(false);
            }
        };

        loadBotSettings();
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

    // Helper to reload bots
    const reloadBots = async () => {
        try {
            const data = await api.getImBotSettingsList();
            if (data?.success && Array.isArray(data.settings)) {
                setBots(data.settings);
            }
        } catch (err) {
            console.error('Failed to reload bot settings:', err);
        }
    };

    const showNotification = (message: string, severity: 'success' | 'error') => {
        setSnackbar({ open: true, message, severity });
    };

    // Tab handlers
    const handleTabChange = (_event: React.SyntheticEvent, newValue: CredentialTab) => {
        setActiveTab(newValue);
        // Update URL for deep linking
        const params = new URLSearchParams(searchParams);
        if (newValue === 'api-keys') {
            params.delete('tab');
        } else {
            params.set('tab', newValue);
        }
        navigate({ search: params.toString() }, { replace: true });
    };

    // Add menu handlers
    const handleAddClick = (event: React.MouseEvent<HTMLElement>) => {
        setAddMenuAnchorEl(event.currentTarget);
    };

    const handleAddMenuClose = () => {
        setAddMenuAnchorEl(null);
    };

    const handleAddApiKey = () => {
        handleAddMenuClose();
        setApiKeyDialogMode('add');
        setProviderFormData({
            uuid: undefined,
            name: '',
            apiBase: '',
            apiStyle: undefined,
            token: '',
            enabled: true,
            noKeyRequired: false,
            proxyUrl: '',
        } as any);
        setApiKeyDialogOpen(true);
    };

    const handleAddOAuth = () => {
        handleAddMenuClose();
        setOAuthDialogOpen(true);
    };

    const handleAddBot = () => {
        handleAddMenuClose();
        handleOpenBotTokenDialog();
    };

    // Bot handlers
    const handleEditBot = (uuid: string) => {
        handleOpenBotTokenDialog(uuid);
    };

    const handleToggleBot = async (uuid: string) => {
        try {
            const result = await api.toggleImBotSetting(uuid);
            if (result?.success) {
                showNotification(result.enabled ? 'Bot enabled' : 'Bot disabled', 'success');
                await reloadBots();
            } else {
                showNotification(`Failed to toggle bot: ${result?.error}`, 'error');
            }
        } catch (err) {
            showNotification('Failed to toggle bot', 'error');
        }
    };

    const handleDeleteBot = async (uuid: string) => {
        try {
            const result = await api.deleteImBotSetting(uuid);
            if (result?.success) {
                showNotification('Bot deleted successfully', 'success');
                await reloadBots();
            } else {
                showNotification(`Failed to delete bot: ${result?.error}`, 'error');
            }
        } catch (err) {
            showNotification('Failed to delete bot', 'error');
        }
    };

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        } else {
            showNotification(`Failed to load providers: ${result.error}`, 'error');
        }
        setLoading(false);
    };

    const handleOpenBotTokenDialog = (editUuid?: string) => {
        setBotNotice(null);
        setBotError(null);

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
    };

    const handleSaveBotToken = async () => {
        setBotSaving(true);
        setBotNotice(null);
        setBotError(null);

        try {
            const allowlist = botAllowlistDraft
                .split(/[\n,]+/)
                .map((entry) => entry.trim())
                .filter((entry) => entry.length > 0);

            // Get platform config to validate required fields
            const platformConfig = botPlatforms.find(p => p.platform === botPlatformDraft);
            if (!platformConfig) {
                setBotError(`Unknown platform: ${botPlatformDraft}`);
                return;
            }

            // Validate required auth fields
            const missingFields = platformConfig.fields
                .filter(f => f.required && !botAuthDraft[f.key]?.trim())
                .map(f => f.label);

            if (missingFields.length > 0) {
                setBotError(`Missing required fields: ${missingFields.join(', ')}`);
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
                setBotError(result.error || 'Failed to save bot settings');
                return;
            }

            // Reload bots
            await reloadBots();

            setBotNotice(`Bot ${botDialogMode === 'edit' ? 'updated' : 'created'} successfully.`);
            setBotTokenDialogOpen(false);
        } catch (err) {
            console.error('Failed to save bot settings:', err);
            setBotError('Failed to save bot settings');
        } finally {
            setBotSaving(false);
        }
    };

    // API Key handlers
    const handleProviderSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            token: providerFormData.token,
            no_key_required: (providerFormData as any).noKeyRequired || false,
            ...(apiKeyDialogMode === 'add' && { proxy_url: (providerFormData as any).proxyUrl ?? '' }),
            ...(apiKeyDialogMode === 'edit' && { enabled: providerFormData.enabled }),
        };

        const result = apiKeyDialogMode === 'add'
            ? await api.addProvider(providerData)
            : await api.updateProvider(providerFormData.uuid!, {
                name: providerData.name,
                api_base: providerData.api_base,
                api_style: providerData.api_style,
                token: providerData.token || undefined,
                no_key_required: providerData.no_key_required,
                enabled: providerData.enabled,
                proxy_url: (providerFormData as any).proxyUrl ?? '',
            });

        if (result.success) {
            showNotification(`Provider ${apiKeyDialogMode === 'add' ? 'added' : 'updated'} successfully!`, 'success');
            setApiKeyDialogOpen(false);
            loadProviders();
        } else {
            showNotification(`Failed to ${apiKeyDialogMode === 'add' ? 'add' : 'update'} provider: ${result.error}`, 'error');
        }
    };

    const handleProviderForceAdd = async () => {
        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            token: providerFormData.token,
            no_key_required: (providerFormData as any).noKeyRequired || false,
            ...(apiKeyDialogMode === 'add' && { proxy_url: (providerFormData as any).proxyUrl ?? '' }),
            ...(apiKeyDialogMode === 'edit' && { enabled: providerFormData.enabled }),
        };

        const result = apiKeyDialogMode === 'add'
            ? await api.addProvider(providerData, true)
            : await api.updateProvider(providerFormData.uuid!, {
                name: providerData.name,
                api_base: providerData.api_base,
                api_style: providerData.api_style,
                token: providerData.token || undefined,
                no_key_required: providerData.no_key_required,
                enabled: providerData.enabled,
                proxy_url: (providerFormData as any).proxyUrl ?? '',
            });

        if (result.success) {
            showNotification(`Provider ${apiKeyDialogMode === 'add' ? 'added' : 'updated'} successfully!`, 'success');
            setApiKeyDialogOpen(false);
            loadProviders();
        } else {
            showNotification(`Failed to ${apiKeyDialogMode === 'add' ? 'add' : 'update'} provider: ${result.error}`, 'error');
        }
    };

    const handleDeleteProvider = async (uuid: string) => {
        const result = await api.deleteProvider(uuid);

        if (result.success) {
            showNotification('Provider deleted successfully!', 'success');
            loadProviders();
        } else {
            showNotification(`Failed to delete provider: ${result.error}`, 'error');
        }
    };

    const handleToggleProvider = async (uuid: string) => {
        const result = await api.toggleProvider(uuid);

        if (result.success) {
            showNotification(result.message, 'success');
            loadProviders();
        } else {
            showNotification(`Failed to toggle provider: ${result.error}`, 'error');
        }
    };

    const handleEditProvider = async (uuid: string) => {
        const result = await api.getProvider(uuid);

        if (result.success) {
            const provider = result.data;
            if (provider.auth_type === 'oauth') {
                // Handle OAuth edit
                setOAuthDetailProvider(result.data);
                setOAuthDetailDialogOpen(true);
            } else {
                // Handle API Key edit
                setApiKeyDialogMode('edit');
                setProviderFormData({
                    uuid: provider.uuid,
                    name: provider.name,
                    apiBase: provider.api_base,
                    apiStyle: provider.api_style || 'openai',
                    token: provider.token || "",
                    enabled: provider.enabled,
                    noKeyRequired: provider.no_key_required || false,
                    proxyUrl: provider.proxy_url || '',
                } as any);
                setApiKeyDialogOpen(true);
            }
        } else {
            showNotification(`Failed to load provider details: ${result.error}`, 'error');
        }
    };

    // OAuth handlers
    const handleOAuthSuccess = () => {
        showNotification('OAuth provider added successfully!', 'success');
        loadProviders();
    };

    const handleRefreshToken = async (providerUuid: string) => {
        try {
            const { oauthApi } = await api.instances();
            const response = await oauthApi.apiV1OauthRefreshPost({ provider_uuid: providerUuid });

            if (response.data.success) {
                showNotification('Token refreshed successfully!', 'success');
                await loadProviders();
            } else {
                showNotification(`Failed to refresh token: ${response.data.message || 'Unknown error'}`, 'error');
            }
        } catch (error: any) {
            const errorMessage = error?.response?.data?.error || error?.message || 'Unknown error';
            showNotification(`Failed to refresh token: ${errorMessage}`, 'error');
        }
    };

    // Derived state
    const { apiKeyProviders, oauthProviders, credentialCounts } = useMemo(() => {
        const apiKeys = providers.filter((p: any) => p.auth_type !== 'oauth');
        const oauth = providers.filter((p: any) => p.auth_type === 'oauth');
        return {
            apiKeyProviders: apiKeys,
            oauthProviders: oauth,
            credentialCounts: {
                apiKeys: apiKeys.length,
                oauth: oauth.length,
                bots: bots.length,
                total: providers.length + bots.length,
            },
        };
    }, [providers, bots]);

    return (
        <PageLayout loading={loading}>
            <UnifiedCard
                title="Credentials"
                subtitle={`Managing ${credentialCounts.total} credential${credentialCounts.total > 1 ? 's' : ''}`}
                size="full"
                rightAction={
                    <Stack direction="row" spacing={1}>
                        <Button
                            variant="contained"
                            startIcon={<Add />}
                            onClick={handleAddClick}
                            size="small"
                            endIcon={<ExpandMore />}
                        >
                            Add Credential
                        </Button>
                        <Menu
                            anchorEl={addMenuAnchorEl}
                            open={Boolean(addMenuAnchorEl)}
                            onClose={handleAddMenuClose}
                            anchorOrigin={{
                                vertical: 'bottom',
                                horizontal: 'right',
                            }}
                            transformOrigin={{
                                vertical: 'top',
                                horizontal: 'right',
                            }}
                        >
                            <MenuItem onClick={handleAddApiKey}>
                                <Add sx={{ mr: 1 }} fontSize="small" />
                                Add API Key
                            </MenuItem>
                            <MenuItem onClick={handleAddOAuth}>
                                <VpnKey sx={{ mr: 1 }} fontSize="small" />
                                Add OAuth Provider
                            </MenuItem>
                            <MenuItem onClick={handleAddBot}>
                                <SmartToy sx={{ mr: 1 }} fontSize="small" />
                                Add Bot
                            </MenuItem>
                        </Menu>
                    </Stack>
                }
            >
                    {/* Tab Navigation */}
                    <Tabs
                        value={activeTab}
                        onChange={handleTabChange}
                        sx={{
                            borderBottom: 1,
                            borderColor: 'divider',
                            mb: 2,
                            minHeight: 48,
                            '& .MuiTab-root': {
                                textTransform: 'none',
                                fontWeight: 500,
                                fontSize: '0.875rem',
                                minHeight: 48,
                            },
                        }}
                    >
                        <Tab
                            label={
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    <span>API Keys</span>
                                    <Chip
                                        label={credentialCounts.apiKeys}
                                        size="small"
                                        color={activeTab === 'api-keys' ? 'primary' : 'default'}
                                        variant={activeTab === 'api-keys' ? 'filled' : 'outlined'}
                                        sx={{ height: 20, minWidth: 20, fontSize: '0.7rem' }}
                                    />
                                </Stack>
                            }
                            value="api-keys"
                        />
                        <Tab
                            label={
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    <span>OAuth</span>
                                    <Chip
                                        label={credentialCounts.oauth}
                                        size="small"
                                        color={activeTab === 'oauth' ? 'primary' : 'default'}
                                        variant={activeTab === 'oauth' ? 'filled' : 'outlined'}
                                        sx={{ height: 20, minWidth: 20, fontSize: '0.7rem' }}
                                    />
                                </Stack>
                            }
                            value="oauth"
                        />
                        <Tab
                            label={
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    <span>Bots</span>
                                    <Chip
                                        label={credentialCounts.bots}
                                        size="small"
                                        color={activeTab === 'bot-token' ? 'primary' : 'default'}
                                        variant={activeTab === 'bot-token' ? 'filled' : 'outlined'}
                                        sx={{ height: 20, minWidth: 20, fontSize: '0.7rem' }}
                                    />
                                </Stack>
                            }
                            value="bot-token"
                        />
                    </Tabs>

                    {/* Tab Content */}
                    {activeTab === 'api-keys' && (
                        <>
                            {credentialCounts.apiKeys > 0 ? (
                                <ApiKeyTable
                                    providers={apiKeyProviders}
                                    onEdit={handleEditProvider}
                                    onToggle={handleToggleProvider}
                                    onDelete={handleDeleteProvider}
                                />
                            ) : (
                                <EmptyStateGuide
                                    title="No API Keys Configured"
                                    description="Configure API keys to access AI services like OpenAI, Anthropic, etc."
                                    showOAuthButton={false}
                                    showHeroIcon={false}
                                    primaryButtonLabel="Add API Key"
                                    onAddApiKeyClick={handleAddApiKey}
                                />
                            )}
                        </>
                    )}

                    {activeTab === 'oauth' && (
                        <>
                            {credentialCounts.oauth > 0 ? (
                                <OAuthTable
                                    providers={oauthProviders}
                                    onEdit={handleEditProvider}
                                    onToggle={handleToggleProvider}
                                    onDelete={handleDeleteProvider}
                                    onRefreshToken={handleRefreshToken}
                                />
                            ) : (
                                <EmptyStateGuide
                                    title="No OAuth Providers Configured"
                                    description="Configure OAuth providers like Claude Code, Gemini CLI, Qwen, etc."
                                    showOAuthButton={false}
                                    showHeroIcon={false}
                                    primaryButtonLabel="Add OAuth Provider"
                                    onAddApiKeyClick={handleAddOAuth}
                                />
                            )}
                        </>
                    )}

                    {activeTab === 'bot-token' && (
                        <Stack spacing={2}>
                            {botNotice && (
                                <Alert severity="success" onClose={() => setBotNotice(null)}>
                                    {botNotice}
                                </Alert>
                            )}
                            {botError && (
                                <Alert severity="error" onClose={() => setBotError(null)}>
                                    {botError}
                                </Alert>
                            )}
                            {bots.length > 0 ? (
                                <BotTable
                                    bots={bots}
                                    platforms={botPlatforms}
                                    onEdit={handleEditBot}
                                    onToggle={handleToggleBot}
                                    onDelete={handleDeleteBot}
                                />
                            ) : (
                                <EmptyStateGuide
                                    title="No Bots Configured"
                                    description="Configure bots to enable remote-coder chat integration."
                                    showOAuthButton={false}
                                    showHeroIcon={false}
                                    primaryButtonLabel="Add Bot"
                                    onAddApiKeyClick={() => handleOpenBotTokenDialog()}
                                />
                            )}
                            {botLoading && (
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <CircularProgress size={16} />
                                    <Typography variant="body2" color="text.secondary">
                                        Loading bot settings...
                                    </Typography>
                                </Stack>
                            )}
                        </Stack>
                    )}
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
                    <Typography variant="h6">{botDialogMode === 'edit' ? 'Edit Bot Configuration' : 'Add Bot Configuration'}</Typography>
                    <Stack spacing={2}>
                        <TextField
                            label="Name"
                            placeholder="My Bot"
                            value={botNameDraft}
                            onChange={(e) => setBotNameDraft(e.target.value)}
                            fullWidth
                            size="small"
                            helperText="Optional: a friendly name for this bot configuration."
                            disabled={botSaving}
                        />

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

            {/* API Key Provider Dialog */}
            <ProviderFormDialog
                open={apiKeyDialogOpen}
                onClose={() => setApiKeyDialogOpen(false)}
                onSubmit={handleProviderSubmit}
                onForceAdd={handleProviderForceAdd}
                data={providerFormData}
                onChange={(field, value) => setProviderFormData(prev => ({ ...prev, [field]: value }))}
                mode={apiKeyDialogMode}
            />

            {/* OAuth Add Dialog */}
            <OAuthDialog
                open={oauthDialogOpen}
                onClose={() => setOAuthDialogOpen(false)}
                onSuccess={handleOAuthSuccess}
            />

            {/* OAuth Detail/Edit Dialog */}
            <OAuthDetailDialog
                open={oauthDetailDialogOpen}
                provider={oauthDetailProvider}
                onClose={() => setOAuthDetailDialogOpen(false)}
                onSubmit={async (data: OAuthEditFormData) => {
                    if (!oauthDetailProvider?.uuid) return;
                    const result = await api.updateProvider(oauthDetailProvider.uuid, {
                        name: data.name,
                        api_base: data.apiBase,
                        api_style: data.apiStyle,
                        enabled: data.enabled,
                        proxy_url: data.proxyUrl ?? '',
                    });
                    if (!result.success) {
                        throw new Error(result.error || 'Failed to update provider');
                    }
                    showNotification('Provider updated successfully!', 'success');
                    loadProviders();
                }}
            />

            {/* Snackbar for notifications */}
            <Snackbar
                open={snackbar.open}
                autoHideDuration={6000}
                onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            >
                <Alert
                    onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
                    severity={snackbar.severity}
                    variant="filled"
                    sx={{ width: '100%' }}
                >
                    {snackbar.message}
                </Alert>
            </Snackbar>
        </PageLayout>
    );
};

export default CredentialPage;
