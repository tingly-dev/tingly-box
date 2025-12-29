import { ContentCopy as CopyIcon, Refresh as RefreshIcon, Terminal as TerminalIcon } from '@mui/icons-material';
import {
    Box,
    Grid,
    IconButton,
    Snackbar,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../services/api';
import UnifiedCard from './UnifiedCard';

interface ServerInfoCardProps {
    currentToken?: string;
}

const ServerInfoCard = ({ currentToken }: ServerInfoCardProps) => {
    const { t } = useTranslation();
    const [snackbarOpen, setSnackbarOpen] = useState(false);
    const [snackbarMessage, setSnackbarMessage] = useState('');
    const [generatedToken, setGeneratedToken] = useState<string>('');
    const [modelToken, setModelToken] = useState<string>('');
    const [showToken, setShowToken] = useState(false);

    useEffect(() => {
        const fetchToken = async () => {
            const result = await api.getToken();
            if (result.success && result.data && result.data.token) {
                setModelToken(result.data.token);
            }
            console.log(result)
            console.log(modelToken)
        };
        fetchToken();
    }, []);

    const copyToClipboard = async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            setSnackbarMessage(t('serverInfo.notifications.copied', { label }));
            setSnackbarOpen(true);
        } catch (err) {
            setSnackbarMessage(t('serverInfo.notifications.copyFailed'));
            setSnackbarOpen(true);
        }
    };

    const generateToken = async () => {
        const clientId = 'web';
        const result = await api.generateToken(clientId);
        if (result.success) {
            setGeneratedToken(result.data.token);
            setShowToken(true);
            copyToClipboard(result.data.token, 'Token');
        } else {
            setSnackbarMessage(t('serverInfo.notifications.generateFailed', { error: result.error }));
            setSnackbarOpen(true);
        }
    };

    const baseUrl = window.location.origin;
    const openaiBaseUrl = `${baseUrl}/openai/v1`;
    const anthropicBaseUrl = `${baseUrl}/anthropic/v1`;
    const token = generatedToken || modelToken

    return (
        <>
            <UnifiedCard
                // title="Server Information"
                // subtitle="Quick access to server endpoints and credentials"
                size="header"
            >
                <Box>
                    <Grid container spacing={3}>
                        <Grid size={{ xs: 12, md: 7 }}>
                            <Stack spacing={2}>
                                <Typography variant="h6" color="primary" fontWeight={600}>
                                    {t('serverInfo.title')}
                                </Typography>

                                <Grid container spacing={2}>
                                    <Grid size={{ xs: 12, sm: 6 }}>
                                        <TextField
                                            label={t('serverInfo.openAI.label')}
                                            value={openaiBaseUrl}
                                            fullWidth
                                            size="small"
                                            slotProps={{
                                                input: {
                                                    readOnly: true,
                                                    endAdornment: (
                                                        <Stack direction="row" spacing={0.5}>
                                                            <IconButton
                                                                onClick={() => copyToClipboard(openaiBaseUrl, t('serverInfo.openAI.label'))}
                                                                size="small"
                                                                title={t('serverInfo.openAI.copyTooltip')}
                                                            >
                                                                <CopyIcon fontSize="small" />
                                                            </IconButton>
                                                            <IconButton
                                                                onClick={() => {
                                                                    const openaiCurl = `curl -X POST "${openaiBaseUrl}/chat/completions" \\
  -H "Authorization: Bearer ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{"messages": [{"role": "user", "content": "Hello!"}]}'
                                                                    `;
                                                                    copyToClipboard(openaiCurl, 'OpenAI cURL command');
                                                                }}
                                                                size="small"
                                                                title={t('serverInfo.openAI.copyCurlTooltip')}
                                                            >
                                                                <TerminalIcon fontSize="small" />
                                                            </IconButton>
                                                        </Stack>
                                                    ),
                                                },
                                            }}
                                        />
                                    </Grid>

                                    <Grid size={{ xs: 12, sm: 6 }}>
                                        <TextField
                                            label={t('serverInfo.anthropic.label')}
                                            value={anthropicBaseUrl}
                                            fullWidth
                                            size="small"
                                            slotProps={{
                                                input: {
                                                    readOnly: true,
                                                    endAdornment: (
                                                        <Stack direction="row" spacing={0.5}>
                                                            <IconButton
                                                                onClick={() => copyToClipboard(anthropicBaseUrl, t('serverInfo.anthropic.label'))}
                                                                size="small"
                                                                title={t('serverInfo.anthropic.copyTooltip')}
                                                            >
                                                                <CopyIcon fontSize="small" />
                                                            </IconButton>
                                                            <IconButton
                                                                onClick={() => {
                                                                    const anthropicCurl = `curl -X POST "${anthropicBaseUrl}/messages" \\
  -H "Authorization: Bearer ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{"messages": [{"role": "user", "content": "Hello!"}], "max_tokens": 100}'
                                                                    `;
                                                                    copyToClipboard(anthropicCurl, 'Anthropic cURL command');
                                                                }}
                                                                size="small"
                                                                title={t('serverInfo.anthropic.copyCurlTooltip')}
                                                            >
                                                                <TerminalIcon fontSize="small" />
                                                            </IconButton>
                                                        </Stack>
                                                    ),
                                                },
                                            }}
                                        />
                                    </Grid>
                                </Grid>
                            </Stack>
                        </Grid>

                        <Grid size={{ xs: 12, md: 5 }}>
                            <Stack spacing={2}>
                                <Typography variant="h6" color="primary" fontWeight={600}>
                                    {t('serverInfo.authentication.title')}
                                </Typography>

                                <TextField
                                    label={t('serverInfo.authentication.apiKeyLabel')}
                                    value={showToken ? token : token.replace(/./g, '•')}
                                    fullWidth
                                    size="small"
                                    slotProps={{
                                        input: {
                                            readOnly: true,
                                            type: showToken ? 'text' : 'password',
                                            endAdornment: (
                                                <Stack direction="row" spacing={0.5}>
                                                    <IconButton
                                                        onClick={() => setShowToken(!showToken)}
                                                        size="small"
                                                        title={showToken ? t('serverInfo.authentication.hideTokenTooltip') : t('serverInfo.authentication.showTokenTooltip')}
                                                    >
                                                        <Typography variant="caption">
                                                            {showToken ? '👁️' : '👁️‍🗨️'}
                                                        </Typography>
                                                    </IconButton>
                                                    <IconButton
                                                        onClick={() => copyToClipboard(token, t('serverInfo.authentication.apiKeyLabel'))}
                                                        size="small"
                                                        title={t('serverInfo.authentication.copyTokenTooltip')}
                                                    >
                                                        <CopyIcon fontSize="small" />
                                                    </IconButton>
                                                    <IconButton
                                                        onClick={generateToken}
                                                        size="small"
                                                        title={t('serverInfo.authentication.generateTooltip')}
                                                    >
                                                        <RefreshIcon fontSize="small" />
                                                    </IconButton>
                                                </Stack>
                                            ),
                                        },
                                    }}
                                />
                            </Stack>
                        </Grid>
                    </Grid>
                </Box>
            </UnifiedCard>

            <Snackbar
                open={snackbarOpen}
                autoHideDuration={3000}
                onClose={() => setSnackbarOpen(false)}
                message={snackbarMessage}
            />
        </>
    );
};

export default ServerInfoCard;