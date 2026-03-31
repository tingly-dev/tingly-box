import { Fragment, useEffect, useMemo, useState } from 'react';
import {
    Alert,
    Box,
    Button,
    Chip,
    Collapse,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Paper,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
} from '@mui/material';
import {
    DeleteOutline,
    History as HistoryIcon,
    KeyboardArrowDown,
    KeyboardArrowUp,
    Refresh as RefreshIcon,
} from '@mui/icons-material';
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
    credential_refs?: string[];
    credential_names?: string[];
    alias_hits?: string[];
    reasons?: Array<{ policy_id?: string; policy_name?: string; reason?: string }>;
};

const VERDICT_OPTIONS = [
    { value: 'all', label: 'All' },
    { value: 'allow', label: 'allow' },
    { value: 'review', label: 'review' },
    { value: 'block', label: 'block' },
    { value: 'mask', label: 'mask' },
] as const;
const TIME_OPTIONS = [
    { value: 'all', label: 'All Time' },
    { value: '1h', label: '1 Hour' },
    { value: '24h', label: '24 Hours' },
    { value: '7d', label: '7 Days' },
] as const;

type TimeFilter = (typeof TIME_OPTIONS)[number]['value'];

const verdictColor = (verdict: string) => {
    switch (verdict) {
        case 'block':
            return 'error';
        case 'review':
            return 'warning';
        case 'mask':
            return 'secondary';
        case 'allow':
            return 'success';
        default:
            return 'default';
    }
};

const compactList = (values?: string[]) => {
    if (!values || values.length === 0) return '-';
    if (values.length === 1) return values[0];
    return `${values[0]} +${values.length - 1}`;
};

const formatTimestamp = (timestamp: string) => {
    try {
        return new Date(timestamp).toLocaleString();
    } catch {
        return timestamp;
    }
};

const toggleVerdict = (values: string[], target: string) => {
    if (target === 'all') return [];
    const next = values.includes(target)
        ? values.filter((value) => value !== target)
        : [...values, target];
    return next;
};

const withinTimeWindow = (timestamp: string, timeFilter: TimeFilter) => {
    if (timeFilter === 'all') return true;
    const entryTime = new Date(timestamp).getTime();
    if (Number.isNaN(entryTime)) return true;

    const now = Date.now();
    const diff = now - entryTime;
    if (timeFilter === '1h') return diff <= 60 * 60 * 1000;
    if (timeFilter === '24h') return diff <= 24 * 60 * 60 * 1000;
    if (timeFilter === '7d') return diff <= 7 * 24 * 60 * 60 * 1000;
    return true;
};

const GuardrailsHistoryPage = () => {
    const [loading, setLoading] = useState(true);
    const [entries, setEntries] = useState<GuardrailsHistoryEntry[]>([]);
    const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
    const [selectedVerdicts, setSelectedVerdicts] = useState<string[]>([]);
    const [timeFilter, setTimeFilter] = useState<TimeFilter>('all');
    const [clearConfirmOpen, setClearConfirmOpen] = useState(false);
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
            setExpandedRows(new Set());
            setClearConfirmOpen(false);
            setActionMessage({ type: 'success', text: 'Guardrails history cleared.' });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to clear history.' });
        }
    };

    const toggleRow = (index: number) => {
        const next = new Set(expandedRows);
        if (next.has(index)) {
            next.delete(index);
        } else {
            next.add(index);
        }
        setExpandedRows(next);
    };

    const filteredEntries = useMemo(() => {
        return entries.filter((entry) => {
            if (selectedVerdicts.length > 0 && !selectedVerdicts.includes(entry.verdict)) {
                return false;
            }
            if (!withinTimeWindow(entry.time, timeFilter)) {
                return false;
            }
            return true;
        });
    }, [entries, selectedVerdicts, timeFilter]);

    const rowCountLabel = useMemo(() => `Events (${filteredEntries.length})`, [filteredEntries.length]);
    const filtersActive = selectedVerdicts.length > 0 || timeFilter !== 'all';

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Event History"
                    subtitle="Recent Guardrails activity. Expand a row only when you need the full context."
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Button variant="outlined" startIcon={<RefreshIcon />} onClick={loadHistory}>
                                Refresh
                            </Button>
                            <Button variant="outlined" color="error" startIcon={<DeleteOutline />} onClick={() => setClearConfirmOpen(true)}>
                                Clear
                            </Button>
                        </Stack>
                    }
                >
                    {actionMessage && (
                        <Alert severity={actionMessage.type} onClose={() => setActionMessage(null)}>
                            {actionMessage.text}
                        </Alert>
                    )}
                </UnifiedCard>

                <UnifiedCard title={rowCountLabel} size="full">
                    <Stack spacing={2}>
                        <Stack direction={{ xs: 'column', lg: 'row' }} spacing={2} alignItems={{ xs: 'flex-start', lg: 'center' }} justifyContent="space-between">
                            <Stack spacing={1.25}>
                                <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} alignItems={{ xs: 'flex-start', md: 'center' }}>
                                    <Typography variant="caption" color="text.secondary" sx={{ minWidth: 44 }}>
                                        Verdict
                                    </Typography>
                                    <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
                                        {VERDICT_OPTIONS.map((option) => {
                                            const selected = option.value === 'all' ? selectedVerdicts.length === 0 : selectedVerdicts.includes(option.value);
                                            return (
                                                <Chip
                                                    key={option.value}
                                                    size="small"
                                                    label={option.label}
                                                    color={option.value === 'all' ? (selected ? 'primary' : 'default') : (selected ? verdictColor(option.value) : 'default')}
                                                    variant={selected ? 'filled' : 'outlined'}
                                                    onClick={() => setSelectedVerdicts((current) => toggleVerdict(current, option.value))}
                                                    sx={{ textTransform: option.value === 'all' ? 'none' : 'capitalize' }}
                                                />
                                            );
                                        })}
                                    </Stack>
                                </Stack>
                                <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} alignItems={{ xs: 'flex-start', md: 'center' }}>
                                    <Typography variant="caption" color="text.secondary" sx={{ minWidth: 44 }}>
                                        Time
                                    </Typography>
                                    <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
                                        {TIME_OPTIONS.map((option) => (
                                            <Chip
                                                key={option.value}
                                                size="small"
                                                label={option.label}
                                                color={timeFilter === option.value ? 'primary' : 'default'}
                                                variant={timeFilter === option.value ? 'filled' : 'outlined'}
                                                onClick={() => setTimeFilter(option.value)}
                                            />
                                        ))}
                                    </Stack>
                                </Stack>
                            </Stack>
                            {filtersActive && (
                                <Button
                                    size="small"
                                    variant="text"
                                    onClick={() => {
                                        setSelectedVerdicts([]);
                                        setTimeFilter('all');
                                    }}
                                >
                                    Reset
                                </Button>
                            )}
                        </Stack>

                        <TableContainer component={Paper} variant="outlined">
                            <Table size="small">
                                <TableHead>
                                    <TableRow>
                                        <TableCell padding="checkbox" />
                                        <TableCell sx={{ width: 180 }}>Time</TableCell>
                                        <TableCell sx={{ width: 110 }}>Verdict</TableCell>
                                        <TableCell sx={{ width: 140 }}>Phase</TableCell>
                                        <TableCell sx={{ width: 150 }}>Scenario</TableCell>
                                        <TableCell sx={{ width: 150 }}>Policy Input</TableCell>
                                        <TableCell>Summary</TableCell>
                                    </TableRow>
                                </TableHead>
                                <TableBody>
                                    {filteredEntries.length === 0 ? (
                                        <TableRow>
                                            <TableCell colSpan={7} align="center" sx={{ py: 6 }}>
                                                <HistoryIcon sx={{ fontSize: 36, color: 'text.disabled', mb: 1 }} />
                                                <Typography variant="body2" color="text.secondary">
                                                    {entries.length === 0 ? 'No Guardrails events recorded yet.' : 'No events match the current filters.'}
                                                </Typography>
                                            </TableCell>
                                        </TableRow>
                                    ) : (
                                        filteredEntries.map((entry, index) => {
                                            const summary = entry.block_message || entry.preview || entry.command_name || '-';
                                            const inputLabel = entry.command_name || compactList(entry.credential_names);
                                            return (
                                                <Fragment key={`${entry.time}-${index}`}>
                                                    <TableRow hover sx={{ cursor: 'pointer' }} onClick={() => toggleRow(index)}>
                                                        <TableCell padding="checkbox">
                                                            <IconButton size="small">
                                                                {expandedRows.has(index) ? <KeyboardArrowUp /> : <KeyboardArrowDown />}
                                                            </IconButton>
                                                        </TableCell>
                                                        <TableCell sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>
                                                            {formatTimestamp(entry.time)}
                                                        </TableCell>
                                                        <TableCell>
                                                            <Chip
                                                                size="small"
                                                                label={entry.verdict}
                                                                color={verdictColor(entry.verdict)}
                                                                variant={entry.verdict === 'allow' ? 'outlined' : 'filled'}
                                                                sx={{ height: 22, textTransform: 'capitalize' }}
                                                            />
                                                        </TableCell>
                                                        <TableCell sx={{ fontSize: '0.8rem' }}>{entry.phase || '-'}</TableCell>
                                                        <TableCell sx={{ fontSize: '0.8rem' }}>{entry.scenario || '-'}</TableCell>
                                                        <TableCell sx={{ fontSize: '0.8rem', color: 'text.secondary' }}>{inputLabel}</TableCell>
                                                        <TableCell sx={{ fontSize: '0.8rem' }}>
                                                            <Typography
                                                                variant="body2"
                                                                sx={{
                                                                    fontSize: '0.8rem',
                                                                    display: '-webkit-box',
                                                                    WebkitLineClamp: 1,
                                                                    WebkitBoxOrient: 'vertical',
                                                                    overflow: 'hidden',
                                                                    wordBreak: 'break-word',
                                                                }}
                                                            >
                                                                {summary}
                                                            </Typography>
                                                        </TableCell>
                                                    </TableRow>
                                                    <TableRow>
                                                        <TableCell colSpan={7} sx={{ p: 0, border: 'none' }}>
                                                            <Collapse in={expandedRows.has(index)} timeout="auto" unmountOnExit>
                                                                <Box sx={{ px: 2, py: 2, bgcolor: 'action.hover' }}>
                                                                    <Stack spacing={2}>
                                                                        <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                                                                            <Typography variant="body2" color="text.secondary">
                                                                                provider: {entry.provider || 'unknown'}
                                                                            </Typography>
                                                                            <Typography variant="body2" color="text.secondary">
                                                                                model: {entry.model || 'unknown'}
                                                                            </Typography>
                                                                            <Typography variant="body2" color="text.secondary">
                                                                                direction: {entry.direction || 'unknown'}
                                                                            </Typography>
                                                                        </Stack>

                                                                        {entry.block_message && (
                                                                            <Box>
                                                                                <Typography variant="caption" color="text.secondary">
                                                                                    Message
                                                                                </Typography>
                                                                                <Typography variant="body2" sx={{ mt: 0.5, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                                                                                    {entry.block_message}
                                                                                </Typography>
                                                                            </Box>
                                                                        )}

                                                                        {entry.preview && (
                                                                            <Box>
                                                                                <Typography variant="caption" color="text.secondary">
                                                                                    Preview
                                                                                </Typography>
                                                                                <Typography variant="body2" sx={{ mt: 0.5, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                                                                                    {entry.preview}
                                                                                </Typography>
                                                                            </Box>
                                                                        )}

                                                                        {entry.credential_names && entry.credential_names.length > 0 && (
                                                                            <Box>
                                                                                <Typography variant="caption" color="text.secondary">
                                                                                    Credential Names
                                                                                </Typography>
                                                                                <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap" sx={{ mt: 0.75 }}>
                                                                                    {entry.credential_names.map((name) => (
                                                                                        <Chip key={name} size="small" label={name} variant="outlined" />
                                                                                    ))}
                                                                                </Stack>
                                                                            </Box>
                                                                        )}

                                                                        {entry.alias_hits && entry.alias_hits.length > 0 && (
                                                                            <Box>
                                                                                <Typography variant="caption" color="text.secondary">
                                                                                    Alias Hits
                                                                                </Typography>
                                                                                <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap" sx={{ mt: 0.75 }}>
                                                                                    {entry.alias_hits.map((alias) => (
                                                                                        <Chip key={alias} size="small" label={alias} variant="outlined" />
                                                                                    ))}
                                                                                </Stack>
                                                                            </Box>
                                                                        )}

                                                                        {entry.reasons && entry.reasons.length > 0 && (
                                                                            <Box>
                                                                                <Typography variant="caption" color="text.secondary">
                                                                                    Matched Policies
                                                                                </Typography>
                                                                                <Stack spacing={0.75} sx={{ mt: 0.75 }}>
                                                                                    {entry.reasons.map((reason, reasonIndex) => (
                                                                                        <Typography
                                                                                            key={`${reason.policy_id || 'reason'}-${reasonIndex}`}
                                                                                            variant="body2"
                                                                                            color="text.secondary"
                                                                                        >
                                                                                            {(reason.policy_name || reason.policy_id || 'Policy') + ': ' + (reason.reason || 'matched')}
                                                                                        </Typography>
                                                                                    ))}
                                                                                </Stack>
                                                                            </Box>
                                                                        )}
                                                                    </Stack>
                                                                </Box>
                                                            </Collapse>
                                                        </TableCell>
                                                    </TableRow>
                                                </Fragment>
                                            );
                                        })
                                    )}
                                </TableBody>
                            </Table>
                        </TableContainer>
                    </Stack>
                </UnifiedCard>
            </Stack>

            <Dialog open={clearConfirmOpen} onClose={() => setClearConfirmOpen(false)} disableRestoreFocus>
                <DialogTitle>Clear Event History</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary">
                        This will remove all local Guardrails event history.
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setClearConfirmOpen(false)}>Cancel</Button>
                    <Button color="error" variant="contained" onClick={handleClear}>Clear</Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default GuardrailsHistoryPage;
