import { PageLayout } from '@/components/PageLayout';
import ModelSelectDialog, { type ProviderSelectTabOption } from '@/components/ModelSelectDialog';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import type { Provider } from '@/types/provider';
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
    Divider,
    FormControlLabel,
    IconButton,
    InputAdornment,
    Paper,
    Snackbar,
    Stack,
    Switch,
    Tab,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Tabs,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import {
    Add as AddIcon,
    ContentCopy as CopyIcon,
    DeleteOutline as DeleteOutlineIcon,
    Edit as EditIcon,
    InfoOutlined as InfoIcon,
    PowerSettingsNew as PowerIcon,
    Visibility as VisibilityIcon,
    VisibilityOff as VisibilityOffIcon,
} from '@mui/icons-material';
import { useEffect, useState, useCallback } from 'react';
import { useLocation } from 'react-router-dom';
import MCPSourceEditor from './MCPSourceEditor';
import {
    BUILTIN_ADVISOR_ID,
    BUILTIN_IDS,
    BUILTIN_WEBTOOLS_ID,
    MCP_DEFAULT_CWD,
    defaultMCPSourceFormValue,
    formValueToSource,
    sourceToFormValue,
    type MCPConfigResponse,
    type MCPSourceConfig,
    type MCPSourceFormValue,
} from './types';

// ─── Constants ───────────────────────────────────────────────────────────────

const MCP_ADD_COMMAND = `claude mcp add --transport http tb "http://localhost:12580/api/v1/mcp/tb" --header "Authorization: Bearer $(cat ~/.tingly-box/config.json | jq -r '.user_token')"`;

const CODEX_ADD_COMMAND = `codex mcp add tb -- http http://localhost:12580/api/v1/mcp/tb \\\n  --header "Authorization: Bearer $(cat ~/.tingly-box/config.json | jq -r '.user_token')"`;

const OPENCODE_CONFIG_PATH = '~/.config/opencode/opencode.json';
const OPENCODE_CONFIG_SNIPPET = `"mcp": {
  "http://localhost:12580/api/v1/mcp/tb": {
    "type": "remote",
    "url": "http://localhost:12580/api/v1/mcp/tb",
    "oauth": false,
    "headers": {
      "Authorization": "Bearer {MY_API_KEY}"
    }
  }
}`;

const ADVISOR_VISIBILITY_STORAGE_KEY = 'tb.mcp.showAdvisor';

const shouldShowAdvisorSection = (search: string): boolean => {
    if (typeof window === 'undefined') {
        return false;
    }

    const params = new URLSearchParams(search);
    const advisorParam = params.get('advisor');

    if (advisorParam === '1' || advisorParam === 'true' || advisorParam === 'on') {
        window.localStorage.setItem(ADVISOR_VISIBILITY_STORAGE_KEY, 'true');
        return true;
    }

    if (advisorParam === '0' || advisorParam === 'false' || advisorParam === 'off') {
        window.localStorage.removeItem(ADVISOR_VISIBILITY_STORAGE_KEY);
        return false;
    }

    return window.localStorage.getItem(ADVISOR_VISIBILITY_STORAGE_KEY) === 'true';
};
// ─── Helpers ──────────────────────────────────────────────────────────────────

const maskSecret = (val: string): string => {
    if (!val) return '';
    if (val.length <= 8) return '●'.repeat(val.length);
    return val.slice(0, 4) + '●'.repeat(Math.min(val.length - 6, 8)) + val.slice(-2);
};

// ─── Sub-components ──────────────────────────────────────────────────────────

interface ConfigRowProps {
    label: string;
    hint?: string;
    children: React.ReactNode;
}

const ConfigRow: React.FC<ConfigRowProps> = ({ label, hint, children }) => (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 1.25, maxWidth: 700 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 160, flexShrink: 0 }}>
            <Typography variant="subtitle2" color="text.secondary" fontWeight={500}>
                {label}
            </Typography>
            {hint && (
                <Tooltip title={hint} arrow placement="top">
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

// ─── Part 1: Add to Agents ────────────────────────────────────────────────────

interface CopyCommandBlockProps {
    text: string;
}

const CopyCommandBlock: React.FC<CopyCommandBlockProps> = ({ text }) => {
    const [copied, setCopied] = useState(false);
    const handleCopy = useCallback(() => {
        void navigator.clipboard.writeText(text.replace(/\\\n\s*/g, ' '));
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    }, [text]);
    return (
        <Box sx={{ bgcolor: 'action.hover', border: '1px solid', borderColor: 'divider', borderRadius: 1.5, p: 1.5, display: 'flex', alignItems: 'flex-start', gap: 1 }}>
            <Typography
                component="pre"
                sx={{ fontFamily: 'monospace', fontSize: '0.78rem', flex: 1, minWidth: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all', color: 'text.primary', m: 0 }}
            >
                {text}
            </Typography>
            <Tooltip title={copied ? 'Copied!' : 'Copy'} arrow>
                <IconButton size="small" onClick={handleCopy} color={copied ? 'success' : 'default'} sx={{ flexShrink: 0 }}>
                    <CopyIcon fontSize="small" />
                </IconButton>
            </Tooltip>
        </Box>
    );
};

const AddToAgentsCard: React.FC = () => {
    const [tab, setTab] = useState(0);

    return (
        <UnifiedCard title="Add to Agents" size="full">
            <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2, borderBottom: 1, borderColor: 'divider' }}>
                <Tab label="Claude Code" />
                <Tab label="Codex" />
                <Tab label="OpenCode" />
            </Tabs>

            {tab === 0 && (
                <Stack spacing={1}>
                    <Typography variant="body2" color="text.secondary">
                        Run this command to register Tingly Box as an MCP server:
                    </Typography>
                    <CopyCommandBlock text={MCP_ADD_COMMAND} />
                </Stack>
            )}

            {tab === 1 && (
                <Stack spacing={1}>
                    <Typography variant="body2" color="text.secondary">
                        Run this command to register Tingly Box as an MCP server:
                    </Typography>
                    <CopyCommandBlock text={CODEX_ADD_COMMAND} />
                </Stack>
            )}

            {tab === 2 && (
                <Stack spacing={1.5}>
                    <Typography variant="body2" color="text.secondary">
                        Add the following to <Typography component="span" sx={{ fontFamily: 'monospace', fontSize: '0.85em' }}>{OPENCODE_CONFIG_PATH}</Typography>:
                    </Typography>
                    <CopyCommandBlock text={OPENCODE_CONFIG_SNIPPET} />
                    <Alert severity="info" sx={{ py: 0.5 }}>
                        Set <code>MY_API_KEY</code> to your token. Run <code>{'cat ~/.tingly-box/config.json | jq -r \'.user_token\''}</code> to get it.
                    </Alert>
                </Stack>
            )}
        </UnifiedCard>
    );
};

// ─── Part 2: Builtin servers (webtools + advisor in one card) ────────────────

interface BuiltinServersCardProps {
    webtoolsSource: MCPSourceConfig | undefined;
    advisorSource: MCPSourceConfig | undefined;
    onSave: (patch: MCPSourceConfig) => Promise<void>;
    saving: boolean;
}

const BuiltinServersCard: React.FC<BuiltinServersCardProps> = ({ webtoolsSource, advisorSource, onSave }) => {
    // webtools state
    const [serperKey, setSerperKey] = useState('');
    const [webtoolsSaving, setWebtoolsSaving] = useState(false);

    // advisor state
    const [model, setModel] = useState('');
    const [selectedProviderUuid, setSelectedProviderUuid] = useState('');
    const [advisorSaving, setAdvisorSaving] = useState(false);
    const [providerCatalog, setProviderCatalog] = useState<Provider[]>([]);
    const [advisorModelDialogOpen, setAdvisorModelDialogOpen] = useState(false);

    useEffect(() => {
        setSerperKey(webtoolsSource?.env?.['SERPER_API_KEY'] ?? '');
    }, [webtoolsSource]);

    useEffect(() => {
        const providerUuid = advisorSource?.env?.['ADVISOR_PROVIDER_UUID'] ?? '';
        const m = advisorSource?.advisor?.model ?? advisorSource?.env?.['ADVISOR_MODEL'] ?? '';
        setSelectedProviderUuid(providerUuid);
        setModel(m);
    }, [advisorSource]);

    useEffect(() => {
        const loadProviders = async () => {
            const result = await api.getProviders();
            if (result?.success && Array.isArray(result.data)) {
                setProviderCatalog(result.data as Provider[]);
            } else {
                setProviderCatalog([]);
            }
        };
        void loadProviders();
    }, []);

    const handleWebtoolsToggle = (enabled: boolean) => {
        if (!webtoolsSource) return;
        const { is_client_tool, ...rest } = webtoolsSource as any;
        void onSave({ ...rest, enabled });
    };

    const handleAdvisorToggle = (enabled: boolean) => {
        if (!advisorSource) return;
        const { is_client_tool, transport, command, args, cwd, ...rest } = advisorSource as any;
        void onSave({ ...rest, enabled });
    };

    const handleWebtoolsSave = async () => {
        setWebtoolsSaving(true);
        try {
            const { is_client_tool, ...baseRest } = (webtoolsSource ?? {
                id: BUILTIN_WEBTOOLS_ID,
                name: 'Built-in Web Tools',
                transport: 'stdio',
                command: 'tingly-box',
                args: ['mcp-builtin'],
                tools: ['mcp_web_search', 'mcp_web_fetch'],
                enabled: true,
            }) as any;
            await onSave({ ...baseRest, env: serperKey ? { SERPER_API_KEY: serperKey } : {} });
        } finally {
            setWebtoolsSaving(false);
        }
    };

    const handleAdvisorSave = async () => {
        setAdvisorSaving(true);
        try {
            const selectedProvider = providerCatalog.find((p) => p.uuid === selectedProviderUuid);
            // Strip transport/command/args/cwd — advisor is an in-process virtual tool,
            // not a stdio process. Sending transport:'stdio' causes the runtime to attempt
            // a subprocess connection and fail with "empty command".
            const { is_client_tool, transport, command, args, cwd, ...baseRest } = (advisorSource ?? {
                id: BUILTIN_ADVISOR_ID,
                name: 'Built-in Adviser',
                tools: ['advisor'],
                enabled: false,
            }) as any;
            await onSave({
                ...baseRest,
                advisor: {
                    ...(baseRest.advisor ?? {}),
                    base_url: selectedProvider?.api_base || undefined,
                    api_key: selectedProvider?.token || undefined,
                    model: model || undefined,
                },
                env: {
                    ...(selectedProviderUuid ? { ADVISOR_PROVIDER_UUID: selectedProviderUuid } : {}),
                    ...(selectedProvider?.api_base ? { ADVISOR_BASE_URL: selectedProvider.api_base } : {}),
                    ...(selectedProvider?.token ? { ADVISOR_API_KEY: selectedProvider.token } : {}),
                    ...(model ? { ADVISOR_MODEL: model } : {}),
                },
            });
        } finally {
            setAdvisorSaving(false);
        }
    };

    const webtoolsEnabled = webtoolsSource?.enabled ?? true;
    const advisorEnabled = advisorSource?.enabled ?? false;
    const webtoolsTools = webtoolsSource?.tools ?? ['mcp_web_search', 'mcp_web_fetch'];
    const selectedProvider = providerCatalog.find((p) => p.uuid === selectedProviderUuid);

    return (
        <UnifiedCard title="Builtin Servers" size="full">
            <Stack spacing={0}>
                {/* ── Web Tools section ── */}
                <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 0.5 }}>
                    <Typography variant="subtitle1" fontWeight={600}>Web Tools</Typography>
                    <Chip label="Client Tool" size="small" color="info" variant="outlined" sx={{ fontSize: '0.65rem', height: 18 }} />
                    <Stack direction="row" spacing={0.5} sx={{ flex: 1 }}>
                        {webtoolsTools.map((t) => (
                            <Chip key={t} label={t} size="small" variant="outlined" />
                        ))}
                    </Stack>
                    {webtoolsSource && (
                        <FormControlLabel
                            control={
                                <Switch size="small" checked={webtoolsEnabled} onChange={(e) => handleWebtoolsToggle(e.target.checked)} disabled={webtoolsSaving} />
                            }
                            label={webtoolsEnabled ? 'Enabled' : 'Disabled'}
                            sx={{ mr: 0 }}
                        />
                    )}
                </Stack>

                <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                    Client-side web search and fetch tools available for agents to call. Powered by{' '}
                    <Typography component="a" href="https://serper.dev" target="_blank" rel="noreferrer" variant="body2" color="primary">Serper</Typography>
                    {' '}for search and a built-in HTTP fetcher for page content.
                </Typography>

                <ConfigRow label="Serper API Key" hint="Required for mcp_web_search. Get your free key at serper.dev">
                    <SecretInput
                        value={serperKey}
                        onChange={(v) => { setSerperKey(v); }}
                        placeholder="Enter Serper API key"
                    />
                </ConfigRow>

                <Stack direction="row" justifyContent="flex-end" sx={{ pb: 2 }}>
                    <Button
                        variant="contained"
                        size="small"
                        onClick={() => { void handleWebtoolsSave(); }}
                        disabled={webtoolsSaving}
                    >
                        {webtoolsSaving ? 'Saving...' : 'Save'}
                    </Button>
                </Stack>

                {advisorSource && (
                    <>
                <Divider sx={{ my: 1 }} />

                {/* ── Advisor section ── */}
                <Stack direction="row" alignItems="center" spacing={1} sx={{ mt: 2, mb: 0.5 }}>
                    <Typography variant="subtitle1" fontWeight={600}>Advisor</Typography>
                    <Chip label="Server Tool" size="small" color="success" variant="outlined" sx={{ fontSize: '0.65rem', height: 18 }} />
                    <Chip label="Experimental" size="small" color="warning" variant="outlined" sx={{ fontSize: '0.65rem', height: 18 }} />
                    <Box sx={{ flex: 1 }} />
                    {advisorSource && (
                        <FormControlLabel
                            control={
                                <Switch size="small" checked={advisorEnabled} onChange={(e) => handleAdvisorToggle(e.target.checked)} disabled={advisorSaving} />
                            }
                            label={advisorEnabled ? 'Enabled' : 'Disabled'}
                            sx={{ mr: 0 }}
                        />
                    )}
                </Stack>

                <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                    An in-process LLM tool that agents can consult for hard decisions. Connects to any{' '}
                    <Typography component="a" href="https://platform.openai.com/docs/api-reference" target="_blank" rel="noreferrer" variant="body2" color="primary">OpenAI-compatible</Typography>
                    {' '}or{' '}
                    <Typography component="a" href="https://docs.anthropic.com/en/api/getting-started" target="_blank" rel="noreferrer" variant="body2" color="primary">Anthropic</Typography>
                    {' '}endpoint. Best used for critical decision points to avoid excess latency.
                </Typography>

                <ConfigRow label="Model" hint="Choose advisor provider and model">
                    <Stack direction="row" spacing={1} alignItems="center" sx={{ width: '100%' }}>
                        <Button size="small" variant="outlined" onClick={() => setAdvisorModelDialogOpen(true)}>
                            Choose Model
                        </Button>
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                            {selectedProvider
                                ? `${selectedProvider.name} (${selectedProvider.api_style}) / ${model || '(no model selected)'}`
                                : '(no provider selected)'}
                        </Typography>
                    </Stack>
                </ConfigRow>

                <Stack direction="row" justifyContent="flex-end" sx={{ pt: 1 }}>
                    <Button
                        variant="contained"
                        size="small"
                        onClick={() => { void handleAdvisorSave(); }}
                        disabled={advisorSaving}
                    >
                        {advisorSaving ? 'Saving...' : 'Save'}
                    </Button>
                </Stack>
                <Dialog open={advisorModelDialogOpen} onClose={() => setAdvisorModelDialogOpen(false)} maxWidth="lg" fullWidth>
                    <DialogTitle sx={{ textAlign: 'center' }}>Choose Model</DialogTitle>
                    <DialogContent sx={{ height: '70vh' }}>
                        <ModelSelectDialog
                            providers={providerCatalog}
                            selectedProvider={selectedProviderUuid || undefined}
                            selectedModel={model || undefined}
                            onSelected={(option: ProviderSelectTabOption) => {
                                setSelectedProviderUuid(option.provider.uuid);
                                setModel(option.model || '');
                                setAdvisorModelDialogOpen(false);
                            }}
                        />
                    </DialogContent>
                    <DialogActions>
                        <Button size="small" onClick={() => setAdvisorModelDialogOpen(false)}>Close</Button>
                    </DialogActions>
                </Dialog>
                    </>
                )}
            </Stack>
        </UnifiedCard>
    );
};

// ─── Part 3: Custom servers ───────────────────────────────────────────────────

interface CustomServersCardProps {
    sources: MCPSourceConfig[];
    onSave: (sources: MCPSourceConfig[]) => Promise<void>;
    saving: boolean;
}

const CustomServersCard: React.FC<CustomServersCardProps> = ({ sources, onSave, saving }) => {
    const [editingId, setEditingId] = useState<string>('');
    const [editorMode, setEditorMode] = useState<'none' | 'add' | 'edit'>('none');
    const [editorForm, setEditorForm] = useState<MCPSourceFormValue>(() => ({
        ...defaultMCPSourceFormValue(),
        args: [],
        tools: ['*'],
        envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY'],
        useGlobalProxy: true,
    }));

    const openAdd = () => {
        setEditingId('');
        setEditorForm({
            ...defaultMCPSourceFormValue(),
            args: [],
            tools: ['*'],
            envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY'],
            useGlobalProxy: true,
        });
        setEditorMode('add');
    };

    const openEdit = (source: MCPSourceConfig) => {
        setEditingId(source.id || '');
        setEditorForm(sourceToFormValue(source));
        setEditorMode('edit');
    };

    const handleDelete = async (id?: string) => {
        if (!id) return;
        await onSave(sources.filter((s) => s.id !== id));
        if (editingId === id) { setEditingId(''); setEditorMode('none'); }
    };

    const handleToggle = async (id: string | undefined, enabled: boolean) => {
        if (!id) return;
        await onSave(sources.map((s) => (s.id === id ? { ...s, enabled } : s)));
        if (editingId === id) setEditorForm((prev) => ({ ...prev, enabled }));
    };

    const handleSave = async () => {
        const source = formValueToSource(editorForm);
        if (!source.id) return;
        const next = [...sources];
        const idx = next.findIndex((s) => s.id === source.id);
        if (idx >= 0) { next[idx] = source; } else { next.push(source); }
        await onSave(next);
        setEditorMode('none');
        setEditingId('');
    };

    return (
        <UnifiedCard title="Custom Servers" size="full">
            <Stack spacing={2}>
                <Stack direction="row" justifyContent="flex-end">
                    <Tooltip title="Add Server">
                        <IconButton onClick={openAdd} color="primary">
                            <AddIcon />
                        </IconButton>
                    </Tooltip>
                </Stack>

                {sources.length > 0 ? (
                    <TableContainer component={Paper} variant="outlined">
                        <Table size="small">
                            <TableHead>
                                <TableRow>
                                    <TableCell sx={{ fontWeight: 600 }}>Name</TableCell>
                                    <TableCell sx={{ fontWeight: 600 }}>Type</TableCell>
                                    <TableCell sx={{ fontWeight: 600 }}>Connection</TableCell>
                                    <TableCell sx={{ fontWeight: 600 }}>Tools</TableCell>
                                    <TableCell sx={{ fontWeight: 600 }}>State</TableCell>
                                    <TableCell sx={{ fontWeight: 600 }}>Actions</TableCell>
                                </TableRow>
                            </TableHead>
                            <TableBody>
                                {sources.map((source) => {
                                    const isEnabled = source.enabled ?? true;
                                    const tools = source.tools ?? [];
                                    const connectionInfo = source.transport === 'http'
                                        ? source.endpoint || '-'
                                        : source.command
                                            ? `${source.command}${source.args?.length ? ' ' + source.args.join(' ') : ''}`
                                            : '-';
                                    return (
                                        <TableRow key={source.id} hover sx={{ cursor: 'pointer' }} onClick={() => source.id && setEditingId(source.id)}>
                                            <TableCell>
                                                <Stack direction="row" spacing={0.5} alignItems="center">
                                                    <Typography variant="body2" fontWeight={500}>{source.id}</Typography>
                                                    <Chip
                                                        label={source.is_client_tool ? 'Client' : 'Server'}
                                                        size="small"
                                                        color={source.is_client_tool ? 'info' : 'success'}
                                                        variant="outlined"
                                                        sx={{ fontSize: '0.65rem', height: 18 }}
                                                    />
                                                </Stack>
                                            </TableCell>
                                            <TableCell>
                                                <Chip label={(source.transport || 'stdio').toUpperCase()} size="small" variant="outlined" />
                                            </TableCell>
                                            <TableCell>
                                                <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem', maxWidth: 260, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={connectionInfo}>
                                                    {connectionInfo}
                                                </Typography>
                                            </TableCell>
                                            <TableCell>
                                                <Stack direction="row" spacing={0.5} flexWrap="wrap">
                                                    {tools.slice(0, 2).map((t) => <Chip key={t} label={t} size="small" variant="outlined" sx={{ fontSize: '0.7rem', height: 20 }} />)}
                                                    {tools.length > 2 && <Chip label={`+${tools.length - 2}`} size="small" variant="outlined" sx={{ fontSize: '0.7rem', height: 20 }} />}
                                                </Stack>
                                            </TableCell>
                                            <TableCell>
                                                <Chip label={isEnabled ? 'Enabled' : 'Disabled'} size="small" color={isEnabled ? 'success' : 'default'} variant={isEnabled ? 'filled' : 'outlined'} />
                                            </TableCell>
                                            <TableCell>
                                                <Stack direction="row" spacing={0.5} onClick={(e) => e.stopPropagation()}>
                                                    <Tooltip title={isEnabled ? 'Disable' : 'Enable'}>
                                                        <IconButton size="small" color={isEnabled ? 'success' : 'default'} onClick={() => handleToggle(source.id, !isEnabled)}>
                                                            <PowerIcon fontSize="small" />
                                                        </IconButton>
                                                    </Tooltip>
                                                    <Tooltip title="Edit">
                                                        <IconButton size="small" color="primary" onClick={() => openEdit(source)}>
                                                            <EditIcon fontSize="small" />
                                                        </IconButton>
                                                    </Tooltip>
                                                    <Tooltip title="Delete">
                                                        <IconButton size="small" color="error" onClick={() => handleDelete(source.id)}>
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
                    <Typography variant="body2" color="text.secondary">No custom MCP servers yet.</Typography>
                )}

                {editorMode !== 'none' && (
                    <>
                        <MCPSourceEditor
                            title={editorMode === 'edit' ? 'Edit custom MCP' : 'Connect to a custom MCP'}
                            value={editorForm}
                            onChange={setEditorForm}
                        />
                        <Stack direction="row" justifyContent="space-between">
                            <Button variant="text" onClick={() => setEditorMode('none')}>Cancel</Button>
                            <Button variant="contained" onClick={handleSave} disabled={saving}>
                                {saving ? 'Saving...' : 'Save'}
                            </Button>
                        </Stack>
                    </>
                )}
            </Stack>
        </UnifiedCard>
    );
};

// ─── Main page ────────────────────────────────────────────────────────────────

const MCPRegisteredServers = () => {
    const location = useLocation();
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);
    const [notification, setNotification] = useState({ open: false, message: '', severity: 'success' as 'success' | 'error' });
    const showAdvisorSection = shouldShowAdvisorSection(location.search);

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
        // Builtin sources must not include is_client_tool — backend rejects it.
        const sanitized = sources.map((s) => {
            if (!BUILTIN_IDS.includes(s.id as any)) return s;
            const { is_client_tool, ...rest } = s as any;
            return rest as MCPSourceConfig;
        });
        const result = await api.setMCPConfig({ sources: sanitized });
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
    const advisorSource = allSources.find((s) => s.id === BUILTIN_ADVISOR_ID);
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
            <Stack spacing={2.5}>
                {/* Part 1 */}
                <AddToAgentsCard />

                {/* Part 2 */}
                <BuiltinServersCard
                    webtoolsSource={webtoolsSource}
                    advisorSource={showAdvisorSection ? advisorSource : undefined}
                    onSave={upsertSource}
                    saving={saving}
                />

                {/* Part 3 */}
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
