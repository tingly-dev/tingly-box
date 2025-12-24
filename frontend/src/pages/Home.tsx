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
    Tabs
} from '@mui/material';
import React, { useEffect, useState } from 'react';
import { OpenAI, Anthropic } from '@lobehub/icons';
import TerminalIcon from '@mui/icons-material/Terminal';
import CodeIcon from '@mui/icons-material/Code';
import { PageLayout } from '../components/PageLayout';
import { api, getBaseUrl } from '../services/api';
import UseOpenAIPage from './UseOpenAIPage';
import UseAnthropicPage from './UseAnthropicPage';
import UseClaudeCodePage from './UseClaudeCodePage';
import UseLiteLLMPage from './UseLiteLLMPage';

const Home = () => {
    const [activeTab, setActiveTab] = useState(0);
    const [baseUrl, setBaseUrl] = useState<string>('');
    const [generatedToken, setGeneratedToken] = useState<string>('');
    const [apiKey, setApiKey] = useState<string>('');
    const [showTokenModal, setShowTokenModal] = useState(false);
    const [showRefreshConfirmation, setShowRefreshConfirmation] = useState(false);
    const [notification, setNotification] = useState<{
        open: boolean;
        message?: string;
        severity?: 'success' | 'info' | 'warning' | 'error';
        autoHideDuration?: number;
    }>({ open: false });

    const token = generatedToken || apiKey;

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
            showNotification(`${label} copied to clipboard!`, 'success');
        } catch (err) {
            showNotification('Failed to copy to clipboard', 'error');
        }
    };

    const loadBaseUrl = async () => {
        const baseUrl = await getBaseUrl();
        setBaseUrl(baseUrl);
    };

    const loadToken = async () => {
        const result = await api.getToken();
        if (result.token) {
            setApiKey(result.token);
        }
    };

    const generateToken = async () => {
        const clientId = 'web';
        const result = await api.generateToken(clientId);
        if (result.success) {
            setGeneratedToken(result.data.token);
            copyToClipboard(result.data.token, 'Token');
        } else {
            showNotification(`Failed to generate token: ${result.error}`, 'error');
        }
    };

    const handleRefreshToken = () => {
        setShowRefreshConfirmation(true);
    };

    const confirmRefreshToken = () => {
        setShowRefreshConfirmation(false);
        generateToken();
    };

    useEffect(() => {
        loadBaseUrl();
        loadToken();
    }, []);

    const handleTabChange = (_: React.SyntheticEvent, newValue: number) => {
        setActiveTab(newValue);
    };

    return (
        <PageLayout notification={notification}>
            {/* Main Tabs */}
            <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 3 }}>
                <Tabs value={activeTab} onChange={handleTabChange} aria-label="Plugin tabs">
                    <Tab
                        icon={<OpenAI size={16} />}
                        label="Use OpenAI"
                        iconPosition="start"
                    />
                    <Tab
                        icon={<Anthropic size={16} />}
                        label="Use Anthropic"
                        iconPosition="start"
                    />
                    <Tab
                        icon={<CodeIcon fontSize="small" />}
                        label="Use Claude Code"
                        iconPosition="start"
                    />
                    <Tab
                        icon={<TerminalIcon fontSize="small" />}
                        label="Use LiteLLM"
                        iconPosition="start"
                    />
                </Tabs>
            </Box>

            {/* Tab Content */}
            {activeTab === 0 && (
                <UseOpenAIPage
                    showTokenModal={showTokenModal}
                    setShowTokenModal={setShowTokenModal}
                    token={token}
                    showNotification={showNotification}
                />
            )}
            {activeTab === 1 && (
                <UseAnthropicPage
                    showTokenModal={showTokenModal}
                    setShowTokenModal={setShowTokenModal}
                    token={token}
                    showNotification={showNotification}
                />
            )}
            {activeTab === 2 && (
                <UseClaudeCodePage
                    showTokenModal={showTokenModal}
                    setShowTokenModal={setShowTokenModal}
                    token={token}
                    showNotification={showNotification}
                />
            )}
            {activeTab === 3 && (
                <UseLiteLLMPage
                    showTokenModal={showTokenModal}
                    setShowTokenModal={setShowTokenModal}
                    token={token}
                    showNotification={showNotification}
                />
            )}

            {/* Token Refresh Confirmation Dialog */}
            <Dialog
                open={showRefreshConfirmation}
                onClose={() => setShowRefreshConfirmation(false)}
                aria-labelledby="refresh-token-dialog-title"
                aria-describedby="refresh-token-dialog-description"
            >
                <DialogTitle id="refresh-token-dialog-title">
                    Confirm Token Refresh
                </DialogTitle>
                <DialogContent>
                    <Alert severity="warning" sx={{ mb: 2 }}>
                        Important Reminder
                    </Alert>
                    <DialogContentText id="refresh-token-dialog-description">
                        Modifying the token will cause configured tools to become unavailable. Are you sure you want to continue generating a new token?
                    </DialogContentText>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setShowRefreshConfirmation(false)} color="primary">
                        Cancel
                    </Button>
                    <Button onClick={confirmRefreshToken} color="error" variant="contained">
                        Confirm Refresh
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default Home;
