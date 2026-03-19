import React, { useState, useEffect, memo } from 'react';
import {
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Button,
    Box,
    Typography,
    Chip,
    LinearProgress,
    Alert,
    Paper,
    IconButton,
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Tooltip,
} from '@mui/material';
import {
    CheckCircle as CheckIcon,
    Error as ErrorIcon,
    ExpandMore as ExpandMoreIcon,
    Speed as SpeedIcon,
    Token as TokenIcon,
    Build as ToolIcon,
    ContentCopy as CopyIcon,
    Refresh as RefreshIcon,
} from '@mui/icons-material';
import { useTheme } from '@mui/material/styles';
import type { ProbeV2TestMode, ProbeV2TargetType } from '@/types/probe-v2.ts';

// Get user auth token from localStorage
const getUserAuthToken = (): string | null => {
    return localStorage.getItem('user_auth_token');
};

interface ProbeV2DialogProps {
    open: boolean;
    onClose: () => void;
    targetType: ProbeV2TargetType;
    targetId: string;
    targetName: string;
    scenario?: string;
    model?: string;
    testMode: ProbeV2TestMode;
}

const TEST_MODE_LABELS: Record<ProbeV2TestMode, string> = {
    simple: 'Direct Request',
    streaming: 'Streaming Request',
    tool: 'Tool Calling',
};

const TEST_MODE_ICONS: Record<ProbeV2TestMode, React.ReactNode> = {
    simple: <SpeedIcon fontSize="small" />,
    streaming: <SpeedIcon fontSize="small" />,
    tool: <ToolIcon fontSize="small" />,
};

// Preset messages
const getDefaultMessage = (mode: ProbeV2TestMode): string => {
    switch (mode) {
        case 'tool':
            return 'Please use the add_numbers tool to calculate 123 + 456.';
        default:
            return 'Hello, this is a test message. Please respond with a short greeting.';
    }
};

// Status Response Card Component
const StatusResponseCard = memo(({
    result,
    isExpanded,
    onToggleDetails,
    theme
}: {
    result: ProbeV2Response;
    isExpanded: boolean;
    onToggleDetails: () => void;
    theme: any;
}) => (
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
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                    <Box>
                        {result.success ?
                            <CheckIcon sx={{ color: theme.palette.success.main, fontSize: 24 }} /> :
                            <ErrorIcon sx={{ color: theme.palette.error.main, fontSize: 24 }} />
                        }
                    </Box>
                    <Box>
                        <Typography variant="subtitle2" fontWeight={600} color="text.primary">
                            {result.success ? 'Success' : 'Failed'}
                        </Typography>
                        {result.data && (
                            <Box sx={{ display: 'flex', gap: 1.5, mt: 0.5, flexWrap: 'wrap' }}>
                                {result.data.latency_ms !== undefined && result.data.latency_ms > 0 && (
                                    <Chip
                                        icon={<SpeedIcon sx={{ fontSize: 14 }} />}
                                        label={`${result.data.latency_ms}ms`}
                                        size="small"
                                        variant="filled"
                                        sx={{ height: 24, minWidth: 'auto' }}
                                    />
                                )}
                                {result.data.usage && (
                                    <Chip
                                        icon={<TokenIcon sx={{ fontSize: 14 }} />}
                                        label={`${result.data.usage.total_tokens} tokens`}
                                        size="small"
                                        variant="outlined"
                                        sx={{ height: 24, minWidth: 'auto' }}
                                    />
                                )}
                                {result.data.tool_calls && result.data.tool_calls.length > 0 && (
                                    <Chip
                                        icon={<ToolIcon sx={{ fontSize: 14 }} />}
                                        label={`${result.data.tool_calls.length} tool calls`}
                                        size="small"
                                        variant="outlined"
                                        sx={{ height: 24, minWidth: 'auto' }}
                                    />
                                )}
                            </Box>
                        )}
                    </Box>
                </Box>
                <Typography variant="body2" color="text.secondary" sx={{ fontSize: '0.8rem' }}>
                    details
                </Typography>
            </Box>
        </AccordionSummary>
        <AccordionDetails sx={{ p: 0 }}>
            <Box>
                {/* Response Content */}
                {result.data?.content && (
                    <Box sx={{ p: 2, bgcolor: 'background.paper' }}>
                        <Typography variant="body2" sx={{ fontWeight: 600, mb: 1, color: 'primary.main' }}>
                            Response
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
                            <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', margin: 0 }}>
                                {result.data.content}
                            </pre>
                        </Paper>
                    </Box>
                )}

                {/* Tool Calls */}
                {result.data?.tool_calls && result.data.tool_calls.length > 0 && (
                    <Box sx={{ p: 2, bgcolor: 'info.50' }}>
                        <Typography variant="body2" sx={{ fontWeight: 600, mb: 1, color: 'primary.main' }}>
                            Tool Calls
                        </Typography>
                        {result.data.tool_calls.map((tc, index: number) => (
                            <Paper
                                key={index}
                                variant="outlined"
                                sx={{ p: 2, mb: 1, bgcolor: 'background.paper' }}
                            >
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                                    <Typography variant="body2" fontWeight="bold" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                        {tc.name}
                                    </Typography>
                                    {tc.id && (
                                        <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: 'text.secondary' }}>
                                            ID: {tc.id}
                                        </Typography>
                                    )}
                                </Box>
                                <Typography
                                    variant="caption"
                                    component="pre"
                                    sx={{
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-word',
                                        fontFamily: 'monospace',
                                        fontSize: '0.75rem',
                                    }}
                                >
                                    {tc.arguments && Object.keys(tc.arguments).length > 0
                                        ? JSON.stringify(tc.arguments, null, 2)
                                        : '(no arguments)'}
                                </Typography>
                            </Paper>
                        ))}
                    </Box>
                )}

                {/* Token Usage */}
                {result.data?.usage && (
                    <Box sx={{ p: 2, borderTop: `1px solid ${theme.palette.divider}` }}>
                        <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500, display: 'block', mb: 1 }}>
                            Token Usage
                        </Typography>
                        <Box sx={{ display: 'flex', gap: 2 }}>
                            <Typography variant="body2" sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>
                                Prompt: {result.data.usage.prompt_tokens}
                            </Typography>
                            <Typography variant="body2" sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>
                                Completion: {result.data.usage.completion_tokens}
                            </Typography>
                            <Typography variant="body2" sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>
                                Total: {result.data.usage.total_tokens}
                            </Typography>
                        </Box>
                    </Box>
                )}

                {/* Request URL (debug info) */}
                {result.data?.request_url && (
                    <Box sx={{ p: 2, borderTop: `1px solid ${theme.palette.divider}` }}>
                        <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500, display: 'block', mb: 0.5 }}>
                            Request URL
                        </Typography>
                        <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', wordBreak: 'break-all' }}>
                            {result.data.request_url}
                        </Typography>
                    </Box>
                )}
            </Box>
        </AccordionDetails>
    </Accordion>
));

// Error Details Component
const ErrorDetails = memo(({ result }: { result: ProbeV2Response }) => (
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
        {result.error?.type && (
            <Typography variant="caption" sx={{ mt: 1, color: 'text.secondary' }}>
                Type: {result.error.type}
            </Typography>
        )}
    </Alert>
));

interface ProbeV2Response {
    success: boolean;
    error?: {
        message: string;
        type: string;
    };
    data?: {
        content?: string;
        tool_calls?: ProbeV2ToolCall[];
        usage?: {
            prompt_tokens: number;
            completion_tokens: number;
            total_tokens: number;
        };
        latency_ms: number;
        request_url?: string;
    };
}

interface ProbeV2ToolCall {
    id: string;
    name: string;
    arguments: Record<string, unknown>;
}

export const ProbeV2Dialog: React.FC<ProbeV2DialogProps> = ({
    open,
    onClose,
    targetType,
    targetId,
    targetName,
    scenario,
    model,
    testMode,
}) => {
    const theme = useTheme();
    const [isLoading, setIsLoading] = useState(false);
    const [result, setResult] = useState<ProbeV2Response | null>(null);
    const [detailsExpanded, setDetailsExpanded] = useState(false);
    const [copyTooltipOpen, setCopyTooltipOpen] = useState(false);

    // Reset state when dialog opens
    useEffect(() => {
        if (open) {
            setIsLoading(false);
            setResult(null);
            setDetailsExpanded(false);

            // Auto-start test
            runTest();
        }
    }, [open, testMode]);

    const runTest = async () => {
        setIsLoading(true);
        setResult(null);

        const requestBody = {
            target_type: targetType,
            ...(targetType === 'rule' ? {
                scenario: scenario || 'openai',
                rule_uuid: targetId,
            } : {
                provider_uuid: targetId,
                model: model || '',
            }),
            test_mode: testMode,
            message: getDefaultMessage(testMode),
        };

        try {
            await runProbe(requestBody);
        } catch (err: any) {
            setResult({
                success: false,
                error: {
                    message: err.message || 'Probe failed',
                    type: 'client_error',
                },
            });
        } finally {
            setIsLoading(false);
        }
    };

    const runProbe = async (requestBody: any) => {
        const token = getUserAuthToken();
        const headers: Record<string, string> = {
            'Content-Type': 'application/json',
        };
        if (token) {
            headers['Authorization'] = `Bearer ${token}`;
        }

        const response = await fetch('/api/v2/probe', {
            method: 'POST',
            headers,
            body: JSON.stringify(requestBody),
        });

        if (!response.ok) {
            let errorMessage = `HTTP ${response.status}`;
            try {
                const errorData = await response.json();
                errorMessage = errorData.error?.message || errorMessage;
            } catch (e) {
                // Ignore parse error
            }
            setResult({
                success: false,
                error: {
                    message: errorMessage,
                    type: 'http_error',
                },
            });
            return;
        }

        const data = await response.json();
        setResult(data);
    };

    const getTargetTypeLabel = () => {
        switch (targetType) {
            case 'provider':
                return 'Provider';
            case 'rule':
                return 'Rule';
            default:
                return 'Target';
        }
    };

    const handleReRun = () => {
        runTest();
    };

    const handleCopyResponse = () => {
        if (!result) return;

        const responseText = JSON.stringify(result, null, 2);
        navigator.clipboard.writeText(responseText).then(() => {
            setCopyTooltipOpen(true);
            setTimeout(() => setCopyTooltipOpen(false), 2000);
        });
    };

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="md"
            fullWidth
            PaperProps={{
                sx: { minHeight: '400px' }
            }}
        >
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                    {/*<Typography variant="h6">*/}
                    {/*    Test {getTargetTypeLabel()}*/}
                    {/*</Typography>*/}
                    <Chip
                        label={TEST_MODE_LABELS[testMode]}
                        icon={TEST_MODE_ICONS[testMode]}
                        color="info"
                        size="small"
                    />
                    {model && (
                        <Chip
                            label={`${targetName} | ${model}`}
                            size="small"
                            variant="outlined"
                            sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
                        />
                    )}

                </Box>
                <Box sx={{ display: 'flex', gap: 0.5 }}>
                    {!isLoading && result && (
                        <>
                            <Tooltip
                                title={copyTooltipOpen ? 'Copied!' : 'Copy response'}
                                open={copyTooltipOpen}
                                onClose={() => setCopyTooltipOpen(false)}
                                disableHoverListener
                            >
                                <IconButton
                                    onClick={handleCopyResponse}
                                    size="small"
                                    sx={{ color: 'text.secondary' }}
                                >
                                    <CopyIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                            <Tooltip title="Re-run">
                                <IconButton
                                    onClick={handleReRun}
                                    size="small"
                                    sx={{ color: 'text.secondary' }}
                                >
                                    <RefreshIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        </>
                    )}
                </Box>
            </DialogTitle>

            <DialogContent>
                {isLoading ? (
                    <Box sx={{ textAlign: 'center', py: 8 }}>
                        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                            Running test...
                        </Typography>
                        <LinearProgress sx={{ height: 6, borderRadius: 3 }} />
                    </Box>
                ) : result ? (
                    <Box>
                        {result.success ? (
                            <StatusResponseCard
                                result={result}
                                isExpanded={detailsExpanded}
                                onToggleDetails={() => setDetailsExpanded(!detailsExpanded)}
                                theme={theme}
                            />
                        ) : (
                            <ErrorDetails result={result} />
                        )}
                    </Box>
                ) : (
                    <Box sx={{ textAlign: 'center', py: 8 }}>
                        <Typography variant="body2" color="text.secondary">
                            Initializing test...
                        </Typography>
                    </Box>
                )}
            </DialogContent>
        </Dialog>
    );
};

export default ProbeV2Dialog;
