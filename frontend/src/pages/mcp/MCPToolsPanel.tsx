import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Button, Card, Chip, CircularProgress, Stack, Typography } from '@mui/material';
import MCPToolList from './MCPToolList';
import MCPToolTestDialog from './MCPToolTestDialog';
import { connectionSummary, isBuiltinSource, isServerToolSource, isServerWebtoolsSource, toolPolicyPatch, toolsPolicyPatch, type MCPToolPatch } from './toolPresentation';
import type { MCPCatalogTool, MCPRouteSource, MCPSourceConfig } from './types';

// Every connection and Server Tool in one list. The applied routing snapshot is
// the source of tool state, so what is shown is what the gateway will do.
export default function MCPToolsPanel({
    sources,
    routes,
    loading,
    enabled,
    saveSource,
    onConfigureSource,
}: {
    sources: MCPSourceConfig[];
    routes: MCPRouteSource[];
    loading: boolean;
    enabled: boolean;
    saveSource: (patch: MCPSourceConfig) => Promise<void>;
    onConfigureSource: (id: string) => void;
}) {
    const { t } = useTranslation();
    const workspace = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [testing, setTesting] = useState<MCPCatalogTool | null>(null);
    const save = async (patch: MCPSourceConfig) => {
        setBusy(true);
        setError('');
        try {
            await saveSource(patch);
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };
    const policy = (tool: MCPCatalogTool, patch: MCPToolPatch) => {
        const source = sources.find((s) => s.id === tool.source_id);
        return source ? save(toolPolicyPatch(source, tool, patch)) : undefined;
    };
    return (
        <Stack spacing={2}>
            {error && <Alert severity="error">{error}</Alert>}
            {loading && <CircularProgress size={20} />}
            {!loading && sources.length === 0 && (
                <Typography color="text.secondary">
                    {workspace(
                        'emptyConnectionsHint',
                        'Paste a tool service URL or enter a local command. We will discover its tools for you.'
                    )}
                </Typography>
            )}
            {sources.map((source) => {
                const route = routes.find((r) => r.id === source.id);
                const tools = route?.tools || [];
                const advisor = source.transport === 'advisor' || !!source.advisor;
                const serverTool = isServerToolSource(source);
                const webtools = isServerWebtoolsSource(source);
                const summary = connectionSummary(source);
                const failed = route?.state === 'error' || route?.state === 'unconfigured';
                const builtin = isBuiltinSource(source) && !webtools && !advisor;
                // Neutral unless the connection needs the user (amber) or failed (red).
                const status: { text: string; tone: 'neutral' | 'warning' | 'error' } =
                    source.enabled === false
                        ? { text: workspace('off', 'Off'), tone: 'neutral' }
                        : !route || loading
                          ? { text: workspace('checking', 'Checking…'), tone: 'neutral' }
                          : route.state === 'connected'
                            ? { text: workspace('connected', 'Connected'), tone: 'neutral' }
                            : route.state === 'error'
                              ? { text: workspace('failed', 'Failed'), tone: 'error' }
                              : { text: workspace('needsAttention', 'Needs attention'), tone: 'warning' };
                const toneColor = { neutral: 'text.secondary', warning: 'warning.main', error: 'error.main' }[status.tone];
                return (
                    <Card
                        component="article"
                        variant="outlined"
                        aria-label={source.name || source.id}
                        key={source.id}
                        sx={{ minWidth: 0, borderRadius: 1.5, overflow: 'hidden' }}
                    >
                        <Stack
                            direction="row"
                            sx={{ alignItems: 'center', gap: 1.25, px: 2, py: 1.75, flexWrap: 'wrap' }}
                        >
                            <Typography component="h2" sx={{ fontWeight: 600, fontSize: 14, overflowWrap: 'anywhere' }}>
                                {source.name || source.id}
                            </Typography>
                            <Typography variant="caption" color="text.disabled">
                                {serverTool
                                    ? workspace('serverToolLabel', 'Server Tool')
                                    : t(`mcp.relationships.${route?.origin || source.origin || 'external'}`, {
                                          defaultValue: route?.origin || source.origin || 'external',
                                      })}
                            </Typography>
                            <Box sx={{ flex: 1 }} />
                            <Stack direction="row" sx={{ alignItems: 'center', gap: 0.75 }}>
                                <Box
                                    aria-hidden
                                    sx={{
                                        width: 7,
                                        height: 7,
                                        borderRadius: '50%',
                                        bgcolor: status.tone === 'neutral' ? 'text.disabled' : toneColor,
                                    }}
                                />
                                <Typography
                                    variant="caption"
                                    sx={{ color: toneColor }}
                                >
                                    {status.text}
                                </Typography>
                            </Stack>
                            <Button
                                size="small"
                                variant={failed ? 'contained' : 'outlined'}
                                color={failed ? 'primary' : 'inherit'}
                                sx={failed ? undefined : { color: 'text.secondary', borderColor: 'divider' }}
                                onClick={() => onConfigureSource(source.id!)}
                            >
                                {webtools
                                    ? workspace('webtoolsSettings', 'Web tool settings')
                                    : failed
                                      ? workspace('fixConnection', 'Fix connection')
                                      : advisor
                                        ? workspace('configureAdvisor', 'Configure Advisor model')
                                        : workspace('connectionSettings', 'Settings')}
                            </Button>
                        </Stack>
                        <Stack sx={{ px: 2, pb: 1.5, gap: 1 }}>
                            {builtin ? (
                                <Typography variant="caption" color="text.secondary">
                                    {workspace('builtinRuns', 'Runs inside Tingly Box.')}
                                </Typography>
                            ) : (
                                <Stack direction="row" sx={{ alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                                    <Typography
                                        color="text.secondary"
                                        sx={{ fontFamily: 'monospace', fontSize: 12, wordBreak: 'break-all' }}
                                    >
                                        {summary.target || workspace('notSet', 'Not set')}
                                    </Typography>
                                    {summary.details.map(({ key, count }) => (
                                        <Chip
                                            key={key}
                                            size="small"
                                            variant="outlined"
                                            sx={{ fontSize: 11.5, height: 22 }}
                                            label={`${workspace(`detail_${key}`, key)}${key === 'proxy' ? '' : ` · ${count}`}`}
                                        />
                                    ))}
                                    {summary.missing.map((key) => (
                                        <Chip
                                            key={key}
                                            size="small"
                                            color="warning"
                                            variant="outlined"
                                            clickable
                                            sx={{ fontSize: 11.5, height: 22 }}
                                            onClick={() => onConfigureSource(source.id!)}
                                            label={workspace(`missing_${key}`, `Needs ${key}`)}
                                        />
                                    ))}
                                </Stack>
                            )}
                            {route && route.state !== 'connected' && !!route.error && (
                                <Typography variant="caption" color={failed ? 'error.main' : 'text.secondary'}>
                                    {route.error}
                                </Typography>
                            )}
                            {webtools && (
                                <Typography variant="caption" color="text.secondary">
                                    {workspace('webtoolsContext', 'Configure Search with a Serper API key and enable Search / Fetch separately. Ready tools replace native web tools on supported model routes. The MCP version has independent settings.')}
                                </Typography>
                            )}
                            {advisor && (
                                <Typography variant="caption" color="text.secondary">
                                    {workspace(
                                        'advisorContext',
                                        'Advisor is run by the gateway during model requests, so it has a switch and a consultation model but is not offered through profiles.'
                                    )}
                                </Typography>
                            )}
                        </Stack>
                        {tools.length > 0 && (
                            <MCPToolList
                                tools={tools}
                                source={source}
                                enabled={enabled}
                                busy={busy || loading}
                                onChange={(tool, patch) => void policy(tool, patch)}
                                onBulk={(list, on) => void save(toolsPolicyPatch(source, list, on))}
                                onTest={setTesting}
                            />
                        )}
                        {!loading && source.enabled !== false && route?.state === 'connected' && tools.length === 0 && (
                            <Typography variant="body2" color="text.secondary" sx={{ px: 2, pb: 1.75 }}>
                                {workspace('emptyDiscovery', 'The connection returned no tools.')}
                            </Typography>
                        )}
                    </Card>
                );
            })}
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
