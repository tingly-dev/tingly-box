import {
    CheckCircle as CheckIcon,
    Error as ErrorIcon,
    ExpandMore as ExpandMoreIcon,
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
import { useState } from 'react';
import { api } from '../services/api';
import UnifiedCard from './UnifiedCard';

interface ProbeResponse {
    success: boolean;
    data?: {
        request: {
            provider: string;
            model: string;
            timestamp: string;
            processing_time_ms: number;
            messages: Array<{ role: string; content: string }>;
        };
        response: {
            content: string | null;
            model: string;
            provider: string;
            usage: {
                prompt_tokens: number;
                completion_tokens: number;
                total_tokens: number;
            };
            finish_reason: string;
            error?: string;
        };
        rule_tested: {
            name: string;
            provider: string;
            model: string;
            timestamp: string;
        };
        test_result: {
            success: boolean;
            message: string;
        };
    };
    error?: {
        code: string;
        message: string;
        details?: any;
    } | string;
}

const Probe = ({ rule, provider, model }) => {
    const theme = useTheme();
    const [isProbing, setIsProbing] = useState(false);
    const [probeResult, setProbeResult] = useState<ProbeResponse | null>(null);
    const [detailsExpanded, setDetailsExpanded] = useState(false);

    const handleProbe = async () => {
        setIsProbing(true);
        setProbeResult(null);

        try {
            console.log(rule, provider, model)
            const result = await api.probeRule(rule, provider, model);
            setProbeResult(result);
        } catch (error) {
            console.error('Probe error:', error);
            setProbeResult({
                success: false,
                error: (error as Error).message
            });
        } finally {
            setIsProbing(false);
        }
    };

    const StatusMetrics = ({ result }) => (
        <Box sx={{ mb: 3, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Paper
                elevation={0}
                variant="outlined"
                sx={{
                    p: 2,
                    borderRadius: 2,
                    width: '100%',
                    border: `1px solid ${result.success ? theme.palette.success.main : theme.palette.error.main}30`,
                    bgcolor: `${result.success ? theme.palette.success.main : theme.palette.error.main}04`
                }}
            >
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
                            <Stack direction="row" spacing={1} alignItems="center" sx={{ flexWrap: 'nowrap' }}>
                                {result.data.rule_tested && (
                                    <Chip
                                        label={result.data.rule_tested.provider}
                                        size="small"
                                        variant="filled"
                                        color="primary"
                                        sx={{ fontFamily: 'monospace', fontSize: '0.6rem', height: 18, minWidth: 'auto' }}
                                    />
                                )}
                                {result.data.rule_tested && (
                                    <Chip
                                        label={result.data.rule_tested.model}
                                        size="small"
                                        variant="outlined"
                                        sx={{ fontFamily: 'monospace', fontSize: '0.6rem', height: 18, minWidth: 'auto' }}
                                    />
                                )}
                                <Stack direction="row" alignItems="center" spacing={0.3}>
                                    <TimerIcon sx={{ fontSize: 11, color: 'text.secondary' }} />
                                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem', whiteSpace: 'nowrap' }}>
                                        {result.data.request?.processing_time_ms || 0}ms
                                    </Typography>
                                </Stack>
                                <Stack direction="row" alignItems="center" spacing={0.3}>
                                    <TokenIcon sx={{ fontSize: 11, color: 'text.secondary' }} />
                                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem', whiteSpace: 'nowrap' }}>
                                        {result.data.response?.usage?.total_tokens || 0}t
                                    </Typography>
                                </Stack>
                            </Stack>
                        )}
                    </Stack>
                </Stack>
            </Paper>
        </Box>
    );

    const RequestResponseDetails = ({ result }) => (
        <Accordion
            expanded={detailsExpanded}
            onChange={(_, isExpanded) => setDetailsExpanded(isExpanded)}
            disableGutters
            elevation={0}
            sx={{
                '&:before': { display: 'none' },
                border: `1px solid ${theme.palette.divider}`,
                borderRadius: 2,
                overflow: 'hidden'
            }}
        >
            <AccordionSummary
                expandIcon={<ExpandMoreIcon />}
                sx={{
                    px: 2,
                    py: 1.5,
                    bgcolor: 'grey.50',
                    '&:hover': { bgcolor: 'grey.100' }
                }}
            >
                <Typography variant="subtitle2" fontWeight={500}>
                    Request & Response Details
                </Typography>
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
                            {result.data.request?.messages?.[0]?.content || 'No request content'}
                        </Paper>
                    </Box>

                    <Box sx={{ p: 2, bgcolor: result.success ? 'success.50' : 'error.50' }}>
                        <Typography variant="body2" sx={{ fontWeight: 600, mb: 1, color: result.success ? 'success.dark' : 'error.dark' }}>
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
                            {result.data.response?.content || result.data.response?.error || 'No response content'}
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
                                    {result.data.response?.finish_reason || 'unknown'}
                                </Typography>
                            </Stack>
                        </Box>
                        <Box>
                            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
                                Token Usage
                            </Typography>
                            <Stack direction="row" spacing={2} sx={{ mt: 0.5 }}>
                                <Typography variant="body2" sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>
                                    Prompt: {result.data.response?.usage?.prompt_tokens || 0}
                                </Typography>
                                <Typography variant="body2" sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>
                                    Completion: {result.data.response?.usage?.completion_tokens || 0}
                                </Typography>
                                <Typography variant="body2" sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>
                                    Total: {result.data.response?.usage?.total_tokens || 0}
                                </Typography>
                            </Stack>
                        </Box>
                    </Stack>
                </Box>
            </AccordionDetails>
        </Accordion>
    );

    const ErrorDetails = ({ result }) => (
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
                {typeof result.error === 'string'
                    ? result.error
                    : result.error?.message || result.data?.test_result?.message || 'Unknown error occurred'}
            </Typography>
        </Alert>
    );

    return (
        <UnifiedCard
            title="Configuration Probe"
            subtitle="Test rule validation with a sample request"
            size="full"
            rightAction={
                <Button
                    variant="contained"
                    startIcon={isProbing ? <CircularProgress size={16} color="inherit" /> : <TestIcon />}
                    onClick={handleProbe}
                    disabled={isProbing}
                    sx={{ minWidth: 140 }}
                >
                    {isProbing ? 'Testing...' : 'Run Test'}
                </Button>
            }
        >
            <Box sx={{ width: '100%', maxWidth: 900 }}>
                {probeResult && !isProbing ? (
                    <Box>
                        <StatusMetrics result={probeResult} />
                        {probeResult.data && <RequestResponseDetails result={probeResult} />}
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
                    </Box>
                )}
            </Box>
        </UnifiedCard>
    );
};

export default Probe;