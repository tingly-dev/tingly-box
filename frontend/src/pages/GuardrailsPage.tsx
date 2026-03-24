import { useEffect, useMemo, useRef, useState } from 'react';
import {
    Alert,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Grid,
    IconButton,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import {
    FileDownload,
    FileUpload,
    FolderOpen,
    Terminal,
    ArticleOutlined,
    HelpOutline,
} from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

type GuardrailsHistoryEntry = {
    time: string;
    verdict: string;
    phase: string;
    scenario: string;
    alias_hits?: string[];
    credential_names?: string[];
};

const GuardrailsPage = () => {
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [configContent, setConfigContent] = useState('');
    const [policies, setPolicies] = useState<any[]>([]);
    const [historyEntries, setHistoryEntries] = useState<GuardrailsHistoryEntry[]>([]);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [importDialogOpen, setImportDialogOpen] = useState(false);
    const [importText, setImportText] = useState('');
    const [importing, setImporting] = useState(false);

    const loadGuardrails = async () => {
        try {
            setLoading(true);
            const [guardrailsConfig, guardrailsHistory] = await Promise.all([
                api.getGuardrailsConfig(),
                api.getGuardrailsHistory(),
            ]);
            setPolicies(guardrailsConfig?.config?.policies || []);
            setConfigContent(guardrailsConfig?.content || '');
            setHistoryEntries(Array.isArray(guardrailsHistory?.data) ? guardrailsHistory.data : []);
            setLoadError(null);
        } catch (error: any) {
            console.error('Failed to load guardrails config:', error);
            setPolicies([]);
            setConfigContent('');
            setHistoryEntries([]);
            setLoadError('Failed to load guardrails config');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadGuardrails();
    }, []);

    const stats = useMemo(() => {
        const total = policies.length;
        const enabled = policies.filter((item) => item?.enabled !== false).length;
        const disabled = policies.filter((item) => item?.enabled === false).length;
        const resourceAccess = policies.filter((item) => item?.kind === 'resource_access').length;
        const commandExecution = policies.filter((item) => item?.kind === 'command_execution').length;
        const content = policies.filter((item) => item?.kind === 'content').length;
        const blockedEvents = historyEntries.filter((entry) => entry?.verdict === 'block').length;
        const maskedEvents = historyEntries.filter((entry) => entry?.verdict === 'mask').length;
        const reviewedEvents = historyEntries.filter((entry) => entry?.verdict === 'review').length;
        const allowedEvents = historyEntries.filter((entry) => entry?.verdict === 'allow').length;
        return {
            total,
            enabled,
            disabled,
            resourceAccess,
            commandExecution,
            content,
            historyCount: historyEntries.length,
            allowedEvents,
            reviewedEvents,
            blockedEvents,
            maskedEvents,
        };
    }, [historyEntries, policies]);

    const handleImportClick = () => {
        setImportText('');
        setImportDialogOpen(true);
    };

    const handleImportFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) {
            return;
        }
        try {
            const content = await file.text();
            setImportText(content);
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to read config file' });
        } finally {
            e.target.value = '';
        }
    };

    const handleImportSubmit = async () => {
        if (!importText.trim()) {
            setActionMessage({ type: 'error', text: 'Paste config text or choose a file first.' });
            return;
        }

        try {
            setImporting(true);
            const result = await api.updateGuardrailsConfig(importText);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to update guardrails config' });
                return;
            }
            setImportDialogOpen(false);
            setImportText('');
            setActionMessage({ type: 'success', text: 'Guardrails config updated.' });
            await loadGuardrails();
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to update guardrails config' });
        } finally {
            setImporting(false);
        }
    };

    const handleExport = () => {
        if (!configContent) {
            setActionMessage({ type: 'error', text: 'No guardrails config content available to export.' });
            return;
        }
        const blob = new Blob([configContent], { type: 'text/yaml' });
        const link = document.createElement('a');
        link.href = URL.createObjectURL(blob);
        link.download = 'guardrails.yaml';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        URL.revokeObjectURL(link.href);
    };

    const actionAlert = actionMessage ? (
        <Alert severity={actionMessage.type} onClose={() => setActionMessage(null)}>
            {actionMessage.text}
        </Alert>
    ) : null;

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Guardrails"
                    subtitle="Manage rule-based safety checks for tool calls and tool results."
                    size="full"
                    rightAction={
                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
                            <Button variant="outlined" startIcon={<FileUpload />} onClick={handleImportClick}>
                                Import Config
                            </Button>
                            <Button variant="outlined" startIcon={<FileDownload />} onClick={handleExport}>
                                Export Config
                            </Button>
                        </Stack>
                    }
                >
                    <Stack spacing={2}>
                        {loadError && <Alert severity="error">{loadError}</Alert>}
                        {actionAlert}
                        <input
                            ref={fileInputRef}
                            type="file"
                            accept=".yaml,.yml,.json"
                            style={{ display: 'none' }}
                            onChange={handleImportFile}
                        />
                    </Stack>
                </UnifiedCard>

                <Grid container spacing={2}>
                    <Grid size={{ xs: 12, lg: 6 }}>
                        <UnifiedCard
                            title="Policy Breakdown"
                            size="full"
                            leftAction={
                                <Tooltip title="Shows total policies, how many are enabled or disabled, and the count in each policy category.">
                                    <IconButton size="small">
                                        <HelpOutline fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            }
                        >
                            <Stack spacing={1.75}>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2" color="text.secondary">
                                        Total Policies
                                    </Typography>
                                    <Chip size="small" label={`${stats.total}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2" color="text.secondary">
                                        Enabled
                                    </Typography>
                                    <Chip size="small" color="success" label={`${stats.enabled}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2" color="text.secondary">
                                        Disabled
                                    </Typography>
                                    <Chip size="small" variant="outlined" label={`${stats.disabled}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Stack direction="row" spacing={1.25} alignItems="center">
                                        <FolderOpen color="primary" fontSize="small" />
                                        <Typography variant="body2">Resource Access</Typography>
                                    </Stack>
                                    <Chip size="small" label={`${stats.resourceAccess}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Stack direction="row" spacing={1.25} alignItems="center">
                                        <Terminal color="primary" fontSize="small" />
                                        <Typography variant="body2">Command Execution</Typography>
                                    </Stack>
                                    <Chip size="small" label={`${stats.commandExecution}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Stack direction="row" spacing={1.25} alignItems="center">
                                        <ArticleOutlined color="primary" fontSize="small" />
                                        <Typography variant="body2">Content</Typography>
                                    </Stack>
                                    <Chip size="small" label={`${stats.content}`} />
                                </Stack>
                            </Stack>
                        </UnifiedCard>
                    </Grid>
                    <Grid size={{ xs: 12, lg: 6 }}>
                        <UnifiedCard
                            title="Event Summary"
                            size="full"
                            leftAction={
                                <Tooltip title="Summarizes recorded Guardrails events by final verdict. Masked events are rewrites, blocked events are stops, and review events are non-blocking interventions.">
                                    <IconButton size="small">
                                        <HelpOutline fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            }
                        >
                            <Stack spacing={1.75}>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2" color="text.secondary">
                                        Total Events
                                    </Typography>
                                    <Chip size="small" label={`${stats.historyCount}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2" color="text.secondary">
                                        Allow
                                    </Typography>
                                    <Chip size="small" variant="outlined" label={`${stats.allowedEvents}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2" color="text.secondary">
                                        Review
                                    </Typography>
                                    <Chip size="small" color="warning" variant="outlined" label={`${stats.reviewedEvents}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2" color="text.secondary">
                                        Blocked
                                    </Typography>
                                    <Chip size="small" color="error" label={`${stats.blockedEvents}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2" color="text.secondary">
                                        Masked
                                    </Typography>
                                    <Chip size="small" color="warning" label={`${stats.maskedEvents}`} />
                                </Stack>
                            </Stack>
                        </UnifiedCard>
                    </Grid>
                </Grid>
            </Stack>
            <Dialog
                open={importDialogOpen}
                onClose={() => !importing && setImportDialogOpen(false)}
                fullWidth
                maxWidth="md"
            >
                <DialogTitle>Import Guardrails Config</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ pt: 1 }}>
                        <Typography variant="body2" color="text.secondary">
                            Import from a local file or paste YAML or JSON directly. Saving replaces the current Guardrails config.
                        </Typography>
                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
                            <Button variant="outlined" startIcon={<FileUpload />} onClick={() => fileInputRef.current?.click()}>
                                Choose File
                            </Button>
                        </Stack>
                        <TextField
                            label="Config Content"
                            value={importText}
                            onChange={(e) => setImportText(e.target.value)}
                            multiline
                            minRows={16}
                            fullWidth
                            placeholder={'groups:\n  - id: high-risk\n    name: High Risk\npolicies:\n  - id: block-ssh-read\n    kind: resource_access\n    ...'}
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setImportDialogOpen(false)} disabled={importing}>
                        Cancel
                    </Button>
                    <Button variant="contained" onClick={handleImportSubmit} disabled={importing}>
                        Import
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default GuardrailsPage;
