import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import {
    Alert,
    Box,
    Button,
    Checkbox,
    Chip,
    CircularProgress,
    FormControlLabel,
    IconButton,
    Paper,
    Snackbar,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Tooltip,
    Typography,
} from '@mui/material';
import {
    Add as AddIcon,
    DeleteOutline as DeleteOutlineIcon,
    Edit as EditIcon,
    PowerSettingsNew as PowerIcon,
} from '@mui/icons-material';
import { useEffect, useState } from 'react';
import MCPSourceEditor from './MCPSourceEditor';
import {
    BUILTIN_IDS,
    MCP_DEFAULT_CWD,
    defaultMCPSourceFormValue,
    formValueToSource,
    sourceToFormValue,
    type MCPConfigResponse,
    type MCPSourceConfig,
    type MCPSourceFormValue,
} from './types';

const weatherTemplate = (): MCPSourceFormValue => ({
    ...defaultMCPSourceFormValue(),
    id: 'weather',
    transport: 'stdio',
    command: 'python3',
    args: ['mcp_weather_tools.py'],
    cwd: MCP_DEFAULT_CWD,
    tools: ['get_current_weather'],
    useGlobalProxy: true,
    envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY'],
});

const emptyCustomTemplate = (): MCPSourceFormValue => ({
    ...defaultMCPSourceFormValue(),
    args: [],
    tools: ['*'],
    envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY'],
    useGlobalProxy: true,
});

const defaultBuiltinForm = (): MCPSourceFormValue => ({
    ...defaultMCPSourceFormValue(),
    id: 'webtools',
    name: 'Built-in Web Tools',
    transport: 'stdio',
    command: 'builtin', // Special marker for built-in Go tools
    args: [],
    tools: ['mcp_web_search', 'mcp_web_fetch'],
    envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY'],
    useGlobalProxy: true,
    isClientTool: true, // Built-in tools are client tools by default
});

const MCPRegisteredServers = () => {
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [notification, setNotification] = useState({ open: false, message: '', severity: 'success' as 'success' | 'error' });
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);

    // Editor state
    const [editingId, setEditingId] = useState<string>('');
    const [editorMode, setEditorMode] = useState<'none' | 'add' | 'edit'>('none');
    const [editorForm, setEditorForm] = useState<MCPSourceFormValue>(emptyCustomTemplate());

    // Builtin checkbox state
    const [enableSearch, setEnableSearch] = useState(true);
    const [enableFetch, setEnableFetch] = useState(true);

    useEffect(() => {
        void loadData();
    }, []);

    const loadData = async () => {
        setLoading(true);
        const configResult: MCPConfigResponse = await api.getMCPConfig();
        if (configResult.success && configResult.config) {
            const sources = configResult.config.sources || [];
            setAllSources(sources);
        }

        setLoading(false);
    };

    const isBuiltin = (id?: string) => id ? BUILTIN_IDS.includes(id) : false;

    const openAdd = () => {
        setEditingId('');
        setEditorForm(emptyCustomTemplate());
        setEditorMode('add');
    };

    const openEdit = (source: MCPSourceConfig) => {
        setEditingId(source.id || '');
        const mapped = sourceToFormValue(source);
        if (isBuiltin(source.id)) {
            const tools = mapped.tools || [];
            setEnableSearch(tools.includes('*') || tools.includes('mcp_web_search'));
            setEnableFetch(tools.includes('*') || tools.includes('mcp_web_fetch'));
            setEditorForm({ ...mapped, id: 'webtools' as const, cwd: MCP_DEFAULT_CWD });
        } else {
            setEditorForm(mapped);
        }
        setEditorMode('edit');
    };

    const toggleEnabled = (id: string | undefined, enabled: boolean) => {
        if (!id) return;
        setAllSources((prev) => prev.map((s) => (s.id === id ? { ...s, enabled } : s)));
        if (editingId === id) {
            setEditorForm((prev) => ({ ...prev, enabled }));
        }
    };

    const deleteSource = (id?: string) => {
        if (!id) return;
        const updated = allSources.filter((s) => s.id !== id);
        setAllSources(updated);
        if (editingId === id) {
            setEditingId('');
            setEditorMode('none');
            setEditorForm(emptyCustomTemplate());
        }
    };

    const saveAll = async () => {
        let next = [...allSources];

        if (editorMode !== 'none') {
            if (!editorForm.id) {
                setNotification({ open: true, message: 'Server name is required', severity: 'error' });
                return;
            }
            if (editorForm.transport === 'http' && !editorForm.endpoint) {
                setNotification({ open: true, message: 'HTTP endpoint is required', severity: 'error' });
                return;
            }
            if (editorForm.transport === 'stdio' && !editorForm.command) {
                setNotification({ open: true, message: 'Command is required', severity: 'error' });
                return;
            }

            let source: MCPSourceConfig;
            if (isBuiltin(editorForm.id)) {
                const tools: string[] = [];
                if (enableSearch) tools.push('mcp_web_search');
                if (enableFetch) tools.push('mcp_web_fetch');
                if (tools.length === 0) {
                    setNotification({ open: true, message: 'At least one builtin tool must be enabled', severity: 'error' });
                    return;
                }
                source = formValueToSource({ ...editorForm, id: 'webtools' as const, tools });
            } else {
                source = formValueToSource(editorForm);
            }

            const idx = next.findIndex((s) => s.id === source.id);
            if (idx >= 0) {
                next[idx] = source;
            } else {
                next.push(source);
            }
        }

        setSaving(true);
        const result = await api.setMCPConfig({ sources: next });
        if (result.success) {
            setNotification({ open: true, message: 'Saved. Restart server to apply.', severity: 'success' });
            setAllSources(next);
            setEditorMode('none');
            setEditingId('');
            setEditorForm(emptyCustomTemplate());
        } else {
            setNotification({ open: true, message: result.error || 'Failed to save', severity: 'error' });
        }
        setSaving(false);
    };

    if (loading) {
        return (
            <PageLayout loading={true}>
                <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
                    <CircularProgress />
                </Box>
            </PageLayout>
        );
    }

    return (
        <PageLayout loading={false}>
            <Stack spacing={2.5}>
                <Alert severity="info">
                    Manage registered MCP servers. Builtin servers are marked with a tag.
                </Alert>

                <UnifiedCard title="Registered Servers" size="full">
                    <Stack spacing={2}>
                        <Stack direction="row" justifyContent="flex-end">
                            <Tooltip title="Add Server">
                                <IconButton onClick={openAdd} color="primary">
                                    <AddIcon />
                                </IconButton>
                            </Tooltip>
                        </Stack>
                        {allSources.length > 0 ? (
                            <TableContainer component={Paper} variant="outlined">
                                <Table size="small">
                                    <TableHead>
                                        <TableRow>
                                            <TableCell sx={{ fontWeight: 600 }}>Name</TableCell>
                                            <TableCell sx={{ fontWeight: 600 }}>Connection Type</TableCell>
                                            <TableCell sx={{ fontWeight: 600 }}>Connection Info</TableCell>
                                            <TableCell sx={{ fontWeight: 600 }}>Enabled Tools</TableCell>
                                            <TableCell sx={{ fontWeight: 600 }}>State</TableCell>
                                            <TableCell sx={{ fontWeight: 600 }}>Actions</TableCell>
                                        </TableRow>
                                    </TableHead>
                                    <TableBody>
                                        {allSources.map((source) => {
                                            const connectionType = (source.transport || 'stdio').toUpperCase();
                                            const connectionInfo = source.transport === 'http'
                                                ? source.endpoint || '-'
                                                : source.command
                                                    ? `${source.command}${source.args && source.args.length > 0 ? ' ' + source.args.join(' ') : ''}`
                                                    : '-';
                                            const tools = source.tools || [];
                                            const isEnabled = source.enabled ?? true;
                                            const isAutoRegistered = (source as any).auto_registered ?? false;
                                            const builtin = isBuiltin(source.id);

                                            return (
                                                <TableRow
                                                    key={source.id}
                                                    hover
                                                    sx={{ cursor: 'pointer' }}
                                                    onClick={() => source.id && setEditingId(source.id)}
                                                >
                                                    <TableCell>
                                                        <Stack direction="row" spacing={0.5} alignItems="center">
                                                            <Typography variant="body2" fontWeight={500}>
                                                                {source.id || '-'}
                                                            </Typography>
                                                            {builtin && (
                                                                <Chip
                                                                    label="Builtin"
                                                                    size="small"
                                                                    color="primary"
                                                                    variant="outlined"
                                                                    sx={{ fontSize: '0.65rem', height: 18 }}
                                                                />
                                                            )}
                                                            {isAutoRegistered && (
                                                                <Chip
                                                                    label="Auto"
                                                                    size="small"
                                                                    color="warning"
                                                                    variant="outlined"
                                                                    sx={{ fontSize: '0.65rem', height: 18 }}
                                                                />
                                                            )}
                                                        </Stack>
                                                    </TableCell>
                                                    <TableCell>
                                                        <Chip
                                                            label={connectionType}
                                                            size="small"
                                                            color={source.transport === 'http' ? 'info' : 'default'}
                                                            variant="outlined"
                                                        />
                                                    </TableCell>
                                                    <TableCell>
                                                        <Typography
                                                            variant="body2"
                                                            sx={{
                                                                fontFamily: 'monospace',
                                                                fontSize: '0.75rem',
                                                                maxWidth: 300,
                                                                overflow: 'hidden',
                                                                textOverflow: 'ellipsis',
                                                                whiteSpace: 'nowrap',
                                                            }}
                                                            title={connectionInfo}
                                                        >
                                                            {connectionInfo}
                                                        </Typography>
                                                    </TableCell>
                                                    <TableCell>
                                                        <Stack direction="row" spacing={0.5} flexWrap="wrap">
                                                            {tools.slice(0, 2).map((t) => (
                                                                <Chip
                                                                    key={t}
                                                                    label={t}
                                                                    size="small"
                                                                    variant="outlined"
                                                                    sx={{ fontSize: '0.7rem', height: 20 }}
                                                                />
                                                            ))}
                                                            {tools.length > 2 && (
                                                                <Chip
                                                                    label={`+${tools.length - 2}`}
                                                                    size="small"
                                                                    variant="outlined"
                                                                    sx={{ fontSize: '0.7rem', height: 20 }}
                                                                />
                                                            )}
                                                        </Stack>
                                                    </TableCell>
                                                    <TableCell>
                                                        <Chip
                                                            label={isEnabled ? 'Enabled' : 'Disabled'}
                                                            size="small"
                                                            color={isEnabled ? 'success' : 'default'}
                                                            variant={isEnabled ? 'filled' : 'outlined'}
                                                        />
                                                    </TableCell>
                                                    <TableCell>
                                                        <Stack direction="row" spacing={0.5} onClick={(e) => e.stopPropagation()}>
                                                            <Tooltip title={isEnabled ? 'Disable' : 'Enable'}>
                                                                <IconButton
                                                                    size="small"
                                                                    color={isEnabled ? 'success' : 'default'}
                                                                    onClick={() => toggleEnabled(source.id, !isEnabled)}
                                                                >
                                                                    <PowerIcon fontSize="small" />
                                                                </IconButton>
                                                            </Tooltip>
                                                            <Tooltip title="Edit">
                                                                <IconButton
                                                                    size="small"
                                                                    color="primary"
                                                                    onClick={() => openEdit(source)}
                                                                >
                                                                    <EditIcon fontSize="small" />
                                                                </IconButton>
                                                            </Tooltip>
                                                            <Tooltip title="Delete">
                                                                <IconButton
                                                                    size="small"
                                                                    color="error"
                                                                    onClick={() => deleteSource(source.id)}
                                                                >
                                                                    <DeleteOutlineIcon fontSize="small" />
                                                                </IconButton>
                                                            </Tooltip>
                                                        </Stack>
                                                    </TableCell>
                                                </TableRow>
                                            );
                                        })}
                                    </TableBody>
                                </Table>
                            </TableContainer>
                        ) : (
                            <Typography variant="body2" color="text.secondary">
                                No registered MCP servers yet.
                            </Typography>
                        )}
                    </Stack>
                </UnifiedCard>

                {editorMode !== 'none' && (
                    <>
                        {isBuiltin(editorForm.id) && (
                            <Stack direction="row" spacing={2}>
                                <FormControlLabel
                                    control={<Checkbox checked={enableSearch} onChange={(e) => setEnableSearch(e.target.checked)} />}
                                    label="mcp_web_search"
                                />
                                <FormControlLabel
                                    control={<Checkbox checked={enableFetch} onChange={(e) => setEnableFetch(e.target.checked)} />}
                                    label="mcp_web_fetch"
                                />
                            </Stack>
                        )}

                        <MCPSourceEditor
                            title={isBuiltin(editorForm.id) ? 'Edit builtin MCP' : (editorMode === 'edit' ? 'Edit custom MCP' : 'Connect to a custom MCP')}
                            value={editorForm}
                            onChange={setEditorForm}
                            lockId={isBuiltin(editorForm.id)}
                            onUseExample={!isBuiltin(editorForm.id) && editorMode === 'add' ? () => {
                                setEditingId('');
                                setEditorForm(weatherTemplate());
                                setEditorMode('add');
                            } : undefined}
                        />

                        <Stack direction="row" justifyContent="space-between">
                            <Button variant="text" onClick={() => setEditorMode('none')}>Cancel</Button>
                        </Stack>
                    </>
                )}

                <Stack direction="row" justifyContent="flex-end">
                    <Button variant="contained" onClick={saveAll} disabled={saving}>
                        {saving ? 'Saving...' : 'Save'}
                    </Button>
                </Stack>
            </Stack>

            <Snackbar
                open={notification.open}
                autoHideDuration={3000}
                onClose={() => setNotification({ open: false, message: '', severity: 'success' })}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
            >
                <Alert severity={notification.severity} sx={{ width: '100%' }}>
                    {notification.message}
                </Alert>
            </Snackbar>
        </PageLayout>
    );
};

export default MCPRegisteredServers;
