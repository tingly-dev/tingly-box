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
    Stack,
    Switch,
    Tooltip,
    Typography,
    Button,
} from '@mui/material';
import {
    Add as AddIcon,
    DeleteOutline as DeleteOutlineIcon,
    Edit as EditIcon,
} from '@/components/icons';
import { useEffect, useState } from 'react';
import { useNotify } from '@/hooks/useNotify';
import MCPSourceEditor from './MCPSourceEditor';
import { MCP_DEFAULT_CWD, defaultMCPSourceFormValue, formValueToSource, sourceToFormValue, type MCPConfigResponse, type MCPSourceConfig, type MCPSourceFormValue } from './types';

const defaultBuiltinForm = (): MCPSourceFormValue => ({
    ...defaultMCPSourceFormValue(),
    id: 'webtools',
    name: 'Built-in Web Tools',
    transport: 'stdio',
    command: 'tingly-box', // Actual command for built-in tools
    args: ['mcp-builtin'],
    tools: ['mcp_web_search', 'mcp_web_fetch'],
    envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY', 'SERPER_API_KEY'],
    useGlobalProxy: true,
    visibility: 'client',
});

const MCPBuiltin = () => {
    const notify = useNotify();
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
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
        setEnableSearch(tools.includes('*') || tools.includes('mcp_web_search'));
        setEnableFetch(tools.includes('*') || tools.includes('mcp_web_fetch'));
        setForm({ ...mapped, id: 'webtools' as const, cwd: MCP_DEFAULT_CWD });
        setEditorMode('edit');
    };

    const removeBuiltin = () => {
        const next = allSources.filter((s) => s.id !== 'webtools');
        setAllSources(next);
        setEditorMode('none');
        notify.success('Builtin server removed from draft');
    };

    const toggleBuiltinEnabled = async (enabled: boolean) => {
        const updated = allSources.map((s) => (s.id === 'webtools' ? { ...s, enabled } : s));
        setAllSources(updated);
        if (builtinSource) {
            setForm((prev) => ({ ...prev, enabled }));
        }
        // Auto-save after toggle
        setSaving(true);
        const result = await api.setMCPConfig({ sources: updated });
        if (result.success) {
            notify.success(enabled ? 'Enabled. Restart server to apply.' : 'Disabled. Restart server to apply.');
        } else {
            notify.error(result.error || 'Failed to update');
            // Revert on failure
            setAllSources(allSources);
        }
        setSaving(false);
    };

    const saveAll = async () => {
        let next = [...allSources];
        if (editorMode !== 'none') {
            const tools: string[] = [];
            if (enableSearch) tools.push('mcp_web_search');
            if (enableFetch) tools.push('mcp_web_fetch');
            if (tools.length === 0) {
                notify.error('At least one tool must be enabled');
                return;
            }
            const source = formValueToSource({ ...form, id: 'webtools' as const, tools });
            next = [...allSources.filter((s) => s.id !== 'webtools'), source];
        }

        setSaving(true);
        const result = await api.setMCPConfig({ sources: next });
        if (result.success) {
            setAllSources(next);
            setEditorMode('none');
            notify.success('Saved. Restart server to apply.');
        } else {
            notify.error(result.error || 'Failed to save');
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
                    Builtin MCP keeps mcp_web_search/mcp_web_fetch in one server. Click Add/Edit to open the same connection form used by Custom.
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
                            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                                <Chip label="webtools" color="primary" />
                                <Chip label={builtinSource.transport || 'stdio'} />
                                <Chip
                                    label={(builtinSource.visibility === 'server') ? 'Server Tool' : 'Client Tool'}
                                    color={(builtinSource.visibility === 'server') ? 'success' : 'info'}
                                    size="small"
                                />
                                {(builtinSource.tools || []).map((t) => <Chip key={t} label={t} variant="outlined" />)}
                                <FormControlLabel
                                    sx={{ ml: 0.5 }}
                                    control={(
                                        <Switch
                                            size="small"
                                            checked={builtinSource.enabled ?? true}
                                            onChange={(e) => toggleBuiltinEnabled(e.target.checked)}
                                        />
                                    )}
                                    label="Enable"
                                />
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
                                label="mcp_web_search"
                            />
                            <FormControlLabel
                                control={<Checkbox checked={enableFetch} onChange={(e) => setEnableFetch(e.target.checked)} />}
                                label="mcp_web_fetch"
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
        </PageLayout>
    );
};

export default MCPBuiltin;
