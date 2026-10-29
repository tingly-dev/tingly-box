import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
    Alert,
    Box,
    Button,
    Checkbox,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    IconButton,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { Add, Close, Delete, Edit, Refresh } from '@/components/icons';
import { PageLayout } from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import ConfirmDialog from '@/components/ConfirmDialog';
import AgentAccessSection, { useAccessExpanded } from '@/pages/scenario/components/AgentAccessSection';
import { agentPageHeaderSx } from '@/pages/scenario/components/ScenarioCardHeader';
import { getApiBaseUrl } from '@/utils/protocol';
import { api } from '@/services/api';
import ProfileConnect from './ProfileConnect';
import GitHubScope from './GitHubScope';
import MCPWorkspaceAlerts from './MCPWorkspaceAlerts';
import { useMCPWorkspace } from './useMCPWorkspace';
import {
    addProfileTools,
    defaultProfile,
    isDefaultProfile,
    profileCandidates,
    profileRows,
    profileServerName,
    removeProfileTool,
    setProfileToolOn,
} from './profileState';
import { nextMCPId } from './workspaceState';
import { publishMCPProfiles } from './profileStore';
import { MCP_DEFAULT_PROFILE_ID, type MCPProfile } from './types';

// Keyed by profile so each one remembers its own folded state.
function ProfileAccess({ profile, url }: { profile: MCPProfile; url: string }) {
    const access = useAccessExpanded(`mcp-profile-${profile.id}`, true);
    return (
        <AgentAccessSection expanded={access.expanded} onToggle={access.toggle} url={url}>
            <Box sx={{ px: 2 }}>
                <ProfileConnect profileId={profile.id} serverName={profileServerName(profile)} />
            </Box>
        </AgentAccessSection>
    );
}

// Profiles are entry points. Default holds every tool; a custom profile holds
// only the tools added to it. Inside a profile each tool has its own switch, on
// top of the tool's global switch on the Tools page.
export default function MCPProfilesPage() {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const { profileId } = useParams();
    const navigate = useNavigate();
    const ws = useMCPWorkspace();
    const { config, routing, enabled, busy, checking } = ws;
    const saved = config.profiles || [];
    const profiles: MCPProfile[] = useMemo(
        () => [saved.find((p) => p.id === MCP_DEFAULT_PROFILE_ID) || defaultProfile(), ...saved.filter((p) => p.id !== MCP_DEFAULT_PROFILE_ID)],
        [saved]
    );
    const creating = profileId === 'new';
    const profile = profiles.find((p) => p.id === profileId) || profiles[0];
    const select = (id: string) => navigate(`/mcp/profile/${id}`, { replace: true });
    // The sidebar lists these, so keep it in step with what was loaded or saved.
    useEffect(() => {
        if (!ws.loading) publishMCPProfiles(config.profiles);
    }, [ws.loading, config.profiles]);
    const [adding, setAdding] = useState(false);
    const [picked, setPicked] = useState<string[]>([]);
    const [renaming, setRenaming] = useState<string | null>(null);
    const [apiBase, setApiBase] = useState('');
    useEffect(() => {
        void getApiBaseUrl().then(setApiBase);
    }, []);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const [probe, setProbe] = useState<{ success: boolean; tools: string[]; error?: string } | null>(null);
    const rows = profileRows(profile, routing);
    const route = routing?.profiles.find((p) => p.id === profile.id);
    const exposed = enabled && route?.enabled ? route.sources.reduce((sum, source) => sum + source.tools.length, 0) : 0;
    const save = (next: MCPProfile) =>
        ws.run(async () => {
            await ws.acceptConfig(await api.saveMCPProfile(next));
        });
    const create = () =>
        ws.run(async () => {
            const id = nextMCPId(
                'prof',
                profiles.map((p) => p.id)
            );
            await ws.acceptConfig(await api.saveMCPProfile({ id, name: label('newProfile', 'New profile'), tools: [] }));
            select(id);
        });
    const createdOnce = useRef(false);
    useEffect(() => {
        // Re-armed once the new profile's page replaces /new, so "New profile" works again.
        if (!creating) createdOnce.current = false;
        else if (!ws.loading && !createdOnce.current) {
            createdOnce.current = true;
            void create();
        }
    });
    const available = profileCandidates(routing).filter(
        ({ tool }) => !(profile.tools || []).some((entry) => entry.name === tool.normalized_name)
    );
    const groups = [...new Set(available.map(({ sourceName }) => sourceName))];
    const custom = !isDefaultProfile(profile);
    const endpointUrl = `${apiBase}/api/v1/mcp/${profile.id}`;
    const statusText = !routing
        ? label('checking', 'Checking…')
        : [
              custom ? profile.id : label('defaultCaption', 'default'),
              profile.enabled === false ? label('off', 'Off') : label('profileOnStatus', 'On'),
              `${label('profileToolsAvailable', 'Tools available')}: ${exposed}`,
          ].join(' · ');
    return (
        <PageLayout loading={ws.loading}>
            <Stack spacing={2}>
                <PageHeader
                    // The profile's name leads; rename and delete act on it, so they sit
                    // beside it (the same shape as the Claude Code profile page).
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                            <Box component="span" sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                                {custom ? profile.name || profile.id : label('defaultProfileName', 'MCP Integration')}
                            </Box>
                            {custom && (
                                <>
                                    <Tooltip title={label('renameProfile', 'Rename profile')}>
                                        <IconButton
                                            size="small"
                                            aria-label={label('renameProfile', 'Rename profile')}
                                            disabled={busy}
                                            onClick={() => setRenaming(profile.name)}
                                        >
                                            <Edit fontSize="small" sx={{ color: 'text.secondary' }} />
                                        </IconButton>
                                    </Tooltip>
                                    <Tooltip title={label('removeProfile', 'Remove profile')}>
                                        <IconButton
                                            size="small"
                                            color="error"
                                            aria-label={label('removeProfile', 'Remove profile')}
                                            disabled={busy}
                                            onClick={() => setConfirmDelete(true)}
                                        >
                                            <Delete fontSize="small" />
                                        </IconButton>
                                    </Tooltip>
                                </>
                            )}
                        </Box>
                    }
                    meta={
                        <Typography variant="body2" color="text.secondary" data-testid="mcp-profile-status">
                            {statusText}
                        </Typography>
                    }
                    // Looks-in and switches first, the key action filled and rightmost.
                    actions={
                        <>
                            <FormControlLabel
                                sx={{ m: 0 }}
                                control={
                                    <Switch
                                        size="small"
                                        disabled={busy}
                                        checked={profile.enabled !== false}
                                        onChange={(e) => void save({ ...profile, enabled: e.target.checked })}
                                    />
                                }
                                label={label('profileOn', 'Profile on')}
                            />
                            <IconButton
                                aria-label={label('refresh', 'Refresh connections')}
                                disabled={busy || checking}
                                onClick={() => void ws.run(ws.reload)}
                            >
                                <Refresh />
                            </IconButton>
                            <Button
                                variant="outlined"
                                size="small"
                                disabled={busy || !enabled || profile.enabled === false}
                                onClick={() =>
                                    void ws.run(async () => {
                                        setProbe(null);
                                        const result = await api.probeMCPProfile(profile.id, {});
                                        setProbe({ success: result.success, tools: result.tools || [], error: result.error });
                                    })
                                }
                            >
                                {label('verifyProfile', 'Verify this profile')}
                            </Button>
                            {custom && (
                                <Button
                                    variant="contained"
                                    size="small"
                                    startIcon={<Add />}
                                    disabled={busy || available.length === 0}
                                    onClick={() => {
                                        setPicked([]);
                                        setAdding(true);
                                    }}
                                >
                                    {label('addProfileTools', 'Add tools')}
                                </Button>
                            )}
                        </>
                    }
                    sx={agentPageHeaderSx()}
                />
                <MCPWorkspaceAlerts ws={ws} />
                {probe && (
                    <Alert severity={probe.success ? 'success' : 'error'} data-testid="mcp-profile-probe">
                        {probe.success
                            ? `${label('profileProbePassed', 'Actual gateway initialization and tool discovery passed.')} ${probe.tools.length} ${label('toolCount', 'tools')}`
                            : probe.error}
                    </Alert>
                )}
                <ProfileAccess key={profile.id} profile={profile} url={endpointUrl} />
                {custom && (config.sources || []).some((source) => source.preset === 'github') && (
                    <GitHubScope profile={profile} busy={busy} onSave={(next) => void save(next)} />
                )}
                <Box component="section" aria-label={label('profileTools', 'Tools in this profile')}>
                    <Typography variant="h6" sx={{ fontWeight: 600, mb: 0.5 }}>
                        {label('profileTools', 'Tools in this profile')}
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                        {custom
                            ? label(
                                  'customHint',
                                  'Only the tools you add here are offered through this profile; new tools are never added automatically. Switch a tool off to keep it listed but unavailable.'
                              )
                            : label(
                                  'defaultHint',
                                  'Default always contains every tool, including ones you connect later. Switch a tool off to hide it from this profile only.'
                              )}
                    </Typography>
                    {rows.length === 0 && (
                        <Typography color="text.secondary">
                            {!custom
                                ? label('noToolsYet', 'No tools yet. Connect one on the MCP Tools page.')
                                : label('emptyProfile', 'This profile has no tools. Add the ones this agent should use.')}
                        </Typography>
                    )}
                    <Stack spacing={1} sx={{ mt: 1 }}>
                        {rows.map((row) => {
                            const globallyOff = row.tool ? !row.tool.enabled : true;
                            return (
                                <Stack
                                    key={row.name}
                                    direction="row"
                                    aria-label={`${row.sourceName} / ${row.tool?.name || row.name}`}
                                    role="group"
                                    sx={{
                                        alignItems: 'center',
                                        justifyContent: 'space-between',
                                        gap: 1,
                                        px: 1.5,
                                        py: 0.75,
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 1.5,
                                        opacity: globallyOff ? 0.6 : 1,
                                    }}
                                >
                                    <Box sx={{ minWidth: 0 }}>
                                        <Typography variant="body2" style={{ fontWeight: 600 }}>
                                            {row.sourceName ? `${row.sourceName} / ` : ''}
                                            {row.tool?.name || row.name}
                                        </Typography>
                                        {globallyOff && (
                                            <Typography variant="caption" color="text.secondary">
                                                {row.tool
                                                    ? label('globallyOff', 'Off on the MCP Tools page, so it is unavailable here')
                                                    : label('toolGone', 'This tool no longer exists')}
                                            </Typography>
                                        )}
                                    </Box>
                                    <Stack direction="row" sx={{ alignItems: 'center', gap: 0.5 }}>
                                        <Switch
                                            size="small"
                                            disabled={busy}
                                            checked={row.on}
                                            slotProps={{ input: { 'aria-label': row.tool?.name || row.name } }}
                                            onChange={(e) => void save(setProfileToolOn(profile, row.name, e.target.checked))}
                                        />
                                        {custom && (
                                            <IconButton
                                                size="small"
                                                disabled={busy}
                                                aria-label={label('removeFromProfile', 'Remove from profile')}
                                                onClick={() => void save(removeProfileTool(profile, row.name))}
                                            >
                                                <Close fontSize="small" />
                                            </IconButton>
                                        )}
                                    </Stack>
                                </Stack>
                            );
                        })}
                    </Stack>
                </Box>
            </Stack>
            <ConfirmDialog
                open={confirmDelete}
                title={label('removeProfile', 'Remove profile')}
                description={label(
                    'deleteProfileHint',
                    'Agents connected to this profile lose access. Tools and other profiles are not affected.'
                )}
                confirmLabel={label('confirmRemoveProfile', 'Confirm removal')}
                confirmColor="error"
                loading={busy}
                onClose={() => setConfirmDelete(false)}
                onConfirm={() =>
                    void ws.run(async () => {
                        await ws.acceptConfig(await api.deleteMCPProfile(profile.id));
                        setConfirmDelete(false);
                        select(MCP_DEFAULT_PROFILE_ID);
                    })
                }
            />
            <Dialog open={renaming !== null} onClose={() => setRenaming(null)} maxWidth="xs" fullWidth>
                <DialogTitle>{label('renameProfile', 'Rename profile')}</DialogTitle>
                <DialogContent>
                    <TextField
                        autoFocus
                        fullWidth
                        margin="dense"
                        size="small"
                        label={label('profileName', 'Profile name')}
                        value={renaming ?? ''}
                        onChange={(e) => setRenaming(e.target.value)}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setRenaming(null)}>{label('cancel', 'Cancel')}</Button>
                    <Button
                        variant="contained"
                        disabled={busy || !renaming?.trim()}
                        onClick={() => void save({ ...profile, name: renaming!.trim() }).then(() => setRenaming(null))}
                    >
                        {label('save', 'Save')}
                    </Button>
                </DialogActions>
            </Dialog>
            <Dialog open={adding} onClose={() => setAdding(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{label('addProfileTools', 'Add tools')}</DialogTitle>
                <DialogContent dividers>
                    <Stack spacing={1.5}>
                        {groups.map((group) => {
                            const tools = available.filter(({ sourceName }) => sourceName === group);
                            const names = tools.map(({ tool }) => tool.normalized_name);
                            const all = names.every((n) => picked.includes(n));
                            return (
                                <Box key={group}>
                                    <FormControlLabel
                                        control={
                                            <Checkbox
                                                checked={all}
                                                indeterminate={!all && names.some((n) => picked.includes(n))}
                                                onChange={(e) =>
                                                    setPicked(
                                                        e.target.checked
                                                            ? [...new Set([...picked, ...names])]
                                                            : picked.filter((n) => !names.includes(n))
                                                    )
                                                }
                                            />
                                        }
                                        label={<Typography style={{ fontWeight: 600 }}>{group}</Typography>}
                                    />
                                    <Stack sx={{ pl: 4 }}>
                                        {tools.map(({ tool }) => (
                                            <FormControlLabel
                                                key={tool.normalized_name}
                                                control={
                                                    <Checkbox
                                                        checked={picked.includes(tool.normalized_name)}
                                                        onChange={(e) =>
                                                            setPicked(
                                                                e.target.checked
                                                                    ? [...picked, tool.normalized_name]
                                                                    : picked.filter((n) => n !== tool.normalized_name)
                                                            )
                                                        }
                                                    />
                                                }
                                                label={tool.name}
                                            />
                                        ))}
                                    </Stack>
                                </Box>
                            );
                        })}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setAdding(false)}>{label('cancel', 'Cancel')}</Button>
                    <Button
                        variant="contained"
                        disabled={busy || picked.length === 0}
                        onClick={() =>
                            void save(addProfileTools(profile, picked)).then(() => setAdding(false))
                        }
                    >
                        {label('addSelected', 'Add selected')}
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
}
