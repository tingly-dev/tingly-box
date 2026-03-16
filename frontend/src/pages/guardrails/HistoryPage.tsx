import { useEffect, useState } from 'react';
import {
    Alert,
    Box,
    Button,
    Chip,
    Divider,
    Stack,
    Typography,
} from '@mui/material';
import { History as HistoryIcon, Refresh as RefreshIcon, DeleteOutline } from '@mui/icons-material';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

type GuardrailsHistoryEntry = {
    time: string;
    scenario: string;
    model: string;
    provider: string;
    direction: string;
    phase: string;
    verdict: string;
    block_message?: string;
    preview?: string;
    command_name?: string;
    reasons?: Array<{ rule_id?: string; rule_name?: string; reason?: string }>;
};

const GuardrailsHistoryPage = () => {
    const [loading, setLoading] = useState(true);
    const [entries, setEntries] = useState<GuardrailsHistoryEntry[]>([]);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

    const loadHistory = async () => {
        try {
            setLoading(true);
            const result = await api.getGuardrailsHistory();
            setEntries(Array.isArray(result?.data) ? result.data : []);
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to load guardrails history.' });
            setEntries([]);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadHistory();
    }, []);

    const handleClear = async () => {
        try {
            const result = await api.clearGuardrailsHistory();
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to clear history.' });
                return;
            }
            setEntries([]);
            setActionMessage({ type: 'success', text: 'Guardrails history cleared.' });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to clear history.' });
        }
    };

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Interception History"
                    subtitle="Recent Guardrails blocks captured in memory for debugging and policy tuning."
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Button variant="outlined" startIcon={<RefreshIcon />} onClick={loadHistory}>
                                Refresh
                            </Button>
                            <Button variant="outlined" color="error" startIcon={<DeleteOutline />} onClick={handleClear}>
                                Clear
                            </Button>
                        </Stack>
                    }
                >
                    <Stack spacing={2}>
                        {actionMessage && (
                            <Alert severity={actionMessage.type} onClose={() => setActionMessage(null)}>
                                {actionMessage.text}
                            </Alert>
                        )}
                        <Typography variant="body2" color="text.secondary">
                            This view only shows recent in-memory events. It is intended for debugging current Guardrails behavior, not long-term audit storage.
                        </Typography>
                    </Stack>
                </UnifiedCard>

                <UnifiedCard title={`Events (${entries.length})`} size="full">
                    <Stack spacing={1.5}>
                        {entries.length === 0 && (
                            <Box sx={{ py: 6, textAlign: 'center' }}>
                                <HistoryIcon sx={{ fontSize: 40, color: 'text.disabled', mb: 1 }} />
                                <Typography variant="body2" color="text.secondary">
                                    No Guardrails interception events recorded yet.
                                </Typography>
                            </Box>
                        )}
                        {entries.map((entry, index) => (
                            <Box key={`${entry.time}-${index}`} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                                <Stack spacing={1.25}>
                                    <Stack direction={{ xs: 'column', lg: 'row' }} spacing={1} justifyContent="space-between">
                                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} useFlexGap flexWrap="wrap">
                                            <Chip size="small" label={entry.verdict} color={entry.verdict === 'block' ? 'error' : 'default'} />
                                            <Chip size="small" label={`phase: ${entry.phase}`} variant="outlined" />
                                            <Chip size="small" label={`scenario: ${entry.scenario || 'unknown'}`} variant="outlined" />
                                            {entry.command_name && <Chip size="small" label={`tool: ${entry.command_name}`} variant="outlined" />}
                                        </Stack>
                                        <Typography variant="caption" color="text.secondary">
                                            {new Date(entry.time).toLocaleString()}
                                        </Typography>
                                    </Stack>
                                    <Typography variant="body2" color="text.secondary">
                                        model: {entry.model || 'unknown'} | provider: {entry.provider || 'unknown'} | direction: {entry.direction || 'unknown'}
                                    </Typography>
                                    {entry.block_message && (
                                        <Typography variant="body2">
                                            {entry.block_message}
                                        </Typography>
                                    )}
                                    {entry.preview && (
                                        <Box sx={{ p: 1.5, borderRadius: 1.5, bgcolor: 'action.hover' }}>
                                            <Typography variant="caption" color="text.secondary">
                                                Content Preview
                                            </Typography>
                                            <Typography variant="body2" sx={{ mt: 0.5, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                                                {entry.preview}
                                            </Typography>
                                        </Box>
                                    )}
                                    {entry.reasons && entry.reasons.length > 0 && (
                                        <>
                                            <Divider />
                                            <Stack spacing={0.75}>
                                                {entry.reasons.map((reason, reasonIndex) => (
                                                    <Typography key={`${reason.rule_id || 'reason'}-${reasonIndex}`} variant="body2" color="text.secondary">
                                                        {(reason.rule_name || reason.rule_id || 'Rule') + ': ' + (reason.reason || 'matched')}
                                                    </Typography>
                                                ))}
                                            </Stack>
                                        </>
                                    )}
                                </Stack>
                            </Box>
                        ))}
                    </Stack>
                </UnifiedCard>
            </Stack>
        </PageLayout>
    );
};

export default GuardrailsHistoryPage;
