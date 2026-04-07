import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import {
    Alert,
    Box,
    Chip,
    CircularProgress,
    Checkbox,
    FormControlLabel,
    IconButton,
    Snackbar,
    Stack,
    Tooltip,
    Typography,
    Button,
} from '@mui/material';
import {
    Add as AddIcon,
    DeleteOutline as DeleteOutlineIcon,
    Edit as EditIcon,
} from '@mui/icons-material';
import { useEffect, useState } from 'react';
import MCPSourceEditor from './MCPSourceEditor';
import { BUILTIN_IDS, MCP_DEFAULT_CWD, defaultMCPSourceFormValue, formValueToSource, sourceToFormValue, type MCPConfigResponse, type MCPSourceConfig, type MCPSourceFormValue } from './types';

const defaultBuiltinForm = (): MCPSourceFormValue => ({
    ...defaultMCPSourceFormValue(),
    id: 'webtools',
    transport: 'stdio',
    command: 'python3',
    args: ['mcp_web_tools.py'],
    tools: ['web_search', 'web_fetch'],
    envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY'],
    useGlobalProxy: true,
});

const MCPBuiltin = () => {
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [notification, setNotification] = useState({ open: false, message: '', severity: 'success' as 'success' | 'error' });
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);
    const [form, setForm] = useState<MCPSourceFormValue>(defaultBuiltinForm());
    const [editorMode, setEditorMode] = useState<'none' | 'add' | 'edit'>('none');
    const [enableSearch, setEnableSearch] = useState(true);
    const [enableFetch, setEnableFetch] = useState(true);

    const builtinSource = allSources.find((s) => s.id === 'webtools');

    useEffect(() => {
        void loadMCPConfig();
    }, []);

    const loadMCPConfig = async () => {
        setLoading(true);
        const result: MCPConfigResponse = await api.getMCPConfig();
        if (result.success && result.config) {
            const sources = result.config.sources || [];
            setAllSources(sources);
        }
        setForm(defaultBuiltinForm());
        setEnableSearch(true);
        setEnableFetch(true);
        setEditorMode('none');
        setLoading(false);
    };

    const openAdd = () => {
        setForm(defaultBuiltinForm());
        setEnableSearch(true);
        setEnableFetch(true);
        setEditorMode('add');
    };

    const openEdit = () => {
        if (!builtinSource) {
            openAdd();
            return;
        }
        const mapped = sourceToFormValue(builtinSource);
        const tools = mapped.tools || [];
        setEnableSearch(tools.includes('*') || tools.includes('web_search'));
        setEnableFetch(tools.includes('*') || tools.includes('web_fetch'));
        setForm({ ...mapped, id: 'webtools', cwd: MCP_DEFAULT_CWD });
        setEditorMode('edit');
    };

    const removeBuiltin = () => {
        const next = allSources.filter((s) => s.id !== 'webtools');
        setAllSources(next);
        setEditorMode('none');
        setNotification({ open: true, message: 'Builtin server removed from draft', severity: 'success' });
    };

    const saveAll = async () => {
        let next = [...allSources];
        if (editorMode !== 'none') {
            const tools: string[] = [];
            if (enableSearch) tools.push('web_search');
            if (enableFetch) tools.push('web_fetch');
            if (tools.length === 0) {
                setNotification({ open: true, message: 'At least one tool must be enabled', severity: 'error' });
                return;
            }
            const source = formValueToSource({ ...form, id: 'webtools', tools });
            next = [...allSources.filter((s) => !BUILTIN_IDS.includes(s.id || '')), source];
        }

        setSaving(true);
        const result = await api.setMCPConfig({ sources: next });
        if (result.success) {
            setAllSources(next);
            setEditorMode('none');
            setNotification({ open: true, message: 'Saved. Restart server to apply.', severity: 'success' });
        } else {
            setNotification({ open: true, message: result.error || 'Failed to save', severity: 'error' });
        }
        setSaving(false);
    };

    if (loading) {
        return (
            <PageLayout>
                <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
                    <CircularProgress />
                </Box>
            </PageLayout>
        );
    }

    return (
        <PageLayout>
            <Stack spacing={2.5}>
                <Alert severity="info">
                    Builtin MCP keeps web_search/web_fetch in one server. Click Add/Edit to open the same connection form used by Custom.
                </Alert>

                <UnifiedCard title="Builtin MCP Servers" size="full">
                    <Stack spacing={1.5}>
                        <Stack direction="row" justifyContent="flex-end">
                            {builtinSource ? (
                                <Stack direction="row" spacing={1}>
                                    <Tooltip title="Edit">
                                        <IconButton size="small" color="primary" onClick={openEdit}>
                                            <EditIcon fontSize="small" />
                                        </IconButton>
                                    </Tooltip>
                                    <Tooltip title="Delete">
                                        <IconButton size="small" color="error" onClick={removeBuiltin}>
                                            <DeleteOutlineIcon fontSize="small" />
                                        </IconButton>
                                    </Tooltip>
                                </Stack>
                            ) : (
                                <Tooltip title="Add Server">
                                    <IconButton size="small" color="primary" onClick={openAdd}>
                                        <AddIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            )}
                        </Stack>
                        {builtinSource ? (
                            <Stack direction="row" spacing={1} alignItems="center">
                                <Chip label="webtools" color="primary" />
                                <Chip label={builtinSource.transport || 'stdio'} />
                                {(builtinSource.tools || []).map((t) => <Chip key={t} label={t} variant="outlined" />)}
                            </Stack>
                        ) : (
                            <Typography variant="body2" color="text.secondary">No builtin MCP server configured.</Typography>
                        )}
                    </Stack>
                </UnifiedCard>

                {editorMode !== 'none' && (
                    <>
                        <Stack direction="row" spacing={2}>
                            <FormControlLabel
                                control={<Checkbox checked={enableSearch} onChange={(e) => setEnableSearch(e.target.checked)} />}
                                label="web_search"
                            />
                            <FormControlLabel
                                control={<Checkbox checked={enableFetch} onChange={(e) => setEnableFetch(e.target.checked)} />}
                                label="web_fetch"
                            />
                        </Stack>

                        <MCPSourceEditor
                            title="Connect to a builtin MCP"
                            value={form}
                            onChange={setForm}
                            lockId
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

export default MCPBuiltin;
