import {
    Alert,
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
    Tab,
    Tabs,
    Typography
} from '@mui/material';
import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import OpenAI from "@lobehub/icons/es/OpenAI"
import Anthropic from "@lobehub/icons/es/Anthropic"
import Claude from "@lobehub/icons/es/Claude"
import { Add as AddIcon } from '@mui/icons-material';
import { PageLayout } from '../components/PageLayout';
import PresetProviderFormDialog, { type EnhancedProviderFormData } from '../components/PresetProviderFormDialog';
import { api } from '../services/api';
import type { Provider } from '../types/provider';
import UseOpenAIPage from './UseOpenAIPage';
import UseAnthropicPage from './UseAnthropicPage';
import UseClaudeCodePage from './UseClaudeCodePage';
import UseLiteLLMPage from './UseLiteLLMPage';

const Home = () => {
    const { t } = useTranslation();
    const [activeTab, setActiveTab] = useState(0);
    const [generatedToken, setGeneratedToken] = useState<string>('');
    const [apiKey, setApiKey] = useState<string>('');
    const [showTokenModal, setShowTokenModal] = useState(false);
    const [showRefreshConfirmation, setShowRefreshConfirmation] = useState(false);
    const [notification, setNotification] = useState<{
        open: boolean;
        message?: string;
        severity?: 'success' | 'info' | 'warning' | 'error';
        autoHideDuration?: number;
        onClose?: () => void;
    }>({ open: false });

    // Provider state
    const [providers, setProviders] = useState<Provider[]>([]);
    const [loading, setLoading] = useState(true);
    const [addDialogOpen, setAddDialogOpen] = useState(false);
    const [providerFormData, setProviderFormData] = useState<EnhancedProviderFormData>({
        name: '',
        apiBase: '',
        apiStyle: undefined,
        token: '',
    });

    const token = generatedToken || apiKey;
    const hasProviders = providers.length > 0;

    const showNotification = (message: string, severity: 'success' | 'info' | 'warning' | 'error' = 'info', autoHideDuration: number = 6000) => {
        setNotification({
            open: true,
            message,
            severity,
            autoHideDuration,
            onClose: () => setNotification(prev => ({ ...prev, open: false }))
        });
    };

    const copyToClipboard = async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            showNotification(t('home.token.generated', { label }), 'success');
        } catch (err) {
            showNotification(t('home.token.copyFailed'), 'error');
        }
    };

    const loadToken = async () => {
        const result = await api.getToken();
        if (result.token) {
            setApiKey(result.token);
        }
    };

    const loadProviders = async () => {
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        }
        setLoading(false);
    };

    const loadData = async () => {
        await Promise.all([loadToken(), loadProviders()]);
    };

    const generateToken = async () => {
        const clientId = 'web';
        const result = await api.generateToken(clientId);
        if (result.success) {
            setGeneratedToken(result.data.token);
            copyToClipboard(result.data.token, 'Token');
        } else {
            showNotification(t('home.token.generationFailed', { error: result.error }), 'error');
        }
    };

    const handleRefreshToken = () => {
        setShowRefreshConfirmation(true);
    };

    const confirmRefreshToken = () => {
        setShowRefreshConfirmation(false);
        generateToken();
    };

    // Add provider dialog handlers
    const handleAddProviderClick = () => {
        setProviderFormData({
            name: '',
            apiBase: '',
            apiStyle: undefined,
            token: '',
        });
        setAddDialogOpen(true);
    };

    const handleProviderFormChange = (field: keyof EnhancedProviderFormData, value: any) => {
        setProviderFormData(prev => ({
            ...prev,
            [field]: value,
        }));
    };

    const handleAddProviderSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            token: providerFormData.token,
        };

        const result = await api.addProvider(providerData);

        if (result.success) {
            showNotification(t('home.notifications.providerAdded'), 'success');
            setProviderFormData({
                name: '',
                apiBase: '',
                apiStyle: undefined,
                token: '',
            });
            setAddDialogOpen(false);
            await loadProviders();
        } else {
            showNotification(t('home.notifications.providerAddFailed', { error: result.error }), 'error');
        }
    };

    useEffect(() => {
        loadData();
    }, []);

    const handleTabChange = (_: React.SyntheticEvent, newValue: number) => {
        setActiveTab(newValue);
    };

    // Scroll to top when tab changes
    useEffect(() => {
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }, [activeTab]);

    // Empty state component
    const emptyState = (
        <Box textAlign="center" py={8} width="100%">
            <Button
                variant="contained"
                startIcon={<AddIcon />}
                onClick={handleAddProviderClick}
                size="large"
                sx={{
                    backgroundColor: 'primary.main',
                    color: 'white',
                    width: 80,
                    height: 80,
                    borderRadius: 2,
                    mb: 3,
                    '&:hover': {
                        backgroundColor: 'primary.dark',
                        transform: 'scale(1.05)',
                    },
                }}
            >
                <AddIcon sx={{ fontSize: 40 }} />
            </Button>
            <Typography variant="h5" sx={{ fontWeight: 600, mb: 2 }}>
                {t('home.emptyState.title')}
            </Typography>
            <Typography variant="body1" color="text.secondary" sx={{ mb: 3, maxWidth: 500, mx: 'auto' }}>
                {t('home.emptyState.description')}
            </Typography>
            <Button
                variant="contained"
                startIcon={<AddIcon />}
                onClick={handleAddProviderClick}
                size="large"
            >
                {t('home.emptyState.button')}
            </Button>
        </Box>
    );

    return (
        <PageLayout notification={notification} loading={loading}>
            {/* Main Tabs */}
            <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 3 }}>
                <Tabs value={activeTab} onChange={handleTabChange} aria-label="Plugin tabs">
                    <Tab
                        icon={<OpenAI size={16} />}
                        label={t('home.tabs.useOpenAI')}
                        iconPosition="start"
                    />
                    <Tab
                        icon={<Anthropic size={16} />}
                        label={t('home.tabs.useAnthropic')}
                        iconPosition="start"
                    />
                    <Tab
                        icon={<Claude fontSize="small" />}
                        label={t('home.tabs.useClaudeCode')}
                        iconPosition="start"
                    />
                </Tabs>
            </Box>

            {/* Show empty state if no providers, otherwise show tab content */}
            {!hasProviders ? (
                emptyState
            ) : (
                <>
                    {/* Tab Content */}
                    {activeTab === 0 && (
                        <UseOpenAIPage
                            showTokenModal={showTokenModal}
                            setShowTokenModal={setShowTokenModal}
                            token={token}
                            showNotification={showNotification}
                            providers={providers}
                        />
                    )}
                    {activeTab === 1 && (
                        <UseAnthropicPage
                            showTokenModal={showTokenModal}
                            setShowTokenModal={setShowTokenModal}
                            token={token}
                            showNotification={showNotification}
                            providers={providers}
                        />
                    )}
                    {activeTab === 2 && (
                        <UseClaudeCodePage
                            showTokenModal={showTokenModal}
                            setShowTokenModal={setShowTokenModal}
                            token={token}
                            showNotification={showNotification}
                            providers={providers}
                        />
                    )}
                </>
            )}

            {/* Add Provider Dialog */}
            <PresetProviderFormDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddProviderSubmit}
                data={providerFormData}
                onChange={handleProviderFormChange}
                mode="add"
            />

            {/* Token Refresh Confirmation Dialog */}
            <Dialog
                open={showRefreshConfirmation}
                onClose={() => setShowRefreshConfirmation(false)}
                aria-labelledby="refresh-token-dialog-title"
                aria-describedby="refresh-token-dialog-description"
            >
                <DialogTitle id="refresh-token-dialog-title">
                    {t('home.token.refresh.title')}
                </DialogTitle>
                <DialogContent>
                    <Alert severity="warning" sx={{ mb: 2 }}>
                        {t('home.token.refresh.alert')}
                    </Alert>
                    <DialogContentText id="refresh-token-dialog-description">
                        {t('home.token.refresh.description')}
                    </DialogContentText>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setShowRefreshConfirmation(false)} color="primary">
                        {t('common.cancel')}
                    </Button>
                    <Button onClick={confirmRefreshToken} color="error" variant="contained">
                        {t('home.token.refresh.button')}
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default Home;
