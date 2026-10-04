import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Alert,
    Box,
    Button,
    Card,
    Chip,
    CircularProgress,
    Divider,
    Stack,
    MenuItem,
    TextField,
    Typography,
} from '@mui/material';
import { Hub, Psychology, Security, Server, Settings, Terminal } from '@/components/icons';
import { ArrowNode } from '@/components/nodes/ArrowNode';
import { graphRowStyles, StyledBotGraphNode } from '@/components/nodes/styles';
import MCPSourceRelationships from './MCPSourceRelationships';
import type { MCPClientProfile, MCPRouteSource, MCPRoutingSnapshot, MCPSourceConfig } from './types';

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
    routing,
    enabled,
    loading,
    error,
    onRefresh,
    sources,
    profiles,
    focusSource,
    onFocusSource,
}: {
    onEditSource: (id: string) => void;
    onClient: (id?: string, edit?: boolean) => void;
    onTools: (kind: 'client' | 'gateway', sourceId?: string) => void;
    routing: MCPRoutingSnapshot | null;
    enabled: boolean;
    loading: boolean;
    error?: string;
    onRefresh: () => void;
    sources: MCPSourceConfig[];
    profiles: MCPClientProfile[];
    focusSource?: string;
    onFocusSource: (id?: string) => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.routing.${key}`, { defaultValue: fallback });
    const [expanded, setExpanded] = useState(false);
    const focused = routing?.sources.find((source) => source.id === focusSource);
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
                onClick={() => onFocusSource(source.id)}
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
    const specialSources = serverSources
        .filter((source) => source.processing === 'advisor')
        .map((source) => routing?.sources.find((item) => item.id === source.id) || source);
    return (
        <Stack spacing={2.5}>
            <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="body2" color="text.secondary">
                    {label(
                        'hint',
                        'Inspect effective tool access or focus a connection to review its consumers and blocked tools.'
                    )}
                </Typography>
                <Button disabled={loading} onClick={onRefresh}>
                    {label('refresh', 'Refresh relationships')}
                </Button>
            </Stack>
            <TextField
                select
                size="small"
                label={t('mcp.relationships.focus', { defaultValue: 'Inspect a connection' })}
                value={focusSource && routing?.sources.some((source) => source.id === focusSource) ? focusSource : ''}
                onChange={(event) => onFocusSource(event.target.value || undefined)}
                sx={{ maxWidth: 360 }}
            >
                <MenuItem value="">{t('mcp.relationships.all', { defaultValue: 'All usage relationships' })}</MenuItem>
                {(routing?.sources || []).map((source) => (
                    <MenuItem key={source.id} value={source.id}>
                        {source.name}
                    </MenuItem>
                ))}
            </TextField>
            {error && <Alert severity="error">{error}</Alert>}
            {loading && (
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <CircularProgress size={18} />
                    <Typography variant="body2">{label('discovering', 'Discovering effective routes…')}</Typography>
                </Stack>
            )}
            {focusSource && routing && !loading && !focused && (
                <Alert severity="info">
                    {t('mcp.relationships.missing', {
                        defaultValue:
                            'This connection no longer exists. Select another connection or view all relationships.',
                    })}
                </Alert>
            )}
            {focused && routing && !loading && !error && (
                <MCPSourceRelationships
                    source={focused}
                    snapshot={routing}
                    config={sources.find((source) => source.id === focused.id)}
                    profiles={profiles}
                    enabled={enabled}
                    onEditSource={onEditSource}
                    onClient={onClient}
                    onTools={onTools}
                />
            )}
            {routing && !loading && !error && !focusSource && (
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
                                            <Button onClick={() => onClient(client.id, true)}>
                                                {label('configureGrants', 'Configure client grants')}
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
                                                onClick={() => onClient(client.id, true)}
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
                                        onClick={() => onTools('gateway')}
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
                                                                onClick={() => onTools('gateway', source.id)}
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
        </Stack>
    );
}
