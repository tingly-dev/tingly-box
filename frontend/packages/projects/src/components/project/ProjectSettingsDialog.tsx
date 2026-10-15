import { useEffect, useState } from 'react';
import {
    Alert, Autocomplete, Box, Button, Dialog, DialogContent, DialogTitle, Divider, IconButton, MenuItem, Stack,
    Tab, Table, TableBody, TableCell, TableHead, TableRow, Tabs, TextField, Typography,
} from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type MemoryFile, type paths, type Project, type ProjectRuntime, type Usage } from '../../client';
import { ConfirmDialog } from '../ConfirmDialog';
import { TextEditDialog } from '../TextEditDialog';
import { Add, Close, Delete, Description } from '../icons';
import { useModels } from '../../hooks/useModels';
import { useNotify } from '../../hooks/useNotify';
import { useProjectsHost } from '../../host/ProjectsHost';
import { formatBytes, formatTokens, timeAgo } from '../../utils/format';
import { RepoField, type RepoEntry } from './RepoField';

export type SettingsTab = 'general' | 'memory' | 'environment' | 'usage';

const MAX_INSTRUCTIONS = 16000;
const mono = { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' } as const;

type Patch = NonNullable<paths['/api/v1/projects/{id}']['patch']['requestBody']>['content']['application/json'];
type Mode = NonNullable<Patch['thread_permission_mode']>;

function ModelField({ label, value, onChange, help }: { label: string; value: string; onChange: (v: string) => void; help: string }) {
    const { models } = useModels('projects');
    const { t } = useTranslation('projects');
    return (
        <Autocomplete
            freeSolo
            options={models.map((m) => m.id)}
            value={value}
            onInputChange={(_, v) => onChange(v)}
            renderInput={(params) => (
                <TextField {...params} label={label} placeholder={t('projectSettings.modelDefault')} helperText={help}
                    slotProps={{ ...params.slotProps, inputLabel: { shrink: true }, htmlInput: { ...params.slotProps?.htmlInput, style: { ...mono, fontSize: 13 } } }} />
            )}
        />
    );
}

function General({ project, patch }: { project: Project; patch: (p: Patch) => Promise<void> }) {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const notify = useNotify();
    const { onProjectsChanged, paths: routes } = useProjectsHost();
    const [name, setName] = useState(project.name);
    const [goal, setGoal] = useState(project.goal);
    const [coord, setCoord] = useState(project.coordinator_model);
    const [threadModel, setThreadModel] = useState(project.thread_model);
    const [mode, setMode] = useState<Mode>((project.thread_permission_mode || 'acceptEdits') as Mode);
    const [deleting, setDeleting] = useState(false);
    const dirty = name !== project.name || goal !== project.goal || coord !== project.coordinator_model || threadModel !== project.thread_model || mode !== (project.thread_permission_mode || 'acceptEdits');

    return (
        <Stack spacing={2.5}>
            <TextField label={t('project.name')} value={name} onChange={(e) => setName(e.target.value)} fullWidth />
            <TextField label={t('project.goal')} value={goal} onChange={(e) => setGoal(e.target.value)} placeholder={t('project.goalPlaceholder')} helperText={t('project.goalHelp')} fullWidth />
            <ModelField label={t('projectSettings.coordinatorModel')} value={coord} onChange={setCoord} help={t('projectSettings.coordinatorModelHelp')} />
            <ModelField label={t('projectSettings.threadModel')} value={threadModel} onChange={setThreadModel} help={t('projectSettings.threadModelHelp')} />
            <TextField select label={t('projectSettings.permissionMode')} value={mode} onChange={(e) => setMode(e.target.value as Mode)} helperText={t(`projectSettings.mode.${mode}Help`)}>
                {['acceptEdits', 'default', 'auto', 'bypassPermissions'].map((m) => (
                    <MenuItem key={m} value={m}><Box component="span" sx={{ ...mono, fontSize: 13, mr: 1 }}>{m}</Box>{t(`projectSettings.mode.${m}`)}</MenuItem>
                ))}
            </TextField>
            <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                <Button variant="contained" disabled={!dirty || !name.trim()} onClick={() => void patch({ name, goal, coordinator_model: coord, thread_model: threadModel, thread_permission_mode: mode })}>{t('common.save')}</Button>
            </Box>
            <Divider />
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                <Box sx={{ flex: 1 }}>
                    <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{project.paused ? t('projectSettings.resume') : t('projectSettings.pause')}</Typography>
                    <Typography variant="body2" color="text.secondary">{t('projectSettings.pauseHelp')}</Typography>
                </Box>
                <Button variant="outlined" onClick={() => void patch({ paused: !project.paused })}>{project.paused ? t('projectSettings.resume') : t('projectSettings.pause')}</Button>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                <Box sx={{ flex: 1 }}>
                    <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{project.archived_at ? t('projectSettings.unarchive') : t('projectSettings.archive')}</Typography>
                    <Typography variant="body2" color="text.secondary">{t('projectSettings.archiveHelp')}</Typography>
                </Box>
                <Button variant="outlined" onClick={() => void patch({ archived: !project.archived_at })}>{project.archived_at ? t('projectSettings.unarchive') : t('projectSettings.archive')}</Button>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                <Box sx={{ flex: 1 }}>
                    <Typography sx={{ fontWeight: 600, color: 'error.main' }}>{t('projectSettings.delete')}</Typography>
                    <Typography variant="body2" color="text.secondary">{t('projectSettings.deleteHelp')}</Typography>
                </Box>
                <Button variant="outlined" color="error" onClick={() => setDeleting(true)}>{t('common.delete')}</Button>
            </Box>
            <ConfirmDialog
                open={deleting}
                title={t('projectSettings.deleteTitle', { name: project.name })}
                message={t('projectSettings.deleteHelp')}
                confirmLabel={t('projectSettings.deleteConfirm')}
                danger
                onClose={() => setDeleting(false)}
                onConfirm={() => {
                    unwrap(api.DELETE('/api/v1/projects/{id}', { params: { path: { id: project.id } } }))
                        .then(() => {
                            onProjectsChanged();
                            navigate(routes.list);
                        })
                        .catch((e: Error) => notify(e.message, 'error'));
                }}
            />
        </Stack>
    );
}

function Memory({ project, patch, refreshKey }: { project: Project; patch: (p: Patch) => Promise<void>; refreshKey: number }) {
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const [instructions, setInstructions] = useState(project.instructions);
    const [files, setFiles] = useState<MemoryFile[]>([]);
    const [dir, setDir] = useState('');
    const [editing, setEditing] = useState<{ name: string; content: string; isNew?: boolean } | null>(null);
    const [tick, setTick] = useState(0);

    useEffect(() => setInstructions(project.instructions), [project.instructions]);
    useEffect(() => {
        unwrap(api.GET('/api/v1/projects/{id}/memory', { params: { path: { id: project.id } } }))
            .then((r) => {
                setFiles(r.files);
                setDir(r.dir);
            })
            .catch((e: Error) => notify(e.message, 'error'));
    }, [project.id, refreshKey, tick, notify]);

    const open = async (name: string) => {
        try {
            const f = await unwrap(api.GET('/api/v1/projects/{id}/memory/{name}', { params: { path: { id: project.id, name } } }));
            setEditing({ name, content: f.content });
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };
    const over = instructions.length > MAX_INSTRUCTIONS;

    return (
        <Stack spacing={2}>
            <Box>
                <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{t('projectSettings.instructions')}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>{t('projectSettings.instructionsHelp')}</Typography>
                <TextField multiline minRows={8} maxRows={18} fullWidth value={instructions} onChange={(e) => setInstructions(e.target.value)}
                    placeholder={t('projectSettings.instructionsPlaceholder')} error={over}
                    helperText={`${instructions.length.toLocaleString()} / ${MAX_INSTRUCTIONS.toLocaleString()}`}
                    slotProps={{ htmlInput: { style: { fontSize: 13.5, lineHeight: 1.6 } } }} />
                <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 1 }}>
                    <Button variant="contained" disabled={instructions === project.instructions || over} onClick={() => void patch({ instructions })}>{t('common.save')}</Button>
                </Box>
            </Box>
            <Divider />
            <Box>
                <Box sx={{ display: 'flex', alignItems: 'center' }}>
                    <Typography sx={{ fontWeight: 600, color: 'text.primary', flex: 1 }}>{t('projectSettings.memory')}</Typography>
                    <Button size="small" startIcon={<Add />} onClick={() => setEditing({ name: '', content: '', isNew: true })}>{t('projectSettings.newMemory')}</Button>
                </Box>
                <Typography variant="body2" color="text.secondary">{t('projectSettings.memoryHelp')}</Typography>
                {dir && <Typography variant="caption" sx={{ ...mono, color: 'text.disabled', display: 'block', mt: 0.5, wordBreak: 'break-all' }}>{dir}</Typography>}
                <Box sx={{ mt: 1 }}>
                    {files.map((f) => (
                        <Box key={f.name} role="button" tabIndex={0} onClick={() => void open(f.name)}
                            sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 0.75, px: 1, borderRadius: 1.5, '&:hover': { bgcolor: 'action.hover' } }}>
                            <Description sx={{ fontSize: 18, color: 'text.secondary' }} />
                            <Typography sx={{ ...mono, fontSize: 13.5, flex: 1, color: 'text.primary' }}>{f.name}</Typography>
                            <Typography variant="caption" color="text.secondary">{formatBytes(f.size)} · {timeAgo(f.updated_at, i18n.language)}</Typography>
                            {f.name !== 'MEMORY.md' && (
                                <IconButton size="small" aria-label={t('common.delete')} onClick={(e) => {
                                    e.stopPropagation();
                                    unwrap(api.DELETE('/api/v1/projects/{id}/memory/{name}', { params: { path: { id: project.id, name: f.name } } }))
                                        .then(() => setTick((x) => x + 1))
                                        .catch((err: Error) => notify(err.message, 'error'));
                                }}><Delete fontSize="small" /></IconButton>
                            )}
                        </Box>
                    ))}
                </Box>
            </Box>
            <TextEditDialog
                open={!!editing}
                title={editing?.isNew ? t('projectSettings.newMemory') : editing?.name ?? ''}
                nameField={editing?.isNew ? { label: t('projectSettings.memoryFileName'), initial: '' } : undefined}
                initialText={editing?.content ?? ''}
                onClose={() => setEditing(null)}
                onSubmit={async ({ text, name }) => {
                    const fileName = editing?.isNew ? (name.endsWith('.md') ? name : `${name}.md`) : editing!.name;
                    try {
                        await unwrap(api.PUT('/api/v1/projects/{id}/memory/{name}', { params: { path: { id: project.id, name: fileName } }, body: { content: text } }));
                        setTick((x) => x + 1);
                    } catch (e) {
                        notify((e as Error).message, 'error');
                        throw e;
                    }
                }}
            />
        </Stack>
    );
}

function Environment({ project, patch }: { project: Project; patch: (p: Patch) => Promise<void> }) {
    const { t } = useTranslation('projects');
    const [runtime, setRuntime] = useState<ProjectRuntime | null>(null);
    useEffect(() => {
        unwrap(api.GET('/api/v1/projects/runtime')).then(setRuntime).catch(() => {});
    }, []);
    const initial = project.repos.map((r) => ({ root: r.path, branch: r.base_branch }));
    const [repos, setRepos] = useState<RepoEntry[]>(initial);
    useEffect(() => setRepos(project.repos.map((r) => ({ root: r.path, branch: r.base_branch }))), [project.repos]);
    const dirty = JSON.stringify(repos.map((r) => r.root)) !== JSON.stringify(initial.map((r) => r.root));
    return (
        <Stack spacing={2}>
            <Box>
                <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{t('project.repos')}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>{t('projectSettings.reposHelp')}</Typography>
                <RepoField value={repos} onChange={setRepos} />
                <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 1 }}>
                    <Button variant="contained" disabled={!dirty} onClick={() => void patch({ repos: repos.map((r) => r.root) })}>{t('common.save')}</Button>
                </Box>
            </Box>
            <Divider />
            <Box>
                <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{t('projectSettings.runtime')}</Typography>
                {!runtime ? null : runtime.cli_path ? (
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                        {t('projectSettings.runtimeCli')} <Box component="span" sx={{ ...mono, color: 'text.primary' }}>{runtime.cli_path}</Box>
                    </Typography>
                ) : (
                    <Alert severity="warning" sx={{ mt: 1 }}>{t('projectSettings.runtimeMissing')} <Box component="code" sx={mono}>npm install -g @anthropic-ai/claude-code</Box></Alert>
                )}
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                    {runtime?.base_url
                        ? <>{t('projectSettings.runtimeVia')} <Box component="span" sx={{ ...mono, color: 'text.primary' }}>{runtime.base_url}</Box></>
                        : runtime && t('projectSettings.runtimeOwnLogin')}
                </Typography>
            </Box>
        </Stack>
    );
}

function UsageTab({ projectId }: { projectId: string }) {
    const { t } = useTranslation('projects');
    const [usage, setUsage] = useState<Usage | null>(null);
    useEffect(() => {
        unwrap(api.GET('/api/v1/projects/{id}/usage', { params: { path: { id: projectId } } })).then(setUsage).catch(() => {});
    }, [projectId]);
    if (!usage) return null;
    return (
        <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">{t('projectSettings.usageHelp')}</Typography>
            <Table size="small">
                <TableHead><TableRow><TableCell>{t('projectSettings.model')}</TableCell><TableCell align="right">{t('overview.threads')}</TableCell><TableCell align="right">In</TableCell><TableCell align="right">Out</TableCell></TableRow></TableHead>
                <TableBody>
                    {usage.by_model.map((m) => (
                        <TableRow key={m.model}><TableCell sx={mono}>{m.model || '—'}</TableCell><TableCell align="right">{m.threads}</TableCell><TableCell align="right">{formatTokens(m.input_tokens)}</TableCell><TableCell align="right">{formatTokens(m.output_tokens)}</TableCell></TableRow>
                    ))}
                </TableBody>
            </Table>
            <Table size="small">
                <TableHead><TableRow><TableCell>{t('projectSettings.thread')}</TableCell><TableCell align="right">{t('projectSettings.turns')}</TableCell><TableCell align="right">In</TableCell><TableCell align="right">Out</TableCell></TableRow></TableHead>
                <TableBody>
                    {usage.by_thread.map((u) => (
                        <TableRow key={u.thread_id}><TableCell>{u.title}</TableCell><TableCell align="right">{u.turns}</TableCell><TableCell align="right">{formatTokens(u.input_tokens)}</TableCell><TableCell align="right">{formatTokens(u.output_tokens)}</TableCell></TableRow>
                    ))}
                </TableBody>
            </Table>
        </Stack>
    );
}

interface Props {
    project: Project;
    open: boolean;
    tab: SettingsTab;
    onTab: (t: SettingsTab) => void;
    onClose: () => void;
    onChanged: (p: Project) => void;
    memoryKey: number;
}

export function ProjectSettingsDialog({ project, open, tab, onTab, onClose, onChanged, memoryKey }: Props) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const { onProjectsChanged } = useProjectsHost();
    const patch = async (body: Patch) => {
        try {
            const p = await unwrap(api.PATCH('/api/v1/projects/{id}', { params: { path: { id: project.id } }, body }));
            onChanged(p);
            onProjectsChanged();
            notify(t('settings.saved'), 'success');
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };
    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth slotProps={{ paper: { sx: { height: { md: '82vh' } } } }}>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', pb: 0 }}>
                <Box component="span" sx={{ flex: 1 }}>{t('projectSettings.title')}</Box>
                <IconButton onClick={onClose} aria-label={t('common.close')}><Close /></IconButton>
            </DialogTitle>
            <Tabs value={tab} onChange={(_, v) => onTab(v)} sx={{ px: 3, borderBottom: 1, borderColor: 'divider' }}>
                <Tab value="general" label={t('projectSettings.tabGeneral')} />
                <Tab value="memory" label={t('projectSettings.tabMemory')} />
                <Tab value="environment" label={t('projectSettings.tabEnvironment')} />
                <Tab value="usage" label={t('projectSettings.tabUsage')} />
            </Tabs>
            <DialogContent sx={{ pt: 2.5 }}>
                {tab === 'general' && <General project={project} patch={patch} />}
                {tab === 'memory' && <Memory project={project} patch={patch} refreshKey={memoryKey} />}
                {tab === 'environment' && <Environment project={project} patch={patch} />}
                {tab === 'usage' && <UsageTab projectId={project.id} />}
            </DialogContent>
        </Dialog>
    );
}
