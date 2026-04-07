import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    IconButton,
    Snackbar,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import {
    Add as AddIcon,
    DeleteOutline as DeleteOutlineIcon,
    Edit as EditIcon,
} from '@mui/icons-material';
import { useEffect, useState } from 'react';
import MCPSourceEditor from './MCPSourceEditor';
import { BUILTIN_IDS, defaultMCPSourceFormValue, formValueToSource, sourceToFormValue, type MCPConfigResponse, type MCPSourceConfig, type MCPSourceFormValue } from './types';

const weatherTemplate = (): MCPSourceFormValue => ({
    ...defaultMCPSourceFormValue(),
    id: 'weather',
    transport: 'stdio',
    command: 'python3',
    args: ['mcp_weather_tools.py'],
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

const MCPCustom = () => {
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [notification, setNotification] = useState({ open: false, message: '', severity: 'success' as 'success' | 'error' });
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);
    const [customSources, setCustomSources] = useState<MCPSourceConfig[]>([]);
    const [editingId, setEditingId] = useState<string>('');
    const [form, setForm] = useState<MCPSourceFormValue>(emptyCustomTemplate());
    const [editorMode, setEditorMode] = useState<'none' | 'add' | 'edit'>('none');

    useEffect(() => {
        void loadMCPConfig();
    }, []);

    const loadMCPConfig = async () => {
        setLoading(true);
        const result: MCPConfigResponse = await api.getMCPConfig();
        if (result.success && result.config) {
            const sources = result.config.sources || [];
            const custom = sources.filter((s) => !BUILTIN_IDS.includes(s.id || ''));
            setAllSources(sources);
            setCustomSources(custom);
            setEditingId('');
            setForm(emptyCustomTemplate());
            setEditorMode('none');
        }
        setLoading(false);
    };

    const deleteSource = (id?: string) => {
        if (!id) return;
        const updated = customSources.filter((s) => s.id !== id);
        setCustomSources(updated);
        if (editingId === id) {
            setEditingId(updated[0]?.id || '');
            setForm(updated[0] ? sourceToFormValue(updated[0]) : emptyCustomTemplate());
        }
    };

    const saveAll = async () => {
        const source = formValueToSource(form);
        const hasEditorOpen = editorMode !== 'none';
        let mergedCustom = [...customSources];

        if (hasEditorOpen) {
            if (!source.id) {
                setNotification({ open: true, message: 'Server name is required', severity: 'error' });
                return;
            }
            if (source.transport === 'http' && !source.endpoint) {
                setNotification({ open: true, message: 'HTTP endpoint is required', severity: 'error' });
                return;
            }
            if (source.transport === 'stdio' && !source.command) {
                setNotification({ open: true, message: 'Command is required', severity: 'error' });
                return;
            }
            const idx = mergedCustom.findIndex((s) => s.id === source.id);
            if (idx >= 0) {
                mergedCustom[idx] = source;
            } else {
                mergedCustom.push(source);
            }
        }

        setSaving(true);
        const builtinSources = allSources.filter((s) => BUILTIN_IDS.includes(s.id || ''));
        const newSources = [...builtinSources, ...mergedCustom];
        const result = await api.setMCPConfig({ sources: newSources });
        if (result.success) {
            setNotification({ open: true, message: 'Saved. Restart server to apply.', severity: 'success' });
            setAllSources(newSources);
            setCustomSources(mergedCustom);
            setEditorMode('none');
        } else {
            setNotification({ open: true, message: result.error || 'Failed to save', severity: 'error' });
        }
        setSaving(false);
    };

    const openAdd = () => {
        setEditingId('');
        setForm(emptyCustomTemplate());
        setEditorMode('add');
    };

    const openEdit = (source: MCPSourceConfig) => {
        setEditingId(source.id || '');
        setForm(sourceToFormValue(source));
        setEditorMode('edit');
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
                    Custom MCP supports local stdio and remote streamable HTTP servers. Builtin web_search/web_fetch stays in Builtin tab.
                </Alert>

                <UnifiedCard
                    title="Custom MCP Servers"
                    size="full"
                >
                    <Stack spacing={1.5}>
                        <Stack direction="row" justifyContent="flex-end">
                            <Tooltip title="Add Server">
                                <IconButton onClick={openAdd} color="primary">
                                    <AddIcon />
                                </IconButton>
                            </Tooltip>
                        </Stack>
                        {customSources.length > 0 ? (
                            <Stack spacing={1}>
                                {customSources.map((source) => {
                                    const active = source.id === editingId;
                                    return (
                                        <Stack key={source.id} direction="row" justifyContent="space-between" alignItems="center">
                                            <Chip
                                                label={`${source.id} (${source.transport || 'stdio'})`}
                                                color={active ? 'primary' : 'default'}
                                                onClick={() => setEditingId(source.id || '')}
                                            />
                                            <Stack direction="row" spacing={0.5}>
                                                <Tooltip title="Edit">
                                                    <IconButton size="small" color="primary" onClick={() => openEdit(source)}>
                                                        <EditIcon fontSize="small" />
                                                    </IconButton>
                                                </Tooltip>
                                                <Tooltip title="Delete">
                                                    <IconButton size="small" color="error" onClick={() => deleteSource(source.id)}>
                                                        <DeleteOutlineIcon fontSize="small" />
                                                    </IconButton>
                                                </Tooltip>
                                            </Stack>
                                        </Stack>
                                    );
                                })}
                            </Stack>
                        ) : (
                            <Typography variant="body2" color="text.secondary">No custom MCP servers yet.</Typography>
                        )}
                    </Stack>
                </UnifiedCard>

                {editorMode !== 'none' && (
                    <>
                        <MCPSourceEditor
                            title="Connect to a custom MCP"
                            value={form}
                            onChange={setForm}
                            onUseExample={() => {
                                setEditingId('');
                                setForm(weatherTemplate());
                                setEditorMode('add');
                            }}
                        />

                        <Stack direction="row" justifyContent="flex-start">
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

export default MCPCustom;
