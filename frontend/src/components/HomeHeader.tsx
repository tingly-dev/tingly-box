import {
    ContentCopy as CopyIcon,
    Edit as EditIcon,
    Refresh as RefreshIcon,
    Terminal as TerminalIcon
} from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {
    Alert,
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
    IconButton,
    Tab,
    Tabs,
    Tooltip,
    Typography
} from '@mui/material';
import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { OpenAI, Anthropic } from '@lobehub/icons';
import { ApiConfigRow } from './ApiConfigRow';

interface HomeHeaderProps {
    activeTab: number;
    setActiveTab: (tab: number) => void;
    openaiBaseUrl: string;
    anthropicBaseUrl: string;
    token: string;
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    generateToken: () => void;
    copyToClipboard: (text: string, label: string) => void;
}

export const HomeHeader: React.FC<HomeHeaderProps> = ({
                                                          activeTab,
                                                          setActiveTab,
                                                          openaiBaseUrl,
                                                          anthropicBaseUrl,
                                                          token,
                                                          showTokenModal,
                                                          setShowTokenModal,
                                                          generateToken,
                                                          copyToClipboard
                                                      }) => {
    const [showRefreshConfirmation, setShowRefreshConfirmation] = useState(false);
    const navigate = useNavigate();

    const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
        setActiveTab(newValue);
    };

    const handleRefreshToken = () => {
        setShowRefreshConfirmation(true);
    };

    const confirmRefreshToken = () => {
        setShowRefreshConfirmation(false);
        generateToken();
    };

    const ProviderTab = ({ baseUrl, baseUrlLabel, curlCommand }: {
        baseUrl: string;
        baseUrlLabel: string;
        curlCommand: string;
    }) => (
        <Box sx={{ p: 2 }}>
            <ApiConfigRow
                label="Base URL"
                value={`${baseUrl}`}
                onCopy={() => copyToClipboard(baseUrl, baseUrlLabel)}
                isClickable={true}
            >
                <Box sx={{ display: 'flex', gap: 0.5, ml: 'auto' }}>
                    <IconButton
                        onClick={() => copyToClipboard(baseUrl, baseUrlLabel)}
                        size="small"
                        title={`Copy ${baseUrlLabel}`}
                    >
                        <CopyIcon fontSize="small" />
                    </IconButton>
                    {
                        curlCommand && <IconButton
                            onClick={() => copyToClipboard(curlCommand, `${baseUrlLabel} cURL command`)}
                            size="small"
                            title={`Copy ${baseUrlLabel} cURL Example`}
                        >
                            <TerminalIcon fontSize="small" />
                        </IconButton>
                    }
                </Box>
            </ApiConfigRow>

            <ApiConfigRow
                label="API Key"
                showEllipsis={true}
            >
                <Box sx={{ display: 'flex', gap: 0.5, ml: 'auto' }}>
                    <Tooltip title="View Token">
                        <IconButton
                            onClick={() => setShowTokenModal(true)}
                            size="small"
                        >
                            <VisibilityIcon></VisibilityIcon>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Refresh Token">
                        <IconButton
                            onClick={handleRefreshToken}
                            size="small"
                            title="Generate New Token"
                        >
                            <RefreshIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <IconButton
                        onClick={() => copyToClipboard(token, 'API Key')}
                        size="small"
                        title="Copy Token"
                    >
                        <CopyIcon fontSize="small" />
                    </IconButton>
                </Box>
            </ApiConfigRow>

            <ApiConfigRow
                label="Model Name"
                value="tingly"
                onCopy={() => copyToClipboard('tingly', 'Model Name')}
                isClickable={true}
            >
                <Box sx={{ display: 'flex', gap: 0.5, ml: 'auto' }}>
                    <Tooltip title="Edit Rule">
                        <IconButton
                            onClick={() => navigate('/routing?expand=tingly')}
                            size="small"
                        >
                            <EditIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <IconButton
                        onClick={() => copyToClipboard('tingly', 'LLM API Model')}
                        size="small"
                        title="Copy Model"
                    >
                        <CopyIcon fontSize="small" />
                    </IconButton>
                </Box>
            </ApiConfigRow>
        </Box>
    );

    const OpenAITab = () => {
        const openaiCurl = `curl -X POST "${openaiBaseUrl}/v1/chat/completions" -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"messages": [{"role": "user", "content": "Hello!"}]}'`;
        return <ProviderTab
            baseUrl={openaiBaseUrl}
            baseUrlLabel="OpenAI Base URL"
            // curlCommand={openaiCurl}
        />;
    };

    const AnthropicTab = () => {
        const anthropicCurl = `curl -X POST "${anthropicBaseUrl}/v1/messages" -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"messages": [{"role": "user", "content": "Hello!"}], "max_tokens": 100}'`;
        return <ProviderTab
            baseUrl={anthropicBaseUrl}
            baseUrlLabel="Anthropic Base URL"
            // curlCommand={anthropicCurl}
        />;
    };

    return (
        <>
            <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
                <Tabs value={activeTab} onChange={handleTabChange} aria-label="API configuration tabs">
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
                </Tabs>
            </Box>
            {activeTab === 0 && <OpenAITab />}
            {activeTab === 1 && <AnthropicTab />}

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
        </>
    );
};