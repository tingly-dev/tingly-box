import { Add, Edit, ExpandMore, VpnKey } from '@mui/icons-material';
import {
    Alert,
    Button,
    Chip,
    Menu,
    MenuItem,
    Snackbar,
    Stack,
    Tab,
    Tabs,
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

type ProviderFormData = EnhancedProviderFormData;

interface OAuthEditFormData {
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic';
    enabled: boolean;
    proxyUrl?: string;
}

type CredentialTab = 'api-keys' | 'oauth';

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

    // URL param handling for auto-opening dialogs
    useEffect(() => {
        const dialog = searchParams.get('dialog');
        const style = searchParams.get('style') as 'openai' | 'anthropic' | null;
        const tab = searchParams.get('tab');

        // Set tab from URL
        if (tab === 'oauth') {
            setActiveTab('oauth');
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
    }, []);

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
                total: providers.length,
            },
        };
    }, [providers]);

    return (
        <PageLayout loading={loading}>
            <UnifiedCard
                title="Credentials"
                subtitle={`Managing ${credentialCounts.total} credential${credentialCounts.total !== 1 ? 's' : ''}`}
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
            </UnifiedCard>

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
