import {
    CheckCircle as CheckIcon,
    Error as ErrorIcon,
    Refresh as RefreshIcon,
    PlayArrow as TestIcon
} from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Divider,
    LinearProgress,
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
            content: string;
            model: string;
            provider: string;
            usage: {
                prompt_tokens: number;
                completion_tokens: number;
                total_tokens: number;
            };
            finish_reason: string;
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
    error?: string;
}

const Probe = () => {
    const theme = useTheme();
    const [isProbing, setIsProbing] = useState(false);
    const [probeResult, setProbeResult] = useState<ProbeResponse | null>(null);

    const handleProbe = async () => {
        setIsProbing(true);
        setProbeResult(null);

        try {
            const result = await api.probeRule();

            if (result.success) {
                setProbeResult({
                    success: true,
                    data: result.data
                });
            } else {
                setProbeResult({
                    success: false,
                    error: result.error || 'Unknown error occurred'
                });
            }
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

    return (
        <UnifiedCard
            title="Configuration Probe"
            subtitle="Test rule validation with a sample request"
            size="full"
        >
            <Box sx={{ width: '100%', maxWidth: 800 }}>
                <Stack spacing={3}>

                    {/* Action Button */}
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                        <Button
                            variant="contained"
                            startIcon={isProbing ? <CircularProgress size={16} color="inherit" /> : <TestIcon />}
                            onClick={handleProbe}
                            disabled={isProbing}
                            sx={{ minWidth: 140 }}
                        >
                            {isProbing ? 'Testing...' : 'Run Test'}
                        </Button>

                        {probeResult && (
                            <Button
                                variant="text"
                                startIcon={<RefreshIcon />}
                                onClick={() => {
                                    setProbeResult(null);
                                    handleProbe();
                                }}
                                disabled={isProbing}
                                size="small"
                            >
                                Retry
                            </Button>
                        )}
                    </Box>

                    {/* Loading State */}
                    {isProbing && (
                        <Box>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                Running configuration test...
                            </Typography>
                            <LinearProgress sx={{ height: 2, borderRadius: 1 }} />
                        </Box>
                    )}

                    {/* Results */}
                    {probeResult && !isProbing && (
                        <Alert
                            severity={probeResult.success ? 'success' : 'error'}
                            variant="outlined"
                            sx={{
                                borderRadius: 2,
                                border: `1px solid ${probeResult.success ? theme.palette.success.main : theme.palette.error.main}20`,
                                bgcolor: `${probeResult.success ? theme.palette.success.main : theme.palette.error.main}08`
                            }}
                        >
                            <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 2 }}>
                                {probeResult.success ? <CheckIcon color="success" /> : <ErrorIcon color="error" />}
                                <Typography variant="subtitle2" fontWeight={600}>
                                    {probeResult.success ? 'Test Successful' : 'Test Failed'}
                                </Typography>
                            </Stack>

                            {probeResult.data && probeResult.data.rule_tested && (
                                <Box sx={{ mb: 2 }}>
                                    <Typography variant="body2" sx={{ fontWeight: 500, mb: 1 }}>
                                        Configuration Tested:
                                    </Typography>
                                    <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap', gap: 1 }}>
                                        <Chip
                                            label={probeResult.data.rule_tested.provider}
                                            size="small"
                                            variant="outlined"
                                            sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
                                        />
                                        <Chip
                                            label={probeResult.data.rule_tested.model}
                                            size="small"
                                            color="primary"
                                            variant="outlined"
                                            sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
                                        />
                                    </Stack>
                                </Box>
                            )}

                            {probeResult.success && probeResult.data ? (
                                <>
                                    <Divider sx={{ my: 2 }} />

                                    {/* Request Details */}
                                    <Box sx={{ mb: 2 }}>
                                        <Typography variant="body2" sx={{ fontWeight: 500, mb: 1 }}>
                                            📤 Request Sent:
                                        </Typography>
                                        <Box sx={{
                                            p: 2,
                                            bgcolor: 'grey.50',
                                            borderRadius: 1.5,
                                            fontFamily: 'monospace',
                                            fontSize: '0.8rem',
                                            color: 'text.primary',
                                            border: `1px solid ${theme.palette.divider}`,
                                            maxHeight: 120,
                                            overflow: 'auto'
                                        }}>
                                            {probeResult.data.request.messages[0]?.content}
                                        </Box>
                                    </Box>

                                    {/* Response Details */}
                                    <Box sx={{ mb: 2 }}>
                                        <Typography variant="body2" sx={{ fontWeight: 500, mb: 1 }}>
                                            📥 Response Received:
                                        </Typography>
                                        <Box sx={{
                                            p: 2,
                                            bgcolor: 'grey.50',
                                            borderRadius: 1.5,
                                            fontFamily: 'monospace',
                                            fontSize: '0.8rem',
                                            color: 'text.primary',
                                            border: `1px solid ${theme.palette.divider}`,
                                            maxHeight: 120,
                                            overflow: 'auto'
                                        }}>
                                            {probeResult.data.response.content}
                                        </Box>
                                    </Box>

                                    {/* Test Metrics */}
                                    <Stack direction="row" spacing={3} sx={{ flexWrap: 'wrap' }}>
                                        <Typography variant="caption" color="text.secondary">
                                            ⏱️ {probeResult.data.request.processing_time_ms}ms
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            🔢 {probeResult.data.response.usage.total_tokens} tokens
                                            ({probeResult.data.response.usage.prompt_tokens}→{probeResult.data.response.usage.completion_tokens})
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            ✅ {probeResult.data.response.finish_reason}
                                        </Typography>
                                    </Stack>
                                </>
                            ) : (
                                <Typography variant="body2" color="error.main">
                                    {probeResult.error}
                                </Typography>
                            )}
                        </Alert>
                    )}
                </Stack>
            </Box>
        </UnifiedCard>
    );
};

export default Probe;