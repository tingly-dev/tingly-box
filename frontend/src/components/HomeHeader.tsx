import {
    ContentCopy as CopyIcon,
    Refresh as RefreshIcon,
    Terminal as TerminalIcon
} from '@mui/icons-material';
import {
    Box,
    IconButton,
    Tab,
    Tabs,
    Tooltip,
    Typography
} from '@mui/material';
import React from 'react';
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
    const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
        setActiveTab(newValue);
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
                            <Typography variant="caption">
                                👁️
                            </Typography>
                        </IconButton>
                    </Tooltip>
                    <IconButton
                        onClick={generateToken}
                        size="small"
                        title="Generate New Token"
                    >
                        <RefreshIcon fontSize="small" />
                    </IconButton>
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
                    <Tab label="Use OpenAI" />
                    <Tab label="Use Anthropic" />
                </Tabs>
            </Box>
            {activeTab === 0 && <OpenAITab />}
            {activeTab === 1 && <AnthropicTab />}
        </>
    );
};