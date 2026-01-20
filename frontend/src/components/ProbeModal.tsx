import {
    CheckCircle as CheckIcon,
    Logout as CompletionIcon,
    Error as ErrorIcon,
    ExpandMore as ExpandMoreIcon,
    Login as PromptIcon,
    PlayArrow as TestIcon,
    Timer as TimerIcon,
    Token as TokenIcon
} from '@mui/icons-material';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    LinearProgress,
    Paper,
    Stack,
    Typography,
    useTheme
} from '@mui/material';
import React, { useState, memo } from 'react';
import UnifiedCard from './UnifiedCard';
import type { ProbeResponse, ErrorDetail } from '../client';

interface ProbeProps {
    provider: any;
    model: any;
    isProbing?: boolean;
    probeResult?: ProbeResponse | null;
    onToggleDetails?: () => void;
    detailsExpanded?: boolean;
}

interface StatusResponseCardProps {
    result: ProbeResponse;
    isExpanded: boolean;
    onToggleDetails: () => void;
    theme: any;
}

// Move component outside to avoid "async component" error
const StatusResponseCard = memo(({ result, isExpanded, onToggleDetails, theme }: StatusResponseCardProps) => (
    <Accordion
        expanded={isExpanded}
        onChange={(_, isExpanded) => onToggleDetails()}
        disableGutters
        elevation={0}
        sx={{
            '&:before': { display: 'none' },
            border: `1px solid ${result.success ? theme.palette.success.main : theme.palette.error.main}30`,
            borderRadius: 2,
            overflow: 'hidden'
        }}
    >
        <AccordionSummary
            expandIcon={<ExpandMoreIcon />}
            sx={{
                px: 2,
                py: 1.5,
                bgcolor: `${result.success ? theme.palette.success.main : theme.palette.error.main}04`,
                '&:hover': { bgcolor: `${result.success ? theme.palette.success.main : theme.palette.error.main}08` }
            }}
        >
            <Box sx={{ width: '100%', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Stack direction="row" alignItems="center" spacing={2}>
                    <Box>
                        {result.success ?
                            <CheckIcon sx={{ color: theme.palette.success.main, fontSize: 24 }} /> :
                            <ErrorIcon sx={{ color: theme.palette.error.main, fontSize: 24 }} />
                        }
                    </Box>
                    <Stack spacing={1}>
                        <Typography variant="subtitle2" fontWeight={600} color="text.primary">
                            {result.success ? 'Success' : 'Failed'}
                        </Typography>
                        {result.data && (
                            <Stack direction="row" spacing={1.5} alignItems="center" sx={{ flexWrap: 'nowrap' }}>
                                <Chip
                                    label={result.data.request.provider}
                                    size="small"
                                    variant="filled"
                                    color="primary"
                                    sx={{
                                        fontFamily: 'monospace',
                                        height: 24,
                                        minWidth: 'auto'
                                    }}
                                />
                                <Chip
                                    label={result.data.request.model}
                                    size="small"
                                    variant="outlined"
                                    sx={{
                                        fontFamily: 'monospace',
                                        height: 24,
                                        minWidth: 'auto'
                                    }}
                                />

                                <Stack direction="row" spacing={1.5}>
                                    <Stack direction="row" alignItems="center" spacing={0.5}>
                                        <PromptIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                                        <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                                            Prompt:{result.data.usage?.prompt_tokens || 0}
                                        </Typography>
                                    </Stack>
                                    <Stack direction="row" alignItems="center" spacing={0.5}>
                                        <CompletionIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                                        <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                                            Completion:{result.data.usage?.completion_tokens || 0}
                                        </Typography>
                                    </Stack>
                                </Stack>
                                <Stack direction="row" alignItems="center" spacing={0.5}>
                                    <TokenIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                                    <Typography variant="body2" color="text.secondary">
                                        {result.data.usage?.total_tokens || 0} tokens
                                    </Typography>
                                </Stack>
                                <Stack direction="row" alignItems="center" spacing={0.5}>
                                    <TimerIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                                    <Typography variant="body2" color="text.secondary">
                                        {result.data.usage?.time_cost || 0}ms
                                    </Typography>
                                </Stack>
                            </Stack>
                        )}
                    </Stack>
                </Stack>
                <Typography variant="body2" color="text.secondary" sx={{ fontSize: '0.8rem' }}>
                    details
                </Typography>
            </Box>
        </AccordionSummary>
        <AccordionDetails sx={{ p: 0 }}>
            <Box>
                <Box sx={{ p: 2, bgcolor: 'background.paper' }}>
                    <Typography variant="body2" sx={{ fontWeight: 600, mb: 1, color: 'primary.main' }}>
                        Request
                    </Typography>
                    <Paper
                        variant="outlined"
                        sx={{
                            p: 2,
                            fontFamily: 'monospace',
                            fontSize: '0.8rem',
                            bgcolor: 'grey.50',
                            maxHeight: 200,
                            overflow: 'auto',
                            borderRadius: 1.5
                        }}
                    >
                        {(result.data?.request?.messages?.[0] as any)?.content || 'No request content'}
                    </Paper>
                </Box>

                <Box sx={{ p: 2, bgcolor: result.success ? 'success.50' : 'error.50' }}>
                    <Typography variant="body2" sx={{
                        fontWeight: 600,
                        mb: 1,
                        color: result.success ? 'success.dark' : 'error.dark'
                    }}>
                        Response
                    </Typography>
                    <Paper
                        variant="outlined"
                        sx={{
                            p: 2,
                            fontFamily: 'monospace',
                            fontSize: '0.8rem',
                            bgcolor: 'background.paper',
                            maxHeight: 200,
                            overflow: 'auto',
                            borderRadius: 1.5,
                            borderColor: result.success ? 'success.light' : 'error.light'
                        }}
                    >
                        {result.data?.response?.content || result.data?.response?.error || 'No response content'}
                    </Paper>
                </Box>

                <Stack direction="row" spacing={3} sx={{ p: 2, borderTop: `1px solid ${theme.palette.divider}` }}>
                    <Box>
                        <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
                            Finish Reason
                        </Typography>
                        <Stack direction="row" spacing={2} sx={{ mt: 0.5 }}>
                            <Typography variant="body2" sx={{
                                fontSize: '0.8rem', fontFamily: 'monospace'
                            }}>
                                {result.data?.response?.finish_reason || 'unknown'}
                            </Typography>
                        </Stack>
                    </Box>
                    <Box>
                        <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
                            Token Usage
                        </Typography>
                        <Stack direction="row" spacing={2} sx={{ mt: 0.5 }}>
                            <Typography variant="body2" sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>
                                Prompt: {result.data?.usage?.prompt_tokens || 0}
                            </Typography>
                            <Typography variant="body2" sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>
                                Completion: {result.data?.usage?.completion_tokens || 0}
                            </Typography>
                            <Typography variant="body2" sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>
                                Total: {result.data?.usage?.total_tokens || 0}
                            </Typography>
                        </Stack>
                    </Box>
                </Stack>
            </Box>
        </AccordionDetails>
    </Accordion>
));

interface ErrorDetailsProps {
    result: ProbeResponse;
}

// Move component outside to avoid "async component" error
const ErrorDetails = memo(({ result }: ErrorDetailsProps) => (
    <Alert
        severity="error"
        variant="outlined"
        sx={{
            mt: 2,
            borderRadius: 2,
            '& .MuiAlert-message': { width: '100%' }
        }}
    >
        <Typography variant="body2" sx={{ fontWeight: 500, mb: 1 }}>
            Error Details
        </Typography>
        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
            {result.error?.message || 'Unknown error occurred'}
        </Typography>
    </Alert>
));

const ProbeModal = ({ provider, model, isProbing = false, probeResult = null, onToggleDetails, detailsExpanded = false }: ProbeProps) => {
    const theme = useTheme();

    // Internal state for details expansion if not controlled externally
    const [internalDetailsExpanded, setInternalDetailsExpanded] = useState(false);
    const isExpanded = detailsExpanded !== undefined ? detailsExpanded : internalDetailsExpanded;

    const handleToggleDetails = () => {
        if (onToggleDetails) {
            onToggleDetails();
        } else {
            setInternalDetailsExpanded(!isExpanded);
        }
    };

    return (
        <Box sx={{ width: '100%', maxWidth: 900 }}>
            {probeResult && !isProbing ? (
                <Box>
                    {probeResult.data && <StatusResponseCard result={probeResult} isExpanded={isExpanded} onToggleDetails={handleToggleDetails} theme={theme} />}
                    {!probeResult.success && probeResult.error && <ErrorDetails result={probeResult} />}
                </Box>
            ) : (
                <Box sx={{ textAlign: 'center', py: 8 }}>
                    {isProbing && (
                        <Box sx={{ mt: 3 }}>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                                Running configuration test...
                            </Typography>
                            <LinearProgress sx={{ height: 6, borderRadius: 3 }} />
                        </Box>
                    )}
                    {!isProbing && !probeResult && (
                        <Typography variant="body2" color="text.secondary">
                            Click "Test Connection" to check the provider and model configuration
                        </Typography>
                    )}
                </Box>
            )}
        </Box>
    );
};

export default memo(ProbeModal);
