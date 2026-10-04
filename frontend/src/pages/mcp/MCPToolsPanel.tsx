import { useCallback, useEffect, useState } from 'react';
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
import type { MCPCatalogTool, MCPSourceConfig, MCPSourceStatus } from './types';

export default function MCPToolsPanel({
    sources,
    enabled,
    saveSource,
}: {
    sources: MCPSourceConfig[];
    enabled: boolean;
    saveSource: (patch: MCPSourceConfig) => Promise<void>;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.center.${key}`, { defaultValue: fallback });
    const [catalog, setCatalog] = useState<MCPSourceStatus[]>([]);
    const [busy, setBusy] = useState(true);
    const [error, setError] = useState('');
    const [testing, setTesting] = useState<MCPCatalogTool | null>(null);
    const [args, setArgs] = useState('{}');
    const [result, setResult] = useState<unknown>(null);
    const discover = useCallback(
        () =>
            api
                .getMCPCatalog()
                .then((response) => {
                    if (!response?.success) throw new Error(response?.error || 'Discovery failed');
                    setCatalog(response.sources || []);
                })
                .catch((e) => setError(e instanceof Error ? e.message : String(e)))
                .finally(() => setBusy(false)),
        []
    );
    useEffect(() => {
        void discover();
    }, [discover]);
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
    const tools = catalog.flatMap((s) => s.tools || []);
    return (
        <Stack spacing={2}>
            <Stack direction="row" sx={{ justifyContent: 'space-between' }}>
                <Typography color="text.secondary">
                    {label('toolsHint', 'Discover schemas, choose usage and test tools through the gateway runtime.')}
                </Typography>
                <Button disabled={busy} onClick={() => void refresh()}>
                    {label(busy ? 'discovering' : 'discover', busy ? 'Discovering…' : 'Discover tools')}
                </Button>
            </Stack>
            {error && <Alert severity="error">{error}</Alert>}
            {catalog
                .filter((s) => s.state !== 'connected')
                .map((s) => (
                    <Alert key={s.source_id} severity={s.error ? 'error' : 'info'}>
                        {s.source_id}: {s.error || s.state}
                    </Alert>
                ))}
            {!busy && tools.length === 0 && (
                <Typography>
                    {label('noTools', 'No tools discovered. Check server configuration and connection status.')}
                </Typography>
            )}
            {tools.map((tool) => (
                <Card variant="outlined" key={tool.normalized_name}>
                    <CardContent>
                        <Stack spacing={1}>
                            <Typography variant="subtitle1">
                                {tool.source_id} / {tool.name}
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
                            <Stack direction="row" sx={{ flexWrap: 'wrap' }}>
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
                                <FormControlLabel
                                    control={
                                        <Checkbox
                                            disabled={busy}
                                            checked={tool.usage.client}
                                            onChange={(e) =>
                                                void policy(tool, {
                                                    usage: { ...tool.usage, client: e.target.checked },
                                                })
                                            }
                                        />
                                    }
                                    label={label('clientUsage', 'MCP clients')}
                                />
                                <FormControlLabel
                                    control={
                                        <Checkbox
                                            disabled={busy}
                                            checked={tool.usage.gateway}
                                            onChange={(e) =>
                                                void policy(tool, {
                                                    usage: { ...tool.usage, gateway: e.target.checked },
                                                })
                                            }
                                        />
                                    }
                                    label={label('gatewayUsage', 'Gateway model calls')}
                                />
                                <Button
                                    disabled={busy || !tool.enabled || !enabled}
                                    onClick={() => {
                                        setTesting(tool);
                                        setArgs('{}');
                                        setResult(null);
                                    }}
                                >
                                    {label('testTool', 'Test tool')}
                                </Button>
                            </Stack>
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
