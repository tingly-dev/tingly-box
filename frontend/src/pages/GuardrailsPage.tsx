import { useEffect, useMemo, useRef, useState } from 'react';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
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
    Policy,
    Rule,
    Security,
    History as HistoryIcon,
    Refresh as RefreshIcon,
    FileDownload,
    FileUpload,
    Storefront,
    VpnKey,
    FolderOpen,
    Hub,
    Terminal,
    ArticleOutlined,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

type GuardrailsHistoryEntry = {
    time: string;
    verdict: string;
    phase: string;
    scenario: string;
    alias_hits?: string[];
    credential_refs?: string[];
    credential_names?: string[];
};

const GuardrailsPage = () => {
    const navigate = useNavigate();
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [configContent, setConfigContent] = useState('');
    const [configPath, setConfigPath] = useState('');
    const [configExists, setConfigExists] = useState(false);
    const [groups, setGroups] = useState<any[]>([]);
    const [supportedScenarios, setSupportedScenarios] = useState<string[]>([]);
    const [policies, setPolicies] = useState<any[]>([]);
    const [historyEntries, setHistoryEntries] = useState<GuardrailsHistoryEntry[]>([]);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [importDialogOpen, setImportDialogOpen] = useState(false);
    const [importText, setImportText] = useState('');
    const [importing, setImporting] = useState(false);

    const loadGuardrails = async (showReloadMessage = false) => {
        try {
            setLoading(true);
            const [guardrailsConfig, guardrailsHistory] = await Promise.all([
                api.getGuardrailsConfig(),
                api.getGuardrailsHistory(),
            ]);
            const nextPolicies = guardrailsConfig?.config?.policies || [];
            const nextGroups = guardrailsConfig?.config?.groups || [];
            setPolicies(nextPolicies);
            setGroups(nextGroups);
            setConfigContent(guardrailsConfig?.content || '');
            setConfigPath(guardrailsConfig?.path || '');
            setConfigExists(!!guardrailsConfig?.exists);
            setSupportedScenarios(Array.isArray(guardrailsConfig?.supported_scenarios) ? guardrailsConfig.supported_scenarios : []);
            setHistoryEntries(Array.isArray(guardrailsHistory?.data) ? guardrailsHistory.data : []);
            setLoadError(null);
            if (showReloadMessage) {
                setActionMessage({ type: 'success', text: 'Guardrails config reloaded.' });
            }
        } catch (error: any) {
            console.error('Failed to load guardrails config:', error);
            setPolicies([]);
            setGroups([]);
            setConfigContent('');
            setConfigPath('');
            setConfigExists(false);
            setSupportedScenarios([]);
            setHistoryEntries([]);
            setLoadError('Failed to load guardrails config');
            if (showReloadMessage) {
                setActionMessage({ type: 'error', text: error?.message || 'Failed to reload guardrails config' });
            }
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadGuardrails();
    }, []);

    const stats = useMemo(() => {
        const totalCount = policies.length;
        const enabled = policies.filter((item) => item?.enabled !== false).length;
        const blocking = policies.filter((item) => item?.verdict === 'block').length;
        const resourceAccess = policies.filter((item) => item?.kind === 'resource_access').length;
        const commandExecution = policies.filter((item) => item?.kind === 'command_execution').length;
        const content = policies.filter((item) => item?.kind === 'content').length;
        const blockedEvents = historyEntries.filter((entry) => entry?.verdict === 'block').length;
        const maskedEvents = historyEntries.filter((entry) => entry?.verdict === 'mask').length;
        const toolUseEvents = historyEntries.filter((entry) => entry?.phase === 'tool_use').length;
        const toolResultEvents = historyEntries.filter((entry) => entry?.phase === 'tool_result').length;
        const totalAliasHits = historyEntries.reduce((sum, entry) => sum + (entry?.alias_hits?.length || 0), 0);
        const credentialMaskRefs = new Set<string>();
        historyEntries.forEach((entry) => {
            (entry?.credential_refs || []).forEach((ref) => credentialMaskRefs.add(ref));
        });
        const latestEvent = historyEntries.length > 0 ? historyEntries[0] : null;
        return {
            total: totalCount,
            enabled,
            blocking,
            groups: groups.length,
            scenarios: supportedScenarios.length,
            resourceAccess,
            commandExecution,
            content,
            historyCount: historyEntries.length,
            blockedEvents,
            maskedEvents,
            toolUseEvents,
            toolResultEvents,
            totalAliasHits,
            maskedCredentialCount: credentialMaskRefs.size,
            latestEvent,
        };
    }, [groups.length, historyEntries, policies, supportedScenarios.length]);

    const handleReload = async () => {
        try {
            setLoading(true);
            const result = await api.reloadGuardrailsConfig();
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to reload guardrails config' });
                return;
            }
            await loadGuardrails(true);
        } finally {
            setLoading(false);
        }
    };

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
                        <Tooltip title="Reload guardrails config">
                            <IconButton onClick={handleReload} size="small" aria-label="Reload guardrails config">
                                <RefreshIcon />
                            </IconButton>
                        </Tooltip>
                    }
                >
                    <Stack spacing={2}>
                        <Typography variant="body2" color="text.secondary">
                            Use Guardrails to block risky tool calls before execution and filter sensitive tool results before they go back to the model.
                        </Typography>
                        {loadError && <Alert severity="error">{loadError}</Alert>}
                        {actionAlert}
                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
                            <Button variant="contained" startIcon={<Rule />} onClick={() => navigate('/guardrails/rules')}>
                                Manage Policies
                            </Button>
                            <Button variant="outlined" startIcon={<VpnKey />} onClick={() => navigate('/guardrails/credentials')}>
                                Protected Credentials
                            </Button>
                            <Button variant="outlined" startIcon={<Storefront />} onClick={() => navigate('/guardrails/market')}>
                                Browse Builtins
                            </Button>
                            <Button variant="outlined" startIcon={<HistoryIcon />} onClick={() => navigate('/guardrails/history')}>
                                View History
                            </Button>
                            <input
                                ref={fileInputRef}
                                type="file"
                                accept=".yaml,.yml,.json"
                                style={{ display: 'none' }}
                                onChange={handleImportFile}
                            />
                            <Button variant="outlined" startIcon={<FileUpload />} onClick={handleImportClick}>
                                Import Config
                            </Button>
                            <Button variant="outlined" startIcon={<FileDownload />} onClick={handleExport}>
                                Export Config
                            </Button>
                        </Stack>
                    </Stack>
                </UnifiedCard>

                <Grid container spacing={2}>
                    <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
                        <Card sx={{ height: '100%' }}>
                            <CardContent>
                                <Stack direction="row" alignItems="center" spacing={2}>
                                    <Box
                                        sx={{
                                            width: 48,
                                            height: 48,
                                            borderRadius: 2,
                                            bgcolor: 'primary.main',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                        }}
                                    >
                                        <Policy sx={{ color: 'white', fontSize: 24 }} />
                                    </Box>
                                    <Box>
                                        <Typography variant="h4" sx={{ fontWeight: 600 }}>
                                            {stats.total}
                                        </Typography>
                                        <Typography variant="body2" color="text.secondary">
                                            Total Policies
                                        </Typography>
                                    </Box>
                                </Stack>
                            </CardContent>
                        </Card>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
                        <Card sx={{ height: '100%' }}>
                            <CardContent>
                                <Stack direction="row" alignItems="center" spacing={2}>
                                    <Box
                                        sx={{
                                            width: 48,
                                            height: 48,
                                            borderRadius: 2,
                                            bgcolor: 'success.main',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                        }}
                                    >
                                        <Security sx={{ color: 'white', fontSize: 24 }} />
                                    </Box>
                                    <Box>
                                        <Typography variant="h4" sx={{ fontWeight: 600 }}>
                                            {stats.enabled}
                                        </Typography>
                                        <Typography variant="body2" color="text.secondary">
                                            Enabled
                                        </Typography>
                                    </Box>
                                </Stack>
                            </CardContent>
                        </Card>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
                        <Card sx={{ height: '100%' }}>
                            <CardContent>
                                <Stack direction="row" alignItems="center" spacing={2}>
                                    <Box
                                        sx={{
                                            width: 48,
                                            height: 48,
                                            borderRadius: 2,
                                            bgcolor: 'warning.main',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                        }}
                                    >
                                        <Rule sx={{ color: 'white', fontSize: 24 }} />
                                    </Box>
                                    <Box>
                                        <Typography variant="h4" sx={{ fontWeight: 600 }}>
                                            {stats.blocking}
                                        </Typography>
                                        <Typography variant="body2" color="text.secondary">
                                            Block Policies
                                        </Typography>
                                    </Box>
                                </Stack>
                            </CardContent>
                        </Card>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
                        <Card sx={{ height: '100%' }}>
                            <CardContent>
                                <Stack direction="row" alignItems="center" spacing={2}>
                                    <Box
                                        sx={{
                                            width: 48,
                                            height: 48,
                                            borderRadius: 2,
                                            bgcolor: 'secondary.main',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                        }}
                                    >
                                        <Policy sx={{ color: 'white', fontSize: 24 }} />
                                    </Box>
                                    <Box>
                                        <Typography variant="h4" sx={{ fontWeight: 600 }}>
                                            {stats.groups}
                                        </Typography>
                                        <Typography variant="body2" color="text.secondary">
                                            Policy Groups
                                        </Typography>
                                    </Box>
                                </Stack>
                            </CardContent>
                        </Card>
                    </Grid>
                </Grid>

                <Grid container spacing={2}>
                    <Grid size={{ xs: 12, lg: 6 }}>
                        <UnifiedCard title="Policy Breakdown" size="full">
                            <Stack spacing={1.5}>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Stack direction="row" spacing={1.25} alignItems="center">
                                        <FolderOpen color="primary" fontSize="small" />
                                        <Typography variant="body2">Filesystem Access</Typography>
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
                                        <Typography variant="body2">Sensitive Output Protection</Typography>
                                    </Stack>
                                    <Chip size="small" label={`${stats.content}`} />
                                </Stack>
                            </Stack>
                        </UnifiedCard>
                    </Grid>
                    <Grid size={{ xs: 12, lg: 6 }}>
                        <UnifiedCard title="Config & Coverage" size="full">
                            <Stack spacing={1.5}>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2" color="text.secondary">
                                        Config file
                                    </Typography>
                                    <Chip
                                        size="small"
                                        color={configExists ? 'success' : 'default'}
                                        label={configExists ? 'Detected' : 'Not created'}
                                        variant={configExists ? 'filled' : 'outlined'}
                                    />
                                </Stack>
                                <Typography variant="body2" sx={{ wordBreak: 'break-all' }}>
                                    {configPath || 'No config path available'}
                                </Typography>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Stack direction="row" spacing={1.25} alignItems="center">
                                        <Hub color="primary" fontSize="small" />
                                        <Typography variant="body2">Supported scenarios</Typography>
                                    </Stack>
                                    <Chip size="small" label={`${stats.scenarios}`} />
                                </Stack>
                                <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap">
                                    {supportedScenarios.length > 0 ? (
                                        supportedScenarios.map((scenario) => (
                                            <Chip key={scenario} size="small" label={scenario} variant="outlined" />
                                        ))
                                    ) : (
                                        <Typography variant="body2" color="text.secondary">
                                            No scenario coverage reported.
                                        </Typography>
                                    )}
                                </Stack>
                            </Stack>
                        </UnifiedCard>
                    </Grid>
                </Grid>

                <Grid container spacing={2}>
                    <Grid size={{ xs: 12, lg: 6 }}>
                        <UnifiedCard title="Interception Summary" size="full">
                            <Stack spacing={1.5}>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2">Recorded events</Typography>
                                    <Chip size="small" label={`${stats.historyCount}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2">Blocked events</Typography>
                                    <Chip size="small" color="error" label={`${stats.blockedEvents}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2">Masked events</Typography>
                                    <Chip size="small" color="warning" label={`${stats.maskedEvents}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2">Tool-use interceptions</Typography>
                                    <Chip size="small" label={`${stats.toolUseEvents}`} variant="outlined" />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2">Tool-result interceptions</Typography>
                                    <Chip size="small" label={`${stats.toolResultEvents}`} variant="outlined" />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2">Masked credential refs</Typography>
                                    <Chip size="small" label={`${stats.maskedCredentialCount}`} variant="outlined" />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Typography variant="body2">Alias hits</Typography>
                                    <Chip size="small" label={`${stats.totalAliasHits}`} variant="outlined" />
                                </Stack>
                            </Stack>
                        </UnifiedCard>
                    </Grid>
                    <Grid size={{ xs: 12, lg: 6 }}>
                        <UnifiedCard title="Latest Interception" size="full">
                            {stats.latestEvent ? (
                                <Stack spacing={1.5}>
                                    <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap">
                                        <Chip size="small" label={stats.latestEvent.verdict} color={stats.latestEvent.verdict === 'block' ? 'error' : 'default'} />
                                        <Chip size="small" label={`phase: ${stats.latestEvent.phase || 'unknown'}`} variant="outlined" />
                                        <Chip size="small" label={`scenario: ${stats.latestEvent.scenario || 'unknown'}`} variant="outlined" />
                                        {stats.latestEvent.alias_hits && stats.latestEvent.alias_hits.length > 0 && (
                                            <Chip size="small" label={`alias hits: ${stats.latestEvent.alias_hits.length}`} variant="outlined" />
                                        )}
                                    </Stack>
                                    <Typography variant="body2" color="text.secondary">
                                        {stats.latestEvent.time ? new Date(stats.latestEvent.time).toLocaleString() : 'Unknown time'}
                                    </Typography>
                                    <Button variant="outlined" startIcon={<HistoryIcon />} onClick={() => navigate('/guardrails/history')}>
                                        Open Full History
                                    </Button>
                                </Stack>
                            ) : (
                                <Stack spacing={1.5}>
                                    <Typography variant="body2" color="text.secondary">
                                        No Guardrails interceptions have been recorded yet.
                                    </Typography>
                                    <Button variant="outlined" startIcon={<HistoryIcon />} onClick={() => navigate('/guardrails/history')}>
                                        Open Full History
                                    </Button>
                                </Stack>
                            )}
                        </UnifiedCard>
                    </Grid>
                </Grid>

                <UnifiedCard title="How It Works" size="full">
                    <Grid container spacing={3}>
                        <Grid size={{ xs: 12, md: 4 }}>
                            <Stack spacing={1.5} sx={{ p: 1 }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                    Pre-execution checks
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    Evaluate model-generated tool calls before Claude Code executes them locally.
                                </Typography>
                            </Stack>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                            <Stack spacing={1.5} sx={{ p: 1 }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                    Post-execution filters
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    Replace sensitive tool results before they are returned to the upstream model.
                                </Typography>
                            </Stack>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                            <Stack spacing={1.5} sx={{ p: 1 }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                    Rule management
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    Keep policy editing in the dedicated Policies page so overview and operations stay separate.
                                </Typography>
                            </Stack>
                        </Grid>
                    </Grid>
                </UnifiedCard>
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
                            placeholder={'groups:\n  - id: high-risk\n    name: High Risk\npolicies:\n  - id: block-ssh-read\n    kind: operation\n    ...'}
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
