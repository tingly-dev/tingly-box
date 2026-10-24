import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Alert,
    Box,
    Button,
    Card,
    Chip,
    CircularProgress,
    Divider,
    Drawer,
    IconButton,
    Stack,
    Typography,
} from '@mui/material';
import { Close, Hub, Psychology, Security, Server, Settings, Terminal } from '@/components/icons';
import { ArrowNode } from '@/components/nodes/ArrowNode';
import { graphRowStyles, StyledBotGraphNode } from '@/components/nodes/styles';
import { api } from '@/services/api';
import AgentInstallCard from './AgentInstallCard';
import type { MCPClientRoute, MCPRouteSource, MCPRoutingSnapshot, MCPSourceConfig } from './types';

function RouteNode({
    title,
    subtitle,
    tag,
    icon,
    active = true,
    warn = false,
    onClick,
}: {
    title: string;
    subtitle: string;
    tag?: string;
    icon: React.ReactNode;
    active?: boolean;
    warn?: boolean;
    onClick: () => void;
}) {
    return (
        <StyledBotGraphNode
            as="button"
            active={active}
            warn={warn}
            clickable
            aria-label={`${title}: ${subtitle}`}
            onClick={onClick}
            sx={{
                flexShrink: 0,
                height: 84,
                p: 1.5,
                alignItems: 'stretch',
                textAlign: 'left',
                fontFamily: 'inherit',
                color: 'text.primary',
            }}
        >
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                {icon}
                <Typography
                    variant="body2"
                    sx={{
                        fontWeight: 600,
                        flex: 1,
                        minWidth: 0,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                    }}
                >
                    {title}
                </Typography>
                {tag && (
                    <Chip
                        size="small"
                        label={tag}
                        sx={{ height: 19, fontSize: 10, bgcolor: 'action.hover', color: 'text.secondary' }}
                    />
                )}
            </Stack>
            <Divider sx={{ my: 0.75 }} />
            <Typography
                variant="caption"
                sx={{ color: 'text.secondary', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
            >
                {subtitle}
            </Typography>
        </StyledBotGraphNode>
    );
}
const Arrow = () => <ArrowNode size={32} length={24} strokeWidth={1.7} arrowHeadSize={5} />;
const countTools = (sources: MCPRouteSource[]) => sources.reduce((total, source) => total + source.tools.length, 0);

export default function MCPRoutingPanel({
    onEditSource,
    onClient,
    onTools,
    revision,
}: {
    onEditSource: (id: string) => void;
    onClient: (id?: string, edit?: boolean) => void;
    onTools: (kind: 'client' | 'gateway') => void;
    revision: MCPSourceConfig[];
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.routing.${key}`, { defaultValue: fallback });
    const [routing, setRouting] = useState<MCPRoutingSnapshot | null>(null);
    const [enabled, setEnabled] = useState(false);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [expanded, setExpanded] = useState(false);
    const [selected, setSelected] = useState<{
        source?: MCPRouteSource;
        client?: MCPClientRoute;
        server?: boolean;
    } | null>(null);
    const [probe, setProbe] = useState<{ id: string; success: boolean; tools: string[]; error?: string } | null>(null);
    const [probing, setProbing] = useState('');
    const generation = useRef(0);
    const load = useCallback(async () => {
        const current = ++generation.current;
        setLoading(true);
        setSelected(null);
        setError('');
        setProbe(null);
        try {
            const result = await api.getMCPRouting();
            if (current !== generation.current) return;
            if (!result.success) throw new Error(result.error || 'Could not load MCP routes');
            setRouting(result.routing);
            setEnabled(result.enabled);
        } catch (e) {
            if (current === generation.current) setError(e instanceof Error ? e.message : String(e));
        } finally {
            if (current === generation.current) setLoading(false);
        }
    }, []);
    useEffect(() => {
        let active = true;
        // Schedule discovery after the effect; cleanup also invalidates an
        // in-flight response so old grants cannot repaint a refreshed graph.
        void Promise.resolve().then(() => {
            if (active) void load();
        });
        return () => {
            active = false;
            generation.current++;
        };
        // revision is the saved source configuration and explicitly invalidates discovery.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [load, revision]);
    const checkClient = async (id: string) => {
        setProbing(id);
        setProbe(null);
        try {
            const result = await api.probeMCPClient(id, {});
            setProbe({ id, success: result.success, tools: result.tools || [], error: result.error });
        } catch (e) {
            setProbe({ id, success: false, tools: [], error: e instanceof Error ? e.message : String(e) });
        } finally {
            setProbing('');
        }
    };
    const sourceNode = (source: MCPRouteSource) => {
        const failed = source.state !== 'connected';
        const state = label(source.state, source.state);
        const summary = failed ? state : `${source.tools.length} ${label('toolCount', 'tools')} · ${source.transport}`;
        return (
            <RouteNode
                key={source.id}
                title={source.name}
                subtitle={summary}
                tag={label(source.origin, source.origin)}
                icon={source.processing === 'advisor' ? <Psychology fontSize="small" /> : <Server fontSize="small" />}
                active={enabled && !failed}
                warn={source.state === 'error' || source.state === 'unconfigured'}
                onClick={() => setSelected({ source })}
            />
        );
    };
    const branches = (sources: MCPRouteSource[]) =>
        sources.length > 0 ? (
            <Box
                sx={{
                    borderLeft: '2px solid',
                    borderColor: 'divider',
                    pl: 2,
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 1.25,
                }}
            >
                {sources.map(sourceNode)}
            </Box>
        ) : (
            <Alert severity="info" sx={{ minWidth: 220 }}>
                {label('noReachableTools', 'No tools are assigned to this route.')}
            </Alert>
        );
    const row = (children: React.ReactNode, key?: string) => (
        <Box key={key} sx={(theme) => ({ ...graphRowStyles(theme), py: 1.5, px: 0.5, gap: 1.1 })}>
            {children}
        </Box>
    );
    const regular = routing?.clients || [];
    const serverSources = routing?.server_tools || [];
    const specialSources = serverSources.filter((source) => source.processing === 'advisor');
    const title = selected?.source?.name || selected?.client?.name || label('serverTools', 'Server Tools');
    return (
        <Stack spacing={2.5}>
            <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="body2" color="text.secondary">
                    {label(
                        'hint',
                        'Follow each entry through the gateway to its tool sources. Click a node to configure it.'
                    )}
                </Typography>
                <Button disabled={loading} onClick={() => void load()}>
                    {label('refresh', 'Refresh routes')}
                </Button>
            </Stack>
            {error && <Alert severity="error">{error}</Alert>}
            {loading && (
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <CircularProgress size={18} />
                    <Typography variant="body2">{label('discovering', 'Discovering effective routes…')}</Typography>
                </Stack>
            )}
            {routing && !loading && (
                <>
                    <Box component="section" aria-label={label('ordinaryTools', 'Ordinary tools')}>
                        <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 1.5 }}>
                            <Box>
                                <Typography variant="h6">{label('ordinaryTools', 'Ordinary tools')}</Typography>
                                <Typography variant="body2" color="text.secondary">
                                    {label(
                                        'ordinaryHint',
                                        'The client calls the tool through MCP; results return to the client.'
                                    )}
                                </Typography>
                            </Box>
                            <Button onClick={() => onTools('client')}>
                                {label('manageOrdinary', 'Manage ordinary tools')}
                            </Button>
                        </Stack>
                        {regular.length === 0 && (
                            <Alert
                                severity="info"
                                action={
                                    <Button onClick={() => onClient()}>
                                        {label('configureClient', 'Configure client')}
                                    </Button>
                                }
                            >
                                {label(
                                    'noClients',
                                    'No client profiles are configured. Explicit profiles control access; the default endpoint is not restored automatically.'
                                )}
                            </Alert>
                        )}
                        <Stack spacing={2}>
                            {regular.map((client) => (
                                <Card
                                    variant="outlined"
                                    key={client.id}
                                    sx={{ p: 2.5, opacity: client.enabled ? 1 : 0.65 }}
                                >
                                    <Stack
                                        direction="row"
                                        sx={{
                                            alignItems: 'center',
                                            justifyContent: 'space-between',
                                            flexWrap: 'wrap',
                                            gap: 1,
                                        }}
                                    >
                                        <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                            {client.name}
                                        </Typography>
                                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                            <Chip
                                                size="small"
                                                label={`${enabled ? countTools(client.sources) : 0} ${label('reachableTools', 'reachable tools')}`}
                                            />
                                            {!client.enabled && (
                                                <Chip size="small" label={label('disabled', 'Disabled')} />
                                            )}
                                            <Button onClick={() => setSelected({ client })}>
                                                {label('install', 'Connection instructions')}
                                            </Button>
                                            <Button
                                                disabled={!enabled || !client.enabled || !!probing}
                                                onClick={() => void checkClient(client.id)}
                                            >
                                                {probing === client.id
                                                    ? label('checking', 'Checking…')
                                                    : label('checkClient', 'Check client route')}
                                            </Button>
                                        </Stack>
                                    </Stack>
                                    {row(
                                        <>
                                            <RouteNode
                                                title={client.name}
                                                subtitle={client.endpoint}
                                                tag={label('client', 'Client')}
                                                icon={<Terminal fontSize="small" />}
                                                active={enabled && client.enabled}
                                                onClick={() => setSelected({ client })}
                                            />
                                            <Arrow />
                                            <RouteNode
                                                title={label('gateway', 'Tingly MCP gateway')}
                                                subtitle={label('clientProcessing', 'Client grants · tool forwarding')}
                                                icon={<Security fontSize="small" />}
                                                active={enabled && client.enabled}
                                                onClick={() => onClient(client.id, true)}
                                            />
                                            <Arrow />
                                            {branches(client.sources)}
                                        </>
                                    )}
                                    {probe?.id === client.id && (
                                        <Alert
                                            severity={probe.success ? 'success' : 'error'}
                                            sx={{ mb: 1 }}
                                            data-testid="mcp-route-probe"
                                        >
                                            {probe.success
                                                ? `${label('probePassed', 'Initialize and tools/list passed through the actual gateway endpoint.')} ${probe.tools.length} ${label('toolCount', 'tools')}`
                                                : probe.error}
                                        </Alert>
                                    )}
                                    <Divider sx={{ my: 1 }} />
                                    <Typography variant="caption" color="text.secondary">
                                        {label(
                                            'ordinaryFooter',
                                            'One gateway endpoint connects this client to both built-in and external tools.'
                                        )}
                                    </Typography>
                                </Card>
                            ))}
                        </Stack>
                    </Box>
                    <Box component="section" aria-label={label('serverTools', 'Server Tools')}>
                        <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 1.5 }}>
                            <Box>
                                <Typography variant="h6">{label('serverTools', 'Server Tools')}</Typography>
                                <Typography variant="body2" color="text.secondary">
                                    {label(
                                        'serverHint',
                                        'The gateway executes model tool calls, supplies their results and continues the model response.'
                                    )}
                                </Typography>
                            </Box>
                            <Button onClick={() => onTools('gateway')}>
                                {label('manageServer', 'Manage Server Tools')}
                            </Button>
                        </Stack>
                        <Card variant="outlined" sx={{ p: 2.5 }}>
                            <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center' }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                    {label('modelCalls', 'Gateway model requests')}
                                </Typography>
                                <Chip
                                    size="small"
                                    label={`${enabled ? countTools(serverSources) : 0} ${label('reachableTools', 'reachable tools')}`}
                                />
                            </Stack>
                            {row(
                                <>
                                    <RouteNode
                                        title={label('modelRequest', 'Model request')}
                                        subtitle={label('modelRequestHint', 'Models connected through Tingly Box')}
                                        tag={label('model', 'Model')}
                                        icon={<Hub fontSize="small" />}
                                        active={enabled}
                                        onClick={() => setSelected({ server: true })}
                                    />
                                    <Arrow />
                                    <RouteNode
                                        title={label('serverExecution', 'Server tool execution')}
                                        subtitle={label('serverProcessing', 'Provide tools → execute → continue')}
                                        tag={label('server', 'Server')}
                                        icon={<Settings fontSize="small" />}
                                        active={enabled}
                                        onClick={() => onTools('gateway')}
                                    />
                                    <Arrow />
                                    {branches(serverSources)}
                                </>
                            )}
                            <Box
                                sx={{ borderBottom: '1px dashed', borderColor: 'divider', mt: 1, mb: 2, px: 2, pb: 1 }}
                            >
                                <Typography variant="caption" color="text.secondary">
                                    ←{' '}
                                    {label(
                                        'continuation',
                                        'Tool results return to the model, which continues its response.'
                                    )}
                                </Typography>
                            </Box>
                            {specialSources.length > 0 && (
                                <>
                                    <Button onClick={() => setExpanded(!expanded)} aria-expanded={expanded}>
                                        {label(
                                            expanded ? 'collapseSpecial' : 'expandSpecial',
                                            expanded ? 'Collapse special processing' : 'Expand special processing'
                                        )}
                                    </Button>
                                    {expanded && (
                                        <Stack
                                            spacing={1.5}
                                            sx={{ mt: 1.5, p: 2, bgcolor: 'action.hover', borderRadius: 1 }}
                                            data-testid="mcp-special-processing"
                                        >
                                            {specialSources.map((source) => (
                                                <Box key={source.id}>
                                                    {row(
                                                        <>
                                                            {sourceNode(source)}
                                                            <Arrow />
                                                            <RouteNode
                                                                title={
                                                                    source.advisor?.provider_name ||
                                                                    label(
                                                                        'unconfiguredProvider',
                                                                        'Configure consultation provider'
                                                                    )
                                                                }
                                                                subtitle={
                                                                    source.advisor?.model ||
                                                                    label(
                                                                        'unconfiguredModel',
                                                                        'Configure consultation model'
                                                                    )
                                                                }
                                                                tag={label('consultationModel', 'Consultation model')}
                                                                icon={<Psychology fontSize="small" />}
                                                                active={enabled && source.state === 'connected'}
                                                                warn={
                                                                    !source.advisor?.model ||
                                                                    !source.advisor?.provider_name
                                                                }
                                                                onClick={() => onEditSource(source.id)}
                                                            />
                                                            <Arrow />
                                                            <Typography
                                                                variant="caption"
                                                                color="text.secondary"
                                                                sx={{ minWidth: 150 }}
                                                            >
                                                                {label(
                                                                    'specialReturn',
                                                                    'Consultation result returns to the tool loop'
                                                                )}
                                                            </Typography>
                                                        </>
                                                    )}
                                                </Box>
                                            ))}
                                        </Stack>
                                    )}
                                </>
                            )}
                            <Divider sx={{ my: 1 }} />
                            <Typography variant="caption" color="text.secondary">
                                {label(
                                    'serverFooter',
                                    'Built-in and external tools can both be Server Tools. Special processing appears only on the relevant branch.'
                                )}
                            </Typography>
                        </Card>
                    </Box>
                </>
            )}
            <Drawer
                anchor="right"
                open={!!selected}
                onClose={() => setSelected(null)}
                slotProps={{ paper: { sx: { width: { xs: '100%', sm: 520 }, p: 3 } } }}
            >
                <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                    <Typography variant="h6">{title}</Typography>
                    <IconButton aria-label={label('close', 'Close details')} onClick={() => setSelected(null)}>
                        <Close />
                    </IconButton>
                </Stack>
                {selected?.source && (
                    <Stack spacing={2}>
                        <Stack direction="row" spacing={1}>
                            <Chip size="small" label={label(selected.source.origin, selected.source.origin)} />
                            <Chip size="small" label={selected.source.transport} />
                            <Chip size="small" label={label(selected.source.state, selected.source.state)} />
                        </Stack>
                        <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                            {selected.source.address}
                        </Typography>
                        {selected.source.error && <Alert severity="error">{selected.source.error}</Alert>}
                        <Typography variant="body2">
                            {label('effectiveTools', 'Tools available on this path')}
                        </Typography>
                        {selected.source.tools.length === 0 && (
                            <Typography color="text.secondary">
                                {label('noDiscoveredTools', 'No available tools were discovered for this path.')}
                            </Typography>
                        )}
                        {selected.source.tools.map((tool) => (
                            <Box key={tool.normalized_name}>
                                <Typography variant="subtitle2">{tool.name}</Typography>
                                <Typography variant="caption" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>
                                    {tool.normalized_name}
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    {tool.description}
                                </Typography>
                            </Box>
                        ))}
                        <Button
                            variant="contained"
                            onClick={() => {
                                onEditSource(selected.source!.id);
                                setSelected(null);
                            }}
                        >
                            {label('configureSource', 'Configure tool source')}
                        </Button>
                    </Stack>
                )}
                {selected?.client && (
                    <Stack spacing={2}>
                        <Typography variant="body2" color="text.secondary">
                            {label(
                                'ordinaryHint',
                                'The client calls the tool through MCP; results return to the client.'
                            )}
                        </Typography>
                        <AgentInstallCard clientId={selected.client.id} />
                        <Button onClick={() => onClient(selected.client!.id, true)}>
                            {label('configureGrants', 'Configure client grants')}
                        </Button>
                    </Stack>
                )}
                {selected?.server && (
                    <Stack spacing={2}>
                        <Typography variant="body2">
                            {label(
                                'serverHint',
                                'The gateway executes model tool calls, supplies their results and continues the model response.'
                            )}
                        </Typography>
                        {['provide', 'dispatch', 'execute', 'continue'].map((step, i) => (
                            <Typography key={step}>
                                {i + 1}. {label(step, step)}
                            </Typography>
                        ))}
                        <Button onClick={() => onTools('gateway')}>
                            {label('manageServer', 'Manage Server Tools')}
                        </Button>
                    </Stack>
                )}
            </Drawer>
        </Stack>
    );
}
