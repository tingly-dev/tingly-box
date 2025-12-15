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
import { api } from '../services/api';
import UnifiedCard from './UnifiedCard';

interface ServerInfoCardProps {
    currentToken?: string;
}

const ServerInfoCard = ({ currentToken }: ServerInfoCardProps) => {
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
            setSnackbarMessage(`${label} copied to clipboard!`);
            setSnackbarOpen(true);
        } catch (err) {
            setSnackbarMessage('Failed to copy to clipboard');
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
            setSnackbarMessage(`Failed to generate token: ${result.error}`);
            setSnackbarOpen(true);
        }
    };

    const baseUrl = import.meta.env.VITE_API_BASE_URL || window.location.origin;
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
                                    API Endpoints
                                </Typography>

                                <Grid container spacing={2}>
                                    <Grid size={{ xs: 12, sm: 6 }}>
                                        <TextField
                                            label="OpenAI Base URL"
                                            value={openaiBaseUrl}
                                            fullWidth
                                            size="small"
                                            slotProps={{
                                                input: {
                                                    readOnly: true,
                                                    endAdornment: (
                                                        <Stack direction="row" spacing={0.5}>
                                                            <IconButton
                                                                onClick={() => copyToClipboard(openaiBaseUrl, 'OpenAI Base URL')}
                                                                size="small"
                                                                title="Copy OpenAI Base URL"
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
                                                                title="Copy OpenAI cURL Example"
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
                                            label="Anthropic Base URL"
                                            value={anthropicBaseUrl}
                                            fullWidth
                                            size="small"
                                            slotProps={{
                                                input: {
                                                    readOnly: true,
                                                    endAdornment: (
                                                        <Stack direction="row" spacing={0.5}>
                                                            <IconButton
                                                                onClick={() => copyToClipboard(anthropicBaseUrl, 'Anthropic Base URL')}
                                                                size="small"
                                                                title="Copy Anthropic Base URL"
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
                                                                title="Copy Anthropic cURL Example"
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
                                    Authentication
                                </Typography>

                                <TextField
                                    label="API Key"
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
                                                        title={showToken ? 'Hide token' : 'Show token'}
                                                    >
                                                        <Typography variant="caption">
                                                            {showToken ? '👁️' : '👁️‍🗨️'}
                                                        </Typography>
                                                    </IconButton>
                                                    <IconButton
                                                        onClick={() => copyToClipboard(token, 'API Key')}
                                                        size="small"
                                                        title="Copy Token"
                                                    >
                                                        <CopyIcon fontSize="small" />
                                                    </IconButton>
                                                    <IconButton
                                                        onClick={generateToken}
                                                        size="small"
                                                        title="Generate New Token"
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