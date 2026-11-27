import { useCallback, useEffect, useRef, useState, type ReactNode, type Ref } from 'react';
import {
    Alert, Autocomplete, Box, Button, Chip, CircularProgress, Dialog, DialogContent, DialogTitle, Divider, IconButton, MenuItem, Stack, Switch,
    Tab, TableHead, TableRow, Tabs, TextField, Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { ConfirmDialog, EmptyState } from '@tingly/ui';
import { api, unwrap, type MemoryFile, type paths, type Project, type ProjectRuntime, type SandboxRuntime, type Usage } from '../../client';
import { TextEditDialog } from '../TextEditDialog';
import { EffortPicker } from '../EffortPicker';
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

export type SettingsTab = 'basic' | 'runtime' | 'memory' | 'usage';

const MAX_INSTRUCTIONS = 16000;
// Until the server reports its limit (/projects/runtime).
const MAX_RUNNING_FALLBACK = 100;
// The groups of GitHub writes a project can let its threads do on their own,
// in the order the server lists them (project.GitHubGroups).
const GITHUB_GROUPS = ['publish', 'rewrite', 'others', 'close', 'merge'] as const;
// The three shortcuts above the five switches. They set the same field.
const GITHUB_PRESETS = [
    { key: 'askAll', groups: [] as string[] },
    { key: 'daily', groups: ['publish'] },
    { key: 'autonomous', groups: [...GITHUB_GROUPS] },
] as const;
const mono = { fontFamily: fontMono } as const;

type Patch = NonNullable<paths['/api/v1/projects/{id}']['patch']['requestBody']>['content']['application/json'];
type Mode = NonNullable<Patch['thread_permission_mode']>;
type Effort = '' | 'low' | 'medium' | 'high' | 'max';
// The isolation axes, in the order the server lists them (sandbox.Axes).
const AXES = ['storage', 'network', 'exec', 'credentials', 'resources'] as const;

const sameGroups = (a: readonly string[], b: readonly string[]) => [...a].sort().join() === [...b].sort().join();

// The runtime threads run in: the ones this server ships, from
// /projects/runtime. Its level on each axis is shown under it, so what a
// choice means is spelled out, whatever the runtime.
function SandboxField({ value, onChange, runtimes, inputRef }: { value: string; onChange: (v: string) => void; runtimes: SandboxRuntime[]; inputRef?: Ref<HTMLInputElement> }) {
    const { t } = useTranslation('projects');
    const name = (s: SandboxRuntime) => t(`projectSettings.sandboxes.${s.name}`, { defaultValue: s.name });
    const current = runtimes.find((s) => s.name === value);
    return (
        <TextField select label={t('projectSettings.sandbox')} value={value} onChange={(e) => onChange(e.target.value)} inputRef={inputRef}
            helperText={current && !current.available ? `${t('projectSettings.sandboxUnavailable')}: ${current.reason}` : undefined}
            error={!!current && !current.available}>
            {runtimes.map((s) => (
                <MenuItem key={s.name} value={s.name} disabled={!s.available && s.name !== value}>
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

// What the chosen runtime gives on each axis, one line each. The project's
// own quotas turn "resources" into "limited" without a second control.
function SandboxAxes({ current, limited, sandbox }: { current?: SandboxRuntime; limited: boolean; sandbox: string }) {
    const { t } = useTranslation('projects');
    if (!current) return null;
    const level = (a: (typeof AXES)[number]) => (a === 'resources' && limited && sandbox !== 'host' ? 'limited' : current.levels[a]);
    return (
        <Box sx={{ border: 1, borderColor: 'divider', borderRadius: radius.card, overflow: 'hidden' }}>
            {AXES.map((a, i) => (
                <Box key={a} sx={{ display: 'flex', gap: 2, px: 1.5, py: 1, borderTop: i === 0 ? 0 : 1, borderColor: 'divider' }}>
                    <Typography variant="body2" color="text.secondary" sx={{ width: 56, flexShrink: 0 }}>{t(`projectSettings.axes.${a}`)}</Typography>
                    <Typography variant="body2" sx={{ color: 'text.primary' }}>{t(`projectSettings.levels.${a}.${level(a)}`, { defaultValue: level(a) ?? '' })}</Typography>
                </Box>
            ))}
        </Box>
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
            <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{title}</Typography>
                <Typography variant="body2" color="text.secondary">{help}</Typography>
            </Box>
            {action}
        </Box>
    );
}

// The per-thread limits: empty is no limit, no value is suggested. They apply
// inside a sandbox, so they wait on the runtime being one.
function QuotaFields({ value, onChange, enabled, onEnableSandbox }: { value: Quota; onChange: (q: Quota) => void; enabled: boolean; onEnableSandbox?: () => void }) {
    const { t } = useTranslation('projects');
    const err = quotaErrors(value);
    const set = (patch: Partial<Quota>) => onChange({ ...value, ...patch });
    return (
        <Box role="group" aria-label={t('projectSettings.sandbox')}>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} sx={{ alignItems: 'flex-start' }}>
                <TextField label={t('projectSettings.quota.memory')} value={value.memory} onChange={(e) => set({ memory: e.target.value })} disabled={!enabled}
                    error={err.memory} helperText={err.memory ? t('projectSettings.quota.memoryMin') : undefined} sx={{ flex: 1.4, width: '100%' }}
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
                    error={err.tasks} helperText={err.tasks ? t('projectSettings.quota.processesMin') : undefined} sx={{ flex: 1, width: '100%' }}
                    slotProps={{ htmlInput: { inputMode: 'numeric', 'aria-label': t('projectSettings.quota.processes') } }} />
                <TextField label={t('projectSettings.quota.cpu')} value={value.cores} onChange={(e) => set({ cores: e.target.value })} disabled={!enabled}
                    error={err.cores} helperText={err.cores ? t('projectSettings.quota.cpuMin') : undefined} sx={{ flex: 1, width: '100%' }}
                    slotProps={{ htmlInput: { inputMode: 'decimal', 'aria-label': t('projectSettings.quota.cpu') } }} />
            </Stack>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap', mt: 0.5 }}>
                <Typography variant="caption" color="text.secondary">{enabled ? t('projectSettings.quota.help') : t('projectSettings.quota.hostHelp')}</Typography>
                {!enabled && onEnableSandbox && <Button size="small" onClick={onEnableSandbox}>{t('projectSettings.switchToSandbox')}</Button>}
            </Box>
        </Box>
    );
}

function Section({ title, help, action, children }: { title: ReactNode; help?: ReactNode; action?: ReactNode; children: ReactNode }) {
    return (
        <Stack spacing={1.25}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography sx={{ fontWeight: 600, color: 'text.primary', flex: 1 }}>{title}</Typography>
                {action}
            </Box>
            {help && <Typography variant="body2" color="text.secondary" sx={{ mt: '-0.5rem' }}>{help}</Typography>}
            {children}
        </Stack>
    );
}

// The footer every tab shares: one save for this tab, and a reset that never
// touches the project until Save is pressed.
function SaveBar({ dirty, canSave, onSave, onDiscard }: { dirty: boolean; canSave: boolean; onSave: () => void; onDiscard: () => void }) {
    const { t } = useTranslation('projects');
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, pt: 1.5, borderTop: 1, borderColor: 'divider', bgcolor: 'background.paper', flexShrink: 0 }}>
            <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>{dirty ? t('projectSettings.unsavedChanges') : ''}</Typography>
            <Button size="small" disabled={!dirty} onClick={onDiscard}>{t('projectSettings.discard')}</Button>
            <Button variant="contained" disabled={!dirty || !canSave} onClick={onSave}>{t('common.save')}</Button>
        </Box>
    );
}

// A tab's body scrolls; its save bar stays at the bottom of the dialog.
function TabPage({ children, footer }: { children: ReactNode; footer: ReactNode }) {
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', pr: 0.5, pt: 1, pb: 2 }}>{children}</Box>
            {footer}
        </Box>
    );
}

function ModelEffort({ label, help, value, onChange, effort, onEffort, launch }: { label: string; help: string; value: string; onChange: (v: string) => void; effort: Effort; onEffort: (v: Effort) => void; launch?: ReactNode }) {
    const { t } = useTranslation('projects');
    return (
        <Stack spacing={0.5} sx={{ flex: 1, minWidth: 0 }}>
            {launch ? (
                <Stack spacing={0.25}>
                    <Typography variant="caption" color="text.secondary">{label}</Typography>
                    {launch}
                    <Typography variant="caption" color="text.secondary">{help}</Typography>
                </Stack>
            ) : <ModelField label={label} value={value} onChange={onChange} help={help} />}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography variant="caption" color="text.secondary">{t('projectSettings.effort')}</Typography>
                <EffortPicker value={effort} onChange={(v) => onEffort(v as Effort)} />
            </Box>
        </Stack>
    );
}

function Basic({ project, patch, runtime, runtimeError, maxLimit, onDirtyChange, onFork }: {
    project: Project; patch: (p: Patch) => Promise<void>; runtime: ProjectRuntime | null; runtimeError: boolean;
    maxLimit: number; onDirtyChange: (dirty: boolean) => void; onFork: () => void;
}) {
    const { t } = useTranslation('projects');
    const { session } = useProjectsHost();
    const [name, setName] = useState(project.name);
    const [goal, setGoal] = useState(project.goal);
    const [coord, setCoord] = useState(project.coordinator_model);
    const [threadModel, setThreadModel] = useState(project.thread_model);
    const [coordProfile, setCoordProfile] = useState(project.coordinator_profile || '');
    const [threadProfile, setThreadProfile] = useState(project.thread_profile || '');
    const [coordEffort, setCoordEffort] = useState<Effort>((project.coordinator_effort || '') as Effort);
    const [threadEffort, setThreadEffort] = useState<Effort>((project.thread_effort || '') as Effort);
    const [mode, setMode] = useState<Mode>((project.thread_permission_mode || 'auto') as Mode);
    const savedGithub = GITHUB_GROUPS.filter((g) => (project.github_auto ?? []).includes(g));
    const [github, setGithub] = useState<string[]>(savedGithub);
    const [maxRunning, setMaxRunning] = useState(String(project.max_running_threads));
    const initialRepos = projectDirectories(project);
    const [repos, setRepos] = useState<RepoEntry[]>(initialRepos);
    const [primary, setPrimary] = useState(primaryPath(project));

    useEffect(() => setName(project.name), [project.name]);
    useEffect(() => setGoal(project.goal), [project.goal]);
    useEffect(() => setCoord(project.coordinator_model), [project.coordinator_model]);
    useEffect(() => setThreadModel(project.thread_model), [project.thread_model]);
    useEffect(() => setCoordProfile(project.coordinator_profile || ''), [project.coordinator_profile]);
    useEffect(() => setThreadProfile(project.thread_profile || ''), [project.thread_profile]);
    useEffect(() => setCoordEffort((project.coordinator_effort || '') as Effort), [project.coordinator_effort]);
    useEffect(() => setThreadEffort((project.thread_effort || '') as Effort), [project.thread_effort]);
    useEffect(() => setMode((project.thread_permission_mode || 'auto') as Mode), [project.thread_permission_mode]);
    useEffect(() => setGithub(GITHUB_GROUPS.filter((g) => (project.github_auto ?? []).includes(g))), [project.github_auto]);
    useEffect(() => setMaxRunning(String(project.max_running_threads)), [project.max_running_threads]);
    useEffect(() => { setRepos(projectDirectories(project)); setPrimary(primaryPath(project)); }, [project.repos, project.primary_repo_path]);

    const runningValue = /^\d+$/.test(maxRunning.trim()) ? Number(maxRunning) : NaN;
    const runningInvalid = !(runningValue >= 1 && runningValue <= maxLimit);
    const dirPayload = directoryPayload(repos, primary);
    const dirDirty = JSON.stringify(dirPayload) !== JSON.stringify(directoryPayload(projectDirectories(project), primaryPath(project)));
    const dirty = name !== project.name || goal !== project.goal || coord !== project.coordinator_model || threadModel !== project.thread_model
        || coordEffort !== (project.coordinator_effort || '') || threadEffort !== (project.thread_effort || '')
        || mode !== (project.thread_permission_mode || 'auto') || github.join() !== savedGithub.join()
        || runningValue !== project.max_running_threads || dirDirty
        || (!!session?.launchPickers && (coordProfile !== (project.coordinator_profile || '') || threadProfile !== (project.thread_profile || '')));
    const canSave = !!name.trim() && !runningInvalid;

    useEffect(() => { onDirtyChange(dirty); }, [dirty, onDirtyChange]);

    const discard = () => {
        setName(project.name); setGoal(project.goal); setCoord(project.coordinator_model); setThreadModel(project.thread_model);
        setCoordProfile(project.coordinator_profile || ''); setThreadProfile(project.thread_profile || '');
        setCoordEffort((project.coordinator_effort || '') as Effort); setThreadEffort((project.thread_effort || '') as Effort);
        setMode((project.thread_permission_mode || 'auto') as Mode); setGithub(savedGithub);
        setMaxRunning(String(project.max_running_threads)); setRepos(projectDirectories(project)); setPrimary(primaryPath(project));
    };

    const save = () => {
        const body: Patch = {
            name, goal, coordinator_model: coord, thread_model: threadModel,
            coordinator_effort: coordEffort, thread_effort: threadEffort,
            thread_permission_mode: mode, github_auto: github, max_running_threads: runningValue,
            ...dirPayload,
        };
        if (session?.launchPickers) {
            body.coordinator_profile = coordProfile;
            body.thread_profile = threadProfile;
        }
        void patch(body);
    };

    const launch = (value: { profile: string; model: string }, onChange: (next: { profile: string; model: string }) => void) =>
        session?.launchPickers?.({ value, onChange });

    return (
        <TabPage footer={<SaveBar dirty={dirty} canSave={canSave} onSave={save} onDiscard={discard} />}>
            <Stack spacing={2.5}>
                <TextField label={t('project.name')} value={name} onChange={(e) => setName(e.target.value)} fullWidth />
                <TextField label={t('project.goal')} value={goal} onChange={(e) => setGoal(e.target.value)} placeholder={t('project.goalPlaceholder')} helperText={t('project.goalHelp')} fullWidth />
                <Divider />

                <Section title={t('projectSettings.threadDefaults')} help={t('projectSettings.threadDefaultsHelp')}>
                    <TextField select label={t('projectSettings.permissionMode')} value={mode} onChange={(e) => setMode(e.target.value as Mode)} helperText={t(`projectSettings.mode.${mode}Help`)}>
                        {['acceptEdits', 'default', 'auto', 'bypassPermissions'].map((m) => (
                            <MenuItem key={m} value={m}><Box component="span" sx={{ ...mono, fontSize: fontSizes.base, mr: 1 }}>{m}</Box>{t(`projectSettings.mode.${m}`)}</MenuItem>
                        ))}
                    </TextField>
                    {mode === 'bypassPermissions' && <Alert severity="warning">{t('projectSettings.bypassWarning')}</Alert>}
                    <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} sx={{ alignItems: { xs: 'stretch', md: 'flex-start' } }}>
                        <ModelEffort label={t('projectSettings.coordinatorModel')} help={t('projectSettings.coordinatorModelHelp')}
                            value={coord} onChange={setCoord} effort={coordEffort} onEffort={setCoordEffort}
                            launch={launch({ profile: coordProfile, model: coord }, (next) => { setCoordProfile(next.profile); setCoord(next.model); })} />
                        <ModelEffort label={t('projectSettings.threadModel')} help={t('projectSettings.threadModelHelp')}
                            value={threadModel} onChange={setThreadModel} effort={threadEffort} onEffort={setThreadEffort}
                            launch={launch({ profile: threadProfile, model: threadModel }, (next) => { setThreadProfile(next.profile); setThreadModel(next.model); })} />
                    </Stack>
                    <TextField label={t('projectSettings.maxRunning')} value={maxRunning} onChange={(e) => setMaxRunning(e.target.value)} error={runningInvalid}
                        helperText={runningInvalid ? t('projectSettings.maxRunningInvalid', { max: maxLimit }) : t('projectSettings.maxRunningHelp')}
                        slotProps={{ htmlInput: { inputMode: 'numeric', 'aria-label': t('projectSettings.maxRunning') } }} sx={{ maxWidth: 280 }} />
                </Section>
                <Divider />

                <Section title={t('projectSettings.githubAuto')}
                    action={<Typography variant="caption" color="text.secondary">{t('projectSettings.githubPresetCount', { on: github.length, total: GITHUB_GROUPS.length })}</Typography>}>
                    <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap' }}>
                        {GITHUB_PRESETS.map((p) => {
                            const active = sameGroups(github, p.groups);
                            return <Chip key={p.key} label={t(`projectSettings.githubPresets.${p.key}`)} color={active ? 'primary' : 'default'}
                                variant={active ? 'filled' : 'outlined'} onClick={() => setGithub(Array.from(p.groups as readonly string[]))} />;
                        })}
                    </Stack>
                    <Stack spacing={1.5}>
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
                    <Typography variant="caption" color="text.secondary">{t('projectSettings.githubAutoFoot')}</Typography>
                </Section>
                <Divider />

                <Section title={t('projectSettings.workingDir')} help={t('projectSettings.reposHelp')}>
                    <RepoField value={repos} onChange={setRepos} primary={primary} onPrimaryChange={setPrimary} />
                </Section>
                <Divider />

                <Section title={t('projectSettings.runtimeDiagnostics')} help={t('projectSettings.runtimeDiagnosticsHelp')}>
                    {runtime ? runtime.cli_path ? (
                        <Typography variant="body2" color="text.secondary">
                            {t('projectSettings.runtimeCli')} <Box component="span" sx={{ ...mono, color: 'text.primary' }}>{runtime.cli_path}</Box>
                        </Typography>
                    ) : (
                        <Alert severity="warning">{t('projectSettings.runtimeMissing')} <Box component="code" sx={mono}>npm install -g @anthropic-ai/claude-code</Box></Alert>
                    ) : (
                        <Typography variant="body2" color="text.secondary">{runtimeError ? t('projectSettings.runtimeLoadFailed') : t('projectSettings.runtimeLoading')}</Typography>
                    )}
                    {runtime && (
                        <Typography variant="body2" color="text.secondary">
                            {runtime.base_url
                                ? <>{t('projectSettings.runtimeVia')} <Box component="span" sx={{ ...mono, color: 'text.primary' }}>{runtime.base_url}</Box></>
                                : t('projectSettings.runtimeOwnLogin')}
                        </Typography>
                    )}
                </Section>
                <Divider />

                <Section title={t('projectSettings.projectManagement')} help={t('projectSettings.projectManagementHelp')}>
                    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: radius.card, p: 1.5, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                        <ActionRow title={project.paused ? t('projectSettings.resume') : t('projectSettings.pause')} help={t('projectSettings.pauseHelp')}
                            action={<Button variant="outlined" onClick={() => void patch({ paused: !project.paused })}>{project.paused ? t('projectSettings.resume') : t('projectSettings.pause')}</Button>} />
                        <Divider />
                        <ActionRow title={project.archived_at ? t('projectSettings.unarchive') : t('projectSettings.archive')} help={t('projectSettings.archiveHelp')}
                            action={<Button variant="outlined" color="error" onClick={() => void patch({ archived: !project.archived_at })}>{project.archived_at ? t('projectSettings.unarchive') : t('projectSettings.archive')}</Button>} />
                        {project.type !== 'folder' && (
                            <>
                                <Divider />
                                <ActionRow title={t('projectSettings.fork')} help={t('projectSettings.forkHelp')}
                                    action={<Button variant="outlined" startIcon={<Duplicate />} onClick={onFork}>{t('projectSettings.fork')}</Button>} />
                            </>
                        )}
                    </Box>
                </Section>
            </Stack>
        </TabPage>
    );
}

function Runtime({ project, patch, runtime, runtimeError, onDirtyChange }: {
    project: Project; patch: (p: Patch) => Promise<void>; runtime: ProjectRuntime | null; runtimeError: boolean; onDirtyChange: (dirty: boolean) => void;
}) {
    const { t } = useTranslation('projects');
    const [sandbox, setSandbox] = useState<string>(project.thread_sandbox || 'host');
    const [quota, setQuota] = useState<Quota>(quotaOf(project));
    const selectRef = useRef<HTMLInputElement>(null);

    useEffect(() => setSandbox(project.thread_sandbox || 'host'), [project.thread_sandbox]);
    useEffect(() => setQuota(quotaOf(project)), [project.thread_memory_bytes, project.thread_tasks, project.thread_cpu]);

    const runtimes = runtime?.sandboxes ?? [];
    const current = runtimes.find((s) => s.name === sandbox);
    const quotaNow = quotaValues(quota);
    const quotaInvalid = Object.values(quotaErrors(quota)).some(Boolean);
    const limited = Object.values(quotaNow).some((v) => v > 0);
    const dirty = sandbox !== (project.thread_sandbox || 'host')
        || quotaNow.thread_memory_bytes !== (project.thread_memory_bytes ?? 0)
        || quotaNow.thread_tasks !== (project.thread_tasks ?? 0)
        || quotaNow.thread_cpu !== (project.thread_cpu ?? 0);

    useEffect(() => { onDirtyChange(dirty); }, [dirty, onDirtyChange]);

    const discard = () => { setSandbox(project.thread_sandbox || 'host'); setQuota(quotaOf(project)); };
    const save = () => void patch({ thread_sandbox: sandbox === 'host' ? '' : sandbox, ...quotaNow });

    return (
        <TabPage footer={<SaveBar dirty={dirty} canSave={!quotaInvalid} onSave={save} onDiscard={discard} />}>
            <Stack spacing={2.5}>
                <Section title={t('projectSettings.sandbox')} help={t('projectSettings.sandboxHelp')}>
                    {runtime ? (runtimes.length > 0 ? (
                        <SandboxField value={sandbox} onChange={setSandbox} runtimes={runtimes} inputRef={selectRef} />
                    ) : (
                        <Alert severity="warning">{t('projectSettings.sandboxNone')}</Alert>
                    )) : (
                        <Typography variant="body2" color="text.secondary">{runtimeError ? t('projectSettings.runtimeLoadFailed') : t('projectSettings.runtimeLoading')}</Typography>
                    )}
                    <SandboxAxes current={current} limited={limited} sandbox={sandbox} />
                </Section>
                <Divider />
                <Section title={t('projectSettings.quotaTitle')}>
                    <QuotaFields value={quota} onChange={setQuota} enabled={sandbox !== 'host'}
                        onEnableSandbox={runtimes.length > 0 ? () => { selectRef.current?.focus(); selectRef.current?.scrollIntoView({ block: 'center' }); } : undefined} />
                </Section>
            </Stack>
        </TabPage>
    );
}

function Memory({ project, patch, refreshKey, onDirtyChange }: { project: Project; patch: (p: Patch) => Promise<void>; refreshKey: number; onDirtyChange: (dirty: boolean) => void }) {
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

    const dirty = instructions !== project.instructions;
    const over = instructions.length > MAX_INSTRUCTIONS;
    useEffect(() => { onDirtyChange(dirty); }, [dirty, onDirtyChange]);

    const open = async (name: string) => {
        try {
            const f = await unwrap(api.GET('/api/v1/projects/{id}/memory/{name}', { params: { path: { id: project.id, name } } }));
            setEditing({ name, content: f.content });
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    return (
        <>
            <TabPage footer={<SaveBar dirty={dirty} canSave={!over} onSave={() => void patch({ instructions })} onDiscard={() => setInstructions(project.instructions)} />}>
                <Stack spacing={2}>
                    <Box>
                        <Typography sx={{ fontWeight: 600, color: 'text.primary' }}>{t('projectSettings.instructions')}</Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>{t('projectSettings.instructionsHelp')}</Typography>
                        <TextField multiline minRows={8} maxRows={18} fullWidth value={instructions} onChange={(e) => setInstructions(e.target.value)}
                            placeholder={t('projectSettings.instructionsPlaceholder')} error={over}
                            helperText={`${instructions.length.toLocaleString()} / ${MAX_INSTRUCTIONS.toLocaleString()}`}
                            slotProps={{ htmlInput: { style: { fontSize: fontSizes.base, lineHeight: 1.6 } } }} />
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
                </Stack>
            </TabPage>
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
        </>
    );
}

function UsageTab({ projectId, onDirtyChange }: { projectId: string; onDirtyChange: (dirty: boolean) => void }) {
    const { t } = useTranslation('projects');
    const [usage, setUsage] = useState<Usage | null>(null);
    useEffect(() => onDirtyChange(false), [onDirtyChange]);
    useEffect(() => {
        unwrap(api.GET('/api/v1/projects/{id}/usage', { params: { path: { id: projectId } } })).then(setUsage).catch(() => {});
    }, [projectId]);
    if (!usage) {
        return (
            <Box sx={{ display: 'grid', placeItems: 'center', py: 6, flex: 1 }}>
                <CircularProgress size={24} aria-label={t('projectSettings.usage.loading')} />
            </Box>
        );
    }
    const { models, threads, totals } = usageEntries(usage);
    if (models.length === 0 && threads.length === 0) {
        return <Box sx={{ flex: 1, overflowY: 'auto' }}><EmptyState compact title={t('projectSettings.usage.emptyTitle')} description={t('projectSettings.usage.emptyBody')} /></Box>;
    }
    // Compact strings carry the tile and cells; the exact figures ride on
    // title attributes for the hover.
    const exact = (n: number) => ({ value: formatTokens(n), title: String(n) });
    return (
        <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', pr: 0.5 }}>
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
        </Box>
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
    const [runtime, setRuntime] = useState<ProjectRuntime | null>(null);
    const [runtimeError, setRuntimeError] = useState(false);
    const [pending, setPending] = useState<SettingsTab | 'close' | null>(null);
    const dirtyRef = useRef(false);
    const onDirtyChange = useCallback((dirty: boolean) => { dirtyRef.current = dirty; }, []);

    useEffect(() => {
        if (!open) return;
        setRuntimeError(false);
        unwrap(api.GET('/api/v1/projects/runtime')).then(setRuntime).catch(() => setRuntimeError(true));
    }, [open]);

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

    const requestTab = (next: SettingsTab) => {
        if (next === tab) return;
        if (dirtyRef.current) setPending(next);
        else onTab(next);
    };
    const requestClose = () => {
        if (dirtyRef.current) setPending('close');
        else onClose();
    };
    const confirmDiscard = () => {
        const next = pending;
        setPending(null);
        dirtyRef.current = false;
        if (next === 'close') onClose();
        else if (next) onTab(next);
    };

    const maxLimit = runtime?.max_running_threads_limit ?? MAX_RUNNING_FALLBACK;

    return (
        <Dialog open={open} onClose={requestClose} maxWidth="md" fullWidth
            slotProps={{ paper: { sx: { height: { xs: 'calc(100vh - 32px)', md: '82vh' }, display: 'flex', flexDirection: 'column' } } }}>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', pb: 0, flexShrink: 0 }}>
                <Box component="span" sx={{ flex: 1 }}>{t('projectSettings.title')}</Box>
                <IconButton onClick={requestClose} aria-label={t('common.close')}><Close /></IconButton>
            </DialogTitle>
            <Tabs value={tab} onChange={(_, v) => requestTab(v as SettingsTab)} variant="scrollable" scrollButtons="auto" sx={{ px: 3, borderBottom: 1, borderColor: 'divider', flexShrink: 0, '& .MuiTab-root': { minWidth: 64 } }}>
                <Tab value="basic" label={t('projectSettings.tabBasic')} />
                <Tab value="runtime" label={t('projectSettings.tabRuntime')} />
                <Tab value="memory" label={t('projectSettings.tabMemory')} />
                <Tab value="usage" label={t('projectSettings.tabUsage')} />
            </Tabs>
            <DialogContent sx={{ pt: 2.5, pb: 0, flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
                {tab === 'basic' && <Basic project={project} patch={patch} runtime={runtime} runtimeError={runtimeError} maxLimit={maxLimit} onDirtyChange={onDirtyChange} onFork={() => setForking(true)} />}
                {tab === 'runtime' && <Runtime project={project} patch={patch} runtime={runtime} runtimeError={runtimeError} onDirtyChange={onDirtyChange} />}
                {tab === 'memory' && <Memory project={project} patch={patch} refreshKey={memoryKey} onDirtyChange={onDirtyChange} />}
                {tab === 'usage' && <UsageTab projectId={project.id} onDirtyChange={onDirtyChange} />}
            </DialogContent>
            {/* Forking can only be armed from Basic's guarded row above. */}
            <ForkProjectDialog project={project} open={forking} onClose={() => setForking(false)} onForked={onClose} />
            <ConfirmDialog
                open={pending !== null}
                title={t('projectSettings.discardTitle')}
                description={t('projectSettings.discardBody')}
                confirmLabel={t('projectSettings.discardConfirm')}
                cancelLabel={t('common.cancel')}
                confirmColor="error"
                onClose={() => setPending(null)}
                onConfirm={confirmDiscard}
            />
        </Dialog>
    );
}
