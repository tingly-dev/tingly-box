import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Grid,
    IconButton,
    InputAdornment,
    MenuItem,
    Select,
    Snackbar,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import {
    Add as AddIcon,
    Delete as DeleteIcon,
    Edit as EditIcon,
    Save as SaveIcon,
    Visibility,
    VisibilityOff,
} from '@mui/icons-material';
import { useEffect, useState } from 'react';
import { api } from '@/services/api';

interface MCPSourceConfig {
    id?: string;
    transport?: 'http' | 'stdio';
    endpoint?: string;
    headers?: Record<string, string>;
    tools?: string[];
    command?: string;
    args?: string[];
    cwd?: string;
    env?: Record<string, string>;
    proxy_url?: string;
}

interface MCPRuntimeConfig {
    sources?: MCPSourceConfig[];
    request_timeout?: number;
}

interface MCPConfigResponse {
    success: boolean;
    config?: MCPRuntimeConfig;
    error?: string;
}

const BUILTIN_IDS = ['webtools'];

const MCPCustom = () => {
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [notification, setNotification] = useState({ open: false, message: '', severity: 'success' as 'success' | 'error' });
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);
    const [customSources, setCustomSources] = useState<MCPSourceConfig[]>([]);

    // Dialog state
    const [dialogOpen, setDialogOpen] = useState(false);
    const [editingIndex, setEditingIndex] = useState<number | null>(null);

    // Dialog form state
    const [formId, setFormId] = useState('');
    const [formTransport, setFormTransport] = useState<'http' | 'stdio'>('stdio');
    const [formEndpoint, setFormEndpoint] = useState('');
    const [formArgs, setFormArgs] = useState('~/.tingly-box/mcp/scripts/mcp_web_tools.py');
    const [formTools, setFormTools] = useState<string[]>(['*']);
    const [formSerperApiKey, setFormSerperApiKey] = useState('');
    const [formShowApiKey, setFormShowApiKey] = useState(false);
    const [formProxyUrl, setFormProxyUrl] = useState('');

    useEffect(() => {
        loadMCPConfig();
    }, []);

    const loadMCPConfig = async () => {
        setLoading(true);
        const result: MCPConfigResponse = await api.getMCPConfig();
        if (result.success && result.config) {
            const sources = result.config.sources || [];
            setAllSources(sources);
            setCustomSources(sources.filter(s => !BUILTIN_IDS.includes(s.id || '')));
        }
        setLoading(false);
    };

    const handleSave = async () => {
        setSaving(true);
        const fullResult: MCPConfigResponse = await api.getMCPConfig();
        let newSources = (fullResult.config?.sources || []).filter(s => BUILTIN_IDS.includes(s.id || ''));
        newSources = [...newSources, ...customSources];

        const result = await api.setMCPConfig({ sources: newSources });
        if (result.success) {
            setNotification({ open: true, message: 'Saved. Restart server to apply.', severity: 'success' });
            setAllSources(newSources);
        } else {
            setNotification({ open: true, message: result.error || 'Failed to save', severity: 'error' });
        }
        setSaving(false);
    };

    const openAddDialog = () => {
        setEditingIndex(null);
        setFormId('');
        setFormTransport('stdio');
        setFormEndpoint('');
        setFormArgs('~/.tingly-box/mcp/scripts/mcp_web_tools.py');
        setFormTools(['*']);
        setFormSerperApiKey('');
        setFormProxyUrl('');
        setDialogOpen(true);
    };

    const openEditDialog = (index: number, source: MCPSourceConfig) => {
        setEditingIndex(index);
        setFormId(source.id || '');
        setFormTransport(source.transport || 'stdio');
        setFormEndpoint(source.endpoint || '');
        setFormArgs(source.args?.join(' ') || '~/.tingly-box/mcp/scripts/mcp_web_tools.py');
        setFormTools(source.tools || []);
        setFormSerperApiKey(source.env?.SERPER_API_KEY || '');
        setFormProxyUrl(source.proxy_url || '');
        setDialogOpen(true);
    };

    const handleDialogSave = () => {
        const source: MCPSourceConfig = {
            id: formId,
            transport: formTransport,
            tools: formTools,
        };

        if (formTransport === 'http') {
            source.endpoint = formEndpoint;
            source.headers = {};
        } else {
            source.command = 'python3';
            source.args = formArgs.trim() ? [formArgs.trim()] : [];
            source.cwd = '~/.tingly-box/mcp';
        }

        const env: Record<string, string> = {};
        if (formSerperApiKey) env['SERPER_API_KEY'] = formSerperApiKey;
        if (formProxyUrl) {
            source.proxy_url = formProxyUrl;
            if (formTransport === 'stdio') {
                env['HTTP_PROXY'] = formProxyUrl;
                env['HTTPS_PROXY'] = formProxyUrl;
            }
        }
        if (Object.keys(env).length > 0) source.env = env;

        if (editingIndex !== null) {
            const updated = [...customSources];
            updated[editingIndex] = source;
            setCustomSources(updated);
            setAllSources([...allSources.filter(s => BUILTIN_IDS.includes(s.id || '')), ...updated]);
        } else {
            setCustomSources([...customSources, source]);
            setAllSources([...allSources.filter(s => BUILTIN_IDS.includes(s.id || '')), ...customSources, source]);
        }
        setDialogOpen(false);
    };

    const handleDeleteSource = (index: number) => {
        const updated = customSources.filter((_, i) => i !== index);
        setCustomSources(updated);
        setAllSources([...allSources.filter(s => BUILTIN_IDS.includes(s.id || '')), ...updated]);
    };

    const handleToolToggle = (tool: string) => {
        if (formTools.includes(tool)) {
            setFormTools(formTools.filter(t => t !== tool));
        } else {
            setFormTools([...formTools, tool]);
        }
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
            <Stack spacing={3}>
                <Alert severity="info">
                    Add custom MCP servers (stdio or HTTP). Builtin web_search/web_fetch is configured on the Builtin tab.
                </Alert>

                <UnifiedCard
                    title="Custom MCP Servers"
                    size="full"
                    rightAction={
                        <Button startIcon={<AddIcon />} onClick={openAddDialog} size="small" variant="outlined">
                            Add Server
                        </Button>
                    }
                >
                    {customSources.length === 0 ? (
                        <Box sx={{ textAlign: 'center', py: 4 }}>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                                No custom MCP servers.
                            </Typography>
                            <Button startIcon={<AddIcon />} onClick={openAddDialog} variant="outlined">
                                Add Server
                            </Button>
                        </Box>
                    ) : (
                        <Stack spacing={1.5}>
                            {/* Header */}
                            <Grid container spacing={2} sx={{ fontWeight: 'bold', borderBottom: '1px solid', borderColor: 'divider', pb: 1 }}>
                                <Grid size={{ xs: 2 }}>ID</Grid>
                                <Grid size={{ xs: 1 }}>Transport</Grid>
                                <Grid size={{ xs: 4 }}>Command / Endpoint</Grid>
                                <Grid size={{ xs: 2 }}>Tools</Grid>
                                <Grid size={{ xs: 3 }} />
                            </Grid>

                            {customSources.map((source, index) => (
                                <Grid container spacing={2} key={source.id} alignItems="center">
                                    <Grid size={{ xs: 2 }}>
                                        <Typography variant="subtitle2" fontWeight="bold">{source.id}</Typography>
                                    </Grid>
                                    <Grid size={{ xs: 1 }}>
                                        <Box
                                            sx={{
                                                px: 1, py: 0.25,
                                                bgcolor: source.transport === 'http' ? 'info.main' : 'success.main',
                                                color: 'white', borderRadius: 0.5,
                                                fontSize: '0.7rem', textAlign: 'center', width: 'fit-content',
                                            }}
                                        >
                                            {source.transport?.toUpperCase()}
                                        </Box>
                                    </Grid>
                                    <Grid size={{ xs: 4 }}>
                                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem', wordBreak: 'break-all' }}>
                                            {source.transport === 'http' ? source.endpoint : `${source.command} ${source.args?.join(' ')}`}
                                        </Typography>
                                    </Grid>
                                    <Grid size={{ xs: 2 }}>
                                        <Stack direction="row" spacing={0.5} flexWrap="wrap">
                                            {source.tools?.map(tool => (
                                                <Chip key={tool} label={tool} size="small" variant="outlined" sx={{ fontSize: '0.7rem', height: 20 }} />
                                            ))}
                                        </Stack>
                                    </Grid>
                                    <Grid size={{ xs: 3 }}>
                                        <Stack direction="row" spacing={0.5}>
                                            <IconButton size="small" onClick={() => openEditDialog(index, source)}>
                                                <EditIcon fontSize="small" />
                                            </IconButton>
                                            <IconButton size="small" onClick={() => handleDeleteSource(index)}>
                                                <DeleteIcon fontSize="small" />
                                            </IconButton>
                                        </Stack>
                                    </Grid>
                                </Grid>
                            ))}
                        </Stack>
                    )}
                </UnifiedCard>

                <Stack direction="row" justifyContent="flex-end">
                    <Button variant="contained" startIcon={<SaveIcon />} onClick={handleSave} disabled={saving}>
                        {saving ? 'Saving...' : 'Save'}
                    </Button>
                </Stack>
            </Stack>

            {/* Add/Edit Dialog */}
            <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{editingIndex !== null ? 'Edit MCP Server' : 'Add MCP Server'}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2.5} sx={{ pt: 1 }}>
                        <TextField
                            size="small" fullWidth label="Server ID"
                            value={formId} onChange={(e) => setFormId(e.target.value)}
                            placeholder="e.g., myserver" required
                        />

                        <Box>
                            <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>Transport</Typography>
                            <Select
                                size="small" fullWidth
                                value={formTransport}
                                onChange={(e) => setFormTransport(e.target.value as 'http' | 'stdio')}
                            >
                                <MenuItem value="stdio">Stdio (Local subprocess)</MenuItem>
                                <MenuItem value="http">HTTP (Remote server)</MenuItem>
                            </Select>
                        </Box>

                        {formTransport === 'http' ? (
                            <TextField
                                size="small" fullWidth label="Endpoint"
                                value={formEndpoint} onChange={(e) => setFormEndpoint(e.target.value)}
                                placeholder="http://localhost:3000"
                            />
                        ) : (
                            <TextField
                                size="small" fullWidth label="Script Path"
                                value={formArgs} onChange={(e) => setFormArgs(e.target.value)}
                                placeholder="~/.tingly-box/mcp/scripts/mcp_web_tools.py"
                                helperText="Python script path"
                            />
                        )}

                        <Box>
                            <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>Tools</Typography>
                            <Stack direction="row" spacing={1}>
                                {['web_search', 'web_fetch', '*'].map(tool => (
                                    <Box
                                        key={tool}
                                        onClick={() => handleToolToggle(tool)}
                                        sx={{
                                            px: 1.5, py: 0.5,
                                            border: '1px solid',
                                            borderColor: formTools.includes(tool) ? 'primary.main' : 'divider',
                                            borderRadius: 1,
                                            bgcolor: formTools.includes(tool) ? 'primary.main' : 'transparent',
                                            color: formTools.includes(tool) ? 'primary.contrastText' : 'text.primary',
                                            cursor: 'pointer', fontSize: '0.875rem',
                                        }}
                                    >
                                        {tool}
                                    </Box>
                                ))}
                            </Stack>
                        </Box>

                        <TextField
                            size="small" fullWidth label="API Key"
                            type={formShowApiKey ? 'text' : 'password'}
                            value={formSerperApiKey} onChange={(e) => setFormSerperApiKey(e.target.value)}
                            placeholder="Optional"
                            InputProps={{
                                endAdornment: (
                                    <InputAdornment position="end">
                                        <IconButton onClick={() => setFormShowApiKey(!formShowApiKey)} size="small">
                                            {formShowApiKey ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
                                        </IconButton>
                                    </InputAdornment>
                                ),
                            }}
                        />

                        <TextField
                            size="small" fullWidth label="Proxy URL"
                            value={formProxyUrl} onChange={(e) => setFormProxyUrl(e.target.value)}
                            placeholder="http://127.0.0.1:7890"
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setDialogOpen(false)}>Cancel</Button>
                    <Button onClick={handleDialogSave} variant="contained" disabled={!formId || (formTransport === 'http' && !formEndpoint)}>
                        {editingIndex !== null ? 'Save' : 'Add'}
                    </Button>
                </DialogActions>
            </Dialog>

            <Snackbar
                open={notification.open} autoHideDuration={4000}
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
