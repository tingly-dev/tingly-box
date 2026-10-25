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
    Typography,
} from '@mui/material';
import { Add, Close, Delete, Refresh } from '@/components/icons';
import { PageLayout } from '@/components/PageLayout';
import { api } from '@/services/api';
import AgentInstallCard from './AgentInstallCard';
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
    const [name, setName] = useState<{ id: string; value: string } | null>(null);
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
        if (creating && !ws.loading && !createdOnce.current) {
            createdOnce.current = true;
            void create();
        }
    });
    const available = profileCandidates(routing).filter(
        ({ tool }) => !(profile.tools || []).some((entry) => entry.name === tool.normalized_name)
    );
    const groups = [...new Set(available.map(({ sourceName }) => sourceName))];
    return (
        <PageLayout loading={ws.loading}>
            <Stack spacing={3}>
                <Stack
                    direction="row"
                    sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 2, flexWrap: 'wrap' }}
                >
                    <Stack>
                        <Typography component="h1" variant="h5" style={{ fontWeight: 700 }}>
                            {profile.name || profile.id}
                        </Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                            {label(
                                'profilesHint',
                                'A profile is one entry point to your tools. Default has every tool; create others to give an agent only what it needs.'
                            )}
                        </Typography>
                    </Stack>
                    <Stack direction="row" spacing={1}>
                        <IconButton
                            aria-label={label('refresh', 'Refresh connections')}
                            disabled={busy || checking}
                            onClick={() => void ws.run(ws.reload)}
                        >
                            <Refresh />
                        </IconButton>
                    </Stack>
                </Stack>
                <MCPWorkspaceAlerts ws={ws} />
                <Stack spacing={2.5} component="section" aria-label={profile.name}>
                    <Stack direction="row" sx={{ alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
                        {!isDefaultProfile(profile) && (
                            <TextField
                                size="small"
                                label={label('profileName', 'Profile name')}
                                disabled={busy}
                                value={name?.id === profile.id ? name.value : profile.name}
                                onChange={(e) => setName({ id: profile.id, value: e.target.value })}
                                onBlur={() => {
                                    if (name?.id === profile.id && name.value.trim() && name.value !== profile.name)
                                        void save({ ...profile, name: name.value.trim() });
                                    setName(null);
                                }}
                            />
                        )}
                        <FormControlLabel
                            control={
                                <Switch
                                    disabled={busy}
                                    checked={profile.enabled !== false}
                                    onChange={(e) => void save({ ...profile, enabled: e.target.checked })}
                                />
                            }
                            label={label('profileOn', 'Profile on')}
                        />
                        <Typography variant="body2" color="text.secondary">
                            {routing
                                ? `${exposed} ${label('profileToolCount', 'tools available through this profile')}`
                                : label('checking', 'Checking…')}
                        </Typography>
                    </Stack>
                    <Box component="section" aria-label={label('profileTools', 'Tools in this profile')}>
                        <Stack
                            direction="row"
                            sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 1, gap: 1 }}
                        >
                            <Typography variant="h6" sx={{ fontWeight: 600 }}>
                                {label('profileTools', 'Tools in this profile')}
                            </Typography>
                            {!isDefaultProfile(profile) && (
                                <Button
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
                        </Stack>
                        {isDefaultProfile(profile) && (
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                {label(
                                    'defaultHint',
                                    'Default always contains every tool, including ones you connect later. Switch a tool off to hide it from this profile only.'
                                )}
                            </Typography>
                        )}
                        {rows.length === 0 && (
                            <Typography color="text.secondary">
                                {isDefaultProfile(profile)
                                    ? label('noToolsYet', 'No tools yet. Connect one on the Tools page.')
                                    : label('emptyProfile', 'This profile has no tools. Add the ones this agent should use.')}
                            </Typography>
                        )}
                        <Stack spacing={1}>
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
                                                        ? label('globallyOff', 'Off on the Tools page, so it is unavailable here')
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
                                            {!isDefaultProfile(profile) && (
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
                    <AgentInstallCard
                        key={profile.id}
                        clientId={profile.id}
                        serverName={profileServerName(profile)}
                        heading={label('copyProfileCommand', 'Connect an agent to this profile')}
                    />
                    <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
                        <Button
                            variant="outlined"
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
                        {!isDefaultProfile(profile) && (
                            <Button
                                color="error"
                                startIcon={<Delete />}
                                disabled={busy}
                                onClick={() => {
                                    if (!confirmDelete) setConfirmDelete(true);
                                    else
                                        void ws.run(async () => {
                                            await ws.acceptConfig(await api.deleteMCPProfile(profile.id));
                                            setConfirmDelete(false);
                                            select(MCP_DEFAULT_PROFILE_ID);
                                        });
                                }}
                            >
                                {confirmDelete
                                    ? label('confirmRemoveProfile', 'Confirm removal')
                                    : label('removeProfile', 'Remove profile')}
                            </Button>
                        )}
                    </Stack>
                    {confirmDelete && (
                        <Alert severity="warning">
                            {label(
                                'deleteProfileHint',
                                'Agents connected to this profile lose access. Tools and other profiles are not affected.'
                            )}
                        </Alert>
                    )}
                    {probe && (
                        <Alert severity={probe.success ? 'success' : 'error'} data-testid="mcp-profile-probe">
                            {probe.success
                                ? `${label('profileProbePassed', 'Actual gateway initialization and tool discovery passed.')} ${probe.tools.length} ${label('toolCount', 'tools')}`
                                : probe.error}
                        </Alert>
                    )}
                </Stack>
            </Stack>
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
