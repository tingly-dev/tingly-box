import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    FormControlLabel,
    Grid,
    InputAdornment,
    IconButton,
    Snackbar,
    Stack,
    Switch,
    TextField,
} from '@mui/material';
import {
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

const MCPBuiltin = () => {
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [notification, setNotification] = useState({ open: false, message: '', severity: 'success' as 'success' | 'error' });
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);

    // Inline edit state for webtools
    const [enabled, setEnabled] = useState(false);
    const [serperApiKey, setSerperApiKey] = useState('');
    const [showApiKey, setShowApiKey] = useState(false);
    const [proxyUrl, setProxyUrl] = useState('');

    useEffect(() => {
        loadMCPConfig();
    }, []);

    const loadMCPConfig = async () => {
        setLoading(true);
        const result: MCPConfigResponse = await api.getMCPConfig();
        if (result.success && result.config) {
            setAllSources(result.config.sources || []);
            const webtools = (result.config.sources || []).find(s => s.id === 'webtools');
            if (webtools) {
                setEnabled(true);
                setSerperApiKey(webtools.env?.SERPER_API_KEY || '');
                setProxyUrl(webtools.proxy_url || '');
            }
        }
        setLoading(false);
    };

    const handleSave = async () => {
        if (enabled && !serperApiKey.trim()) {
            setNotification({ open: true, message: 'Serper API Key is required when enabled', severity: 'error' });
            return;
        }
        setSaving(true);

        let newSources = allSources.filter(s => !BUILTIN_IDS.includes(s.id || ''));
        if (enabled) {
            const env: Record<string, string> = {};
            if (serperApiKey) env['SERPER_API_KEY'] = serperApiKey;
            if (proxyUrl) {
                env['HTTP_PROXY'] = proxyUrl;
                env['HTTPS_PROXY'] = proxyUrl;
            }
            newSources.push({
                id: 'webtools',
                transport: 'stdio',
                command: 'python3',
                args: ['~/.tingly-box/mcp/scripts/mcp_web_tools.py'],
                cwd: '~/.tingly-box/mcp',
                tools: ['web_search', 'web_fetch'],
                env: Object.keys(env).length > 0 ? env : undefined,
                proxy_url: proxyUrl || undefined,
            });
        }

        const result = await api.setMCPConfig({ sources: newSources });
        if (result.success) {
            setNotification({ open: true, message: 'Saved. Restart server to apply.', severity: 'success' });
            setAllSources(newSources);
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

    const builtinServers = allSources.filter(s => BUILTIN_IDS.includes(s.id || ''));

    return (
        <PageLayout>
            <Stack spacing={3}>
                <Alert severity="info">
                    Built-in MCP tools bundled with tingly-box. web_search uses Serper API, web_fetch uses Jina Reader.
                </Alert>

                <UnifiedCard title="Builtin MCP Servers" size="full">
                    <Stack spacing={1.5}>
                        {/* Header */}
                        <Grid container spacing={2} sx={{ fontWeight: 'bold', borderBottom: '1px solid', borderColor: 'divider', pb: 1 }}>
                            <Grid size={{ xs: 1 }}>ID</Grid>
                            <Grid size={{ xs: 1 }}>Transport</Grid>
                            <Grid size={{ xs: 3 }}>Command</Grid>
                            <Grid size={{ xs: 2 }}>Tools</Grid>
                            <Grid size={{ xs: 5 }}>Config</Grid>
                        </Grid>

                        {/* Webtools row - always shown */}
                        <Grid container spacing={2} alignItems="center">
                            <Grid size={{ xs: 1 }}>
                                <Chip label="webtools" size="small" color="primary" />
                            </Grid>
                            <Grid size={{ xs: 1 }}>
                                <Chip label="STDIO" size="small" sx={{ bgcolor: 'success.main', color: 'white' }} />
                            </Grid>
                            <Grid size={{ xs: 3 }}>
                                <TextField
                                    size="small"
                                    fullWidth
                                    value="python3 ~/.tingly-box/mcp/scripts/mcp_web_tools.py"
                                    InputProps={{ readOnly: true }}
                                    sx={{ '& input': { fontFamily: 'monospace', fontSize: '0.75rem' } }}
                                />
                            </Grid>
                            <Grid size={{ xs: 2 }}>
                                <Stack direction="row" spacing={0.5}>
                                    <Chip label="web_search" size="small" variant="outlined" sx={{ fontSize: '0.7rem', height: 20 }} />
                                    <Chip label="web_fetch" size="small" variant="outlined" sx={{ fontSize: '0.7rem', height: 20 }} />
                                </Stack>
                            </Grid>
                            <Grid size={{ xs: 5 }}>
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <FormControlLabel
                                        control={<Switch size="small" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />}
                                        label=""
                                        sx={{ mr: 0 }}
                                    />
                                    <TextField
                                        size="small"
                                        label="API Key"
                                        type={showApiKey ? 'text' : 'password'}
                                        value={serperApiKey}
                                        onChange={(e) => setSerperApiKey(e.target.value)}
                                        sx={{ flex: 1 }}
                                        InputProps={{
                                            endAdornment: (
                                                <InputAdornment position="end">
                                                    <IconButton onClick={() => setShowApiKey(!showApiKey)} size="small">
                                                        {showApiKey ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
                                                    </IconButton>
                                                </InputAdornment>
                                            ),
                                        }}
                                    />
                                    <TextField
                                        size="small"
                                        label="Proxy"
                                        value={proxyUrl}
                                        onChange={(e) => setProxyUrl(e.target.value)}
                                        placeholder="http://127.0.0.1:7890"
                                        sx={{ flex: 1 }}
                                    />
                                </Stack>
                            </Grid>
                        </Grid>
                    </Stack>
                </UnifiedCard>

                <Stack direction="row" justifyContent="flex-end">
                    <Button variant="contained" startIcon={<SaveIcon />} onClick={handleSave} disabled={saving}>
                        {saving ? 'Saving...' : 'Save'}
                    </Button>
                </Stack>
            </Stack>

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

export default MCPBuiltin;
