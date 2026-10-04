import { useCallback, useEffect, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
    Alert,
    Button,
    Card,
    CardContent,
    Checkbox,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    Stack,
    Tab,
    Tabs,
    TextField,
    Typography,
} from '@mui/material';
import { PageLayout } from '@/components/PageLayout';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import { useFeatureFlags } from '@/contexts/FeatureFlagsContext';
import MCPSourceEditor from './MCPSourceEditor';
import AdvisorSettings from './AdvisorSettings';
import MCPToolsPanel from './MCPToolsPanel';
import MCPRoutingPanel from './MCPRoutingPanel';
import MCPClientsPanel from './MCPClientsPanel';
import {
    defaultMCPSourceFormValue,
    formValueToSource,
    sourceToFormValue,
    type MCPConfigResponse,
    type MCPRuntimeConfig,
    type MCPSourceConfig,
    type MCPSourceStatus,
} from './types';

export default function MCPRegisteredServers() {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.center.${key}`, { defaultValue: fallback });
    const notify = useNotify();
    const flags = useFeatureFlags();
    const [search] = useSearchParams();
    const location = useLocation();
    const navigate = useNavigate();
    const tab = ['routes', 'servers', 'tools', 'server-tools', 'clients'].includes(search.get('tab') || '')
        ? search.get('tab')!
        : location.pathname === '/mcp/routes'
          ? 'routes'
          : location.pathname === '/mcp/server-tools'
            ? 'server-tools'
            : location.pathname === '/mcp/tools'
              ? 'tools'
              : location.pathname === '/mcp/clients'
                ? 'clients'
                : 'servers';
    const [config, setConfig] = useState<MCPRuntimeConfig>({});
    const [enabled, setEnabled] = useState(false);
    const [loading, setLoading] = useState(true);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [statuses, setStatuses] = useState<MCPSourceStatus[]>([]);
    const [editing, setEditing] = useState<MCPSourceConfig | null | undefined>(undefined);
    const [form, setForm] = useState(defaultMCPSourceFormValue);
    const acceptConfig = (response: MCPConfigResponse) => {
        if (!response?.success || !response.config)
            throw new Error(response?.error || 'Failed to save MCP configuration');
        setConfig(response.config);
        setEnabled(response.enabled);
        setStatuses([]);
    };
    const load = useCallback(
        () =>
            api
                .getMCPConfig()
                .then((response: MCPConfigResponse) => {
                    if (!response?.success || !response.config)
                        throw new Error(response?.error || 'Failed to load MCP configuration');
                    setConfig(response.config);
                    setEnabled(response.enabled);
                    setStatuses([]);
                })
                .catch((e) => setError(e instanceof Error ? e.message : String(e)))
                .finally(() => setLoading(false)),
        []
    );
    useEffect(() => {
        void load();
    }, [load]);
    const run = async (action: () => Promise<void>) => {
        setBusy(true);
        try {
            await action();
        } catch (e) {
            notify.error(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };
    const sources = config.sources || [];
    const saveSource = async (patch: MCPSourceConfig) => {
        const exists = sources.some((s) => s.id === patch.id);
        acceptConfig(await (exists ? api.patchMCPSource(patch.id!, patch) : api.createMCPSource(patch)));
    };
    const edit = (source: MCPSourceConfig | null) => {
        setEditing(source);
        setForm(sourceToFormValue(source || undefined));
    };
    const check = (id: string, reconnect: boolean) =>
        run(async () => {
            const response = await (reconnect ? api.reconnectMCPSource(id) : api.checkMCPSource(id));
            if (response.status)
                setStatuses((previous) => [...previous.filter((s) => s.source_id !== id), response.status]);
            if (!response.success) throw new Error(response.error || 'Connection failed');
        });
    return (
        <PageLayout loading={loading}>
            <Stack spacing={2.5}>
                <Stack direction="row" sx={{ justifyContent: 'space-between' }}>
                    <Typography variant="h5">{label('title', 'MCP center')}</Typography>
                    <Button
                        disabled={busy}
                        onClick={() => {
                            setLoading(true);
                            setError('');
                            void load();
                        }}
                    >
                        {label('reload', 'Reload')}
                    </Button>
                </Stack>
                {error && (
                    <Alert severity="error" onClose={() => setError('')}>
                        {error}
                    </Alert>
                )}
                {!enabled && (
                    <Alert
                        severity="info"
                        action={
                            <Button
                                disabled={busy}
                                onClick={() =>
                                    void run(async () => {
                                        const response = await api.setScenarioFlag('_global', 'mcp', true);
                                        if (!response?.success) throw new Error(response?.error || 'Enable failed');
                                        await flags.refresh();
                                        await load();
                                    })
                                }
                            >
                                {label('enableExecution', 'Enable execution')}
                            </Button>
                        }
                    >
                        {label(
                            'disabledHint',
                            'MCP execution is disabled. You can configure servers and inspect capabilities here.'
                        )}
                    </Alert>
                )}
                <Tabs
                    value={tab}
                    onChange={(_, value) => navigate(value === 'servers' ? '/mcp/sources' : `/mcp/${value}`)}
                    variant="scrollable"
                >
                    <Tab value="routes" label={label('routes', 'Routing overview')} />
                    <Tab value="tools" label={label('ordinaryTools', 'Ordinary tools')} />
                    <Tab value="server-tools" label={label('serverTools', 'Server Tools')} />
                    <Tab value="servers" label={label('sources', 'Tool sources')} />
                    <Tab value="clients" label={label('clients', 'Client access')} />
                </Tabs>
                {tab === 'servers' && (
                    <Stack spacing={2}>
                        <Stack direction="row" sx={{ justifyContent: 'space-between' }}>
                            <Typography color="text.secondary">
                                {label(
                                    'serversHint',
                                    'Connect servers once, then choose where their tools can be used.'
                                )}
                            </Typography>
                            <Button variant="contained" disabled={busy} onClick={() => edit(null)}>
                                {label('addServer', 'Add server')}
                            </Button>
                        </Stack>
                        {sources.map((source) => {
                            const status = statuses.find((s) => s.source_id === source.id);
                            const usage = source.usage || {
                                client: source.visibility !== 'server' && source.transport !== 'advisor',
                                gateway: source.visibility === 'server' || source.transport === 'advisor',
                            };
                            return (
                                <Card variant="outlined" key={source.id}>
                                    <CardContent>
                                        <Stack spacing={1}>
                                            <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                                <Typography variant="h6" sx={{ flex: 1 }}>
                                                    {source.name || source.id}
                                                </Typography>
                                                <Chip size="small" label={source.transport || 'stdio'} />
                                                <Chip
                                                    size="small"
                                                    label={label(
                                                        status?.state ||
                                                            (source.enabled === false ? 'disabled' : 'unchecked'),
                                                        status?.state ||
                                                            (source.enabled === false ? 'disabled' : 'unchecked')
                                                    )}
                                                    color={status?.state === 'error' ? 'error' : 'default'}
                                                />
                                            </Stack>
                                            <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                                                {source.endpoint ||
                                                    [source.command, ...(source.args || [])]
                                                        .filter(Boolean)
                                                        .join(' ') ||
                                                    source.id}
                                            </Typography>
                                            {status?.error && <Alert severity="error">{status.error}</Alert>}
                                            <Stack direction="row" sx={{ flexWrap: 'wrap', alignItems: 'center' }}>
                                                <FormControlLabel
                                                    control={
                                                        <Checkbox
                                                            disabled={busy}
                                                            checked={source.enabled !== false}
                                                            onChange={(e) =>
                                                                void run(() =>
                                                                    saveSource({
                                                                        id: source.id,
                                                                        enabled: e.target.checked,
                                                                    })
                                                                )
                                                            }
                                                        />
                                                    }
                                                    label={label('enabled', 'Enabled')}
                                                />
                                                <FormControlLabel
                                                    control={
                                                        <Checkbox
                                                            disabled={
                                                                busy ||
                                                                source.transport === 'advisor' ||
                                                                !!source.advisor
                                                            }
                                                            checked={
                                                                usage.client &&
                                                                source.transport !== 'advisor' &&
                                                                !source.advisor
                                                            }
                                                            onChange={(e) =>
                                                                void run(() =>
                                                                    saveSource({
                                                                        id: source.id,
                                                                        usage: { ...usage, client: e.target.checked },
                                                                    })
                                                                )
                                                            }
                                                        />
                                                    }
                                                    label={label('clientUsage', 'MCP clients')}
                                                />
                                                <FormControlLabel
                                                    control={
                                                        <Checkbox
                                                            disabled={busy}
                                                            checked={usage.gateway}
                                                            onChange={(e) =>
                                                                void run(() =>
                                                                    saveSource({
                                                                        id: source.id,
                                                                        usage: { ...usage, gateway: e.target.checked },
                                                                    })
                                                                )
                                                            }
                                                        />
                                                    }
                                                    label={label('gatewayUsage', 'Gateway model calls')}
                                                />
                                                <Button disabled={busy} onClick={() => edit(source)}>
                                                    {label('edit', 'Edit')}
                                                </Button>
                                                <Button
                                                    disabled={busy || source.enabled === false}
                                                    onClick={() => void check(source.id!, false)}
                                                >
                                                    {label('check', 'Test connection')}
                                                </Button>
                                                <Button
                                                    disabled={busy || source.enabled === false}
                                                    onClick={() => void check(source.id!, true)}
                                                >
                                                    {label('reconnect', 'Reconnect')}
                                                </Button>
                                                {!['advisor', 'webtools'].includes(source.id!) && (
                                                    <Button
                                                        color="error"
                                                        disabled={busy}
                                                        onClick={() =>
                                                            void run(async () =>
                                                                acceptConfig(await api.deleteMCPSource(source.id!))
                                                            )
                                                        }
                                                    >
                                                        {label('delete', 'Delete')}
                                                    </Button>
                                                )}
                                            </Stack>
                                        </Stack>
                                    </CardContent>
                                </Card>
                            );
                        })}
                        <Stack direction="row" spacing={2}>
                            <TextField
                                size="small"
                                type="number"
                                label={label('timeout', 'Call timeout (seconds)')}
                                sx={{ width: 240, flexShrink: 0 }}
                                value={config.request_timeout || 30}
                                slotProps={{ htmlInput: { min: 1, max: 600 } }}
                                onChange={(e) =>
                                    setConfig((previous) => ({ ...previous, request_timeout: Number(e.target.value) }))
                                }
                            />
                            <Button
                                disabled={busy}
                                onClick={() =>
                                    void run(async () =>
                                        acceptConfig(
                                            await api.setMCPConfig({ request_timeout: config.request_timeout })
                                        )
                                    )
                                }
                            >
                                {label('saveTimeout', 'Save timeout')}
                            </Button>
                        </Stack>
                    </Stack>
                )}
                {tab === 'routes' && (
                    <MCPRoutingPanel
                        revision={sources}
                        onEditSource={(id) => {
                            const source = sources.find((item) => item.id === id);
                            if (source) edit(source);
                        }}
                        onClient={(id, editing) =>
                            navigate(
                                `/mcp/clients${id ? `?${editing ? 'profile' : 'install'}=${encodeURIComponent(id)}` : ''}`
                            )
                        }
                        onTools={(kind) => navigate(kind === 'client' ? '/mcp/tools' : '/mcp/server-tools')}
                    />
                )}
                {(tab === 'tools' || tab === 'server-tools') && (
                    <MCPToolsPanel
                        key={tab}
                        usage={tab === 'tools' ? 'client' : 'gateway'}
                        sources={sources}
                        enabled={enabled}
                        saveSource={saveSource}
                    />
                )}
                {tab === 'clients' && <MCPClientsPanel config={config} onSaved={acceptConfig} />}
            </Stack>
            <Dialog open={editing !== undefined} onClose={() => !busy && setEditing(undefined)} maxWidth="md" fullWidth>
                <DialogTitle>
                    {label(editing ? 'editServer' : 'addServer', editing ? 'Edit server' : 'Add server')}
                </DialogTitle>
                <DialogContent dividers>
                    {editing?.id === 'advisor' ? (
                        <AdvisorSettings
                            advisorSource={editing}
                            onSave={async (patch) => {
                                await run(async () => {
                                    await saveSource(patch);
                                    setEditing(undefined);
                                });
                            }}
                        />
                    ) : (
                        <MCPSourceEditor value={form} onChange={setForm} lockId={!!editing} />
                    )}
                </DialogContent>
                <DialogActions>
                    <Button disabled={busy} onClick={() => setEditing(undefined)}>
                        {label('cancel', 'Cancel')}
                    </Button>
                    {editing?.id !== 'advisor' && (
                        <Button
                            disabled={busy || !form.id.trim()}
                            onClick={() =>
                                void run(async () => {
                                    const source = formValueToSource(form);
                                    if (!editing && sources.some((s) => s.id === source.id))
                                        throw new Error(label('duplicateId', 'A server with this ID already exists.'));
                                    await saveSource(source);
                                    setEditing(undefined);
                                })
                            }
                        >
                            {label('save', 'Save')}
                        </Button>
                    )}
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
}
