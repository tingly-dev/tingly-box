import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Autocomplete,
    Box,
    Button,
    Checkbox,
    Divider,
    FormControlLabel,
    Stack,
    Switch,
    TextField,
    Typography,
} from '@mui/material';
import { ExpandMore } from '@/components/icons';
import { api } from '@/services/api';
import AgentInstallCard from './AgentInstallCard';
import { nextMCPId, toggleClientSource } from './workspaceState';
import type { MCPClientProfile, MCPRoutingSnapshot, MCPRuntimeConfig } from './types';

export default function MCPClientWorkspace({
    profile,
    config,
    routing,
    enabled,
    grantSource,
    editing = false,
    legacy = false,
    onSave,
    onDelete,
}: {
    profile?: MCPClientProfile;
    config: MCPRuntimeConfig;
    routing: MCPRoutingSnapshot | null;
    enabled: boolean;
    grantSource?: string;
    editing?: boolean;
    legacy?: boolean;
    onSave: (profile: MCPClientProfile) => Promise<void>;
    onDelete: () => Promise<void>;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const existingIds = (config.client_profiles || []).map((p) => p.id);
    const [draft, setDraft] = useState<MCPClientProfile>(() => {
        const initial = profile || {
            id: nextMCPId('Codex', existingIds, 'client'),
            name: 'Codex',
            enabled: true,
            sources: [],
            tools: ['*'],
        };
        return grantSource
            ? toggleClientSource(
                  initial,
                  grantSource,
                  true,
                  (config.sources || []).map((s) => s.id!)
              )
            : initial;
    });
    const [open, setOpen] = useState(!profile || editing || !!grantSource);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [probe, setProbe] = useState<{ success: boolean; tools: string[]; error?: string } | null>(null);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const [customId, setCustomId] = useState(false);
    const sources = (config.sources || []).filter((source) => source.transport !== 'advisor' && !source.advisor);
    const selected = (draft.sources || []).includes('*')
        ? sources
        : sources.filter((source) => draft.sources?.includes(source.id!));
    const tools = (routing?.sources || [])
        .filter((source) => selected.some((s) => s.id === source.id))
        .flatMap((source) => source.tools.filter((tool) => tool.usage.client));
    const toolOptions = [
        ...new Set([
            ...tools.map((tool) => tool.normalized_name),
            ...(draft.tools || []).filter((name) => name !== '*'),
        ]),
    ];
    const allTools = draft.tools?.includes('*') || false;
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
    const clientRoute = routing?.clients.find((client) => client.id === profile?.id);
    const toolCount =
        enabled && clientRoute?.enabled
            ? clientRoute.sources.reduce((total, source) => total + source.tools.length, 0)
            : 0;
    return (
        <Stack spacing={2.5}>
            <Typography color="text.secondary">
                {label(
                    'clientWorkspaceHint',
                    'Choose which MCP-published tools this client can use, then copy its connection command. Model execution is configured in Server Tool.'
                )}
            </Typography>
            {error && <Alert severity="error">{error}</Alert>}
            <Accordion
                expanded={open}
                onChange={(_, value) => setOpen(value)}
                disableGutters
                elevation={0}
                sx={{ border: '1px solid', borderColor: 'divider', '&:before': { display: 'none' } }}
            >
                <AccordionSummary expandIcon={<ExpandMore />}>
                    <Box>
                        <Typography style={{ fontWeight: 600 }}>
                            {label('chooseClientTools', 'Which tools can this client use?')}
                        </Typography>
                        {profile && (
                            <Typography variant="caption" color="text.secondary">
                                {toolCount} {label('ordinaryToolCount', 'tools available through MCP')}
                            </Typography>
                        )}
                    </Box>
                </AccordionSummary>
                <AccordionDetails>
                    <Stack spacing={2}>
                        <TextField
                            label={label('clientName', 'Client name')}
                            disabled={busy}
                            value={draft.name || ''}
                            onChange={(e) =>
                                setDraft({
                                    ...draft,
                                    name: e.target.value,
                                    id:
                                        !profile && !customId
                                            ? nextMCPId(e.target.value, existingIds, 'client')
                                            : draft.id,
                                })
                            }
                        />
                        <Typography variant="body2" color="text.secondary">
                            {label(
                                'selectConnections',
                                'Select the connections this client can access. An empty selection provides no tools.'
                            )}
                        </Typography>
                        {sources.length === 0 && (
                            <Alert severity="info">
                                {label('connectFirst', 'Connect a tool service first, then return to choose it here.')}
                            </Alert>
                        )}
                        <Stack spacing={1}>
                            {sources.map((source) => {
                                const route = routing?.sources.find((s) => s.id === source.id);
                                const count =
                                    route?.tools.filter((tool) => tool.enabled && tool.usage.client).length || 0;
                                return (
                                    <Box
                                        key={source.id}
                                        sx={{
                                            px: 1.5,
                                            py: 0.75,
                                            border: '1px solid',
                                            borderColor: 'divider',
                                            borderRadius: 1,
                                        }}
                                    >
                                        <FormControlLabel
                                            sx={{ m: 0, width: '100%' }}
                                            control={
                                                <Checkbox
                                                    disabled={busy}
                                                    checked={
                                                        draft.sources?.includes('*') ||
                                                        draft.sources?.includes(source.id!) ||
                                                        false
                                                    }
                                                    onChange={(e) =>
                                                        setDraft(
                                                            toggleClientSource(
                                                                draft,
                                                                source.id!,
                                                                e.target.checked,
                                                                sources.map((s) => s.id!)
                                                            )
                                                        )
                                                    }
                                                />
                                            }
                                            label={
                                                <Box>
                                                    <Typography variant="body2" style={{ fontWeight: 600 }}>
                                                        {source.name || source.id}
                                                    </Typography>
                                                    <Typography variant="caption" color="text.secondary">
                                                        {source.enabled === false
                                                            ? label('connectionOffShort', 'Connection disabled')
                                                            : `${count} ${label('ordinaryToolCount', 'tools available through MCP')}`}
                                                    </Typography>
                                                </Box>
                                            }
                                        />
                                    </Box>
                                );
                            })}
                        </Stack>
                        <FormControlLabel
                            control={
                                <Checkbox
                                    disabled={busy}
                                    checked={allTools}
                                    onChange={(e) => setDraft({ ...draft, tools: e.target.checked ? ['*'] : [] })}
                                />
                            }
                            label={label(
                                'allSelectedTools',
                                'Allow all MCP-published tools from the selected connections'
                            )}
                        />
                        {!allTools && (
                            <Autocomplete
                                multiple
                                disabled={busy}
                                options={toolOptions}
                                value={draft.tools || []}
                                onChange={(_, values) => setDraft({ ...draft, tools: values })}
                                getOptionLabel={(name) => {
                                    const tool = tools.find((t) => t.normalized_name === name);
                                    const source = sources.find((s) => s.id === tool?.source_id);
                                    return tool ? `${source?.name || tool.source_id} / ${tool.name}` : name;
                                }}
                                renderInput={(params) => (
                                    <TextField
                                        {...params}
                                        label={label('pickIndividualTools', 'Choose individual tools')}
                                        helperText={label(
                                            'emptyToolGrants',
                                            'Leaving this empty provides no tool access.'
                                        )}
                                    />
                                )}
                            />
                        )}
                        <FormControlLabel
                            control={
                                <Switch
                                    disabled={busy}
                                    checked={draft.enabled !== false}
                                    onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
                                />
                            }
                            label={label('clientEnabled', 'Enable this client connection')}
                        />
                        <Box component="details">
                            <Typography component="summary" variant="body2" sx={{ cursor: 'pointer' }}>
                                {label('advancedIdentifier', 'Advanced: connection identifier')}
                            </Typography>
                            <TextField
                                sx={{ mt: 1 }}
                                fullWidth
                                label={label('clientId', 'Connection identifier')}
                                value={draft.id}
                                disabled={busy || !!profile}
                                onChange={(e) => {
                                    setCustomId(true);
                                    setDraft({ ...draft, id: e.target.value });
                                }}
                                helperText={label(
                                    'identifierHint',
                                    'Generated automatically. Existing identifiers stay fixed so saved commands keep working.'
                                )}
                            />
                        </Box>
                        {!profile && !config.client_profiles_configured && !(config.client_profiles || []).length && (
                            <Alert severity="info">
                                {label(
                                    'legacySwitch',
                                    'Saving the first client configuration replaces the shared default endpoint. Existing clients must use the new command below.'
                                )}
                            </Alert>
                        )}
                        <Button
                            variant="contained"
                            disabled={busy || !draft.name?.trim() || !draft.id.trim()}
                            onClick={() =>
                                void run(async () => {
                                    await onSave(draft);
                                    setOpen(false);
                                    setProbe(null);
                                })
                            }
                        >
                            {label(
                                profile ? 'savePermissions' : 'saveAndConnect',
                                profile ? 'Save access' : 'Save and get connection command'
                            )}
                        </Button>
                    </Stack>
                </AccordionDetails>
            </Accordion>
            {profile && (
                <>
                    {!enabled && (
                        <Alert severity="info">
                            {label(
                                'executionOff',
                                'MCP execution is off. Your configuration is saved; enable execution in the workspace to use it.'
                            )}
                        </Alert>
                    )}
                    {profile.enabled === false && (
                        <Alert severity="info">
                            {label(
                                'clientOff',
                                'This client connection is disabled. Enable it in its access settings before connecting.'
                            )}
                        </Alert>
                    )}
                    {enabled && clientRoute?.enabled && toolCount === 0 && (
                        <Alert severity="info">
                            {label(
                                'noClientTools',
                                'No tools are available to this client. Select connections above and ensure their tools are published through MCP.'
                            )}
                        </Alert>
                    )}
                    <AgentInstallCard
                        defaultRuntime={
                            /codex/i.test(profile.name)
                                ? 'codex'
                                : /opencode/i.test(profile.name)
                                  ? 'opencode'
                                  : 'claude'
                        }
                        clientId={profile.id}
                        heading={label('copyClientCommand', 'Copy this command into your client setup')}
                    />
                    <Button
                        variant="outlined"
                        disabled={busy || !enabled || profile.enabled === false}
                        onClick={() =>
                            void run(async () => {
                                setProbe(null);
                                const result = await api.probeMCPClient(profile.id, {});
                                setProbe({ success: result.success, tools: result.tools || [], error: result.error });
                            })
                        }
                    >
                        {label('verifyClient', 'Verify client connection')}
                    </Button>
                    {probe && (
                        <Alert severity={probe.success ? 'success' : 'error'} data-testid="mcp-workspace-client-probe">
                            {probe.success
                                ? `${label('clientProbePassed', 'Actual gateway initialization and tool discovery passed.')} ${probe.tools.length} ${label('toolCount', 'tools')}`
                                : probe.error}
                        </Alert>
                    )}
                    {!legacy && (
                        <>
                            <Divider />
                            {confirmDelete && (
                                <Alert severity="warning">
                                    {label(
                                        'deleteClientHint',
                                        'This command will stop working after removal. Other client connections keep their access.'
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
                                    confirmDelete ? 'confirmRemoveClient' : 'removeClient',
                                    confirmDelete ? 'Confirm client removal' : 'Remove client connection'
                                )}
                            </Button>
                        </>
                    )}
                </>
            )}
        </Stack>
    );
}
