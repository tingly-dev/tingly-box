import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Button,
    CircularProgress,
    Divider,
    FormControlLabel,
    Stack,
    Switch,
    TextField,
    Typography,
} from '@mui/material';
import { ExpandMore } from '@/components/icons';
import MCPToolCard from './MCPToolCard';
import MCPToolTestDialog from './MCPToolTestDialog';
import { toolPolicyPatch, type MCPToolPatch } from './toolPresentation';
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
    onConfigureConnection,
    onConfigureAdvisor,
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
    onConfigureConnection?: () => void;
    onConfigureAdvisor?: () => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [form, setForm] = useState(() => sourceToFormValue(source));
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [testing, setTesting] = useState<MCPCatalogTool | null>(null);
    const advisor = source.transport === 'advisor' || !!source.advisor;
    const assetsOnly = !usageScope;
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
    const updateTool = (tool: MCPCatalogTool, patch: MCPToolPatch) =>
        run(() => saveSource(toolPolicyPatch(source, tool, patch)));
    return (
        <Stack spacing={2.5}>
            <Typography color="text.secondary">
                {label(
                    usageScope === 'client'
                        ? 'clientSourceHint'
                        : usageScope === 'gateway'
                          ? 'serverSourceHint'
                          : 'sourceWorkspaceHint',
                    'Manage the connection, tool definitions and global switches here. MCP publication and Server Tool execution are configured separately.'
                )}
            </Typography>
            {error && <Alert severity="error">{error}</Alert>}
            {assetsOnly && (
                <FormControlLabel
                    control={
                        <Switch
                            disabled={busy}
                            checked={source.enabled !== false}
                            onChange={(e) => void run(() => saveSource({ id: source.id, enabled: e.target.checked }))}
                        />
                    }
                    label={label('sharedAssetEnabled', 'Enable shared connection (affects MCP and Server Tool)')}
                />
            )}
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
            {!assetsOnly && onRelationships && (
                <Button sx={{ alignSelf: 'flex-start' }} onClick={() => onRelationships(source.id!)}>
                    {t('mcp.relationships.view', { defaultValue: 'View usage relationships' })}
                </Button>
            )}
            {onConfigureConnection && (
                <Button sx={{ alignSelf: 'flex-start' }} onClick={onConfigureConnection}>
                    {label('configureInTool', 'Configure connection in Tool')}
                </Button>
            )}
            {assetsOnly && advisor && onConfigureAdvisor && (
                <Button sx={{ alignSelf: 'flex-start' }} onClick={onConfigureAdvisor}>
                    {label('configureAdvisor', 'Configure Advisor model')}
                </Button>
            )}
            {((assetsOnly && !advisor) || (usageScope === 'gateway' && advisor)) && (
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
                            <AdvisorSettings advisorSource={source} onSave={(patch) => run(() => saveSource(patch))} />
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
            )}
            <Box>
                <Typography variant="h6">
                    {usageScope
                        ? label(
                              usageScope === 'client' ? 'clientToolSection' : 'serverToolSection',
                              usageScope === 'client' ? 'Tools available to clients' : 'Tools available to the gateway'
                          )
                        : label('assetToolSection', 'Tool catalog')}
                </Typography>
                {assetsOnly && (
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                        {label(
                            'assetToolHint',
                            'Manage tool definitions, parameters and global enablement here. Publish through MCP or enable model execution from their respective pages.'
                        )}
                    </Typography>
                )}
            </Box>
            {advisor && (
                <Alert severity="info">
                    {label(
                        'advisorContext',
                        'Advisor needs the model conversation. It is only a Server Tool; configure its consultation model in Server Tool and verify it with a model request.'
                    )}
                </Alert>
            )}
            {!route && <CircularProgress size={20} />}
            {route?.state === 'connected' && route.tools.length === 0 && (
                <Alert severity="info">{label('emptyDiscovery', 'The connection returned no tools.')}</Alert>
            )}
            <Stack spacing={1.5}>
                {(route?.tools || []).map((tool) => (
                    <MCPToolCard
                        key={tool.normalized_name}
                        tool={tool}
                        source={source}
                        mode={usageScope || 'asset'}
                        inWorkspace
                        enabled={enabled}
                        busy={busy}
                        onChange={(patch) => void updateTool(tool, patch)}
                        onTest={() => setTesting(tool)}
                    />
                ))}
            </Stack>
            {testing && (
                <MCPToolTestDialog
                    key={testing.normalized_name}
                    tool={
                        route?.tools.find((tool) => tool.normalized_name === testing.normalized_name) || {
                            ...testing,
                            enabled: false,
                        }
                    }
                    enabled={enabled}
                    onClose={() => setTesting(null)}
                />
            )}
            {!advisor && usageScope !== 'gateway' && (
                <Button variant="outlined" disabled={busy} onClick={onConnectClient}>
                    {assetsOnly
                        ? label('publishInMCP', 'Publish through MCP')
                        : label('useInClient', 'Choose a client to use these tools')}
                </Button>
            )}
            <Divider />
            {assetsOnly && !(source.origin === 'builtin' || ['advisor', 'webtools'].includes(source.id!)) && (
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
