import { useEffect, useMemo, useRef, useState } from 'react';
import {
    Alert,
    Button,
    Checkbox,
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
} from '@/components/icons';
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

type GuardrailsImportRef = {
    path: string;
    name: string;
    policy_ids?: string[];
    policy_count?: number;
};

const GuardrailsPage = () => {
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [policies, setPolicies] = useState<any[]>([]);
    const [imports, setImports] = useState<GuardrailsImportRef[]>([]);
    const [historyEntries, setHistoryEntries] = useState<GuardrailsHistoryEntry[]>([]);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [importDialogOpen, setImportDialogOpen] = useState(false);
    const [importText, setImportText] = useState('');
    const [importFileName, setImportFileName] = useState('');
    const [importing, setImporting] = useState(false);
    const [exportDialogOpen, setExportDialogOpen] = useState(false);
    const [selectedExportPaths, setSelectedExportPaths] = useState<string[]>([]);
    const [exporting, setExporting] = useState(false);

    const loadGuardrails = async () => {
        try {
            setLoading(true);
            const [guardrailsConfig, guardrailsHistory] = await Promise.all([
                api.getGuardrailsConfig(),
                api.getGuardrailsHistory(),
            ]);
            setPolicies(guardrailsConfig?.config?.policies || []);
            setImports(Array.isArray(guardrailsConfig?.imports) ? guardrailsConfig.imports : []);
            setHistoryEntries(Array.isArray(guardrailsHistory?.data) ? guardrailsHistory.data : []);
            setLoadError(null);
        } catch (error: any) {
            console.error('Failed to load guardrails config:', error);
            setPolicies([]);
            setImports([]);
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
        const enabled = policies.filter((item) => item?.enabled === true).length;
        const disabled = policies.filter((item) => item?.enabled !== true).length;
        const resourceAccessPolicies = policies.filter((item) => item?.kind === 'resource_access');
        const commandExecutionPolicies = policies.filter((item) => item?.kind === 'command_execution');
        const contentPolicies = policies.filter((item) => item?.kind === 'content');
        const blockedEvents = historyEntries.filter((entry) => entry?.verdict === 'block').length;
        const maskedEvents = historyEntries.filter((entry) => entry?.verdict === 'mask').length;
        const reviewedEvents = historyEntries.filter((entry) => entry?.verdict === 'review').length;
        const allowedEvents = historyEntries.filter((entry) => entry?.verdict === 'allow').length;
        return {
            total,
            enabled,
            disabled,
            resourceAccess: resourceAccessPolicies.length,
            resourceAccessEnabled: resourceAccessPolicies.filter((item) => item?.enabled === true).length,
            resourceAccessDisabled: resourceAccessPolicies.filter((item) => item?.enabled !== true).length,
            commandExecution: commandExecutionPolicies.length,
            commandExecutionEnabled: commandExecutionPolicies.filter((item) => item?.enabled === true).length,
            commandExecutionDisabled: commandExecutionPolicies.filter((item) => item?.enabled !== true).length,
            content: contentPolicies.length,
            contentEnabled: contentPolicies.filter((item) => item?.enabled === true).length,
            contentDisabled: contentPolicies.filter((item) => item?.enabled !== true).length,
            historyCount: historyEntries.length,
            allowedEvents,
            reviewedEvents,
            blockedEvents,
            maskedEvents,
        };
    }, [historyEntries, policies]);

    const blurActiveElement = () => {
        const active = document.activeElement;
        if (active instanceof HTMLElement) {
            active.blur();
        }
    };

    const closeImportDialog = () => {
        setImportDialogOpen(false);
        blurActiveElement();
    };

    const closeExportDialog = () => {
        setExportDialogOpen(false);
        blurActiveElement();
    };

    const handleImportClick = () => {
        setImportText('');
        setImportFileName('');
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
            setImportFileName(file.name);
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
            const result = await api.importGuardrailsFragment(importText, importFileName || undefined);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to import policy fragment' });
                return;
            }
            closeImportDialog();
            setImportText('');
            setImportFileName('');
            const importedCount = Array.isArray(result?.policy_ids) ? result.policy_ids.length : 0;
            setActionMessage({
                type: 'success',
                text: importedCount > 0 ? `Imported ${importedCount} policy fragment item(s).` : 'Imported policy fragment.',
            });
            await loadGuardrails();
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to import policy fragment' });
        } finally {
            setImporting(false);
        }
    };

    const handleExportClick = () => {
        if (imports.length === 0) {
            setActionMessage({ type: 'error', text: 'No imported policy fragments are available to export.' });
            return;
        }
        setSelectedExportPaths(imports.map((item) => item.path));
        setExportDialogOpen(true);
    };

    const handleToggleExportPath = (path: string) => {
        setSelectedExportPaths((current) =>
            current.includes(path) ? current.filter((item) => item !== path) : [...current, path]
        );
    };

    const handleSelectAllExports = () => {
        setSelectedExportPaths(imports.map((item) => item.path));
    };

    const handleClearExportSelection = () => {
        setSelectedExportPaths([]);
    };

    const handleExportSubmit = async () => {
        if (selectedExportPaths.length === 0) {
            setActionMessage({ type: 'error', text: 'Select at least one imported fragment to export.' });
            return;
        }

        try {
            setExporting(true);
            const result = await api.exportGuardrailsFragments(selectedExportPaths);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to export policy fragments' });
                return;
            }
            const files = Array.isArray(result?.files) ? result.files : [];
            if (files.length === 0) {
                setActionMessage({ type: 'error', text: 'No fragment files were returned for export.' });
                return;
            }
            files.forEach((file: { content?: string; name?: string }) => {
                const blob = new Blob([file.content || ''], { type: 'text/yaml' });
                const link = document.createElement('a');
                link.href = URL.createObjectURL(blob);
                link.download = file.name || 'guardrails-fragment.yaml';
                document.body.appendChild(link);
                link.click();
                document.body.removeChild(link);
                URL.revokeObjectURL(link.href);
            });
            closeExportDialog();
            setActionMessage({
                type: 'success',
                text: files.length === 1 ? `Exported ${files[0].name || 'fragment'}.` : `Exported ${files.length} fragment files.`,
            });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to export policy fragments' });
        } finally {
            setExporting(false);
        }
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
                                Import Policies
                            </Button>
                            <Button variant="outlined" startIcon={<FileDownload />} onClick={handleExportClick}>
                                Export Imports
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
                                <Typography variant="caption" color="text.secondary">
                                    Format: enabled / total
                                </Typography>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Stack direction="row" spacing={1.25} alignItems="center">
                                        <FolderOpen color="primary" fontSize="small" />
                                        <Typography variant="body2">Resource Access</Typography>
                                    </Stack>
                                    <Chip size="small" label={`${stats.resourceAccessEnabled}/${stats.resourceAccess}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Stack direction="row" spacing={1.25} alignItems="center">
                                        <Terminal color="primary" fontSize="small" />
                                        <Typography variant="body2">Command Execution</Typography>
                                    </Stack>
                                    <Chip size="small" label={`${stats.commandExecutionEnabled}/${stats.commandExecution}`} />
                                </Stack>
                                <Stack direction="row" justifyContent="space-between" alignItems="center">
                                    <Stack direction="row" spacing={1.25} alignItems="center">
                                        <ArticleOutlined color="primary" fontSize="small" />
                                        <Typography variant="body2">Privacy</Typography>
                                    </Stack>
                                    <Chip size="small" label={`${stats.contentEnabled}/${stats.content}`} />
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
                onClose={() => !importing && closeImportDialog()}
                disableRestoreFocus
                fullWidth
                maxWidth="md"
            >
                <DialogTitle>Import Policy Fragment</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ pt: 1 }}>
                        {importDialogOpen && actionMessage && (
                            <Alert severity={actionMessage.type} onClose={() => setActionMessage(null)}>
                                {actionMessage.text}
                            </Alert>
                        )}
                        <Typography variant="body2" color="text.secondary">
                            Import a YAML or JSON policy fragment containing one or more policies. Imported policies are appended to `guardrails/custom/import.yaml`.
                        </Typography>
                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
                            <Button variant="outlined" startIcon={<FileUpload />} onClick={() => fileInputRef.current?.click()}>
                                Choose File
                            </Button>
                            {importFileName ? (
                                <Chip size="small" label={importFileName} />
                            ) : null}
                        </Stack>
                        <TextField
                            label="Fragment Content"
                            value={importText}
                            onChange={(e) => setImportText(e.target.value)}
                            multiline
                            minRows={16}
                            fullWidth
                            placeholder={'policies:\n  - id: block-ssh-read\n    name: Block SSH Read\n    kind: resource_access\n    enabled: false\n    groups: [default]\n    ...'}
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={closeImportDialog} disabled={importing}>
                        Cancel
                    </Button>
                    <Button variant="contained" onClick={handleImportSubmit} disabled={importing}>
                        Import
                    </Button>
                </DialogActions>
            </Dialog>
            <Dialog
                open={exportDialogOpen}
                onClose={() => !exporting && closeExportDialog()}
                disableRestoreFocus
                fullWidth
                maxWidth="sm"
            >
                <DialogTitle>Export Imported Fragments</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ pt: 1 }}>
                        {exportDialogOpen && actionMessage && (
                            <Alert severity={actionMessage.type} onClose={() => setActionMessage(null)}>
                                {actionMessage.text}
                            </Alert>
                        )}
                        <Typography variant="body2" color="text.secondary">
                            Choose one or more imported fragment files to download as-is.
                        </Typography>
                        <Stack direction="row" spacing={1}>
                            <Button size="small" variant="outlined" onClick={handleSelectAllExports}>
                                Select All
                            </Button>
                            <Button size="small" variant="outlined" onClick={handleClearExportSelection}>
                                Clear
                            </Button>
                        </Stack>
                        <Stack spacing={1}>
                            {imports.map((item) => (
                                <Stack
                                    key={item.path}
                                    direction="row"
                                    spacing={1.5}
                                    alignItems="flex-start"
                                    sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 1.5 }}
                                >
                                    <Checkbox
                                        checked={selectedExportPaths.includes(item.path)}
                                        onChange={() => handleToggleExportPath(item.path)}
                                        sx={{ mt: -0.5 }}
                                    />
                                    <Stack spacing={0.5} sx={{ minWidth: 0 }}>
                                        <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                            {item.name || item.path}
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            {item.path}
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            {`${item.policy_count || 0} policies`}
                                            {item.policy_ids && item.policy_ids.length > 0 ? ` · ${item.policy_ids.join(', ')}` : ''}
                                        </Typography>
                                    </Stack>
                                </Stack>
                            ))}
                        </Stack>
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={closeExportDialog} disabled={exporting}>
                        Cancel
                    </Button>
                    <Button variant="contained" onClick={handleExportSubmit} disabled={exporting}>
                        Export
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default GuardrailsPage;
