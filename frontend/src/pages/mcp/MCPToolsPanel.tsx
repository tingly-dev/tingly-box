import { useState } from 'react';
import { useTranslation } from 'react-i18next';
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
    Stack,
    Typography,
} from '@mui/material';
import MCPToolCard from './MCPToolCard';
import MCPToolTestDialog from './MCPToolTestDialog';
import {
    isAdvisorTool,
    isToolRestricted,
    toolPolicyPatch,
    type MCPToolMode,
    type MCPToolPatch,
} from './toolPresentation';
import { sourceToFormValue, type MCPCatalogTool, type MCPRouteSource, type MCPSourceConfig } from './types';

// The catalog and routing diagram share one applied snapshot from the workspace.
export default function MCPToolsPanel({
    sources,
    routes,
    loading,
    enabled,
    saveSource,
    mode,
    onConfigureSource,
    onRelationships,
}: {
    sources: MCPSourceConfig[];
    routes: MCPRouteSource[];
    loading: boolean;
    enabled: boolean;
    saveSource: (patch: MCPSourceConfig) => Promise<void>;
    mode: MCPToolMode;
    onConfigureSource?: (id: string) => void;
    onRelationships?: (id: string) => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.center.${key}`, { defaultValue: fallback });
    const workspace = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [choosing, setChoosing] = useState(false);
    const [testing, setTesting] = useState<MCPCatalogTool | null>(null);
    const asset = mode === 'asset';
    const policy = async (tool: MCPCatalogTool, patch: MCPToolPatch) => {
        const source = sources.find((s) => s.id === tool.source_id);
        if (!source) return;
        setBusy(true);
        setError('');
        try {
            await saveSource(toolPolicyPatch(source, tool, patch));
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };
    const groups = sources
        .map((source) => {
            const route = routes.find((r) => r.id === source.id);
            const tools = (route?.tools || []).filter(
                (tool) => asset || ((mode !== 'client' || !isAdvisorTool(tool, source)) && tool.usage[mode])
            );
            const assigned =
                !asset &&
                (sourceToFormValue(source).usage[mode] ||
                    Object.values(source.tool_policies || {}).some((p) => p.usage?.[mode]));
            return { source, route, tools, assigned };
        })
        .filter(
            ({ tools, assigned, route }) => asset || tools.length > 0 || (assigned && route?.state !== 'connected')
        );
    const candidates = routes
        .flatMap((r) => r.tools)
        .filter(
            (tool) =>
                !asset &&
                !tool.usage[mode] &&
                !isToolRestricted(
                    tool,
                    sources.find((s) => s.id === tool.source_id)
                ) &&
                !(
                    mode === 'client' &&
                    isAdvisorTool(
                        tool,
                        sources.find((s) => s.id === tool.source_id)
                    )
                )
        );
    const chooserTitle =
        mode === 'client'
            ? label('chooseOrdinary', 'Choose tools to publish')
            : label('chooseServer', 'Choose Server Tools');
    return (
        <Stack spacing={2}>
            {!asset && (
                <Button sx={{ alignSelf: 'flex-start' }} disabled={busy || loading} onClick={() => setChoosing(true)}>
                    {chooserTitle}
                </Button>
            )}
            {error && <Alert severity="error">{error}</Alert>}
            {loading && <CircularProgress size={20} />}
            {!loading && groups.length === 0 && (
                <Typography color="text.secondary">
                    {asset
                        ? workspace(
                              'emptyConnectionsHint',
                              'Paste a tool service URL or enter a local command. We will discover its tools for you.'
                          )
                        : label(
                              'noAssignedTools',
                              'No tools are assigned to this section. Choose tools to enable them for this purpose.'
                          )}
                </Typography>
            )}
            {groups.map(({ source, route, tools }) => {
                const advisor = source.transport === 'advisor' || !!source.advisor;
                const failed = route?.state === 'error' || route?.state === 'unconfigured';
                return (
                    <Box
                        component="article"
                        aria-label={source.name || source.id}
                        key={source.id}
                        sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1.5, p: 2 }}
                    >
                        <Stack
                            direction="row"
                            sx={{
                                justifyContent: 'space-between',
                                alignItems: 'center',
                                gap: 1,
                                flexWrap: 'wrap',
                                mb: 1.5,
                            }}
                        >
                            <Stack direction="row" sx={{ alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                    {source.name || source.id}
                                </Typography>
                                {asset && (
                                    <Typography variant="caption" color="text.secondary">
                                        {t(`mcp.relationships.${route?.origin || source.origin || 'external'}`, {
                                            defaultValue: route?.origin || source.origin || 'external',
                                        })}
                                    </Typography>
                                )}
                                {asset && (
                                    <Chip
                                        size="small"
                                        variant="outlined"
                                        label={
                                            source.enabled === false
                                                ? workspace('off', 'Off')
                                                : !route || loading
                                                  ? workspace('checking', 'Checking…')
                                                  : route.state === 'connected'
                                                    ? workspace('connected', 'Connected')
                                                    : workspace('needsAttention', 'Needs attention')
                                        }
                                    />
                                )}
                            </Stack>
                            <Stack direction="row" sx={{ gap: 1, flexWrap: 'wrap' }}>
                                {onConfigureSource && (
                                    <Button size="small" onClick={() => onConfigureSource(source.id!)}>
                                        {asset && failed
                                            ? workspace('fixConnection', 'Fix connection')
                                            : advisor && !asset
                                              ? workspace('configureAdvisor', 'Configure Advisor model')
                                              : asset
                                                ? workspace('configureConnection', 'Configure this connection')
                                                : mode === 'client'
                                                  ? workspace('configurePublication', 'Configure MCP publication')
                                                  : workspace('configureExecution', 'Configure execution usage')}
                                    </Button>
                                )}
                                {onRelationships && (
                                    <Button size="small" onClick={() => onRelationships(source.id!)}>
                                        {t('mcp.relationships.view', { defaultValue: 'View usage relationships' })}
                                    </Button>
                                )}
                            </Stack>
                        </Stack>
                        {route && route.state !== 'connected' && (!asset || !!route.error) && (
                            <Alert severity={failed ? 'warning' : 'info'} sx={{ mb: 1 }}>
                                {route.error ||
                                    (source.enabled === false
                                        ? workspace(
                                              'connectionOff',
                                              'This connection is off. Enable it to discover and use its tools.'
                                          )
                                        : workspace(
                                              'connectionPending',
                                              'This connection needs to be checked or configured.'
                                          ))}
                            </Alert>
                        )}
                        {advisor && (
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                                {workspace(
                                    'advisorContext',
                                    'Advisor needs the model conversation. It is only a Server Tool; configure its consultation model in Server Tool and verify it with a model request.'
                                )}
                            </Typography>
                        )}
                        <Stack spacing={1}>
                            {tools.map((tool) => (
                                <MCPToolCard
                                    key={tool.normalized_name}
                                    tool={tool}
                                    source={source}
                                    mode={mode}
                                    enabled={enabled}
                                    busy={busy || loading}
                                    onChange={(patch) => void policy(tool, patch)}
                                    onTest={() => setTesting(tool)}
                                />
                            ))}
                            {!loading && route?.state === 'connected' && tools.length === 0 && (
                                <Typography variant="body2" color="text.secondary">
                                    {workspace('emptyDiscovery', 'The connection returned no tools.')}
                                </Typography>
                            )}
                        </Stack>
                    </Box>
                );
            })}
            <Dialog open={choosing} onClose={() => !busy && setChoosing(false)} maxWidth="md" fullWidth>
                <DialogTitle>{chooserTitle}</DialogTitle>
                <DialogContent dividers>
                    {candidates.length === 0 && (
                        <Typography>{workspace('noAdditionalTools', 'No other available tools to add.')}</Typography>
                    )}
                    {candidates.map((tool) => (
                        <Stack
                            component="article"
                            aria-label={`${sources.find((s) => s.id === tool.source_id)?.name || tool.source_id} / ${tool.name}`}
                            key={tool.normalized_name}
                            direction="row"
                            sx={{ justifyContent: 'space-between', alignItems: 'center', gap: 1, mb: 1 }}
                        >
                            <Typography>
                                {sources.find((s) => s.id === tool.source_id)?.name || tool.source_id} / {tool.name}
                            </Typography>
                            <Button
                                disabled={busy || loading}
                                onClick={() => {
                                    if (mode !== 'asset') void policy(tool, { usage: { ...tool.usage, [mode]: true } });
                                }}
                            >
                                {label('addTool', 'Add tool')}
                            </Button>
                        </Stack>
                    ))}
                </DialogContent>
                <DialogActions>
                    <Button disabled={busy} onClick={() => setChoosing(false)}>
                        {label('close', 'Close')}
                    </Button>
                </DialogActions>
            </Dialog>
            {testing && (
                <MCPToolTestDialog
                    key={testing.normalized_name}
                    tool={
                        routes
                            .flatMap((r) => r.tools)
                            .find((tool) => tool.normalized_name === testing.normalized_name) || {
                            ...testing,
                            enabled: false,
                        }
                    }
                    enabled={enabled}
                    onClose={() => setTesting(null)}
                />
            )}
        </Stack>
    );
}
