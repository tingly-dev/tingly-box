import { PageLayout } from '@/components/PageLayout';
import AgentInstallCard from './AgentInstallCard';
import ToolCard from '@/components/ToolCard';
import ToolFilterBar, { type ToolFilter } from '@/components/ToolFilterBar';
import { api } from '@/services/api';
import {
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    InputAdornment,
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
} from '@/components/icons';
import {
    IconSearch,
    IconWorldDownload,
    IconServer,
} from '@tabler/icons-react';
import { useEffect, useState } from 'react';
import { useNotify } from '@/hooks/useNotify';
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

// ─── Builtin web tool card ────────────────────────────────────────────────────

interface WebtoolCardProps {
    webtoolsSource: MCPSourceConfig | undefined;
    toolName: 'mcp_web_search' | 'mcp_web_fetch';
    onSave: (patch: MCPSourceConfig) => Promise<void>;
    expanded?: boolean;
}

const WebtoolCard: React.FC<WebtoolCardProps> = ({ webtoolsSource, toolName, onSave, expanded }) => {
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
            defaultExpanded={true}
            expanded={expanded}
        />
    );
};

// ─── Add server card ──────────────────────────────────────────────────────────

interface AddServerCardProps {
    onClick: () => void;
}

const AddServerCard: React.FC<AddServerCardProps> = ({ onClick }) => (
    <Box
        onClick={onClick}
        sx={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 1,
            py: 3,
            px: 2,
            border: '1.5px dashed',
            borderColor: 'divider',
            borderRadius: 2,
            cursor: 'pointer',
            color: 'text.disabled',
            transition: 'border-color 0.15s, color 0.15s',
            '&:hover': {
                borderColor: 'text.secondary',
                color: 'text.secondary',
            },
        }}
    >
        <AddIcon sx={{ fontSize: 36 }} />
        <Typography variant="body2" fontWeight={500}>
            Connect a server
        </Typography>
        <Typography variant="caption" color="text.disabled" textAlign="center">
            Add a remote MCP server via stdio, HTTP or SSE.
        </Typography>
    </Box>
);

// ─── Main page ────────────────────────────────────────────────────────────────

const MCPRegisteredServers = () => {
    const notify = useNotify();
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);

    // Shared filter / expand state for the merged "Config your tools" section
    const [toolFilter, setToolFilter] = useState<ToolFilter>('all');
    const [toolExpanded, setToolExpanded] = useState(true);

    // Dialog state
    const [dialogOpen, setDialogOpen] = useState(false);
    const [editingSource, setEditingSource] = useState<MCPSourceConfig | null>(null);
    const [editorForm, setEditorForm] = useState<MCPSourceFormValue>(() => ({
        ...defaultMCPSourceFormValue(),
        args: [],
        tools: ['*'],
        envPassthrough: ['HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY'],
        useGlobalProxy: true,
    }));

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
            notify.success('Saved. Reconnect MCP client to apply.');
        } else {
            notify.error(result.error || 'Failed to save');
        }
        setSaving(false);
    };

    const upsertSource = async (patch: MCPSourceConfig): Promise<void> => {
        const next = [...allSources];
        const idx = next.findIndex((s) => s.id === patch.id);
        if (idx >= 0) { next[idx] = patch; } else { next.push(patch); }
        await saveConfig(next);
    };

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

    const handleDeleteCustom = async (id?: string) => {
        if (!id) return;
        await saveConfig(allSources.filter((s) => s.id !== id));
    };

    const handleToggleCustom = async (id: string | undefined, enabled: boolean) => {
        if (!id) return;
        await saveConfig(allSources.map((s) => (s.id === id ? { ...s, enabled } : s)));
    };

    const handleDialogSave = async () => {
        const source = formValueToSource(editorForm);
        if (!source.id) return;
        const next = [...allSources];
        const idx = next.findIndex((s) => s.id === source.id);
        if (idx >= 0) { next[idx] = source; } else { next.push(source); }
        await saveConfig(next);
        closeDialog();
    };

    const connectionLabel = (source: MCPSourceConfig): string => {
        if (source.transport === 'http' || source.transport === 'sse') return source.endpoint || '-';
        return source.command
            ? `${source.command}${source.args?.length ? ' ' + source.args.join(' ') : ''}`
            : '-';
    };

    const webtoolsSource = allSources.find((s) => s.id === BUILTIN_WEBTOOLS_ID);
    const customSources = allSources.filter((s) => !BUILTIN_IDS.includes(s.id as any));

    // Determine if builtin webtools pass the current filter
    const webtoolsEnabled = webtoolsSource?.enabled ?? true;
    const showBuiltins = toolFilter === 'all' || (toolFilter === 'active' ? webtoolsEnabled : !webtoolsEnabled);

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
                <AgentInstallCard heading="Config your agents" />

                {/* Part 2: Config your tools (builtin + custom servers merged) */}
                <Box>
                    <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 2, mb: 2.5 }}>
                        <Typography
                            sx={{ fontFamily: 'monospace', fontSize: '0.85rem', fontWeight: 700, color: 'text.primary', opacity: 0.35, mt: 0.35, flexShrink: 0, userSelect: 'none', letterSpacing: '0.05em' }}
                        >
                            02
                        </Typography>
                        <Box>
                            <Typography variant="h5" sx={{ fontWeight: 700, lineHeight: 1.2, mb: 0.5 }}>
                                Config your tools
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                Built-in and remote MCP tools available to your agents.
                            </Typography>
                        </Box>
                    </Box>

                    <Stack spacing={1.5}>
                        <ToolFilterBar
                            filter={toolFilter}
                            onFilterChange={setToolFilter}
                            allExpanded={toolExpanded}
                            onToggleExpand={setToolExpanded}
                        />

                        {/* Builtin web tools */}
                        {showBuiltins && (['mcp_web_search', 'mcp_web_fetch'] as const).map((toolName) => (
                            <WebtoolCard
                                key={toolName}
                                webtoolsSource={webtoolsSource}
                                toolName={toolName}
                                onSave={upsertSource}
                                expanded={toolExpanded ? undefined : false}
                            />
                        ))}

                        {/* Custom remote servers */}
                        {customSources
                            .filter((s) => toolFilter === 'all' || (toolFilter === 'active' ? (s.enabled ?? true) : !(s.enabled ?? true)))
                            .map((source) => {
                                const enabled = source.enabled ?? true;
                                const conn = connectionLabel(source);
                                return (
                                    <ToolCard
                                        key={source.id}
                                        icon={<IconServer size={18} />}
                                        name={source.id || ''}
                                        description={conn}
                                        enabled={enabled}
                                        onToggle={(v) => void handleToggleCustom(source.id, v)}
                                        expanded={toolExpanded ? undefined : false}
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
                                                    <IconButton size="small" color="error" onClick={() => void handleDeleteCustom(source.id)}>
                                                        <DeleteOutlineIcon fontSize="small" />
                                                    </IconButton>
                                                </Tooltip>
                                            </Stack>
                                        }
                                    />
                                );
                            })
                        }

                        {/* Add server card — only shown when filter allows it */}
                        {(toolFilter === 'all' || toolFilter === 'off') && (
                            <AddServerCard onClick={openAdd} />
                        )}
                    </Stack>
                </Box>
            </Stack>

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
        </PageLayout>
    );
};

export default MCPRegisteredServers;
