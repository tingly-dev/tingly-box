import { useEffect, useState } from 'react';
import { Box, Button, CircularProgress, Collapse, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { getReadableAccent } from '@tingly/ui/theme/status';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { api, unwrap, type components } from '../../client';
import { Block, Cancel, CheckCircle, ChevronRight, ExpandMore, PlayerStop, Robot, Terminal } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { formatBytes, formatTokens, timeAgo } from '../../utils/format';
import type { BackgroundTaskRow } from './backgroundTasks';
import { radius } from '@tingly/ui/theme/radius';
import { ListRow } from '../ListRow';

type TaskOutput = components['schemas']['TaskOutputResponse'];

interface Props {
    sessionId: string;
    tasks: BackgroundTaskRow[];
    /** Called after a task was stopped, to pick up its final state. */
    onChanged: () => void;
    /** Scrolls the conversation to the call that started a task. */
    onReveal: (callId: string) => void;
}

const mono = { fontFamily: fontMono, fontSize: fontSizes.sm };
const OUTPUT_TAIL = 16 * 1024;
const OUTPUT_REFRESH_MS = 2000;

/**
 * What a session runs in the background, the work that outlives the turn
 * that started it. Each row opens into what the task is: the command and its
 * live output, or the subagent's brief, what it is doing and what it last said.
 */
export function BackgroundTasksPanel({ sessionId, tasks, onChanged, onReveal }: Props) {
    const { t } = useTranslation('projects');
    // A lone task opens by itself: the panel was opened to look at it.
    const [openId, setOpenId] = useState<string | null>(tasks.length === 1 ? tasks[0].taskId : null);
    // Finished work stays listed, newest first: its output and report are
    // what people come back to read.
    const running = tasks.filter((task) => task.status === 'running' && !task.ended);
    const finished = tasks.filter((task) => !(task.status === 'running' && !task.ended));
    const rows = (list: BackgroundTaskRow[]) => list.map((task) => (
        <Box key={task.taskId} sx={{ borderTop: 1, borderColor: 'divider' }}>
            <TaskRow
                sessionId={sessionId}
                task={task}
                open={openId === task.taskId}
                onToggle={() => setOpenId(openId === task.taskId ? null : task.taskId)}
                onChanged={onChanged}
                onReveal={onReveal}
            />
        </Box>
    ));
    if (tasks.length === 0) {
        return (
            <Box sx={{ p: 2, maxWidth: 360 }}>
                <Typography variant="body2" sx={{ color: 'text.primary', fontWeight: 600, mb: 0.5 }}>{t('tasks.none')}</Typography>
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>{t('tasks.hint')}</Typography>
            </Box>
        );
    }
    return (
        <Box sx={{ width: 520, maxWidth: '92vw', maxHeight: '75vh', overflowY: 'auto', pb: 0.5 }}>
            <GroupHeader label={t('tasks.running')} count={running.length} />
            {running.length === 0
                ? <Typography variant="body2" sx={{ px: 1.5, py: 1, color: 'text.secondary' }}>{t('tasks.nothingRunning')}</Typography>
                : rows(running)}
            {finished.length > 0 && (
                <>
                    <GroupHeader label={t('tasks.finished')} count={finished.length} />
                    {rows(finished)}
                </>
            )}
        </Box>
    );
}

function GroupHeader({ label, count }: { label: string; count: number }) {
    return (
        <Typography variant="caption" component="div"
            sx={{ px: 1.5, pt: 1.25, pb: 0.5, color: 'text.secondary', fontWeight: 600, letterSpacing: 0.3, textTransform: 'uppercase', fontSize: fontSizes.xs }}>
            {label} · {count}
        </Typography>
    );
}

export function formatDuration(ms: number): string {
    const sec = Math.max(0, Math.round(ms / 1000));
    if (sec < 60) return `${sec}s`;
    if (sec < 3600) return `${Math.floor(sec / 60)}m ${sec % 60}s`;
    return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`;
}

/** The current time, ticking every second while on, so a running task's elapsed time moves. */
export function useNow(on: boolean): number {
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        if (!on) return;
        const id = setInterval(() => setNow(Date.now()), 1000);
        return () => clearInterval(id);
    }, [on]);
    return now;
}

function TaskRow({ sessionId, task, open, onToggle, onChanged, onReveal }: {
    sessionId: string;
    task: BackgroundTaskRow;
    open: boolean;
    onToggle: () => void;
    onChanged: () => void;
    onReveal: (callId: string) => void;
}) {
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const [stopping, setStopping] = useState(false);
    const running = task.status === 'running' && !task.ended;
    const isAgent = task.taskType === 'local_agent';
    const now = useNow(running);

    const stop = async () => {
        setStopping(true);
        try {
            await unwrap(api.POST('/api/v1/threads/{id}/tasks/{task_id}/stop', { params: { path: { id: sessionId, task_id: task.taskId } } }));
            onChanged();
        } catch (e) {
            notify((e as Error).message, 'error');
            setStopping(false);
        }
    };

    const state = task.ended
        ? t('tasks.ended')
        : { running: task.activity || t('tasks.statusRunning'), completed: t('tasks.statusDone'), stopped: t('tasks.statusStopped'), failed: t('tasks.statusFailed') }[task.status];
    const started = task.startedAt ? Date.parse(task.startedAt) : NaN;
    const elapsed = task.usage?.duration_ms
        ?? (Number.isNaN(started) ? undefined : (running ? now : Date.parse(task.finishedAt ?? '') || now) - started);
    const icon = running
        ? <CircularProgress size={14} />
        : task.status === 'completed'
            ? <CheckCircle sx={{ fontSize: 16, color: 'text.secondary' }} />
            : task.status === 'failed'
                ? <Cancel sx={{ fontSize: 16, color: (theme) => getReadableAccent(theme, 'error') }} />
                : <Block sx={{ fontSize: 16, color: 'text.secondary' }} />;

    return (
        <Box>
            <ListRow aria-expanded={open} onClick={onToggle}
                mark={isAgent ? <Robot sx={{ fontSize: 16 }} /> : <Terminal sx={{ fontSize: 16 }} />}
                title={task.description || task.input?.description || task.taskId}
                subtitle={<Box component="span" sx={{ color: (theme) => task.status === 'failed' ? getReadableAccent(theme, 'error') : undefined }}>{[state, elapsed !== undefined ? formatDuration(elapsed) : ''].filter(Boolean).join(' · ')}</Box>}
                end={<>
                    <Box sx={{ display: 'flex', alignItems: 'center', flexShrink: 0 }} title={state}>{icon}</Box>
                    {open ? <ExpandMore sx={{ fontSize: 16, color: 'text.secondary' }} /> : <ChevronRight sx={{ fontSize: 16, color: 'text.secondary' }} />}
                </>} />
            <Collapse in={open} unmountOnExit>
                <Stack spacing={1.25} sx={{ px: 1.5, pb: 1.5, pl: 4.5 }}>
                    {isAgent ? <AgentDetail task={task} running={running} /> : <CommandDetail sessionId={sessionId} task={task} running={running} />}
                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                        {task.startedAt && (
                            <Typography variant="caption" sx={{ color: 'text.secondary', flex: 1 }}>
                                {t('tasks.started', { when: timeAgo(task.startedAt, i18n.language) })}
                            </Typography>
                        )}
                        {task.callId && <Button size="small" onClick={() => onReveal(task.callId)}>{t('tasks.show')}</Button>}
                        {running && (
                            <Button size="small" color="inherit" variant="outlined" disabled={stopping} onClick={() => void stop()}
                                startIcon={stopping ? <CircularProgress size={12} /> : <PlayerStop sx={{ fontSize: '14px !important' }} />}
                                aria-label={t('tasks.stopTask')} sx={{ borderColor: 'divider' }}>
                                {t('tasks.stop')}
                            </Button>
                        )}
                    </Stack>
                </Stack>
            </Collapse>
        </Box>
    );
}

function Label({ children }: { children: string }) {
    return <Typography variant="caption" component="div" sx={{ color: 'text.secondary', mb: 0.25 }}>{children}</Typography>;
}

const pre = { ...mono, m: 0, p: 1, borderRadius: radius.row, bgcolor: 'action.hover', color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', overflow: 'auto' } as const;

/** The command and its output: loaded on open, refreshed while it runs. */
function CommandDetail({ sessionId, task, running }: { sessionId: string; task: BackgroundTaskRow; running: boolean }) {
    const { t } = useTranslation('projects');
    const [output, setOutput] = useState<TaskOutput | null>(null);
    const [missing, setMissing] = useState(false);

    // A finished command reads the copy kept in the transcript (the file it
    // wrote to is temporary); a running one reads the file, live.
    const snapshot = !running ? task.outputSnapshot : undefined;
    useEffect(() => {
        if (!task.outputFile || snapshot) return;
        let live = true;
        const load = () => unwrap(api.GET('/api/v1/threads/{id}/tasks/{task_id}/output', {
            params: { path: { id: sessionId, task_id: task.taskId }, query: { tail: OUTPUT_TAIL } },
        }))
            .then((o) => {
                if (live) {
                    setOutput(o);
                    setMissing(false);
                }
            })
            .catch(() => live && setMissing(true));
        void load();
        const id = running ? setInterval(load, OUTPUT_REFRESH_MS) : undefined;
        return () => {
            live = false;
            if (id) clearInterval(id);
        };
    }, [sessionId, task.taskId, task.outputFile, running, snapshot]);
    const shown: TaskOutput | null = snapshot
        ? { content: snapshot.content, truncated: snapshot.truncated, size: snapshot.content.length }
        : output;
    const unavailable = !snapshot && (!task.outputFile || missing);

    return (
        <>
            {task.input?.command && (
                <Box>
                    <Label>{t('tasks.command')}</Label>
                    <Box component="pre" sx={{ ...pre, maxHeight: 120 }}>{task.input.command}</Box>
                </Box>
            )}
            {task.summary && !running && task.summary !== task.description && (
                <Typography variant="body2" sx={{ color: 'text.primary' }}>{task.summary}</Typography>
            )}
            <Box>
                <Label>
                    {shown?.truncated
                        ? (snapshot
                            ? t('tasks.outputSnapshotTail', { size: formatBytes(shown.content.length) })
                            : t('tasks.outputTail', { size: formatBytes(shown.content.length), total: formatBytes(shown.size) }))
                        : t('tasks.output')}
                </Label>
                <Box component="pre" sx={{ ...pre, maxHeight: 260 }}>
                    {unavailable ? t('tasks.outputUnavailable') : shown === null ? t('tasks.loading') : shown.content || t('tasks.noOutput')}
                </Box>
            </Box>
        </>
    );
}

/** A subagent's brief and progress; the full run is its card in the conversation. */
function AgentDetail({ task, running }: { task: BackgroundTaskRow; running: boolean }) {
    const { t } = useTranslation('projects');
    const usage = task.usage;
    const kind = task.subagentType || task.input?.subagent_type;
    return (
        <>
            <Stack direction="row" spacing={2} sx={{ flexWrap: 'wrap', rowGap: 0.5 }}>
                {kind && (
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {t('tasks.agentType')}: <Box component="span" sx={{ color: 'text.primary' }}>{kind}</Box>
                    </Typography>
                )}
                {usage && (
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {t('tasks.used')}: <Box component="span" sx={{ color: 'text.primary' }}>{t('tasks.usage', { tools: usage.tool_uses, tokens: formatTokens(usage.total_tokens) })}</Box>
                    </Typography>
                )}
            </Stack>
            {task.input?.prompt && (
                <Box>
                    <Label>{t('tasks.agentPrompt')}</Label>
                    <Typography variant="body2" sx={{ color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 96, overflow: 'auto' }}>{task.input.prompt}</Typography>
                </Box>
            )}
            {running && task.activity && (
                <Box>
                    <Label>{t('tasks.agentNow')}</Label>
                    <Typography variant="body2" sx={{ color: 'text.primary' }}>{task.activity}</Typography>
                </Box>
            )}
            {task.recent && task.recent.length > 0 && (
                <Box>
                    <Label>{t('tasks.agentRecent')}</Label>
                    <Stack spacing={0.25}>
                        {task.recent.map((step, i) => (
                            <Stack key={i} direction="row" spacing={1} sx={{ minWidth: 0, alignItems: 'baseline' }}>
                                <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.primary', flexShrink: 0 }}>{step.name}</Typography>
                                <Typography variant="body2" noWrap sx={{ ...mono, color: 'text.secondary' }}>{step.summary}</Typography>
                            </Stack>
                        ))}
                    </Stack>
                </Box>
            )}
            {(task.reply || (!running && task.summary)) && (
                <Box>
                    <Label>{running ? t('tasks.agentLatest') : t('tasks.agentReport')}</Label>
                    <Typography variant="body2" sx={{ color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 160, overflow: 'auto' }}>{task.reply || task.summary}</Typography>
                </Box>
            )}
        </>
    );
}
