// Tasks — the landing page. The composer sits on top and starts a task
// directly (no wizard: pick a repository, describe the work, go); below it
// the list answers "what is the agent doing for me right now".
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {useNavigate} from 'react-router-dom';
import {
    Box, Button, Card, CardActionArea, Chip, CircularProgress, FormControl, InputLabel, ListSubheader, MenuItem,
    Select, Stack, TextField, ToggleButton, ToggleButtonGroup, Typography,
} from '@mui/material';
import {PageLayout} from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import EmptyState from '@/components/EmptyState';
import {FolderOpen as IconFolder, PlayArrow as IconPlay, Terminal as IconTerminal} from '@/components/icons';
import FolderPickerDialog from './FolderPickerDialog';
import {useNotify} from '@/hooks/useNotify';
import {
    agentApi, isActiveStatus, type AgentEnvironment, type AgentSource, type PermissionMode, type RecentFolder, type SessionListItem,
} from '@/services/agentApi';

// The composer's target is one of two things on separate axes: a folder on
// this host the agent works in directly, or a registered repository it
// clones. Encoded as one select value so there is no mode picker.
const FOLDER_PREFIX = 'folder:';
const SOURCE_PREFIX = 'source:';
const BROWSE = '__browse__';
import {PermissionModeSelect, relativeTime, StatusChip} from './taskShared';

const TasksPage = () => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const notify = useNotify();

    const [sources, setSources] = useState<AgentSource[]>([]);
    const [environments, setEnvironments] = useState<AgentEnvironment[]>([]);
    const [sessions, setSessions] = useState<SessionListItem[]>([]);
    const [loading, setLoading] = useState(true);
    const [filter, setFilter] = useState<'active' | 'all'>('active');

    const [prompt, setPrompt] = useState('');
    // target: "folder:<path>" or "source:<id>"
    const [target, setTarget] = useState('');
    const [folders, setFolders] = useState<RecentFolder[]>([]);
    const [pickerOpen, setPickerOpen] = useState(false);
    const [environmentId, setEnvironmentId] = useState('');
    const [permissionMode, setPermissionMode] = useState<PermissionMode>('');
    const [starting, setStarting] = useState(false);

    const load = useCallback(async () => {
        const [src, env, list, recent] = await Promise.all([
            agentApi.listSources(),
            agentApi.listEnvironments(),
            agentApi.listSessions(filter === 'active' ? {active: true} : {}),
            agentApi.recentFolders(),
        ]);
        if (src.ok) setSources((src.data.sources ?? []).filter((s) => s.kind !== 'local'));
        if (recent.ok) setFolders(recent.data.folders ?? []);
        if (env.ok) setEnvironments(env.data.environments ?? []);
        if (list.ok) setSessions(list.data.sessions ?? []);
        else notify.error(list.error);
        setLoading(false);
    }, [filter, notify]);

    useEffect(() => {
        load();
    }, [load]);

    // Keep the list live while anything is active. Same cadence rationale as
    // the detail page: fast while working, slow once settled.
    useEffect(() => {
        const anyActive = sessions.some((item) => isActiveStatus(item.session.status));
        const timer = setInterval(load, anyActive ? 3000 : 15000);
        return () => clearInterval(timer);
    }, [sessions, load]);

    // Smart defaults: the most recent folder, else the first repository; the
    // default environment.
    useEffect(() => {
        if (target) return;
        if (folders.length > 0) setTarget(FOLDER_PREFIX + folders[0].path);
        else if (sources.length > 0) setTarget(SOURCE_PREFIX + sources[0].id);
    }, [folders, sources, target]);
    useEffect(() => {
        if (!environmentId && environments.length > 0) {
            setEnvironmentId((environments.find((e) => e.is_default) ?? environments[0]).id);
        }
    }, [environments, environmentId]);

    const pickFolder = (path: string) => {
        setPickerOpen(false);
        setTarget(FOLDER_PREFIX + path);
        if (!folders.some((f) => f.path === path)) {
            setFolders([{path, name: path.split(/[\\/]/).filter(Boolean).pop() ?? path, is_repo: false}, ...folders]);
        }
    };

    const start = async () => {
        if (!prompt.trim() || !target) return;
        setStarting(true);
        const res = await agentApi.createSession({
            source_id: target.startsWith(SOURCE_PREFIX) ? target.slice(SOURCE_PREFIX.length) : '',
            local_path: target.startsWith(FOLDER_PREFIX) ? target.slice(FOLDER_PREFIX.length) : '',
            environment_id: environmentId, prompt: prompt.trim(),
            workspace_id: '', base_ref: '', title: '', permission_mode: permissionMode,
        });
        setStarting(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        navigate(`/tasks/${res.data.session.id}`);
    };

    const selectedEnvironment = environments.find((e) => e.id === environmentId);
    const canStart = prompt.trim().length > 0 && !!target && !!environmentId && !starting;

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <PageHeader
                    title={t('tasks.title')}
                    subtitle={t('tasks.subtitle')}
                    icon={<IconTerminal />}
                />

                {/* Composer */}
                <Card variant="outlined" sx={{p: {xs: 2, sm: 2.5}}}>
                    <Stack spacing={2}>
                        <TextField
                            label={t('tasks.composer.prompt')}
                            placeholder={t('tasks.composer.promptPlaceholder')}
                            value={prompt}
                            onChange={(e) => setPrompt(e.target.value)}
                            multiline
                            minRows={2}
                            maxRows={8}
                            fullWidth
                            onKeyDown={(e) => {
                                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && canStart) start();
                            }}
                        />
                        <Stack direction={{xs: 'column', sm: 'row'}} spacing={1.5} sx={{alignItems: {sm: 'center'}}}>
                            <FormControl size="small" sx={{minWidth: 260, flex: {sm: 1}}}>
                                <InputLabel id="task-target">{t('tasks.composer.target')}</InputLabel>
                                <Select
                                    labelId="task-target"
                                    label={t('tasks.composer.target')}
                                    value={target}
                                    onChange={(e) => {
                                        if (e.target.value === BROWSE) setPickerOpen(true);
                                        else setTarget(e.target.value);
                                    }}
                                    renderValue={(v) => {
                                        if (v.startsWith(FOLDER_PREFIX)) {
                                            const p = v.slice(FOLDER_PREFIX.length);
                                            return folders.find((f) => f.path === p)?.name ?? (p.split(/[\\/]/).filter(Boolean).pop() || p);
                                        }
                                        const src = sources.find((s) => SOURCE_PREFIX + s.id === v);
                                        return src ? src.name : v;
                                    }}
                                >
                                    <ListSubheader>{t('tasks.composer.folders')}</ListSubheader>
                                    {folders.map((f) => (
                                        <MenuItem key={f.path} value={FOLDER_PREFIX + f.path}>
                                            <Stack sx={{minWidth: 0}}>
                                                <span>{f.name}</span>
                                                <Typography variant="caption" color="text.secondary" sx={{fontFamily: 'monospace', overflow: 'hidden', textOverflow: 'ellipsis'}}>{f.path}</Typography>
                                            </Stack>
                                        </MenuItem>
                                    ))}
                                    <MenuItem value={BROWSE}>
                                        <IconFolder fontSize="small" sx={{mr: 1}} />
                                        {t('tasks.composer.browseFolder')}
                                    </MenuItem>
                                    {sources.length > 0 && <ListSubheader>{t('tasks.composer.repositories')}</ListSubheader>}
                                    {sources.map((s) => (
                                        <MenuItem key={s.id} value={SOURCE_PREFIX + s.id}>
                                            {s.name}
                                            <Typography component="span" variant="caption" color="text.secondary" sx={{ml: 1}}>
                                                {s.default_branch}
                                            </Typography>
                                        </MenuItem>
                                    ))}
                                </Select>
                            </FormControl>
                            {/* Environment only becomes a choice once there is more
                                than one — the default is applied silently otherwise. */}
                            {environments.length > 1 && (
                                <FormControl size="small" sx={{minWidth: 180}}>
                                    <InputLabel id="task-env">{t('tasks.composer.environment')}</InputLabel>
                                    <Select
                                        labelId="task-env"
                                        label={t('tasks.composer.environment')}
                                        value={environmentId}
                                        onChange={(e) => setEnvironmentId(e.target.value)}
                                    >
                                        {environments.map((e) => (
                                            <MenuItem key={e.id} value={e.id}>
                                                {e.name}
                                                {e.runtime !== 'local' && (
                                                    <Typography component="span" variant="caption" color="text.secondary" sx={{ml: 1}}>
                                                        {e.runtime}
                                                    </Typography>
                                                )}
                                            </MenuItem>
                                        ))}
                                    </Select>
                                </FormControl>
                            )}
                            <PermissionModeSelect
                                value={permissionMode}
                                onChange={setPermissionMode}
                                inheritHint={selectedEnvironment?.permission_mode ? t(`tasks.mode.${selectedEnvironment.permission_mode}`) : undefined}
                                sx={{minWidth: 200}}
                            />
                            <Box sx={{flex: 1, display: {xs: 'none', sm: 'block'}}} />
                            <Button
                                variant="contained"
                                startIcon={starting ? <CircularProgress size={16} color="inherit" /> : <IconPlay />}
                                disabled={!canStart}
                                onClick={start}
                                sx={{alignSelf: {xs: 'stretch', sm: 'auto'}}}
                            >
                                {starting ? t('tasks.composer.starting') : t('tasks.composer.start')}
                            </Button>
                        </Stack>
                    </Stack>
                </Card>

                {/* List */}
                <Stack direction="row" sx={{alignItems: 'center', justifyContent: 'space-between'}}>
                    <ToggleButtonGroup
                        size="small"
                        exclusive
                        value={filter}
                        onChange={(_, v) => v && setFilter(v)}
                    >
                        <ToggleButton value="active">{t('tasks.list.active')}</ToggleButton>
                        <ToggleButton value="all">{t('tasks.list.all')}</ToggleButton>
                    </ToggleButtonGroup>
                </Stack>

                {sessions.length === 0 ? (
                    <EmptyState
                        compact
                        icon={<IconTerminal />}
                        title={t('tasks.list.empty')}
                        description={t('tasks.list.emptyHint')}
                    />
                ) : (
                    <Stack spacing={1.25}>
                        {sessions.map(({session: s, source: src, branch}) => {
                            return (
                                <Card key={s.id} variant="outlined">
                                    <CardActionArea onClick={() => navigate(`/tasks/${s.id}`)} sx={{p: {xs: 1.5, sm: 2}}}>
                                        <Stack spacing={0.75}>
                                            <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                                                <Typography variant="subtitle1" sx={{fontWeight: 600, flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>
                                                    {s.title || s.prompt}
                                                </Typography>
                                                <StatusChip status={s.status} />
                                            </Stack>
                                            <Stack direction="row" spacing={1} sx={{flexWrap: 'wrap', alignItems: 'center', rowGap: 0.5}}>
                                                {src && <Chip size="small" variant="outlined" label={src.name} />}
                                                {branch && (
                                                    <Typography variant="caption" color="text.secondary" sx={{fontFamily: 'monospace'}}>
                                                        {branch}
                                                    </Typography>
                                                )}
                                                {(s.artifact?.changed_files ?? 0) > 0 && (
                                                    <Typography variant="caption" color="text.secondary">
                                                        {t('tasks.list.changedFiles', {count: s.artifact.changed_files})}
                                                    </Typography>
                                                )}
                                                {s.artifact?.pushed && <Chip size="small" color="success" variant="outlined" label={t('tasks.detail.pushed')} />}
                                                <Box sx={{flex: 1}} />
                                                <Typography variant="caption" color="text.secondary">
                                                    {relativeTime(s.last_active_at)}
                                                </Typography>
                                            </Stack>
                                        </Stack>
                                    </CardActionArea>
                                </Card>
                            );
                        })}
                    </Stack>
                )}
            </Stack>
            <FolderPickerDialog open={pickerOpen} onClose={() => setPickerOpen(false)} onPick={pickFolder} />
        </PageLayout>
    );
};

export default TasksPage;
