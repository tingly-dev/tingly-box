// Tasks — the landing page, in the shape of Claude Code's and Codex's: one
// composer in the middle of the page that starts a task directly (folder
// and permissions as small controls under the text, no form), and under
// it the list of what the agent has done and is doing, as rows.
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {useNavigate} from 'react-router-dom';
import {
    Box, Button, Chip, CircularProgress, Divider, InputBase, List, ListItemButton, ListItemText, ListSubheader, Menu, MenuItem,
    Paper, Stack, ToggleButton, ToggleButtonGroup, Typography,
} from '@mui/material';
import {PageLayout} from '@/components/PageLayout';
import {ArrowForward as IconGo, FolderOpen as IconFolder, KeyboardArrowDown as IconCaret, Shield as IconShield} from '@/components/icons';
import FolderPickerDialog from './FolderPickerDialog';
import {useNotify} from '@/hooks/useNotify';
import {
    agentApi, isActiveStatus, type AgentEnvironment, type PermissionMode, type RecentFolder, type SessionListItem,
} from '@/services/agentApi';
import {PERMISSION_MODES, permissionModeKey, relativeTime, StatusChip, StatusDot} from './taskShared';

const COLUMN = 760;

const TasksPage = () => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const notify = useNotify();

    const [environments, setEnvironments] = useState<AgentEnvironment[]>([]);
    const [sessions, setSessions] = useState<SessionListItem[]>([]);
    const [loading, setLoading] = useState(true);
    const [filter, setFilter] = useState<'active' | 'all'>('active');

    const [prompt, setPrompt] = useState('');
    const [folder, setFolder] = useState('');
    const [folders, setFolders] = useState<RecentFolder[]>([]);
    const [pickerOpen, setPickerOpen] = useState(false);
    const [folderAnchor, setFolderAnchor] = useState<HTMLElement | null>(null);
    const [modeAnchor, setModeAnchor] = useState<HTMLElement | null>(null);
    const [permissionMode, setPermissionMode] = useState<PermissionMode>('');
    const [starting, setStarting] = useState(false);

    const load = useCallback(async () => {
        const [env, list, recent] = await Promise.all([
            agentApi.listEnvironments(),
            agentApi.listSessions(filter === 'active' ? {active: true} : {}),
            agentApi.recentFolders(),
        ]);
        if (recent.ok) setFolders(recent.data.folders ?? []);
        if (env.ok) setEnvironments(env.data.environments ?? []);
        if (list.ok) setSessions(list.data.sessions ?? []);
        else notify.error(list.error);
        setLoading(false);
    }, [filter, notify]);

    useEffect(() => {
        load();
    }, [load]);

    useEffect(() => {
        const anyActive = sessions.some((item) => isActiveStatus(item.session.status));
        const timer = setInterval(load, anyActive ? 3000 : 15000);
        return () => clearInterval(timer);
    }, [sessions, load]);

    // Smart default: the most recently added folder.
    useEffect(() => {
        if (!folder && folders.length > 0) setFolder(folders[0].path);
    }, [folders, folder]);

    const environment = environments.find((e) => e.is_default) ?? environments[0];

    const pickFolder = (path: string) => {
        setPickerOpen(false);
        setFolderAnchor(null);
        setFolder(path);
        if (!folders.some((f) => f.path === path)) {
            setFolders([{path, name: path.split(/[\\/]/).filter(Boolean).pop() ?? path, is_repo: false}, ...folders]);
        }
    };

    const canStart = prompt.trim().length > 0 && !!folder && !!environment && !starting;

    const start = async () => {
        if (!canStart || !environment) return;
        setStarting(true);
        const res = await agentApi.createSession({
            source_id: '', local_path: folder, environment_id: environment.id, prompt: prompt.trim(),
            workspace_id: '', base_ref: '', title: '', permission_mode: permissionMode,
        });
        setStarting(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        navigate(`/tasks/${res.data.session.id}`);
    };

    const folderName = folders.find((f) => f.path === folder)?.name ?? (folder ? folder.split(/[\\/]/).filter(Boolean).pop() : undefined);
    const modeKey = permissionModeKey(permissionMode);
    const modeLabel = modeKey === 'inherit' && environment?.permission_mode
        ? t(`tasks.mode.${environment.permission_mode}`)
        : t(`tasks.mode.${modeKey}`);

    return (
        <PageLayout loading={loading}>
            <Box sx={{maxWidth: COLUMN, mx: 'auto', width: '100%', pt: {xs: 1, md: 6}}}>
                <Typography variant="h5" sx={{fontWeight: 600, textAlign: 'center', mb: 2.5}}>
                    {t('tasks.composer.hero')}
                </Typography>

                {/* Composer */}
                <Paper
                    variant="outlined"
                    sx={{borderRadius: 3, px: 2, pt: 1.75, pb: 1.25, boxShadow: (th) => th.shadows[1], '&:focus-within': {borderColor: 'primary.main'}}}
                >
                    <InputBase
                        fullWidth
                        multiline
                        minRows={3}
                        maxRows={10}
                        autoFocus
                        placeholder={t('tasks.composer.promptPlaceholder')}
                        value={prompt}
                        onChange={(e) => setPrompt(e.target.value)}
                        onKeyDown={(e) => {
                            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && canStart) start();
                        }}
                        sx={{fontSize: 15, lineHeight: 1.6, px: 0.5}}
                        inputProps={{'aria-label': t('tasks.composer.prompt')}}
                    />
                    <Stack direction="row" spacing={1} sx={{alignItems: 'center', pt: 1.25, flexWrap: 'wrap', rowGap: 1}}>
                        <Chip
                            variant="outlined"
                            icon={<IconFolder />}
                            deleteIcon={<IconCaret />}
                            onDelete={(e) => setFolderAnchor(e.currentTarget.parentElement)}
                            onClick={(e) => setFolderAnchor(e.currentTarget)}
                            label={folderName ?? t('tasks.composer.chooseFolder')}
                            color={folder ? 'default' : 'primary'}
                            sx={{maxWidth: 260, '& .MuiChip-label': {overflow: 'hidden', textOverflow: 'ellipsis'}}}
                            title={folder}
                        />
                        <Menu open={!!folderAnchor} anchorEl={folderAnchor} onClose={() => setFolderAnchor(null)} slotProps={{paper: {sx: {minWidth: 280, maxWidth: 420}}}}>
                            {folders.length > 0 && <ListSubheader disableSticky>{t('tasks.folder.places')}</ListSubheader>}
                            {folders.map((f) => (
                                <MenuItem key={f.path} selected={f.path === folder} onClick={() => pickFolder(f.path)}>
                                    <ListItemText
                                        primary={f.name}
                                        secondary={f.path}
                                        slotProps={{secondary: {sx: {fontFamily: 'monospace', fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis'}}}}
                                    />
                                </MenuItem>
                            ))}
                            {folders.length > 0 && <Divider />}
                            <MenuItem onClick={() => { setFolderAnchor(null); setPickerOpen(true); }}>
                                <IconFolder fontSize="small" sx={{mr: 1, color: 'text.secondary'}} />
                                {t('tasks.composer.browseFolder')}
                            </MenuItem>
                        </Menu>

                        <Chip
                            variant="outlined"
                            icon={<IconShield />}
                            deleteIcon={<IconCaret />}
                            onDelete={(e) => setModeAnchor(e.currentTarget.parentElement)}
                            onClick={(e) => setModeAnchor(e.currentTarget)}
                            label={modeLabel}
                            title={t(`tasks.mode.${modeKey}Help`)}
                        />
                        <Menu open={!!modeAnchor} anchorEl={modeAnchor} onClose={() => setModeAnchor(null)}>
                            {PERMISSION_MODES.map((m) => {
                                const key = permissionModeKey(m);
                                return (
                                    <MenuItem key={key} selected={permissionMode === m} onClick={() => { setPermissionMode(m); setModeAnchor(null); }}>
                                        <ListItemText primary={t(`tasks.mode.${key}`)} secondary={t(`tasks.mode.${key}Help`)} />
                                    </MenuItem>
                                );
                            })}
                        </Menu>

                        <Box sx={{flex: 1}} />
                        <Typography variant="caption" color="text.disabled" sx={{display: {xs: 'none', sm: 'block'}}}>{t('tasks.composer.shortcut')}</Typography>
                        <Button
                            variant="contained"
                            endIcon={starting ? <CircularProgress size={14} color="inherit" /> : <IconGo />}
                            disabled={!canStart}
                            onClick={start}
                            sx={{borderRadius: 5, px: 2}}
                        >
                            {t('tasks.composer.start')}
                        </Button>
                    </Stack>
                </Paper>

                {/* List */}
                <Stack direction="row" sx={{alignItems: 'center', mt: 5, mb: 1}}>
                    <Typography variant="subtitle2" color="text.secondary" sx={{flex: 1}}>{t('tasks.list.recent')}</Typography>
                    <ToggleButtonGroup size="small" exclusive value={filter} onChange={(_, v) => v && setFilter(v)} sx={{'& .MuiToggleButton-root': {py: 0.25, px: 1.25, border: 0, textTransform: 'none'}}}>
                        <ToggleButton value="active">{t('tasks.list.active')}</ToggleButton>
                        <ToggleButton value="all">{t('tasks.list.all')}</ToggleButton>
                    </ToggleButtonGroup>
                </Stack>
                {sessions.length === 0 ? (
                    <Typography variant="body2" color="text.disabled" sx={{py: 3, textAlign: 'center'}}>
                        {t('tasks.list.emptyHint')}
                    </Typography>
                ) : (
                    <List disablePadding sx={{borderTop: '1px solid', borderColor: 'divider'}}>
                        {sessions.map(({session: s, source: src}) => {
                            const meta = [src?.name, (s.artifact?.changed_files ?? 0) > 0 ? t('tasks.list.changedFiles', {count: s.artifact.changed_files}) : undefined, relativeTime(s.last_active_at)]
                                .filter(Boolean)
                                .join(' · ');
                            const highlight = s.status === 'waiting_input' || s.status === 'failed' || s.status === 'running';
                            return (
                                <ListItemButton key={s.id} onClick={() => navigate(`/tasks/${s.id}`)} sx={{px: 1, py: 1.25, borderBottom: '1px solid', borderColor: 'divider', gap: 1.5}}>
                                    <StatusDot status={s.status} />
                                    <ListItemText
                                        primary={s.title || s.prompt}
                                        secondary={meta}
                                        slotProps={{
                                            primary: {sx: {fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}},
                                            secondary: {sx: {overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}},
                                        }}
                                        sx={{m: 0, minWidth: 0}}
                                    />
                                    {highlight && <StatusChip status={s.status} />}
                                </ListItemButton>
                            );
                        })}
                    </List>
                )}
            </Box>
            <FolderPickerDialog open={pickerOpen} onClose={() => setPickerOpen(false)} onPick={pickFolder} />
        </PageLayout>
    );
};

export default TasksPage;
