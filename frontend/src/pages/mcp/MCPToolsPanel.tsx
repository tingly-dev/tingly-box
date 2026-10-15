import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
    Checkbox,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { api } from '@/services/api';
import { sourceToFormValue } from './types';
import type { MCPCatalogTool, MCPSourceConfig, MCPSourceStatus } from './types';

export default function MCPToolsPanel({
    sources,
    enabled,
    saveSource,
    usage = 'client',
    onConfigureSource,
    onRelationships,
    showIntro = true,
    scopeOnly = false,
    assetsOnly = false,
    showTesting = !scopeOnly,
}: {
    usage?: 'client' | 'gateway';
    onConfigureSource?: (id: string) => void;
    onRelationships?: (id: string) => void;
    showIntro?: boolean;
    scopeOnly?: boolean;
    assetsOnly?: boolean;
    showTesting?: boolean;
    sources: MCPSourceConfig[];
    enabled: boolean;
    saveSource: (patch: MCPSourceConfig) => Promise<void>;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.center.${key}`, { defaultValue: fallback });
    const [catalog, setCatalog] = useState<MCPSourceStatus[]>([]);
    const [busy, setBusy] = useState(true);
    const [error, setError] = useState('');
    const [choosing, setChoosing] = useState(false);
    const [testing, setTesting] = useState<MCPCatalogTool | null>(null);
    const [args, setArgs] = useState('{}');
    const [result, setResult] = useState<unknown>(null);
    const discoveryGeneration = useRef(0);
    const discover = useCallback(async () => {
        const generation = ++discoveryGeneration.current;
        try {
            const response = await api.getMCPCatalog();
            if (generation !== discoveryGeneration.current) return;
            if (!response?.success) throw new Error(response?.error || 'Discovery failed');
            setCatalog(response.sources || []);
            setError('');
        } catch (e) {
            if (generation === discoveryGeneration.current) setError(e instanceof Error ? e.message : String(e));
        } finally {
            if (generation === discoveryGeneration.current) setBusy(false);
        }
    }, []);
    useEffect(() => {
        let active = true;
        void Promise.resolve().then(() => {
            if (active) void discover();
        });
        return () => {
            active = false;
            discoveryGeneration.current++;
        };
        // Saved source snapshots invalidate the server catalog even though
        // discovery reads the applied configuration through the API.
        // oxlint-disable-next-line react/exhaustive-effect-dependencies
    }, [sources, discover]);
    const refresh = async () => {
        setBusy(true);
        setError('');
        await discover();
    };
    const policy = async (tool: MCPCatalogTool, patch: { enabled?: boolean; usage?: MCPCatalogTool['usage'] }) => {
        setBusy(true);
        setError('');
        try {
            const source = sources.find((s) => s.id === tool.source_id)!;
            await saveSource({
                id: source.id,
                tool_policies: {
                    ...source.tool_policies,
                    [tool.name]: { ...source.tool_policies?.[tool.name], ...patch },
                },
            });
            await refresh();
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };
    const test = async () => {
        setBusy(true);
        setError('');
        setResult(null);
        try {
            const argumentsValue: unknown = JSON.parse(args);
            if (!argumentsValue || typeof argumentsValue !== 'object' || Array.isArray(argumentsValue))
                throw new Error(label('objectArgs', 'Arguments must be a JSON object.'));
            const response = await api.callMCPTool({
                source_id: testing!.source_id,
                tool_name: testing!.name,
                arguments: argumentsValue as Record<string, unknown>,
            });
            setResult(response);
            if (!response?.success) throw new Error(response?.error || 'Tool execution failed');
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };
    const restricted = (tool: MCPCatalogTool) => {
        const source = sources.find((s) => s.id === tool.source_id);
        const allowed = source?.tools || [];
        return (
            source?.enabled === false || (allowed.length > 0 && !allowed.includes('*') && !allowed.includes(tool.name))
        );
    };
    const tools = catalog
        .flatMap((s) => s.tools || [])
        .filter((tool) => assetsOnly || (usage === 'client' ? tool.usage.client : tool.usage.gateway));
    return (
        <Stack spacing={2}>
            <Stack direction="row" sx={{ justifyContent: 'space-between', gap: 1 }}>
                {showIntro && (
                    <Typography color="text.secondary">
                        {usage === 'client'
                            ? label(
                                  'ordinaryHint',
                                  'Publish tools for MCP clients here. Manage shared tool connections in Tool.'
                              )
                            : label(
                                  'serverHint',
                                  'The gateway executes these tools during model requests and returns their results to the model to continue its answer.'
                              )}
                    </Typography>
                )}
                {!assetsOnly && (
                    <Button disabled={busy} onClick={() => setChoosing(true)}>
                        {usage === 'client'
                            ? label('chooseOrdinary', 'Choose tools to publish')
                            : label('chooseServer', 'Choose Server Tools')}
                    </Button>
                )}
                <Button disabled={busy} onClick={() => void refresh()}>
                    {label(busy ? 'discovering' : 'discover', busy ? 'Discovering…' : 'Discover tools')}
                </Button>
            </Stack>
            {error && <Alert severity="error">{error}</Alert>}
            {catalog
                .filter((status) => {
                    const source = sources.find((item) => item.id === status.source_id);
                    if (status.state === 'connected') return false;
                    if (assetsOnly) return Boolean(source);
                    if (!source) return Boolean(status.error);
                    return (
                        source.enabled !== false &&
                        (sourceToFormValue(source).usage[usage] ||
                            Object.values(source.tool_policies || {}).some(
                                (policy) => policy.enabled !== false && policy.usage?.[usage]
                            ))
                    );
                })
                .map((s) => (
                    <Alert key={s.source_id} severity={s.error ? 'error' : 'info'}>
                        {s.source_id}: {s.error || s.state}
                    </Alert>
                ))}
            {!busy && tools.length === 0 && (
                <Typography>
                    {label(
                        assetsOnly ? 'noCatalogTools' : 'noAssignedTools',
                        assetsOnly
                            ? 'No tools discovered yet. Connect a source or check its connection.'
                            : 'No tools are assigned to this section. Choose tools to enable them for this purpose.'
                    )}
                </Typography>
            )}
            {tools.map((tool) => (
                <Card
                    variant="outlined"
                    component="article"
                    aria-label={`${sources.find((source) => source.id === tool.source_id)?.name || tool.source_id} / ${tool.name}`}
                    key={tool.normalized_name}
                >
                    <CardContent>
                        <Stack spacing={1}>
                            <Typography variant="subtitle1">
                                {sources.find((source) => source.id === tool.source_id)?.name || tool.source_id} /{' '}
                                {tool.name}
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                {tool.description}
                            </Typography>
                            {restricted(tool) && (
                                <Typography variant="caption" color="text.secondary">
                                    {label(
                                        'sourceRestriction',
                                        'This tool is excluded by the server allow list. Edit the server to allow it.'
                                    )}
                                </Typography>
                            )}
                            {scopeOnly && !tool.enabled && !restricted(tool) && (
                                <Typography variant="caption" color="warning.main">
                                    {t('mcp.workspace.sharedToolOff', {
                                        defaultValue:
                                            'This tool is disabled in the shared connection. Enable it from the Tool page.',
                                    })}
                                </Typography>
                            )}
                            <Stack direction="row" sx={{ flexWrap: 'wrap' }}>
                                {!scopeOnly && (
                                    <FormControlLabel
                                        control={
                                            <Checkbox
                                                disabled={busy || restricted(tool)}
                                                checked={tool.enabled}
                                                onChange={(e) => void policy(tool, { enabled: e.target.checked })}
                                            />
                                        }
                                        label={label('enabled', 'Enabled')}
                                    />
                                )}
                                {!assetsOnly && (
                                    <FormControlLabel
                                        control={
                                            <Checkbox
                                                disabled={busy || restricted(tool)}
                                                checked={usage === 'client' ? tool.usage.client : tool.usage.gateway}
                                                onChange={(e) =>
                                                    void policy(tool, {
                                                        usage: { ...tool.usage, [usage]: e.target.checked },
                                                    })
                                                }
                                            />
                                        }
                                        label={
                                            usage === 'client'
                                                ? label('ordinaryUsage', 'Expose through MCP')
                                                : label('serverUsage', 'Use as a Server Tool')
                                        }
                                    />
                                )}
                                {showTesting && (
                                    <Button
                                        disabled={
                                            busy ||
                                            !tool.enabled ||
                                            !enabled ||
                                            (tool.source_id === 'advisor' && tool.implementation === 'virtual')
                                        }
                                        onClick={() => {
                                            setTesting(tool);
                                            setArgs('{}');
                                            setResult(null);
                                        }}
                                    >
                                        {label('testTool', 'Test tool')}
                                    </Button>
                                )}
                            </Stack>
                            {tool.source_id === 'advisor' && tool.implementation === 'virtual' && (
                                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                    {label(
                                        'advisorTestHint',
                                        'Advisor requires model conversation context. Configure its consultation model in Server Tool, then verify it with a model request.'
                                    )}
                                </Typography>
                            )}
                            {onConfigureSource && (
                                <Button
                                    size="small"
                                    sx={{ alignSelf: 'flex-start' }}
                                    onClick={() => onConfigureSource(tool.source_id)}
                                >
                                    {t(
                                        tool.source_id === 'advisor' && tool.implementation === 'virtual'
                                            ? 'mcp.workspace.configureAdvisor'
                                            : assetsOnly
                                              ? 'mcp.workspace.configureConnection'
                                              : usage === 'client'
                                                ? 'mcp.workspace.configurePublication'
                                                : 'mcp.workspace.configureExecution',
                                        {
                                            defaultValue:
                                                tool.source_id === 'advisor' && tool.implementation === 'virtual'
                                                    ? 'Configure Advisor model'
                                                    : assetsOnly
                                                      ? 'Configure this connection'
                                                      : usage === 'client'
                                                        ? 'Configure MCP publication'
                                                        : 'Configure execution usage',
                                        }
                                    )}
                                </Button>
                            )}
                            {onRelationships && (
                                <Button
                                    size="small"
                                    sx={{ alignSelf: 'flex-start' }}
                                    onClick={() => onRelationships(tool.source_id)}
                                >
                                    {t('mcp.relationships.view', { defaultValue: 'View usage relationships' })}
                                </Button>
                            )}
                            <Box component="details">
                                <Box component="summary" sx={{ cursor: 'pointer' }}>
                                    {label('schema', 'Parameters and output schema')}
                                </Box>
                                <Box component="pre" sx={{ whiteSpace: 'pre-wrap', fontSize: 12 }}>
                                    {JSON.stringify(
                                        {
                                            input: tool.input_schema,
                                            output: tool.output_schema,
                                            annotations: tool.annotations,
                                        },
                                        null,
                                        2
                                    )}
                                </Box>
                            </Box>
                        </Stack>
                    </CardContent>
                </Card>
            ))}
            <Dialog open={choosing} onClose={() => !busy && setChoosing(false)} maxWidth="md" fullWidth>
                <DialogTitle>
                    {usage === 'client'
                        ? label('chooseOrdinary', 'Choose tools to publish')
                        : label('chooseServer', 'Choose Server Tools')}
                </DialogTitle>
                <DialogContent dividers>
                    <Typography color="text.secondary" sx={{ mb: 2 }}>
                        {label(
                            'chooseToolsHint',
                            'Add a tool to this usage without changing its other usage. Built-in and external sources share the same tool catalog.'
                        )}
                    </Typography>
                    {catalog
                        .flatMap((source) => source.tools || [])
                        .filter(
                            (tool) =>
                                !tool.usage[usage] &&
                                !restricted(tool) &&
                                !(
                                    usage === 'client' &&
                                    tool.source_id === 'advisor' &&
                                    tool.implementation === 'virtual'
                                )
                        )
                        .map((tool) => (
                            <Stack
                                component="article"
                                aria-label={`${sources.find((source) => source.id === tool.source_id)?.name || tool.source_id} / ${tool.name}`}
                                key={tool.normalized_name}
                                direction="row"
                                sx={{ justifyContent: 'space-between', alignItems: 'center', mb: 1 }}
                            >
                                <Typography>
                                    {sources.find((source) => source.id === tool.source_id)?.name || tool.source_id} /{' '}
                                    {tool.name}
                                </Typography>
                                <Button
                                    disabled={busy}
                                    onClick={() => void policy(tool, { usage: { ...tool.usage, [usage]: true } })}
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
            <Dialog open={!!testing} onClose={() => !busy && setTesting(null)} maxWidth="md" fullWidth>
                <DialogTitle>
                    {testing?.source_id} / {testing?.name}
                </DialogTitle>
                <DialogContent dividers>
                    <Stack spacing={2}>
                        {error && <Alert severity="error">{error}</Alert>}
                        <Typography>{testing?.description}</Typography>
                        <Box component="pre" sx={{ whiteSpace: 'pre-wrap', fontSize: 12 }}>
                            {JSON.stringify(testing?.input_schema, null, 2)}
                        </Box>
                        <TextField
                            multiline
                            minRows={5}
                            label={label('jsonArgs', 'Arguments (JSON)')}
                            value={args}
                            onChange={(e) => setArgs(e.target.value)}
                        />
                        {result !== null && (
                            <Box
                                component="pre"
                                data-testid="mcp-test-result"
                                sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}
                            >
                                {JSON.stringify(result, null, 2)}
                            </Box>
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button disabled={busy} onClick={() => setTesting(null)}>
                        {label('close', 'Close')}
                    </Button>
                    <Button disabled={busy} onClick={() => void test()}>
                        {label('run', 'Run')}
                    </Button>
                </DialogActions>
            </Dialog>
        </Stack>
    );
}
