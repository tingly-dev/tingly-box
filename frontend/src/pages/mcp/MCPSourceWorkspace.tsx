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
import { isBuiltinSource, isServerToolSource, isServerWebtoolsSource, toolPolicyPatch, type MCPToolPatch } from './toolPresentation';
import WebtoolsSettings from './WebtoolsSettings';
import AdvisorSettings from './AdvisorSettings';
import MCPSourceEditor from './MCPSourceEditor';
import { connectionPatch } from './workspaceState';
import { sourceToFormValue, type MCPCatalogTool, type MCPRouteSource, type MCPSourceConfig } from './types';

export default function MCPSourceWorkspace({
    source,
    route,
    enabled,
    saveSource,
    onDelete,
    onRefresh,
}: {
    source: MCPSourceConfig;
    route?: MCPRouteSource;
    enabled: boolean;
    saveSource: (patch: MCPSourceConfig) => Promise<void>;
    onDelete: () => Promise<void>;
    onRefresh: () => Promise<void>;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [form, setForm] = useState(() => sourceToFormValue(source));
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [testing, setTesting] = useState<MCPCatalogTool | null>(null);
    const advisor = source.transport === 'advisor' || !!source.advisor;
    const webtools = isServerWebtoolsSource(source);
    const serverTool = isServerToolSource(source);
    const [configure, setConfigure] = useState(serverTool || (source.enabled !== false && route?.state !== 'connected'));
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
                {serverTool
                    ? label(
                          'serverToolWorkspaceHint',
                          'Configure the service here, then enable each tool below. Server Tools run inside model requests and are not offered through MCP profiles.'
                      )
                    : label(
                          'sourceWorkspaceHint',
                          'Manage the connection and its tools here. A tool switched off here is off in every profile.'
                      )}
            </Typography>
            {error && <Alert severity="error">{error}</Alert>}
            {!serverTool && (
                <FormControlLabel
                    control={
                        <Switch
                            disabled={busy}
                            checked={source.enabled !== false}
                            onChange={(e) => void run(() => saveSource({ id: source.id, enabled: e.target.checked }))}
                        />
                    }
                    label={label('sharedAssetEnabled', 'Connection on (off turns all its tools off in every profile)')}
                />
            )}
            {!serverTool && (source.enabled === false ? (
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
            ))}
            {(
                <Accordion
                    expanded={configure}
                    onChange={(_, value) => setConfigure(value)}
                    disableGutters
                    elevation={0}
                    sx={{ border: '1px solid', borderColor: 'divider', '&:before': { display: 'none' } }}
                >
                    <AccordionSummary expandIcon={<ExpandMore />}>
                        <Typography style={{ fontWeight: 600 }}>
                            {webtools
                                ? label('webtoolsSettings', 'Web tool settings')
                                : advisor
                                  ? label('consultationSetup', 'Consultation model')
                                  : label('connectionSettings', 'Connection settings')}
                        </Typography>
                    </AccordionSummary>
                    <AccordionDetails>
                        {webtools ? (
                            <WebtoolsSettings source={source} onSave={saveSource} />
                        ) : advisor ? (
                            <AdvisorSettings advisorSource={source} onSave={(patch) => run(() => saveSource(patch))} />
                        ) : (
                            <Stack spacing={2}>
                                <TextField
                                    label={label('connectionName', 'Connection name')}
                                    value={form.name}
                                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                                />
                                <MCPSourceEditor value={form} onChange={setForm} compact hideEnabled />
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
            <Typography variant="h6">{label('assetToolSection', 'Tools')}</Typography>
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
            <Divider />
            {!isBuiltinSource(source) && (
                <Stack spacing={1}>
                    {confirmDelete && (
                        <Alert severity="warning">
                            {label(
                                'deleteHint',
                                'Remove this connection and take its tools out of every profile? Other connections stay in place.'
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
