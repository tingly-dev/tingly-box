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
import { Add, Close, ExpandMore, Refresh, Terminal } from '@/components/icons';
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
    const assetPage = ['/mcp/tools', '/mcp/sources'].includes(location.pathname);
    const gatewayPage = location.pathname === '/mcp/server-tools';
    const publicationPage = !assetPage && !gatewayPage;
    const usageScope = assetPage ? undefined : gatewayPage ? ('gateway' as const) : ('client' as const);
    const pagePath = assetPage ? '/mcp/tools' : gatewayPage ? '/mcp/server-tools' : '/mcp';
    const toolsPath = (usage: 'client' | 'gateway') => (usage === 'client' ? '/mcp' : '/mcp/server-tools');
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
                        pathname:
                            section === 'server-tools'
                                ? '/mcp/server-tools'
                                : section === 'clients'
                                  ? '/mcp'
                                  : '/mcp/tools',
                        search: next.toString() ? `?${next}` : '',
                    },
                    { replace: true }
                );
                return;
            }
            const target =
                location.pathname === '/mcp/sources'
                    ? '/mcp/tools'
                    : location.pathname === '/mcp/clients' || location.pathname === '/mcp/routes'
                      ? '/mcp'
                      : undefined;
            if (target) {
                const next = new URLSearchParams(search);
                if (location.pathname === '/mcp/routes') next.set('section', 'routes');
                navigate({ pathname: target, search: next.toString() ? `?${next}` : '' }, { replace: true });
                return;
            }
            if (!publicationPage && ['profile', 'install', 'grant-source', 'publish'].some((key) => search.has(key))) {
                navigate({ pathname: '/mcp', search: location.search }, { replace: true });
                return;
            }
            if (publicationPage && search.has('source')) {
                navigate({ pathname: '/mcp/tools', search: location.search }, { replace: true });
                return;
            }
            const profile = search.get('profile') || search.get('install');
            const source = search.get('source') || search.get('publish');
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
    }, [loading, location.pathname, location.search, search, navigate, publicationPage]);
    useEffect(() => {
        if (!publicationPage || !showRoutes || !focusSource) return;
        const frame = requestAnimationFrame(() => relationshipsRef.current?.scrollIntoView({ block: 'start' }));
        return () => cancelAnimationFrame(frame);
    }, [publicationPage, showRoutes, focusSource]);
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
        ['section', 'tab', 'profile', 'install', 'source', 'grant-source', 'publish'].forEach((key) =>
            next.delete(key)
        );
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
            usage: { client: false, gateway: false },
        });
        setCustomSourceId(false);
        setConnectionError('');
        setAdding(true);
    };
    const sourceName = (source: MCPSourceConfig) => source.name || source.id || '';
    const sourceRoute = (id: string) => routing?.sources.find((source) => source.id === id);
    const countTools = (id: string) => {
        const client = routing?.clients.find((client) => client.id === id);
        return enabled && client?.enabled ? client.sources.reduce((sum, source) => sum + source.tools.length, 0) : 0;
    };
    const clientSources = (id: string) =>
        routing?.clients
            .find((client) => client.id === id)
            ?.sources.filter((source) => source.tools.length > 0)
            .map((source) => source.name) || [];
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
    const clientCards = (grantSource?: string, allowCreate = true) => (
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
                            ? `${countTools(client.id)} ${label('ordinaryToolCount', 'tools available through MCP')}`
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
            {allowCreate && (
                <Button
                    startIcon={<Add />}
                    sx={{ alignSelf: 'flex-start' }}
                    onClick={() => setSheet({ kind: 'client', grantSource })}
                >
                    {label('connectClient', 'Connect a client')}
                </Button>
            )}
        </Stack>
    );
    const toolCatalog = (
        <Box
            component="section"
            aria-label={
                assetPage
                    ? label('connections', 'Connected tools')
                    : publicationPage
                      ? label('publishedTools', 'Tools published through MCP')
                      : 'Server Tool'
            }
        >
            <Typography variant="h6" sx={{ fontWeight: 600, mb: 1.5 }}>
                {assetPage
                    ? label('connections', 'Connected tools')
                    : publicationPage
                      ? label('publishedTools', 'Tools published through MCP')
                      : label('serverToolSection', 'Tools available to the gateway')}
            </Typography>
            <MCPToolsPanel
                key={pagePath}
                mode={usageScope || 'asset'}
                routes={routing?.sources || []}
                loading={checking}
                sources={sources}
                enabled={enabled}
                saveSource={saveSource}
                onConfigureSource={(id) => {
                    const source = sources.find((item) => item.id === id);
                    if (!assetPage && (source?.transport === 'advisor' || source?.advisor))
                        navigate(`/mcp/server-tools?source=${encodeURIComponent(id)}`);
                    else setSheet({ kind: 'source', id });
                }}
                onRelationships={assetPage ? undefined : openRelationships}
            />
        </Box>
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
                            {publicationPage ? label('overviewTitle', 'MCP') : assetPage ? 'Tool' : 'Server Tool'}
                        </Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                            {publicationPage
                                ? label(
                                      'overviewHint',
                                      'Publish tools through the MCP gateway, manage client access and get connection commands.'
                                  )
                                : assetPage
                                  ? label(
                                        'toolPageHint',
                                        'Connect tool sources, inspect all tools and parameters, test tools and manage shared connection settings.'
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
                        {!assetPage && (
                            <Button onClick={() => navigate('/mcp/tools')}>
                                {label('manageConnections', 'Manage tool connections')}
                            </Button>
                        )}
                        {publicationPage ? (
                            <Button
                                startIcon={<Add />}
                                variant="contained"
                                disabled={busy}
                                onClick={() => setSheet({ kind: 'client' })}
                            >
                                {label('connectClient', 'Connect a client')}
                            </Button>
                        ) : assetPage ? (
                            <Button startIcon={<Add />} variant="contained" disabled={busy} onClick={beginConnection}>
                                {label('connectTools', 'Connect tools')}
                            </Button>
                        ) : null}
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
                            publicationPage ? (
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
                            ) : (
                                <Button onClick={() => navigate('/mcp')}>{label('openMCP', 'Open MCP')}</Button>
                            )
                        }
                    >
                        {label(
                            'executionOff',
                            'MCP execution is off. Your configuration is saved; enable execution in the workspace to use it.'
                        )}
                    </Alert>
                )}
                {publicationPage && (
                    <>
                        <Box component="section" aria-label={label('clientAccessSection', 'Client access')}>
                            <Typography variant="h6" sx={{ fontWeight: 600, mb: 1.5 }}>
                                {label('clientAccessSection', 'Client access')}
                            </Typography>
                            {clientCards(undefined, false)}
                        </Box>
                        {toolCatalog}
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
                                        onEditSource={(id) => navigate(`/mcp/tools?source=${encodeURIComponent(id)}`)}
                                        onClient={(id, edit) =>
                                            navigate(
                                                id
                                                    ? `/mcp?${edit ? 'profile' : 'install'}=${encodeURIComponent(id)}`
                                                    : '/mcp'
                                            )
                                        }
                                        onTools={(usage, sourceId) =>
                                            navigate(
                                                sourceId
                                                    ? `${toolsPath(usage)}?${usage === 'client' ? 'publish' : 'source'}=${encodeURIComponent(sourceId)}`
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
                {(assetPage || gatewayPage) && toolCatalog}
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
                                key={`${selectedSource.id}-${usageScope || 'asset'}`}
                                usageScope={usageScope}
                                source={selectedSource}
                                route={sourceRoute(selectedSource.id!)}
                                enabled={enabled}
                                saveSource={saveSource}
                                onRelationships={assetPage ? undefined : openRelationships}
                                onConfigureConnection={
                                    assetPage
                                        ? undefined
                                        : () => navigate(`/mcp/tools?source=${encodeURIComponent(selectedSource.id!)}`)
                                }
                                onConfigureAdvisor={() =>
                                    navigate(`/mcp/server-tools?source=${encodeURIComponent(selectedSource.id!)}`)
                                }
                                onRefresh={async () => {
                                    const response = await api.reconnectMCPSource(selectedSource.id!);
                                    await refreshRoutes();
                                    if (!response.success) throw new Error(response.error || 'Connection check failed');
                                }}
                                onConnectClient={() => {
                                    if (publicationPage) setSheet({ kind: 'clients', grantSource: selectedSource.id });
                                    else navigate(`/mcp?publish=${encodeURIComponent(selectedSource.id!)}`);
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
                                'Connect a tool source and inspect its catalog. Publication through MCP and Server Tool execution are configured separately.'
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
                        {busy ? <CircularProgress size={18} /> : label('connectAndChoose', 'Connect and inspect tools')}
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
}
