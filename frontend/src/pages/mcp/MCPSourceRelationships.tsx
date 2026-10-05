import { Alert, Box, Button, Card, Chip, Divider, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { ArrowForward, Psychology, Security, Server, Terminal } from '@/components/icons';
import {
    clientToolState,
    configuredClientUsesSource,
    gatewayToolState,
    type RelationshipState,
} from './relationshipState';
import type { MCPClientProfile, MCPRouteSource, MCPRoutingSnapshot, MCPSourceConfig } from './types';

const stateText: Record<RelationshipState, string> = {
    available: 'Available',
    mcpOff: 'MCP execution is off',
    sourceOff: 'Shared connection is disabled',
    discoveryFailed: 'Discovery is incomplete; availability cannot be confirmed',
    failedConnection: 'Connection failed; availability cannot be confirmed',
    advisorUnconfigured: 'Advisor model is not configured; availability cannot be confirmed',
    disconnected: 'Connection is not established; availability cannot be confirmed',
    toolOff: 'Disabled by the shared tool policy',
    allowListOff: 'Source allow list does not include this tool',
    ordinaryOff: 'MCP publication is off',
    gatewayOff: 'Server Tool usage is off',
    clientOff: 'Client is disabled',
    sourceNotGranted: 'Connection is not granted to this client',
    toolNotGranted: 'Tool is not granted to this client',
    advisorContext: 'Advisor requires gateway model context',
    notConfirmed: 'Not confirmed in the effective configuration; refresh to check',
};

export default function MCPSourceRelationships({
    source,
    snapshot,
    config,
    profiles,
    enabled,
    onEditSource,
    onClient,
    onTools,
}: {
    source: MCPRouteSource;
    snapshot: MCPRoutingSnapshot;
    config?: MCPSourceConfig;
    profiles: MCPClientProfile[];
    enabled: boolean;
    onEditSource: (id: string) => void;
    onClient: (id?: string, edit?: boolean) => void;
    onTools: (kind: 'client' | 'gateway', sourceId?: string) => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.relationships.${key}`, { defaultValue: fallback });
    const reason = (state: RelationshipState) => label(state, stateText[state]);
    const profileFor = (id: string) => profiles.find((profile) => profile.id === id);
    const availableClientTools = (id: string) => {
        const client = snapshot.clients.find((item) => item.id === id)!;
        return source.tools.filter(
            (tool) => clientToolState(source, tool, client, profileFor(id), config, enabled) === 'available'
        );
    };
    const gatewayTools = source.tools.filter(
        (tool) => gatewayToolState(source, tool, snapshot, config, enabled) === 'available'
    );
    const associations = profiles.filter((profile) => configuredClientUsesSource(source, profile));
    const sourceBlocked = config?.enabled === false || source.state === 'disabled';
    const unfinished = !sourceBlocked && source.state !== 'connected';
    const summary = (count: number) => `${count} ${label('availableTools', 'available tools')}`;
    const node = (name: string, detail: string, icon: React.ReactNode, action: () => void, available: boolean) => (
        <Button
            variant="outlined"
            onClick={action}
            sx={{
                minWidth: 190,
                flexShrink: 0,
                p: 1.5,
                justifyContent: 'flex-start',
                textAlign: 'left',
                color: available ? 'text.primary' : 'text.secondary',
                borderStyle: available ? 'solid' : 'dashed',
            }}
        >
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                {icon}
                <Box>
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>
                        {name}
                    </Typography>
                    <Typography variant="caption" sx={{ display: 'block' }}>
                        {detail}
                    </Typography>
                </Box>
            </Stack>
        </Button>
    );
    const path = (children: React.ReactNode) => (
        <Stack direction="row" spacing={1.25} sx={{ alignItems: 'center', overflowX: 'auto', pb: 1, maxWidth: '100%' }}>
            {children}
        </Stack>
    );
    const status = (state: RelationshipState) => (
        <Typography variant="body2" color={state === 'available' ? 'success.main' : 'text.secondary'}>
            {reason(state)}
        </Typography>
    );
    return (
        <Stack
            spacing={2.5}
            component="section"
            aria-label={label('sourceView', 'Connection usage relationships')}
            data-testid="mcp-source-relationships"
        >
            <Stack
                direction="row"
                sx={{ justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 1 }}
            >
                <Box>
                    <Typography variant="h6">{source.name}</Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>
                        {source.address}
                    </Typography>
                </Box>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <Chip size="small" label={label(source.origin, source.origin)} />
                    <Chip size="small" label={source.transport} />
                    <Button onClick={() => onEditSource(source.id)}>
                        {label('configureConnection', 'Configure shared connection')}
                    </Button>
                </Stack>
            </Stack>
            {(!enabled || sourceBlocked || unfinished) && (
                <Alert severity="warning">
                    {reason(
                        !enabled
                            ? 'mcpOff'
                            : sourceBlocked
                              ? 'sourceOff'
                              : source.state === 'error'
                                ? 'failedConnection'
                                : source.state === 'unconfigured'
                                  ? 'advisorUnconfigured'
                                  : source.state === 'disconnected'
                                    ? 'disconnected'
                                    : 'discoveryFailed'
                    )}
                </Alert>
            )}
            <Typography variant="caption" color="text.secondary">
                {label(
                    'configurationOnly',
                    'Shows the current effective configuration, not call history. Solid nodes have available tools; dashed nodes have no confirmed tools.'
                )}
            </Typography>
            <Box component="section" aria-label="MCP">
                <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                    MCP · {label('whoCanUse', 'Who can use it?')}
                </Typography>
                <Stack spacing={1.25}>
                    {snapshot.clients.map((client) => {
                        const tools = availableClientTools(client.id);
                        return (
                            <Card
                                variant="outlined"
                                key={client.id}
                                component="article"
                                aria-label={client.name}
                                sx={{ p: 1.75 }}
                            >
                                {path(
                                    <>
                                        {node(
                                            client.name,
                                            client.endpoint,
                                            <Terminal fontSize="small" />,
                                            () => onClient(client.id, true),
                                            tools.length > 0
                                        )}
                                        <ArrowForward fontSize="small" sx={{ flexShrink: 0 }} />
                                        {node(
                                            label('clientAccess', 'MCP client access'),
                                            summary(tools.length),
                                            <Security fontSize="small" />,
                                            () => onClient(client.id, true),
                                            tools.length > 0
                                        )}
                                        <ArrowForward fontSize="small" sx={{ flexShrink: 0 }} />
                                        {node(
                                            source.name,
                                            summary(tools.length),
                                            <Server fontSize="small" />,
                                            () => onEditSource(source.id),
                                            tools.length > 0
                                        )}
                                    </>
                                )}
                                {tools.length > 0 ? (
                                    <Typography
                                        variant="caption"
                                        color="text.secondary"
                                        sx={{ overflowWrap: 'anywhere' }}
                                    >
                                        {tools.map((tool) => tool.name).join(' · ')}
                                    </Typography>
                                ) : (
                                    <Typography variant="body2" color="text.secondary">
                                        {label(
                                            'noAvailableTools',
                                            'No tools are currently available. See the per-tool reasons below.'
                                        )}
                                    </Typography>
                                )}
                            </Card>
                        );
                    })}
                    {snapshot.clients.length === 0 && (
                        <Alert
                            severity="info"
                            action={
                                <Button onClick={() => onClient()}>
                                    {label('configureClient', 'Configure a client')}
                                </Button>
                            }
                        >
                            {label('noClients', 'No client connections are configured.')}
                        </Alert>
                    )}
                </Stack>
            </Box>
            <Box component="section" aria-label="Server Tool">
                <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                    Server Tool · {label('gatewayUse', 'Gateway model use')}
                </Typography>
                <Card variant="outlined" sx={{ p: 1.75 }}>
                    {path(
                        <>
                            {node(
                                label('modelRequests', 'Gateway model requests'),
                                'Server Tool',
                                <Psychology fontSize="small" />,
                                () => onTools('gateway'),
                                gatewayTools.length > 0
                            )}
                            <ArrowForward fontSize="small" sx={{ flexShrink: 0 }} />
                            {node(
                                label('gatewayExecution', 'Execute and continue'),
                                summary(gatewayTools.length),
                                <Security fontSize="small" />,
                                () => onTools('gateway'),
                                gatewayTools.length > 0
                            )}
                            <ArrowForward fontSize="small" sx={{ flexShrink: 0 }} />
                            {node(
                                source.name,
                                summary(gatewayTools.length),
                                <Server fontSize="small" />,
                                () => onEditSource(source.id),
                                gatewayTools.length > 0
                            )}
                        </>
                    )}
                    {gatewayTools.length > 0 ? (
                        <Typography variant="caption" color="text.secondary">
                            {gatewayTools.map((tool) => tool.name).join(' · ')}
                        </Typography>
                    ) : (
                        <Typography variant="body2" color="text.secondary">
                            {label(
                                'noAvailableTools',
                                'No tools are currently available. See the per-tool reasons below.'
                            )}
                        </Typography>
                    )}
                    {source.processing === 'advisor' && (
                        <Box sx={{ mt: 1.5 }}>
                            <Divider sx={{ mb: 1.5 }} />
                            {path(
                                <>
                                    <Typography variant="body2">
                                        {label('consultationModel', 'Consultation model')}
                                    </Typography>
                                    <ArrowForward fontSize="small" sx={{ flexShrink: 0 }} />
                                    {node(
                                        source.advisor?.provider_name ||
                                            label('unconfiguredProvider', 'Provider not configured'),
                                        source.advisor?.model || label('unconfiguredModel', 'Model not configured'),
                                        <Psychology fontSize="small" />,
                                        () => onTools('gateway', source.id),
                                        gatewayTools.length > 0
                                    )}
                                </>
                            )}
                            <Typography variant="caption" color="text.secondary">
                                {label(
                                    'consultationReturn',
                                    'Consultation results return to the gateway model conversation.'
                                )}
                            </Typography>
                        </Box>
                    )}
                </Card>
            </Box>
            <Box component="section" aria-label={label('whyUnavailable', 'Which tools are available, and why not?')}>
                <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                    {label('whyUnavailable', 'Which tools are available, and why not?')}
                </Typography>
                <Stack spacing={1.25}>
                    {source.tools.map((tool) => (
                        <Card
                            variant="outlined"
                            key={tool.normalized_name}
                            component="article"
                            aria-label={tool.name}
                            sx={{ p: 1.75 }}
                        >
                            <Typography variant="subtitle2">{tool.name}</Typography>
                            <Box
                                sx={{
                                    display: 'grid',
                                    gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
                                    gap: 2,
                                    mt: 1,
                                }}
                            >
                                <Box>
                                    <Typography variant="caption" sx={{ fontWeight: 600 }}>
                                        MCP
                                    </Typography>
                                    {snapshot.clients.map((client) => (
                                        <Box key={client.id} sx={{ mt: 0.75 }}>
                                            <Typography variant="body2" sx={{ fontWeight: 500 }}>
                                                {client.name}
                                            </Typography>
                                            {status(
                                                clientToolState(
                                                    source,
                                                    tool,
                                                    client,
                                                    profileFor(client.id),
                                                    config,
                                                    enabled
                                                )
                                            )}
                                        </Box>
                                    ))}
                                    {snapshot.clients.length === 0 && (
                                        <Typography variant="body2" color="text.secondary">
                                            {label('noClients', 'No client connections are configured.')}
                                        </Typography>
                                    )}
                                </Box>
                                <Box>
                                    <Typography variant="caption" sx={{ fontWeight: 600 }}>
                                        Server Tool
                                    </Typography>
                                    {status(gatewayToolState(source, tool, snapshot, config, enabled))}
                                </Box>
                            </Box>
                        </Card>
                    ))}
                    {source.tools.length === 0 && (
                        <Typography variant="body2" color="text.secondary">
                            {source.state === 'connected'
                                ? label('noTools', 'This connection discovered no tools')
                                : label(
                                      'noCatalog',
                                      'No tool catalog is available. Repair or refresh the connection before reviewing individual tools.'
                                  )}
                        </Typography>
                    )}
                </Stack>
                <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 1, mt: 1 }}>
                    {source.processing !== 'advisor' && (
                        <Button onClick={() => onTools('client', source.id)}>
                            {label('configureOrdinary', 'Configure MCP publication')}
                        </Button>
                    )}
                    <Button onClick={() => onTools('gateway', source.id)}>
                        {label('configureGateway', 'Configure Server Tool usage')}
                    </Button>
                </Stack>
            </Box>
            <Card
                variant="outlined"
                component="section"
                aria-label={label('impact', 'Who does a change affect?')}
                sx={{ p: 2 }}
            >
                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                    {label('impact', 'Who does a change affect?')}
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                    {source.processing === 'advisor'
                        ? label(
                              'advisorImpact',
                              'Advisor configuration affects Server Tool and its consultation model. It is not available through ordinary client access.'
                          )
                        : label(
                              'sharedImpact',
                              'Connection settings and global tool policies in Tool affect both paths. MCP publication and Server Tool execution are controlled independently.'
                          )}
                </Typography>
                <Typography variant="body2" sx={{ mt: 1 }}>
                    {label('configuredClients', 'Clients associated by saved grants')}:{' '}
                    {associations.map((profile) => profile.name || profile.id).join(' · ') || label('none', 'None')}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                    {label(
                        'associationHint',
                        'Saved associations can include disabled clients or connections. Availability is shown above; changing a client grant affects only that client.'
                    )}
                </Typography>
            </Card>
        </Stack>
    );
}
