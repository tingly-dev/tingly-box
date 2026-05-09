import { PageLayout } from '@/components/PageLayout';
import AgentInstallCard from '@/components/AgentInstallCard';
import ToolCard from '@/components/ToolCard';
import { api } from '@/services/api';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    InputAdornment,
    Snackbar,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import {
    Add as AddIcon,
    DeleteOutline as DeleteOutlineIcon,
    Edit as EditIcon,
    InfoOutlined as InfoIcon,
    Visibility as VisibilityIcon,
    VisibilityOff as VisibilityOffIcon,
} from '@mui/icons-material';
import {
    IconSearch,
    IconWorldDownload,
    IconServer,
} from '@tabler/icons-react';
import { useEffect, useState } from 'react';
import MCPSourceEditor from './MCPSourceEditor';
import {
    BUILTIN_IDS,
    BUILTIN_WEBTOOLS_ID,
    defaultMCPSourceFormValue,
    formValueToSource,
    sourceToFormValue,
    type MCPConfigResponse,
    type MCPSourceConfig,
    type MCPSourceFormValue,
} from './types';

// ─── Constants ───────────────────────────────────────────────────────────────

// ─── Sub-components ──────────────────────────────────────────────────────────

interface ConfigRowProps {
    label: string;
    hint?: string;
    hintLink?: string;
    children: React.ReactNode;
}

const ConfigRow: React.FC<ConfigRowProps> = ({ label, hint, hintLink, children }) => (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 1.25, maxWidth: 700 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 160, flexShrink: 0 }}>
            <Typography variant="subtitle2" color="text.secondary" fontWeight={500}>
                {label}
            </Typography>
            {hint && (
                <Tooltip
                    title={hintLink
                        ? <span>{hint} <a href={hintLink} target="_blank" rel="noopener noreferrer" style={{ color: 'inherit' }}>{hintLink.replace('https://', '')}</a></span>
                        : hint}
                    arrow
                    placement="top"
                >
                    <InfoIcon sx={{ fontSize: '0.9rem', color: 'text.disabled', cursor: 'help' }} />
                </Tooltip>
            )}
        </Box>
        <Box sx={{ flex: 1, minWidth: 0 }}>
            {children}
        </Box>
    </Box>
);

interface SecretInputProps {
    value: string;
    onChange: (v: string) => void;
    onBlur?: () => void;
    placeholder?: string;
}

const SecretInput: React.FC<SecretInputProps> = ({ value, onChange, onBlur, placeholder }) => {
    const [visible, setVisible] = useState(false);
    return (
        <TextField
            fullWidth
            size="small"
            type={visible ? 'text' : 'password'}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            onBlur={onBlur}
            placeholder={placeholder || 'Enter value'}
            InputProps={{
                endAdornment: (
                    <InputAdornment position="end">
                        <IconButton size="small" onClick={() => setVisible((v) => !v)} edge="end">
                            {visible ? <VisibilityOffIcon fontSize="small" /> : <VisibilityIcon fontSize="small" />}
                        </IconButton>
                    </InputAdornment>
                ),
                sx: { fontFamily: 'monospace', fontSize: '0.8rem' },
            }}
        />
    );
};

// ─── Part 2: Builtin web tools (two separate ToolCards) ──────────────────────

interface WebtoolCardProps {
    webtoolsSource: MCPSourceConfig | undefined;
    toolName: 'mcp_web_search' | 'mcp_web_fetch';
    onSave: (patch: MCPSourceConfig) => Promise<void>;
}

const WebtoolCard: React.FC<WebtoolCardProps> = ({ webtoolsSource, toolName, onSave }) => {
    const [serperKey, setSerperKey] = useState('');
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        setSerperKey(webtoolsSource?.env?.['SERPER_API_KEY'] ?? '');
    }, [webtoolsSource]);

    const enabled = webtoolsSource?.enabled ?? true;

    const handleToggle = (next: boolean) => {
        if (!webtoolsSource) return;
        const { visibility, ...rest } = webtoolsSource as any;
        void onSave({ ...rest, enabled: next });
    };

    const handleSave = async () => {
        setSaving(true);
        try {
            const { visibility, ...base } = (webtoolsSource ?? {
                id: BUILTIN_WEBTOOLS_ID,
                name: 'Built-in Web Tools',
                transport: 'stdio',
                command: 'tingly-box',
                args: ['mcp-builtin'],
                tools: ['mcp_web_search', 'mcp_web_fetch'],
                enabled: true,
            }) as any;
            await onSave({ ...base, env: serperKey ? { SERPER_API_KEY: serperKey } : {} });
        } finally {
            setSaving(false);
        }
    };

    const isSearch = toolName === 'mcp_web_search';
    const needsConfig = isSearch && !serperKey;

    const settings = (
        <Stack spacing={1.5}>
            {isSearch && (
                <ConfigRow
                    label="Serper API Key"
                    hint="Required for web_search. Get your free key at serper.dev"
                    hintLink="https://serper.dev"
                >
                    <SecretInput value={serperKey} onChange={setSerperKey} placeholder="Enter Serper API key" />
                </ConfigRow>
            )}
            {!isSearch && (
                <ConfigRow
                    label="Fetch engine"
                    hint="Uses Jina Reader to convert web pages to clean markdown. No API key required."
                    hintLink="https://jina.ai/reader"
                >
                    <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                        jina.ai/reader
                    </Typography>
                </ConfigRow>
            )}
            {isSearch && (
                <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                    <Button variant="contained" size="small" onClick={() => void handleSave()} disabled={saving}>
                        {saving ? 'Saving...' : 'Save'}
                    </Button>
                </Box>
            )}
        </Stack>
    );

    return (
        <ToolCard
            icon={isSearch ? <IconSearch size={18} /> : <IconWorldDownload size={18} />}
            name={toolName}
            description={isSearch
                ? 'Browser-side web search via Serper. Requires an API key.'
                : 'Fetches public web pages via Jina Reader and returns markdown for agents to read.'}
            enabled={enabled}
            onToggle={handleToggle}
            badges={[{ label: 'Client', color: 'blue' }]}
            settings={settings}
            defaultExpanded={needsConfig}
        />
    );
};

// ─── Part 3: Custom MCP servers ───────────────────────────────────────────────

interface CustomServersCardProps {
    sources: MCPSourceConfig[];
    onSave: (sources: MCPSourceConfig[]) => Promise<void>;
    saving: boolean;
}

const CustomServersCard: React.FC<CustomServersCardProps> = ({ sources, onSave, saving }) => {
    const [dialogOpen, setDialogOpen] = useState(false);
    const [editingSource, setEditingSource] = useState<MCPSourceConfig | null>(null);
    const [editorForm, setEditorForm] = useState<MCPSourceFormValue>(() => ({
        ...defaultMCPSourceFormValue(),
        args: [],
        tools: ['*'],
        envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY'],
        useGlobalProxy: true,
    }));

    const openAdd = () => {
        setEditingSource(null);
        setEditorForm({
            ...defaultMCPSourceFormValue(),
            args: [],
            tools: ['*'],
            envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY'],
            useGlobalProxy: true,
        });
        setDialogOpen(true);
    };

    const openEdit = (source: MCPSourceConfig) => {
        setEditingSource(source);
        setEditorForm(sourceToFormValue(source));
        setDialogOpen(true);
    };

    const closeDialog = () => {
        setDialogOpen(false);
        setEditingSource(null);
    };

    const handleDelete = async (id?: string) => {
        if (!id) return;
        await onSave(sources.filter((s) => s.id !== id));
    };

    const handleToggle = async (id: string | undefined, enabled: boolean) => {
        if (!id) return;
        await onSave(sources.map((s) => (s.id === id ? { ...s, enabled } : s)));
    };

    const handleDialogSave = async () => {
        const source = formValueToSource(editorForm);
        if (!source.id) return;
        const next = [...sources];
        const idx = next.findIndex((s) => s.id === source.id);
        if (idx >= 0) { next[idx] = source; } else { next.push(source); }
        await onSave(next);
        closeDialog();
    };

    const connectionLabel = (source: MCPSourceConfig): string => {
        if (source.transport === 'http' || source.transport === 'sse') return source.endpoint || '-';
        return source.command
            ? `${source.command}${source.args?.length ? ' ' + source.args.join(' ') : ''}`
            : '-';
    };

    return (
        <Stack spacing={1.5}>
            {/* Section header + add button */}
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 2 }}>
                    <Typography
                        sx={{ fontFamily: 'monospace', fontSize: '0.7rem', fontWeight: 600, color: 'text.disabled', mt: 0.5, flexShrink: 0, userSelect: 'none' }}
                    >
                        03
                    </Typography>
                    <Box>
                        <Typography variant="h6" sx={{ fontWeight: 700, lineHeight: 1.2, mb: 0.5 }}>
                            Remote servers
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            Third-party MCP servers connected to your gateway.
                        </Typography>
                    </Box>
                </Box>
                <Tooltip title="Connect server">
                    <IconButton
                        onClick={openAdd}
                        size="medium"
                        sx={{
                            border: '1px solid',
                            borderColor: 'divider',
                            borderRadius: '10px',
                            bgcolor: 'background.paper',
                            '&:hover': { bgcolor: 'action.hover' },
                        }}
                    >
                        <AddIcon />
                    </IconButton>
                </Tooltip>
            </Box>

            {/* Existing servers as ToolCards */}
            {sources.map((source) => {
                const enabled = source.enabled ?? true;
                const conn = connectionLabel(source);
                return (
                    <ToolCard
                        key={source.id}
                        icon={<IconServer size={18} />}
                        name={source.id || ''}
                        description={conn}
                        enabled={enabled}
                        onToggle={(v) => void handleToggle(source.id, v)}
                        badges={[{
                            label: source.visibility === 'server' ? 'Server' : 'Client',
                            color: source.visibility === 'server' ? 'green' : 'blue',
                        }]}
                        tags={source.transport ? [(source.transport).toUpperCase()] : []}
                        actions={
                            <Stack direction="row" spacing={0.5} alignItems="center">
                                <Tooltip title="Edit">
                                    <IconButton size="small" color="primary" onClick={() => openEdit(source)}>
                                        <EditIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title="Delete">
                                    <IconButton size="small" color="error" onClick={() => void handleDelete(source.id)}>
                                        <DeleteOutlineIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Stack>
                        }
                    />
                );
            })}

            {/* Add / Edit dialog */}
            <Dialog open={dialogOpen} onClose={closeDialog} maxWidth="md" fullWidth>
                <DialogTitle>{editingSource ? `Edit — ${editingSource.id}` : 'Connect custom MCP server'}</DialogTitle>
                <DialogContent sx={{ pt: 1 }}>
                    <MCPSourceEditor value={editorForm} onChange={setEditorForm} />
                </DialogContent>
                <DialogActions>
                    <Button onClick={closeDialog}>Cancel</Button>
                    <Button variant="contained" onClick={() => void handleDialogSave()} disabled={saving}>
                        {saving ? 'Saving...' : 'Save'}
                    </Button>
                </DialogActions>
            </Dialog>
        </Stack>
    );
};

// ─── Main page ────────────────────────────────────────────────────────────────

const MCPRegisteredServers = () => {
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);
    const [notification, setNotification] = useState({ open: false, message: '', severity: 'success' as 'success' | 'error' });

    useEffect(() => { void loadData(); }, []);

    const loadData = async () => {
        setLoading(true);
        const result: MCPConfigResponse = await api.getMCPConfig();
        if (result.success && result.config) {
            setAllSources(result.config.sources || []);
        }
        setLoading(false);
    };

    const saveConfig = async (sources: MCPSourceConfig[]): Promise<void> => {
        setSaving(true);
        const result = await api.setMCPConfig({ sources });
        if (result.success) {
            setAllSources(sources);
            setNotification({ open: true, message: 'Saved. Reconnect MCP client to apply.', severity: 'success' });
        } else {
            setNotification({ open: true, message: result.error || 'Failed to save', severity: 'error' });
        }
        setSaving(false);
    };

    const upsertSource = async (patch: MCPSourceConfig): Promise<void> => {
        const next = [...allSources];
        const idx = next.findIndex((s) => s.id === patch.id);
        if (idx >= 0) { next[idx] = patch; } else { next.push(patch); }
        await saveConfig(next);
    };

    const webtoolsSource = allSources.find((s) => s.id === BUILTIN_WEBTOOLS_ID);
    const customSources = allSources.filter((s) => !BUILTIN_IDS.includes(s.id as any));

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
            <Stack spacing={5}>
                {/* Part 1: Install instructions */}
                <AgentInstallCard />

                {/* Part 2: Builtin web tools */}
                <Box>
                    <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 2, mb: 2.5 }}>
                        <Typography
                            sx={{ fontFamily: 'monospace', fontSize: '0.7rem', fontWeight: 600, color: 'text.disabled', mt: 0.5, flexShrink: 0, userSelect: 'none' }}
                        >
                            02
                        </Typography>
                        <Box>
                            <Typography variant="h6" sx={{ fontWeight: 700, lineHeight: 1.2, mb: 0.5 }}>
                                Builtin tools
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                Tools built into the gateway. Click a card to configure.
                            </Typography>
                        </Box>
                    </Box>
                    <Stack spacing={1.5}>
                        <WebtoolCard webtoolsSource={webtoolsSource} toolName="mcp_web_search" onSave={upsertSource} />
                        <WebtoolCard webtoolsSource={webtoolsSource} toolName="mcp_web_fetch" onSave={upsertSource} />
                    </Stack>
                </Box>

                {/* Part 3: Custom remote servers */}
                <CustomServersCard
                    sources={customSources}
                    onSave={async (updated) => {
                        const builtins = allSources.filter((s) => BUILTIN_IDS.includes(s.id as any));
                        await saveConfig([...builtins, ...updated]);
                    }}
                    saving={saving}
                />
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
