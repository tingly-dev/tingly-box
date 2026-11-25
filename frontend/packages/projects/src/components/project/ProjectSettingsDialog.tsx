import { useEffect, useState, type ReactNode } from 'react';
import {
    Alert, Autocomplete, Box, Button, CircularProgress, Dialog, DialogContent, DialogTitle, Divider, IconButton, MenuItem, Stack, Switch,
    Tab, TableHead, TableRow, Tabs, TextField, Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@tingly/ui';
import { api, unwrap, type MemoryFile, type paths, type Project, type ProjectRuntime, type SandboxRuntime, type Usage } from '../../client';
import { TextEditDialog } from '../TextEditDialog';
import { Add, Close, Delete, Description, Duplicate } from '../icons';
import { ForkProjectDialog } from './ForkProjectDialog';
import { useModels } from '../../hooks/useModels';
import { useNotify } from '../../hooks/useNotify';
import { useProjectsHost } from '../../host/ProjectsHost';
import { formatBytes, formatTokens, timeAgo } from '../../utils/format';
import { usageEntries } from '../usage/usageData';
import { UsageCard, UsageHeadCell, UsageRows, UsageStatStrip } from '../usage/UsageViews';
import { quotaErrors, quotaOf, quotaValues, type MemoryUnit, type Quota } from './quota';
import { directoryPayload, primaryPath, projectDirectories, RepoField, type RepoEntry } from './RepoField';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

export type SettingsTab = 'general' | 'memory' | 'environment' | 'usage';

const MAX_INSTRUCTIONS = 16000;
// Until the server reports its limit (/projects/runtime).
const MAX_RUNNING_FALLBACK = 100;
// The groups of GitHub writes a project can let its threads do on their own,
// in the order the server lists them (project.GitHubGroups).
const GITHUB_GROUPS = ['publish', 'rewrite', 'others', 'close', 'merge'] as const;
const mono = { fontFamily: fontMono } as const;

type Patch = NonNullable<paths['/api/v1/projects/{id}']['patch']['requestBody']>['content']['application/json'];
type Mode = NonNullable<Patch['thread_permission_mode']>;
// The isolation axes, in the order the server lists them (sandbox.Axes).
const AXES = ['storage', 'network', 'exec', 'credentials', 'resources'] as const;

// The runtime threads run in: the ones this server ships, from
// /projects/runtime. Its level on each axis is shown under it, so what a
// choice means is spelled out, whatever the runtime.
function SandboxField({ value, onChange, runtimes, limited }: { value: string; onChange: (v: string) => void; runtimes: SandboxRuntime[]; limited: boolean }) {
    const { t } = useTranslation('projects');
    const name = (s: SandboxRuntime) => t(`projectSettings.sandboxes.${s.name}`, { defaultValue: s.name });
    const keyOf = (s: SandboxRuntime) => (s.name === 'host' ? '' : s.name);
    const current = runtimes.find((s) => keyOf(s) === value);
    // The project's own quotas, when set, are what its threads run under.
    const level = (a: (typeof AXES)[number]) => (a === 'resources' && limited && value !== '' ? 'limited' : current?.levels[a]);
    const levels = current
        ? AXES.map((a) => `${t(`projectSettings.axes.${a}`)}: ${t(`projectSettings.levels.${a}.${level(a)}`, { defaultValue: level(a) ?? '' })}`).join(' · ')
        : '';
    return (
        <TextField select label={t('projectSettings.sandbox')} value={value} onChange={(e) => onChange(e.target.value)}
            helperText={current && !current.available ? `${t('projectSettings.sandboxUnavailable')}: ${current.reason}` : levels}
            error={!!current && !current.available}>
            {runtimes.map((s) => (
                <MenuItem key={s.name} value={keyOf(s)} disabled={!s.available && keyOf(s) !== value}>
                    <Box>
                        <Typography>{name(s)}</Typography>
                        {!s.available && <Typography variant="body2" color="text.secondary">{s.reason}</Typography>}
                    </Box>
                </MenuItem>
            ))}
            {!current && <MenuItem value={value}>{value}</MenuItem>}
        </TextField>
    );
}

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
                    slotProps={{ ...params.slotProps, inputLabel: { shrink: true }, htmlInput: { ...params.slotProps?.htmlInput, style: { ...mono, fontSize: fontSizes.base } } }} />
            )}
        />
    );
}

/** One settings action: what it does on the left, its button on the right. */
function ActionRow({ title, help, action }: { title: string; help: string; action: ReactNode }) {
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <Box sx={{ flex: 1 }}>
                <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{title}</Typography>
                <Typography variant="body2" color="text.secondary">{help}</Typography>
            </Box>
            {action}
        </Box>
    );
}

// The per-thread limits: empty is no limit, no value is suggested. They apply
// inside a sandbox, so they wait on the runtime being one.
function QuotaFields({ value, onChange, enabled }: { value: Quota; onChange: (q: Quota) => void; enabled: boolean }) {
    const { t } = useTranslation('projects');
    const err = quotaErrors(value);
    const set = (patch: Partial<Quota>) => onChange({ ...value, ...patch });
    return (
        <Box role="group" aria-label={t('projectSettings.sandbox')}>
            <Stack direction="row" spacing={1.5} sx={{ alignItems: 'flex-start' }}>
                <TextField label={t('projectSettings.quota.memory')} value={value.memory} onChange={(e) => set({ memory: e.target.value })} disabled={!enabled}
                    error={err.memory} helperText={err.memory ? t('projectSettings.quota.memoryMin') : undefined} sx={{ flex: 1.4 }}
                    slotProps={{
                        htmlInput: { inputMode: 'decimal', 'aria-label': t('projectSettings.quota.memory') },
                        input: {
                            endAdornment: (
                                <TextField select variant="standard" value={value.unit} onChange={(e) => set({ unit: e.target.value as MemoryUnit })} disabled={!enabled}
                                    sx={{ minWidth: 64 }} slotProps={{ input: { disableUnderline: true } }}>
                                    <MenuItem value="MiB">MiB</MenuItem>
                                    <MenuItem value="GiB">GiB</MenuItem>
                                </TextField>
                            ),
                        },
                    }} />
                <TextField label={t('projectSettings.quota.processes')} value={value.tasks} onChange={(e) => set({ tasks: e.target.value })} disabled={!enabled}
                    error={err.tasks} helperText={err.tasks ? t('projectSettings.quota.processesMin') : undefined} sx={{ flex: 1 }}
                    slotProps={{ htmlInput: { inputMode: 'numeric', 'aria-label': t('projectSettings.quota.processes') } }} />
                <TextField label={t('projectSettings.quota.cpu')} value={value.cores} onChange={(e) => set({ cores: e.target.value })} disabled={!enabled}
                    error={err.cores} helperText={err.cores ? t('projectSettings.quota.cpuMin') : undefined} sx={{ flex: 1 }}
                    slotProps={{ htmlInput: { inputMode: 'decimal', 'aria-label': t('projectSettings.quota.cpu') } }} />
            </Stack>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>{enabled ? t('projectSettings.quota.help') : t('projectSettings.quota.hostHelp')}</Typography>
        </Box>
    );
}

function General({ project, patch, onFork }: { project: Project; patch: (p: Patch) => Promise<void>; onFork: () => void }) {
    const { t } = useTranslation('projects');
    const { session } = useProjectsHost();
    const [name, setName] = useState(project.name);
    const [goal, setGoal] = useState(project.goal);
    const [coord, setCoord] = useState(project.coordinator_model);
    const [threadModel, setThreadModel] = useState(project.thread_model);
    const [mode, setMode] = useState<Mode>((project.thread_permission_mode || 'auto') as Mode);
    const [sandbox, setSandbox] = useState<string>(project.thread_sandbox || '');
    const savedGithub = GITHUB_GROUPS.filter((g) => (project.github_auto ?? []).includes(g));
    const [github, setGithub] = useState<string[]>(savedGithub);
    const [quota, setQuota] = useState<Quota>(quotaOf(project));
    const [maxRunning, setMaxRunning] = useState(String(project.max_running_threads));
    const [maxLimit, setMaxLimit] = useState(MAX_RUNNING_FALLBACK);
    const [runtimes, setRuntimes] = useState<SandboxRuntime[]>([]);
    useEffect(() => {
        unwrap(api.GET('/api/v1/projects/runtime')).then((r) => {
            setMaxLimit(r.max_running_threads_limit);
            setRuntimes(r.sandboxes ?? []);
        }).catch(() => {});
    }, []);
    const runningValue = /^\d+$/.test(maxRunning.trim()) ? Number(maxRunning) : NaN;
    const runningInvalid = !(runningValue >= 1 && runningValue <= maxLimit);
    const quotaInvalid = Object.values(quotaErrors(quota)).some(Boolean);
    const quotaNow = quotaValues(quota);
    const quotaDirty = quotaNow.thread_memory_bytes !== (project.thread_memory_bytes ?? 0) || quotaNow.thread_tasks !== (project.thread_tasks ?? 0) || quotaNow.thread_cpu !== (project.thread_cpu ?? 0);
    const dirty = (quotaDirty && !quotaInvalid) || (!runningInvalid && runningValue !== project.max_running_threads) || name !== project.name || goal !== project.goal || coord !== project.coordinator_model || threadModel !== project.thread_model || mode !== (project.thread_permission_mode || 'auto') || sandbox !== (project.thread_sandbox || '') || github.join() !== savedGithub.join();

    return (
        <Stack spacing={2.5}>
            <TextField label={t('project.name')} value={name} onChange={(e) => setName(e.target.value)} fullWidth />
            <TextField label={t('project.goal')} value={goal} onChange={(e) => setGoal(e.target.value)} placeholder={t('project.goalPlaceholder')} helperText={t('project.goalHelp')} fullWidth />
            {!session?.launchPickers && <ModelField label={t('projectSettings.coordinatorModel')} value={coord} onChange={setCoord} help={t('projectSettings.coordinatorModelHelp')} />}
            {!session?.launchPickers && <ModelField label={t('projectSettings.threadModel')} value={threadModel} onChange={setThreadModel} help={t('projectSettings.threadModelHelp')} />}
            <TextField select label={t('projectSettings.permissionMode')} value={mode} onChange={(e) => setMode(e.target.value as Mode)} helperText={t(`projectSettings.mode.${mode}Help`)}>
                {['acceptEdits', 'default', 'auto', 'bypassPermissions'].map((m) => (
                    <MenuItem key={m} value={m}><Box component="span" sx={{ ...mono, fontSize: fontSizes.base, mr: 1 }}>{m}</Box>{t(`projectSettings.mode.${m}`)}</MenuItem>
                ))}
            </TextField>
            {runtimes.length > 0 && <SandboxField value={sandbox} onChange={setSandbox} runtimes={runtimes} limited={Object.values(quotaNow).some((v) => v > 0)} />}
            {runtimes.length > 0 && <QuotaFields value={quota} onChange={setQuota} enabled={sandbox !== ''} />}
            <Box role="group" aria-label={t('projectSettings.githubAuto')}>
                <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{t('projectSettings.githubAuto')}</Typography>
                <Stack spacing={1.5} sx={{ mt: 1.5 }}>
                    {GITHUB_GROUPS.map((g) => (
                        <Box key={g} sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography sx={{ color: 'text.primary' }}>{t(`projectSettings.githubGroups.${g}`)}</Typography>
                                <Typography variant="body2" color="text.secondary">{t(`projectSettings.githubGroups.${g}Help`)}</Typography>
                            </Box>
                            <Switch checked={github.includes(g)} onChange={(_, on) => setGithub(GITHUB_GROUPS.filter((x) => (x === g ? on : github.includes(x))))}
                                slotProps={{ input: { 'aria-label': t(`projectSettings.githubGroups.${g}`) } }} />
                        </Box>
                    ))}
                </Stack>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>{t('projectSettings.githubAutoFoot')}</Typography>
            </Box>
            <TextField label={t('projectSettings.maxRunning')} value={maxRunning} onChange={(e) => setMaxRunning(e.target.value)} error={runningInvalid}
                helperText={runningInvalid ? t('projectSettings.maxRunningInvalid', { max: maxLimit }) : t('projectSettings.maxRunningHelp')}
                slotProps={{ htmlInput: { inputMode: 'numeric', 'aria-label': t('projectSettings.maxRunning') } }} />
            <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                <Button variant="contained" disabled={!dirty || !name.trim() || runningInvalid || quotaInvalid} onClick={() => void patch({ name, goal, coordinator_model: coord, thread_model: threadModel, thread_permission_mode: mode, thread_sandbox: sandbox, ...quotaNow, github_auto: github, max_running_threads: runningValue })}>{t('common.save')}</Button>
            </Box>
            <Divider />
            <ActionRow
                title={project.paused ? t('projectSettings.resume') : t('projectSettings.pause')}
                help={t('projectSettings.pauseHelp')}
                action={<Button variant="outlined" onClick={() => void patch({ paused: !project.paused })}>{project.paused ? t('projectSettings.resume') : t('projectSettings.pause')}</Button>}
            />
            <ActionRow
                title={project.archived_at ? t('projectSettings.unarchive') : t('projectSettings.archive')}
                help={t('projectSettings.archiveHelp')}
                action={<Button variant="outlined" onClick={() => void patch({ archived: !project.archived_at })}>{project.archived_at ? t('projectSettings.unarchive') : t('projectSettings.archive')}</Button>}
            />
            {project.type !== 'folder' && (
                <>
                    <Divider />
                    <ActionRow
                        title={t('projectSettings.fork')}
                        help={t('projectSettings.forkHelp')}
                        action={<Button variant="outlined" startIcon={<Duplicate />} onClick={onFork}>{t('projectSettings.fork')}</Button>}
                    />
                </>
            )}
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
                    slotProps={{ htmlInput: { style: { fontSize: fontSizes.base, lineHeight: 1.6 } } }} />
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
                            sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 0.75, px: 1, borderRadius: radius.card, '&:hover': { bgcolor: 'action.hover' } }}>
                            <Description sx={{ fontSize: 18, color: 'text.secondary' }} />
                            <Typography sx={{ ...mono, fontSize: fontSizes.base, flex: 1, color: 'text.primary' }}>{f.name}</Typography>
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
    const initial = projectDirectories(project);
    const [repos, setRepos] = useState<RepoEntry[]>(initial);
    const [primary, setPrimary] = useState(primaryPath(project));
    useEffect(() => { setRepos(projectDirectories(project)); setPrimary(primaryPath(project)); }, [project.repos, project.primary_repo_path]);
    const dirty = JSON.stringify(directoryPayload(repos, primary)) !== JSON.stringify(directoryPayload(initial, primaryPath(project)));
    return (
        <Stack spacing={2}>
            <Box>
                <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{t('project.repos')}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>{t('projectSettings.reposHelp')}</Typography>
                <RepoField value={repos} onChange={setRepos} primary={primary} onPrimaryChange={setPrimary} />
                <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 1 }}>
                    <Button variant="contained" disabled={!dirty} onClick={() => void patch(directoryPayload(repos, primary))}>{t('common.save')}</Button>
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
    if (!usage) {
        return (
            <Box sx={{ display: 'grid', placeItems: 'center', py: 6 }}>
                <CircularProgress size={24} aria-label={t('projectSettings.usage.loading')} />
            </Box>
        );
    }
    const { models, threads, totals } = usageEntries(usage);
    if (models.length === 0 && threads.length === 0) {
        return <EmptyState compact title={t('projectSettings.usage.emptyTitle')} description={t('projectSettings.usage.emptyBody')} />;
    }
    // Compact strings carry the tile and cells; the exact figures ride on
    // title attributes for the hover.
    const exact = (n: number) => ({ value: formatTokens(n), title: String(n) });
    return (
        <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">{t('projectSettings.usage.help')}</Typography>
            <UsageStatStrip stats={[
                { label: t('projectSettings.usage.totalTokens'), ...exact(totals.total) },
                { label: t('projectSettings.usage.input'), ...exact(totals.input) },
                { label: t('projectSettings.usage.output'), ...exact(totals.output) },
                { label: t('projectSettings.usage.threads'), value: String(totals.threads) },
            ]} />
            <UsageCard title={t('projectSettings.usage.models')} count={models.length} maxHeight={280}>
                <TableHead>
                    <TableRow>
                        <UsageHeadCell label={t('projectSettings.usage.model')} />
                        <UsageHeadCell label={t('projectSettings.usage.threads')} numeric />
                        <UsageHeadCell label={t('projectSettings.usage.share')} numeric />
                        <UsageHeadCell label={t('projectSettings.usage.in')} numeric />
                        <UsageHeadCell label={t('projectSettings.usage.out')} numeric />
                    </TableRow>
                </TableHead>
                <UsageRows rows={models} countTitle={t('projectSettings.usage.threads')} formatTokens={formatTokens} />
            </UsageCard>
            <UsageCard title={t('projectSettings.usage.threads')} count={threads.length} maxHeight={320}>
                <TableHead>
                    <TableRow>
                        <UsageHeadCell label={t('projectSettings.usage.thread')} />
                        <UsageHeadCell label={t('projectSettings.usage.turns')} numeric />
                        <UsageHeadCell label={t('projectSettings.usage.share')} numeric />
                        <UsageHeadCell label={t('projectSettings.usage.in')} numeric />
                        <UsageHeadCell label={t('projectSettings.usage.out')} numeric />
                    </TableRow>
                </TableHead>
                <UsageRows rows={threads} countTitle={t('projectSettings.usage.turns')} formatTokens={formatTokens} />
            </UsageCard>
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
    const [forking, setForking] = useState(false);
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
            <Tabs value={tab} onChange={(_, v) => onTab(v)} variant="scrollable" scrollButtons="auto" sx={{ px: 3, borderBottom: 1, borderColor: 'divider' }}>
                <Tab value="general" label={t('projectSettings.tabGeneral')} />
                <Tab value="memory" label={t('projectSettings.tabMemory')} />
                <Tab value="environment" label={t('projectSettings.tabEnvironment')} />
                <Tab value="usage" label={t('projectSettings.tabUsage')} />
            </Tabs>
            <DialogContent sx={{ pt: 2.5 }}>
                {tab === 'general' && <General project={project} patch={patch} onFork={() => setForking(true)} />}
                {tab === 'memory' && <Memory project={project} patch={patch} refreshKey={memoryKey} />}
                {tab === 'environment' && <Environment project={project} patch={patch} />}
                {tab === 'usage' && <UsageTab projectId={project.id} />}
            </DialogContent>
            {/* Forking can only be armed from General's guarded row above. */}
            <ForkProjectDialog project={project} open={forking} onClose={() => setForking(false)} onForked={onClose} />
        </Dialog>
    );
}
