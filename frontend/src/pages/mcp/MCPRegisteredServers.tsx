import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Button,
    Card,
    Chip,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Divider,
    Drawer,
    IconButton,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { Add, ArrowForward, Close, ExpandMore, Psychology, Refresh, Server, Terminal } from '@/components/icons';
import { PageLayout } from '@/components/PageLayout';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import { useFeatureFlags } from '@/contexts/FeatureFlagsContext';
import MCPSourceEditor from './MCPSourceEditor';
import MCPSourceWorkspace from './MCPSourceWorkspace';
import MCPClientWorkspace from './MCPClientWorkspace';
import MCPToolsPanel from './MCPToolsPanel';
import MCPRoutingPanel from './MCPRoutingPanel';
import { nextMCPId } from './workspaceState';
import {
    defaultMCPSourceFormValue,
    formValueToSource,
    type MCPClientProfile,
    type MCPConfigResponse,
    type MCPRouteSource,
    type MCPRoutingSnapshot,
    type MCPRuntimeConfig,
    type MCPSourceConfig,
} from './types';

type Sheet =
    | { kind: 'source'; id: string }
    | { kind: 'client'; id?: string; grantSource?: string; edit?: boolean }
    | { kind: 'clients'; grantSource?: string }
    | null;

export default function MCPRegisteredServers() {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const notify = useNotify();
    const flags = useFeatureFlags();
    const location = useLocation();
    const [search] = useSearchParams();
    const navigate = useNavigate();
    const usageScope = ['/mcp/tools', '/mcp/clients'].includes(location.pathname)
        ? ('client' as const)
        : location.pathname === '/mcp/server-tools'
          ? ('gateway' as const)
          : undefined;
    const overview = !usageScope;
    const pagePath = usageScope === 'client' ? '/mcp/tools' : usageScope === 'gateway' ? '/mcp/server-tools' : '/mcp';
    const toolsPath = (usage: 'client' | 'gateway') => (usage === 'client' ? '/mcp/tools' : '/mcp/server-tools');
    const [config, setConfig] = useState<MCPRuntimeConfig>({});
    const [routing, setRouting] = useState<MCPRoutingSnapshot | null>(null);
    const [enabled, setEnabled] = useState(false);
    const [loading, setLoading] = useState(true);
    const [checking, setChecking] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [routeError, setRouteError] = useState('');
    const [sheet, setSheet] = useState<Sheet>(null);
    const [adding, setAdding] = useState(false);
    const [form, setForm] = useState(defaultMCPSourceFormValue);
    const [customSourceId, setCustomSourceId] = useState(false);
    const [connectionError, setConnectionError] = useState('');
    const [showRoutes, setShowRoutes] = useState(false);
    const relationshipsRef = useRef<HTMLDivElement>(null);
    const focusSource = search.get('relationship') || undefined;
    const routeGeneration = useRef(0);
    const linked = useRef('');
    const sources = useMemo(() => config.sources || [], [config.sources]);
    const profiles = useMemo(() => config.client_profiles || [], [config.client_profiles]);
    const legacy = !config.client_profiles_configured && profiles.length === 0;
    const clients: MCPClientProfile[] = legacy
        ? [
              {
                  id: 'tb',
                  name: label('defaultClient', 'Default client connection'),
                  enabled: true,
                  sources: ['*'],
                  tools: ['*'],
              },
          ]
        : profiles;
    const refreshRoutes = useCallback(async () => {
        const generation = ++routeGeneration.current;
        setChecking(true);
        try {
            const response = await api.getMCPRouting();
            if (generation !== routeGeneration.current) return;
            if (!response.success) throw new Error(response.error || 'Could not check tool connections');
            setRouting(response.routing);
            setEnabled(response.enabled);
            setRouteError('');
        } catch (e) {
            if (generation === routeGeneration.current) {
                setRouting(null);
                setRouteError(e instanceof Error ? e.message : String(e));
            }
        } finally {
            if (generation === routeGeneration.current) setChecking(false);
        }
    }, []);
    const acceptConfig = async (response: MCPConfigResponse) => {
        if (!response?.success || !response.config)
            throw new Error(response?.error || 'Could not save MCP configuration');
        setConfig(response.config);
        setRouting(null);
        setEnabled(response.enabled);
        await refreshRoutes();
    };
    const reload = async () => {
        const response = await api.getMCPConfig();
        await acceptConfig(response);
    };
    useEffect(() => {
        let active = true;
        void api
            .getMCPConfig()
            .then((response: MCPConfigResponse) => {
                if (!active) return;
                if (!response?.success || !response.config)
                    throw new Error(response?.error || 'Could not load MCP configuration');
                setConfig(response.config);
                setEnabled(response.enabled);
                setLoading(false);
                void refreshRoutes();
            })
            .catch((e) => {
                if (active) {
                    setError(e instanceof Error ? e.message : String(e));
                    setLoading(false);
                }
            });
        return () => {
            active = false;
            routeGeneration.current++;
        };
    }, [refreshRoutes]);
    // Keep saved setup/source links contextual; old section links select the
    // corresponding secondary layout rather than opening a tool-list drawer.
    useEffect(() => {
        if (loading) return;
        const key = location.pathname + location.search;
        if (linked.current === key) return;
        let active = true;
        // Resolve saved-link intent after configuration arrives, with cleanup for
        // navigation/unmount and React Strict Mode's repeated effect setup.
        void Promise.resolve().then(() => {
            if (!active || linked.current === key) return;
            linked.current = key;
            const section = search.get('section') || search.get('tab');
            if (section === 'tools' || section === 'server-tools' || section === 'clients') {
                const next = new URLSearchParams(search);
                next.delete('section');
                next.delete('tab');
                navigate(
                    {
                        pathname: section === 'server-tools' ? '/mcp/server-tools' : '/mcp/tools',
                        search: next.toString() ? `?${next}` : '',
                    },
                    { replace: true }
                );
                return;
            }
            const profile = search.get('profile') || search.get('install');
            const source = search.get('source');
            const grantSource = search.get('grant-source');
            if (source) setSheet({ kind: 'source', id: source });
            else if (profile) setSheet({ kind: 'client', id: profile, edit: search.has('profile') });
            else if (grantSource) setSheet({ kind: 'clients', grantSource });
            else setSheet(null);
            if (section === 'routes' || location.pathname === '/mcp/routes' || search.has('relationship'))
                setShowRoutes(true);
        });
        return () => {
            active = false;
        };
    }, [loading, location.pathname, location.search, search, navigate]);
    useEffect(() => {
        if (!overview || !showRoutes || !focusSource) return;
        const frame = requestAnimationFrame(() => relationshipsRef.current?.scrollIntoView({ block: 'start' }));
        return () => cancelAnimationFrame(frame);
    }, [overview, showRoutes, focusSource]);
    const openRelationships = (id?: string) => {
        setSheet(null);
        setShowRoutes(true);
        navigate(id ? `/mcp?relationship=${encodeURIComponent(id)}` : '/mcp?section=routes');
    };
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
    const saveSource = async (patch: MCPSourceConfig) => {
        await acceptConfig(
            await (sources.some((s) => s.id === patch.id)
                ? api.patchMCPSource(patch.id!, patch)
                : api.createMCPSource(patch))
        );
    };
    const closeSheet = () => {
        setSheet(null);
        const next = new URLSearchParams(search);
        ['section', 'tab', 'profile', 'install', 'source', 'grant-source'].forEach((key) => next.delete(key));
        navigate({ pathname: pagePath, search: next.toString() ? `?${next}` : '' }, { replace: true });
    };
    const beginConnection = () => {
        setForm({
            ...defaultMCPSourceFormValue(),
            id: nextMCPId(
                '',
                sources.map((s) => s.id!),
                'tool-service'
            ),
            transport: 'http',
            usage: usageScope === 'gateway' ? { client: false, gateway: true } : { client: true, gateway: false },
        });
        setCustomSourceId(false);
        setConnectionError('');
        setAdding(true);
    };
    const sourceName = (source: MCPSourceConfig) => source.name || source.id || '';
    const sourceRoute = (id: string) => routing?.sources.find((source) => source.id === id);
    const countTools = (id: string) =>
        routing?.clients
            .find((client) => client.id === id)
            ?.sources.reduce((sum, source) => sum + source.tools.length, 0) || 0;
    const clientSources = (id: string) =>
        routing?.clients
            .find((client) => client.id === id)
            ?.sources.filter((source) => source.tools.length > 0)
            .map((source) => source.name) || [];
    const stateLabel = (source: MCPSourceConfig, route?: MCPRouteSource) =>
        source.enabled === false
            ? label('off', 'Off')
            : !route
              ? label('checking', 'Checking…')
              : route.state === 'connected'
                ? label('connected', 'Connected')
                : label('needsAttention', 'Needs attention');
    const activeSources = sources.filter((source) => source.enabled !== false);
    const readySources = routing?.sources.filter((source) => source.state === 'connected').length || 0;
    const serverSources = routing?.server_tools || [];
    const serverCount = serverSources.reduce((sum, source) => sum + source.tools.length, 0);
    const selectedSource = sheet?.kind === 'source' ? sources.find((source) => source.id === sheet.id) : undefined;
    const selectedClient =
        sheet?.kind === 'client' && sheet.id ? clients.find((client) => client.id === sheet.id) : undefined;
    const title =
        sheet?.kind === 'source'
            ? selectedSource
                ? sourceName(selectedSource)
                : label('missingConnection', 'Connection no longer exists')
            : sheet?.kind === 'client'
              ? selectedClient?.name || label('connectClient', 'Connect a client')
              : sheet?.kind === 'clients'
                ? label('chooseClient', 'Choose a client')
                : label('chooseClient', 'Choose a client');
    const clientCards = (grantSource?: string) => (
        <Stack spacing={1.25}>
            {clients.map((client) => (
                <Box key={client.id} sx={{ p: 2, border: '1px solid', borderColor: 'divider', borderRadius: 1.5 }}>
                    <Stack
                        direction="row"
                        sx={{ justifyContent: 'space-between', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}
                    >
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                            <Terminal fontSize="small" />
                            <Typography style={{ fontWeight: 600 }}>{client.name || client.id}</Typography>
                            {client.enabled === false && <Chip size="small" label={label('off', 'Off')} />}
                        </Stack>
                        <Button
                            onClick={() =>
                                setSheet({ kind: 'client', id: client.id, grantSource, edit: !!grantSource })
                            }
                        >
                            {label(
                                grantSource ? 'selectThisClient' : 'configureClient',
                                grantSource ? 'Choose this client' : 'Access & setup'
                            )}
                        </Button>
                    </Stack>
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.75 }}>
                        {routing
                            ? `${countTools(client.id)} ${label('ordinaryToolCount', 'ordinary tools assigned')}`
                            : label('checking', 'Checking…')}
                        {clientSources(client.id).length > 0 ? ` · ${clientSources(client.id).join('、')}` : ''}
                    </Typography>
                </Box>
            ))}
            {clients.length === 0 && (
                <Typography variant="body2" color="text.secondary">
                    {label('noClients', 'No client connections yet. Select tools and get a connection command.')}
                </Typography>
            )}
            <Button
                startIcon={<Add />}
                sx={{ alignSelf: 'flex-start' }}
                onClick={() => setSheet({ kind: 'client', grantSource })}
            >
                {label('connectClient', 'Connect a client')}
            </Button>
        </Stack>
    );
    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <Stack
                    direction="row"
                    sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 2, flexWrap: 'wrap' }}
                >
                    <Box>
                        <Typography component="h1" variant="h5" style={{ fontWeight: 700 }}>
                            {overview
                                ? label('overviewTitle', 'MCP connections overview')
                                : usageScope === 'client'
                                  ? 'Tool'
                                  : 'Server Tool'}
                        </Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                            {overview
                                ? label(
                                      'overviewHint',
                                      'Manage shared tool connections and see which clients or gateway models use them.'
                                  )
                                : usageScope === 'client'
                                  ? label(
                                        'toolPageHint',
                                        'Manage tools called by your clients, their access permissions and setup commands.'
                                    )
                                  : label(
                                        'serverPageHint',
                                        'Manage tools executed by the gateway during model requests. Configure Advisor’s consultation model here.'
                                    )}
                        </Typography>
                    </Box>
                    <Stack direction="row" spacing={1}>
                        <IconButton
                            aria-label={label('refresh', 'Refresh connections')}
                            disabled={busy || checking}
                            onClick={() => void run(reload)}
                        >
                            <Refresh />
                        </IconButton>
                        {!overview && (
                            <Button onClick={() => navigate('/mcp')}>
                                {label('manageConnections', 'Manage connections')}
                            </Button>
                        )}
                        {usageScope === 'client' ? (
                            <Button
                                startIcon={<Add />}
                                variant="contained"
                                disabled={busy}
                                onClick={() => setSheet({ kind: 'client' })}
                            >
                                {label('connectClient', 'Connect a client')}
                            </Button>
                        ) : (
                            <Button startIcon={<Add />} variant="contained" disabled={busy} onClick={beginConnection}>
                                {label('connectTools', 'Connect tools')}
                            </Button>
                        )}
                    </Stack>
                </Stack>
                {error && <Alert severity="error">{error}</Alert>}
                {routeError && (
                    <Alert
                        severity="warning"
                        action={
                            <Button disabled={checking} onClick={() => void refreshRoutes()}>
                                {label('retry', 'Retry connection')}
                            </Button>
                        }
                    >
                        {label(
                            'statusUnavailable',
                            'Connection status could not be refreshed. Saved settings remain available.'
                        )}{' '}
                        {routeError}
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
                                        await reload();
                                    })
                                }
                            >
                                {label('enableMCP', 'Enable MCP')}
                            </Button>
                        }
                    >
                        {label(
                            'executionOff',
                            'MCP execution is off. Your configuration is saved; enable execution in the workspace to use it.'
                        )}
                    </Alert>
                )}
                {overview && (
                    <>
                        <Box component="section" aria-label={label('connections', 'Connected tools')}>
                            <Stack
                                direction="row"
                                sx={{ justifyContent: 'space-between', alignItems: 'center', mb: 1.5 }}
                            >
                                <Typography variant="h6" style={{ fontWeight: 600 }}>
                                    {label('connections', 'Connected tools')}
                                </Typography>
                                <Typography variant="caption" color="text.secondary">
                                    {checking
                                        ? label('checking', 'Checking…')
                                        : `${readySources} / ${activeSources.length} ${label('connectionsReady', 'enabled connections ready')}`}
                                </Typography>
                            </Stack>
                            {sources.length === 0 && (
                                <Card variant="outlined" sx={{ p: 3 }}>
                                    <Typography style={{ fontWeight: 600 }}>
                                        {label('emptyConnections', 'Start with your first tool connection')}
                                    </Typography>
                                    <Typography color="text.secondary" variant="body2" sx={{ mt: 0.5, mb: 2 }}>
                                        {label(
                                            'emptyConnectionsHint',
                                            'Paste a tool service URL or enter a local command. We will discover its tools for you.'
                                        )}
                                    </Typography>
                                    <Button variant="contained" startIcon={<Add />} onClick={beginConnection}>
                                        {label('connectTools', 'Connect tools')}
                                    </Button>
                                </Card>
                            )}
                            <Stack spacing={1.25}>
                                {sources.map((source) => {
                                    const route = sourceRoute(source.id!);
                                    const destinations =
                                        routing?.clients
                                            .filter(
                                                (client) =>
                                                    enabled &&
                                                    client.enabled &&
                                                    client.sources.some((s) => s.id === source.id && s.tools.length > 0)
                                            )
                                            .map((client) =>
                                                client.legacy
                                                    ? label('defaultClient', 'Default client connection')
                                                    : client.name
                                            ) || [];
                                    if (enabled && serverSources.some((s) => s.id === source.id && s.tools.length > 0))
                                        destinations.push(label('gatewayModel', 'Gateway model'));
                                    const failed = source.enabled !== false && route && route.state !== 'connected';
                                    return (
                                        <Card
                                            component="article"
                                            aria-label={source.name || source.id}
                                            variant="outlined"
                                            key={source.id}
                                            sx={{
                                                px: 2.25,
                                                py: 1.75,
                                                borderColor: failed ? 'warning.light' : 'divider',
                                            }}
                                        >
                                            <Box
                                                sx={{
                                                    display: 'grid',
                                                    gridTemplateColumns: {
                                                        xs: '1fr',
                                                        md: 'minmax(180px, 1.1fr) minmax(180px, 1.4fr) auto',
                                                    },
                                                    alignItems: 'center',
                                                    gap: 2,
                                                }}
                                            >
                                                <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center' }}>
                                                    {route?.processing === 'advisor' || source.advisor ? (
                                                        <Psychology sx={{ color: 'text.secondary' }} />
                                                    ) : (
                                                        <Server sx={{ color: 'text.secondary' }} />
                                                    )}
                                                    <Box>
                                                        <Typography style={{ fontWeight: 600 }}>
                                                            {sourceName(source)}
                                                        </Typography>
                                                        <Typography variant="caption" color="text.secondary">
                                                            {source.origin === 'builtin' ||
                                                            (['advisor', 'webtools'].includes(source.id!) &&
                                                                source.origin !== 'external')
                                                                ? label('builtin', 'Built-in')
                                                                : label('external', 'External')}
                                                            {route?.state === 'connected'
                                                                ? ` · ${route.tools.length} ${label('toolCount', 'tools')}`
                                                                : ''}
                                                        </Typography>
                                                    </Box>
                                                </Stack>
                                                <Stack
                                                    direction="row"
                                                    spacing={1}
                                                    sx={{ alignItems: 'center', color: 'text.secondary', minWidth: 0 }}
                                                >
                                                    <ArrowForward fontSize="small" />
                                                    <Button
                                                        aria-label={`${sourceName(source)}: ${t('mcp.relationships.view', { defaultValue: 'View usage relationships' })}`}
                                                        onClick={() => openRelationships(source.id!)}
                                                        sx={{
                                                            textAlign: 'left',
                                                            justifyContent: 'flex-start',
                                                            color: 'text.secondary',
                                                            p: 0.5,
                                                        }}
                                                    >
                                                        {destinations.length
                                                            ? destinations.join(' · ')
                                                            : source.enabled === false
                                                              ? label('connectionOffShort', 'Connection disabled')
                                                              : failed
                                                                ? label(
                                                                      'fixBeforeUse',
                                                                      'Fix the connection to use its tools'
                                                                  )
                                                                : label(
                                                                      'chooseDestination',
                                                                      'Choose a client or Server Tools in settings'
                                                                  )}
                                                    </Button>
                                                </Stack>
                                                <Stack
                                                    direction="row"
                                                    spacing={1}
                                                    sx={{ alignItems: 'center', justifyContent: 'flex-end' }}
                                                >
                                                    <Chip
                                                        size="small"
                                                        variant="outlined"
                                                        color={failed ? 'warning' : 'default'}
                                                        label={stateLabel(source, route)}
                                                    />
                                                    <Button
                                                        onClick={() => setSheet({ kind: 'source', id: source.id! })}
                                                    >
                                                        {label(
                                                            failed ? 'fixConnection' : 'manageTools',
                                                            failed ? 'Fix connection' : 'Configure tools'
                                                        )}
                                                    </Button>
                                                </Stack>
                                            </Box>
                                        </Card>
                                    );
                                })}
                            </Stack>
                        </Box>
                        <Box component="section" aria-label={label('whoUses', 'Who uses these tools?')}>
                            <Typography variant="h6" style={{ fontWeight: 600 }} sx={{ mb: 1.5 }}>
                                {label('whoUses', 'Who uses these tools?')}
                            </Typography>
                            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: '1.35fr 1fr' }, gap: 2 }}>
                                <Card variant="outlined" sx={{ p: 2.5 }}>
                                    <Typography variant="subtitle1" style={{ fontWeight: 600 }}>
                                        {label('ordinaryTools', 'Ordinary tools')}
                                    </Typography>
                                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 2 }}>
                                        {label(
                                            'overviewToolHint',
                                            'Your client calls these tools through MCP and continues its own conversation. Configure access and get its setup command in Tool.'
                                        )}
                                    </Typography>
                                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                                        {routing
                                            ? `${routing.sources.reduce((count, source) => count + source.tools.filter((tool) => tool.enabled && tool.usage.client).length, 0)} ${label('ordinaryToolCount', 'ordinary tools')} · ${clients.length} ${label('clientConnections', 'client connections')}`
                                            : label('checking', 'Checking…')}
                                    </Typography>
                                    <Button variant="outlined" onClick={() => navigate('/mcp/tools')}>
                                        {label('openToolPage', 'Open Tool')}
                                    </Button>
                                </Card>
                                <Card variant="outlined" sx={{ p: 2.5, display: 'flex', flexDirection: 'column' }}>
                                    <Typography variant="subtitle1" style={{ fontWeight: 600 }}>
                                        {label('serverTools', 'Server Tools')}
                                    </Typography>
                                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                                        {label(
                                            'serverHint',
                                            'The gateway executes these tools during model requests, returns their results and continues the model response. No client setup command is needed.'
                                        )}
                                    </Typography>
                                    <Stack spacing={1.5} sx={{ my: 2, flex: 1 }}>
                                        <Typography style={{ fontWeight: 600 }}>
                                            {routing
                                                ? `${serverCount} ${label('serverToolsAssigned', 'Server Tools assigned')}`
                                                : label('checking', 'Checking…')}
                                        </Typography>
                                        <Typography variant="body2" color="text.secondary">
                                            {serverSources
                                                .filter((s) => s.tools.length > 0)
                                                .map((s) => s.name)
                                                .join(' · ') ||
                                                label(
                                                    'noServerTools',
                                                    'Choose Server Tools in a connection’s settings when needed.'
                                                )}
                                        </Typography>
                                    </Stack>
                                    <Button
                                        variant="outlined"
                                        sx={{ alignSelf: 'flex-start' }}
                                        onClick={() => navigate('/mcp/server-tools')}
                                    >
                                        {label('openServerPage', 'Open Server Tool')}
                                    </Button>
                                </Card>
                            </Box>
                        </Box>
                        <Accordion
                            ref={relationshipsRef}
                            expanded={showRoutes}
                            onChange={(_, value) => setShowRoutes(value)}
                            disableGutters
                            elevation={0}
                            sx={{
                                border: '1px solid',
                                borderColor: 'divider',
                                borderRadius: 1.5,
                                '&:before': { display: 'none' },
                            }}
                        >
                            <AccordionSummary expandIcon={<ExpandMore />}>
                                <Box>
                                    <Typography variant="body2" style={{ fontWeight: 600 }}>
                                        {label('showRelationships', 'Usage relationships')}
                                    </Typography>
                                    <Typography variant="caption" color="text.secondary">
                                        {label(
                                            'relationshipsHint',
                                            'Inspect who can use a connection, why a tool is unavailable and who a change affects.'
                                        )}
                                    </Typography>
                                </Box>
                            </AccordionSummary>
                            <AccordionDetails>
                                {showRoutes && (
                                    <MCPRoutingPanel
                                        sources={sources}
                                        profiles={clients}
                                        routing={routing}
                                        enabled={enabled}
                                        loading={checking}
                                        error={routeError}
                                        onRefresh={() => void run(reload)}
                                        focusSource={focusSource}
                                        onFocusSource={openRelationships}
                                        onEditSource={(id) => setSheet({ kind: 'source', id })}
                                        onClient={(id, edit) =>
                                            navigate(
                                                id
                                                    ? `/mcp/tools?${edit ? 'profile' : 'install'}=${encodeURIComponent(id)}`
                                                    : '/mcp/tools'
                                            )
                                        }
                                        onTools={(usage, sourceId) =>
                                            navigate(
                                                sourceId
                                                    ? `${toolsPath(usage)}?source=${encodeURIComponent(sourceId)}`
                                                    : toolsPath(usage)
                                            )
                                        }
                                    />
                                )}
                            </AccordionDetails>
                        </Accordion>
                        <Box component="details">
                            <Typography
                                component="summary"
                                variant="caption"
                                color="text.secondary"
                                sx={{ cursor: 'pointer' }}
                            >
                                {label('workspaceAdvanced', 'Advanced runtime settings')}
                            </Typography>
                            <Stack direction="row" spacing={1.5} sx={{ mt: 1.5, alignItems: 'center' }}>
                                <TextField
                                    size="small"
                                    type="number"
                                    label={t('mcp.center.timeout', { defaultValue: 'Call timeout (seconds)' })}
                                    value={config.request_timeout || 30}
                                    slotProps={{ htmlInput: { min: 1, max: 600 } }}
                                    onChange={(e) => setConfig({ ...config, request_timeout: Number(e.target.value) })}
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
                                    {label('saveRuntime', 'Save runtime settings')}
                                </Button>
                            </Stack>
                        </Box>
                    </>
                )}
                {usageScope === 'client' && (
                    <Box component="section" aria-label={label('clientAccessSection', 'Client access')}>
                        <Typography variant="h6" style={{ fontWeight: 600 }} sx={{ mb: 1.5 }}>
                            {label('clientAccessSection', 'Client access')}
                        </Typography>
                        {clientCards()}
                    </Box>
                )}
                {usageScope && (
                    <Box component="section" aria-label={usageScope === 'client' ? 'Tool' : 'Server Tool'}>
                        <Typography variant="h6" style={{ fontWeight: 600 }} sx={{ mb: 1.5 }}>
                            {label(
                                usageScope === 'client' ? 'clientToolSection' : 'serverToolSection',
                                usageScope === 'client'
                                    ? 'Tools available to clients'
                                    : 'Tools available to the gateway'
                            )}
                        </Typography>
                        <MCPToolsPanel
                            key={usageScope}
                            usage={usageScope}
                            sources={sources}
                            enabled={enabled}
                            saveSource={saveSource}
                            showIntro={false}
                            scopeOnly
                            onConfigureSource={(id) => setSheet({ kind: 'source', id })}
                            onRelationships={openRelationships}
                        />
                    </Box>
                )}
            </Stack>
            <Drawer
                anchor="right"
                open={!!sheet}
                onClose={closeSheet}
                slotProps={{
                    paper: {
                        role: 'dialog',
                        'aria-label': title,
                        sx: { width: { xs: '100%', sm: 680 }, maxWidth: '100%' },
                    },
                }}
            >
                <Stack sx={{ p: 3 }} spacing={2.5}>
                    <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                        <Typography variant="h5" style={{ fontWeight: 600 }}>
                            {title}
                        </Typography>
                        <IconButton aria-label={label('close', 'Close workspace panel')} onClick={closeSheet}>
                            <Close />
                        </IconButton>
                    </Stack>
                    <Divider />
                    {sheet?.kind === 'source' &&
                        (selectedSource ? (
                            <MCPSourceWorkspace
                                key={`${selectedSource.id}-${usageScope || 'overview'}`}
                                usageScope={usageScope}
                                source={selectedSource}
                                route={sourceRoute(selectedSource.id!)}
                                enabled={enabled}
                                saveSource={saveSource}
                                onRelationships={openRelationships}
                                onRefresh={async () => {
                                    const response = await api.reconnectMCPSource(selectedSource.id!);
                                    await refreshRoutes();
                                    if (!response.success) throw new Error(response.error || 'Connection check failed');
                                }}
                                onConnectClient={() => {
                                    if (usageScope === 'client')
                                        setSheet({ kind: 'clients', grantSource: selectedSource.id });
                                    else navigate(`/mcp/tools?grant-source=${encodeURIComponent(selectedSource.id!)}`);
                                }}
                                onDelete={async () => {
                                    await acceptConfig(await api.deleteMCPSource(selectedSource.id!));
                                    closeSheet();
                                }}
                            />
                        ) : (
                            <Alert severity="info">{label('missingConnection', 'Connection no longer exists')}</Alert>
                        ))}
                    {sheet?.kind === 'client' &&
                        (!sheet.id || selectedClient ? (
                            <MCPClientWorkspace
                                key={sheet.id || `new-${sheet.grantSource || ''}`}
                                profile={selectedClient}
                                legacy={legacy && sheet.id === 'tb'}
                                config={config}
                                routing={routing}
                                enabled={enabled}
                                grantSource={sheet.grantSource}
                                editing={sheet.edit}
                                onSave={async (profile) => {
                                    if (!sheet.id && profiles.some((p) => p.id === profile.id))
                                        throw new Error(label('duplicateId', 'This identifier is already in use.'));
                                    await acceptConfig(await api.saveMCPClientProfile(profile));
                                    setSheet({ kind: 'client', id: profile.id });
                                }}
                                onDelete={async () => {
                                    await acceptConfig(await api.deleteMCPClientProfile(sheet.id!));
                                    closeSheet();
                                }}
                            />
                        ) : (
                            <Alert severity="info">
                                {label('missingClient', 'Client connection no longer exists')}
                            </Alert>
                        ))}
                    {sheet?.kind === 'clients' && (
                        <Stack spacing={2}>
                            {sheet.grantSource && (
                                <Alert severity="info">
                                    {label(
                                        'grantConnectionHint',
                                        'Choose a client below, then review and save its access to this connection.'
                                    )}
                                </Alert>
                            )}
                            {clientCards(sheet.grantSource)}
                        </Stack>
                    )}
                </Stack>
            </Drawer>
            <Dialog open={adding} onClose={() => !busy && setAdding(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{label('connectTools', 'Connect tools')}</DialogTitle>
                <DialogContent dividers>
                    <Stack spacing={2.5}>
                        <Typography variant="body2" color="text.secondary">
                            {label(
                                'connectDialogHint',
                                'Enter a name and the service URL or local command. After connecting, choose how its tools will be used.'
                            )}
                        </Typography>
                        {connectionError && <Alert severity="error">{connectionError}</Alert>}
                        <TextField
                            label={label('connectionName', 'Connection name')}
                            value={form.name}
                            required
                            onChange={(e) =>
                                setForm({
                                    ...form,
                                    name: e.target.value,
                                    id: customSourceId
                                        ? form.id
                                        : nextMCPId(
                                              e.target.value,
                                              sources.map((s) => s.id!),
                                              'tool-service'
                                          ),
                                })
                            }
                            placeholder={label('connectionNamePlaceholder', 'For example: Company knowledge base')}
                        />
                        <MCPSourceEditor
                            compact
                            hideUsage
                            hideEnabled
                            hideTools
                            value={form}
                            onChange={(next) => {
                                if (next.id !== form.id) setCustomSourceId(true);
                                setForm(next);
                            }}
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button disabled={busy} onClick={() => setAdding(false)}>
                        {label('cancel', 'Cancel')}
                    </Button>
                    <Button
                        variant="contained"
                        disabled={
                            busy ||
                            !form.name.trim() ||
                            !form.id.trim() ||
                            (form.transport === 'stdio' ? !form.command.trim() : !form.endpoint.trim())
                        }
                        onClick={() =>
                            void run(async () => {
                                setConnectionError('');
                                try {
                                    if (sources.some((s) => s.id === form.id))
                                        throw new Error(label('duplicateId', 'This identifier is already in use.'));
                                    await saveSource(formValueToSource(form));
                                    setAdding(false);
                                    setSheet({ kind: 'source', id: form.id.trim() });
                                } catch (e) {
                                    setConnectionError(e instanceof Error ? e.message : String(e));
                                    throw e;
                                }
                            })
                        }
                    >
                        {busy ? <CircularProgress size={18} /> : label('connectAndChoose', 'Connect and choose tools')}
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
}
