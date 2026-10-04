import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Button,
    Checkbox,
    CircularProgress,
    Divider,
    FormControlLabel,
    Stack,
    Switch,
    TextField,
    Typography,
} from '@mui/material';
import { ExpandMore } from '@/components/icons';
import { api } from '@/services/api';
import AdvisorSettings from './AdvisorSettings';
import MCPSourceEditor from './MCPSourceEditor';
import { connectionPatch } from './workspaceState';
import { sourceToFormValue, type MCPCatalogTool, type MCPRouteSource, type MCPSourceConfig } from './types';

export default function MCPSourceWorkspace({
    source,
    route,
    enabled,
    saveSource,
    onConnectClient,
    onDelete,
    onRefresh,
    usageScope,
    onRelationships,
}: {
    source: MCPSourceConfig;
    route?: MCPRouteSource;
    enabled: boolean;
    saveSource: (patch: MCPSourceConfig) => Promise<void>;
    onConnectClient: () => void;
    onDelete: () => Promise<void>;
    onRefresh: () => Promise<void>;
    usageScope?: 'client' | 'gateway';
    onRelationships?: (id: string) => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [form, setForm] = useState(() => sourceToFormValue(source));
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [testing, setTesting] = useState<MCPCatalogTool | null>(null);
    const [argumentsText, setArgumentsText] = useState('{}');
    const [testResult, setTestResult] = useState<unknown>(null);
    const advisor = source.transport === 'advisor' || !!source.advisor;
    const [configure, setConfigure] = useState(advisor || (source.enabled !== false && route?.state !== 'connected'));
    const [confirmDelete, setConfirmDelete] = useState(false);
    const run = async (action: () => Promise<void>) => {
        setBusy(true);
        setError('');
        try {
            await action();
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };
    const updateTool = (tool: MCPCatalogTool, patch: { enabled?: boolean; usage?: MCPCatalogTool['usage'] }) =>
        run(async () => {
            await saveSource({
                id: source.id,
                tool_policies: {
                    ...source.tool_policies,
                    [tool.name]: { ...source.tool_policies?.[tool.name], ...patch },
                },
            });
        });
    return (
        <Stack spacing={2.5}>
            <Typography color="text.secondary">
                {label(
                    usageScope === 'client'
                        ? 'clientSourceHint'
                        : usageScope === 'gateway'
                          ? 'serverSourceHint'
                          : 'sourceWorkspaceHint',
                    'Configure this connection and choose how each tool is used, without leaving this workspace.'
                )}
            </Typography>
            {error && <Alert severity="error">{error}</Alert>}
            <FormControlLabel
                control={
                    <Switch
                        disabled={busy}
                        checked={source.enabled !== false}
                        onChange={(e) => void run(() => saveSource({ id: source.id, enabled: e.target.checked }))}
                    />
                }
                label={
                    usageScope
                        ? label('sharedConnectionEnabled', 'Enable shared connection (affects Tool and Server Tool)')
                        : label('connectionEnabled', 'Enable this tool connection')
                }
            />
            {source.enabled === false ? (
                <Alert severity="info">
                    {label('connectionOff', 'This connection is off. Enable it to discover and use its tools.')}
                </Alert>
            ) : (
                route?.state !== 'connected' && (
                    <Alert severity={route?.state === 'error' ? 'warning' : 'info'}>
                        {route?.state === 'error'
                            ? label(
                                  'connectionFailed',
                                  'Connection failed. Check the address and authentication, then retry.'
                              )
                            : label('connectionPending', 'This connection needs to be checked or configured.')}
                        <Button disabled={busy} onClick={() => void run(onRefresh)}>
                            {label('retry', 'Retry connection')}
                        </Button>
                    </Alert>
                )
            )}
            {onRelationships && (
                <Button sx={{ alignSelf: 'flex-start' }} onClick={() => onRelationships(source.id!)}>
                    {t('mcp.relationships.view', { defaultValue: 'View usage relationships' })}
                </Button>
            )}
            <Accordion
                expanded={configure}
                onChange={(_, value) => setConfigure(value)}
                disableGutters
                elevation={0}
                sx={{ border: '1px solid', borderColor: 'divider', '&:before': { display: 'none' } }}
            >
                <AccordionSummary expandIcon={<ExpandMore />}>
                    <Typography style={{ fontWeight: 600 }}>
                        {advisor
                            ? label('consultationSetup', 'Consultation model')
                            : label('connectionSettings', 'Connection settings')}
                    </Typography>
                </AccordionSummary>
                <AccordionDetails>
                    {advisor ? (
                        <AdvisorSettings
                            advisorSource={source}
                            onSave={(patch) => run(() => saveSource(patch))}
                            expanded
                        />
                    ) : (
                        <Stack spacing={2}>
                            <TextField
                                label={label('connectionName', 'Connection name')}
                                value={form.name}
                                onChange={(e) => setForm({ ...form, name: e.target.value })}
                            />
                            <MCPSourceEditor value={form} onChange={setForm} lockId compact hideUsage hideEnabled />
                            <Button
                                variant="contained"
                                disabled={busy}
                                onClick={() =>
                                    void run(async () => {
                                        await saveSource(connectionPatch(form));
                                        setConfigure(false);
                                    })
                                }
                            >
                                {label('saveConnection', 'Save connection')}
                            </Button>
                        </Stack>
                    )}
                </AccordionDetails>
            </Accordion>
            <Box>
                <Typography variant="h6">
                    {usageScope
                        ? label(
                              usageScope === 'client' ? 'clientToolSection' : 'serverToolSection',
                              usageScope === 'client' ? 'Tools available to clients' : 'Tools available to the gateway'
                          )
                        : label('chooseUsage', 'How should these tools be used?')}
                </Typography>
                {!usageScope && (
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                        {label(
                            'usageExplanation',
                            'Ordinary tools are called by your client. Server Tools are executed by the gateway during a model request. A standard tool can be used in both paths.'
                        )}
                    </Typography>
                )}
            </Box>
            {advisor && (
                <Alert severity="info">
                    {label(
                        'advisorContext',
                        'Advisor needs the model conversation. It is only a Server Tool; configure its consultation model above and verify it with a model request.'
                    )}
                </Alert>
            )}
            {!route && <CircularProgress size={20} />}
            {route?.state === 'connected' && route.tools.length === 0 && (
                <Alert severity="info">{label('emptyDiscovery', 'The connection returned no tools.')}</Alert>
            )}
            <Stack spacing={1.5}>
                {(route?.tools || []).map((tool) => {
                    const allowed = source.tools || [];
                    const restricted = allowed.length > 0 && !allowed.includes('*') && !allowed.includes(tool.name);
                    return (
                        <Box
                            key={tool.normalized_name}
                            role="group"
                            aria-label={tool.name}
                            sx={{ p: 2, border: '1px solid', borderColor: 'divider', borderRadius: 1.5 }}
                        >
                            <Stack
                                direction="row"
                                sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1 }}
                            >
                                <Typography style={{ fontWeight: 600 }}>{tool.name}</Typography>
                                <Button
                                    size="small"
                                    disabled={busy || !enabled || !tool.enabled || advisor}
                                    onClick={() => {
                                        setTesting(testing?.normalized_name === tool.normalized_name ? null : tool);
                                        setArgumentsText('{}');
                                        setTestResult(null);
                                    }}
                                >
                                    {label('testTool', 'Test tool')}
                                </Button>
                            </Stack>
                            {tool.description && (
                                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                                    {tool.description}
                                </Typography>
                            )}
                            {restricted && (
                                <Typography variant="caption" color="warning.main">
                                    {label(
                                        'restrictedTool',
                                        'Excluded by the connection allow list. Adjust it in connection settings.'
                                    )}
                                </Typography>
                            )}
                            {usageScope && !tool.enabled && !restricted && (
                                <Typography variant="caption" color="warning.main">
                                    {label(
                                        'sharedToolOff',
                                        'This tool is disabled in the shared connection. Enable it from the connections overview.'
                                    )}
                                </Typography>
                            )}
                            <Stack direction="row" sx={{ flexWrap: 'wrap', mt: 0.5 }}>
                                {!usageScope && (
                                    <FormControlLabel
                                        control={
                                            <Checkbox
                                                disabled={busy || restricted || source.enabled === false}
                                                checked={tool.enabled}
                                                onChange={(e) => void updateTool(tool, { enabled: e.target.checked })}
                                            />
                                        }
                                        label={label('toolEnabled', 'Enabled')}
                                    />
                                )}
                                {usageScope !== 'gateway' && (
                                    <FormControlLabel
                                        control={
                                            <Checkbox
                                                disabled={busy || restricted || advisor}
                                                checked={tool.usage.client && !advisor}
                                                onChange={(e) =>
                                                    void updateTool(tool, {
                                                        usage: { ...tool.usage, client: e.target.checked },
                                                    })
                                                }
                                            />
                                        }
                                        label={label('ordinaryTools', 'Ordinary tools')}
                                    />
                                )}
                                {usageScope !== 'client' && (
                                    <FormControlLabel
                                        control={
                                            <Checkbox
                                                disabled={busy || restricted}
                                                checked={tool.usage.gateway}
                                                onChange={(e) =>
                                                    void updateTool(tool, {
                                                        usage: { ...tool.usage, gateway: e.target.checked },
                                                    })
                                                }
                                            />
                                        }
                                        label={label('serverTools', 'Server Tools')}
                                    />
                                )}
                            </Stack>
                            {testing?.normalized_name === tool.normalized_name && (
                                <Stack spacing={1.5} sx={{ mt: 1 }}>
                                    <TextField
                                        multiline
                                        minRows={2}
                                        label={label('argumentsJSON', 'Arguments (JSON)')}
                                        value={argumentsText}
                                        onChange={(e) => setArgumentsText(e.target.value)}
                                    />
                                    <Button
                                        disabled={busy || !enabled || !tool.enabled}
                                        onClick={() =>
                                            void run(async () => {
                                                const args: unknown = JSON.parse(argumentsText);
                                                if (!args || typeof args !== 'object' || Array.isArray(args))
                                                    throw new Error(
                                                        label('objectArgs', 'Arguments must be a JSON object.')
                                                    );
                                                setTestResult(null);
                                                const response = await api.callMCPTool({
                                                    source_id: source.id!,
                                                    tool_name: tool.name,
                                                    arguments: args as Record<string, unknown>,
                                                });
                                                setTestResult(response);
                                                if (!response.success)
                                                    throw new Error(response.error || 'Tool test failed');
                                            })
                                        }
                                    >
                                        {label('runTest', 'Run test')}
                                    </Button>
                                    {testResult !== null && (
                                        <Box
                                            component="pre"
                                            data-testid="mcp-workspace-tool-result"
                                            sx={{
                                                m: 0,
                                                whiteSpace: 'pre-wrap',
                                                overflowWrap: 'anywhere',
                                                fontSize: 12,
                                            }}
                                        >
                                            {JSON.stringify(testResult, null, 2)}
                                        </Box>
                                    )}
                                </Stack>
                            )}
                            <Box component="details" sx={{ mt: 1 }}>
                                <Typography component="summary" variant="caption" sx={{ cursor: 'pointer' }}>
                                    {label('toolParameters', 'Tool parameters')}
                                </Typography>
                                <Box
                                    component="pre"
                                    sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontSize: 12 }}
                                >
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
                        </Box>
                    );
                })}
            </Stack>
            {!advisor && usageScope !== 'gateway' && (
                <Button variant="outlined" disabled={busy} onClick={onConnectClient}>
                    {label('useInClient', 'Choose a client to use these tools')}
                </Button>
            )}
            <Divider />
            {!(source.origin === 'builtin' || ['advisor', 'webtools'].includes(source.id!)) && (
                <Stack spacing={1}>
                    {confirmDelete && (
                        <Alert severity="warning">
                            {label(
                                'deleteHint',
                                'Remove this connection and its grants from client profiles? Other connections stay in place.'
                            )}
                        </Alert>
                    )}
                    <Button
                        color="error"
                        disabled={busy}
                        onClick={() => {
                            if (!confirmDelete) setConfirmDelete(true);
                            else void run(onDelete);
                        }}
                    >
                        {label(
                            confirmDelete ? 'confirmRemove' : 'removeConnection',
                            confirmDelete ? 'Confirm removal' : 'Remove connection'
                        )}
                    </Button>
                </Stack>
            )}
        </Stack>
    );
}
