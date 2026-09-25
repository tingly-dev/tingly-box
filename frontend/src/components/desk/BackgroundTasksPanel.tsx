import {Block, Cancel, CheckCircle, ChevronRight, ExpandMore, PlayerStop, Robot, Terminal} from '@/components/icons';
import {useNotify} from '@/hooks/useNotify';
import * as deskApi from '@/services/deskApi';
import type {TaskOutput} from '@/services/deskApi';
import {Box, Button, CircularProgress, Collapse, Stack, Typography} from '@mui/material';
import {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import type {BackgroundTask} from './deskUtils';
import {formatTokens, timeAgo} from './deskUtils';

interface BackgroundTasksPanelProps {
    sessionId: string;
    tasks: BackgroundTask[];
    // Called after a task was stopped, to pick up its final state.
    onChanged: () => void;
    // Scrolls the conversation to the call that started a task.
    onReveal: (callId: string) => void;
}

const mono = {fontFamily: 'monospace', fontSize: '0.75rem'};
const OUTPUT_TAIL = 16 * 1024;
const OUTPUT_REFRESH_MS = 2000;

// BackgroundTasksPanel lists what the session runs in the background —
// shell commands and subagents started with run_in_background — the work
// that outlives the turn that started it. Each row opens into what the task
// is: the command and its live output, or the subagent's brief, what it is
// doing and what it last said.
const BackgroundTasksPanel = ({sessionId, tasks, onChanged, onReveal}: BackgroundTasksPanelProps) => {
    const {t} = useTranslation();
    // A lone task opens by itself: the panel was opened to look at it.
    const [openId, setOpenId] = useState<string | null>(tasks.length === 1 ? tasks[0].taskId : null);
    // Finished work stays listed, in its own group, newest first: its
    // command, output and report are what the user comes back to read.
    const running = tasks.filter((task) => task.status === 'running' && !task.ended);
    const finished = tasks.filter((task) => !(task.status === 'running' && !task.ended));
    const rows = (list: BackgroundTask[]) => list.map((task) => (
        <Box key={task.taskId} sx={{borderTop: 1, borderColor: 'divider'}}>
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
            <Box sx={{p: 2, maxWidth: 360}}>
                <Typography variant="body2" sx={{color: 'text.primary', fontWeight: 600, mb: 0.5}}>
                    {t('desk.noBackgroundTasks', {defaultValue: 'No background tasks'})}
                </Typography>
                <Typography variant="body2" sx={{color: 'text.secondary'}}>
                    {t('desk.backgroundTasksHint', {defaultValue: 'Commands and subagents Claude runs in the background show up here, and keep running after its reply.'})}
                </Typography>
            </Box>
        );
    }
    return (
        <Box sx={{width: 520, maxWidth: '92vw', maxHeight: '75vh', overflowY: 'auto', pb: 0.5}}>
            <GroupHeader label={t('desk.tasksRunning', {defaultValue: 'Running'})} count={running.length}/>
            {running.length === 0 ? (
                <Typography variant="body2" sx={{px: 1.5, py: 1, color: 'text.secondary'}}>
                    {t('desk.nothingRunning', {defaultValue: 'Nothing running now.'})}
                </Typography>
            ) : rows(running)}
            {finished.length > 0 && (
                <>
                    <GroupHeader label={t('desk.tasksFinished', {defaultValue: 'Finished'})} count={finished.length}/>
                    {rows(finished)}
                </>
            )}
        </Box>
    );
};

const GroupHeader = ({label, count}: {label: string; count: number}) => (
    <Typography
        variant="caption"
        component="div"
        sx={{px: 1.5, pt: 1.25, pb: 0.5, color: 'text.secondary', fontWeight: 600, letterSpacing: 0.3, textTransform: 'uppercase', fontSize: '0.68rem'}}
    >
        {label} · {count}
    </Typography>
);

const formatDuration = (ms: number): string => {
    const sec = Math.max(0, Math.round(ms / 1000));
    if (sec < 60) return `${sec}s`;
    if (sec < 3600) return `${Math.floor(sec / 60)}m ${sec % 60}s`;
    return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`;
};

// useNow is the current time, ticking every second while on, so a running
// task's elapsed time moves.
const useNow = (on: boolean): number => {
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        if (!on) return;
        const id = setInterval(() => setNow(Date.now()), 1000);
        return () => clearInterval(id);
    }, [on]);
    return now;
};

const TaskRow = ({sessionId, task, open, onToggle, onChanged, onReveal}: {
    sessionId: string;
    task: BackgroundTask;
    open: boolean;
    onToggle: () => void;
    onChanged: () => void;
    onReveal: (callId: string) => void;
}) => {
    const {t} = useTranslation();
    const notify = useNotify();
    const [stopping, setStopping] = useState(false);
    const running = task.status === 'running' && !task.ended;
    const isAgent = task.taskType === 'local_agent';
    const now = useNow(running);

    const stop = async () => {
        setStopping(true);
        try {
            await deskApi.stopTask(sessionId, task.taskId);
            onChanged();
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.stopTaskFailed', {defaultValue: 'Failed to stop the task'}));
            setStopping(false);
        }
    };

    const state = task.ended
        ? t('desk.taskEnded', {defaultValue: 'ended with the session\'s process'})
        : {
            running: task.activity || t('desk.taskRunning', {defaultValue: 'running'}),
            completed: t('desk.taskDone', {defaultValue: 'done'}),
            stopped: t('desk.taskStopped', {defaultValue: 'stopped'}),
            failed: t('desk.taskFailed', {defaultValue: 'failed'}),
        }[task.status];
    const started = task.startedAt ? Date.parse(task.startedAt) : NaN;
    const elapsed = task.usage?.duration_ms
        ?? (Number.isNaN(started) ? undefined : (running ? now : Date.parse(task.finishedAt ?? '') || now) - started);
    const icon = running
        ? <CircularProgress size={14}/>
        : task.status === 'completed'
            ? <CheckCircle sx={{fontSize: 16, color: 'success.main'}}/>
            : task.status === 'failed'
                ? <Cancel sx={{fontSize: 16, color: 'error.main'}}/>
                : <Block sx={{fontSize: 16, color: 'text.secondary'}}/>;

    return (
        <Box>
            <Stack
                direction="row"
                spacing={1}
                role="button"
                aria-expanded={open}
                onClick={onToggle}
                sx={{alignItems: 'center', minWidth: 0, px: 1.5, py: 1, cursor: 'pointer', '&:hover': {bgcolor: 'action.hover'}}}
            >
                {isAgent ? <Robot sx={{fontSize: 16, color: 'text.secondary'}}/> : <Terminal sx={{fontSize: 16, color: 'text.secondary'}}/>}
                <Box sx={{flex: 1, minWidth: 0}}>
                    <Typography variant="body2" noWrap sx={{color: 'text.primary', fontWeight: 500}}>
                        {task.description || task.input?.description || task.taskId}
                    </Typography>
                    <Typography variant="caption" noWrap component="div" sx={{color: task.status === 'failed' ? 'error.main' : 'text.secondary'}}>
                        {[state, elapsed !== undefined ? formatDuration(elapsed) : ''].filter(Boolean).join(' · ')}
                    </Typography>
                </Box>
                <Box sx={{display: 'flex', alignItems: 'center', flexShrink: 0}} title={state}>{icon}</Box>
                {open ? <ExpandMore sx={{fontSize: 16, color: 'text.secondary'}}/> : <ChevronRight sx={{fontSize: 16, color: 'text.secondary'}}/>}
            </Stack>
            <Collapse in={open} unmountOnExit>
                <Stack spacing={1.25} sx={{px: 1.5, pb: 1.5, pl: 4.5}}>
                    {isAgent ? <AgentDetail task={task} running={running}/> : <CommandDetail sessionId={sessionId} task={task} running={running}/>}
                    <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                        {task.startedAt && (
                            <Typography variant="caption" sx={{color: 'text.secondary', flex: 1}}>
                                {t('desk.taskStartedAgo', {defaultValue: 'Started {{when}}', when: timeAgo(task.startedAt)})}
                            </Typography>
                        )}
                        {task.callId && (
                            <Button size="small" onClick={() => onReveal(task.callId)} sx={{textTransform: 'none'}}>
                                {t('desk.showInConversation', {defaultValue: 'Show in conversation'})}
                            </Button>
                        )}
                        {running && (
                            <Button
                                size="small"
                                color="inherit"
                                variant="outlined"
                                startIcon={stopping ? <CircularProgress size={12}/> : <PlayerStop sx={{fontSize: '14px !important'}}/>}
                                disabled={stopping}
                                onClick={() => void stop()}
                                aria-label={t('desk.stopTask', {defaultValue: 'Stop this task'})}
                                sx={{textTransform: 'none', borderColor: 'divider'}}
                            >
                                {t('desk.stop', {defaultValue: 'Stop'})}
                            </Button>
                        )}
                    </Stack>
                </Stack>
            </Collapse>
        </Box>
    );
};

const Label = ({children}: {children: string}) => (
    <Typography variant="caption" component="div" sx={{color: 'text.secondary', mb: 0.25}}>{children}</Typography>
);

// CommandDetail is the command and its output: loaded on open, refreshed
// while it runs, so the panel is a live view of what it prints.
const CommandDetail = ({sessionId, task, running}: {sessionId: string; task: BackgroundTask; running: boolean}) => {
    const {t} = useTranslation();
    const [output, setOutput] = useState<TaskOutput | null>(null);
    const [missing, setMissing] = useState(false);

    // A finished command reads from the copy kept in the transcript: the
    // file it wrote to is temporary. A running one reads the file, live.
    const snapshot = !running ? task.outputSnapshot : undefined;
    useEffect(() => {
        if (!task.outputFile || snapshot) return;
        let live = true;
        const load = () => deskApi.getTaskOutput(sessionId, task.taskId, OUTPUT_TAIL)
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
        ? {content: snapshot.content, truncated: snapshot.truncated, size: snapshot.content.length}
        : output;
    const unavailable = !snapshot && (!task.outputFile || missing);

    return (
        <>
            {task.input?.command && (
                <Box>
                    <Label>{t('desk.command', {defaultValue: 'Command'})}</Label>
                    <Box component="pre" sx={{...mono, m: 0, p: 1, borderRadius: 1, bgcolor: 'action.hover', color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 120, overflow: 'auto'}}>
                        {task.input.command}
                    </Box>
                </Box>
            )}
            {task.summary && !running && task.summary !== task.description && (
                <Typography variant="body2" sx={{color: 'text.primary'}}>{task.summary}</Typography>
            )}
            <Box>
                <Label>
                    {shown?.truncated
                        ? (snapshot
                            ? t('desk.outputSnapshotTail', {defaultValue: 'Output — last {{size}}', size: formatBytes(shown.content.length)})
                            : t('desk.outputTail', {defaultValue: 'Output — last {{size}} of {{total}}', size: formatBytes(shown.content.length), total: formatBytes(shown.size)}))
                        : t('desk.output', {defaultValue: 'Output'})}
                </Label>
                <Box component="pre" sx={{...mono, m: 0, p: 1, borderRadius: 1, bgcolor: 'action.hover', color: 'text.primary', maxHeight: 260, overflow: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>
                    {unavailable
                        ? t('desk.outputUnavailable', {defaultValue: '(output not available)'})
                        : shown === null
                            ? t('common.loading', {defaultValue: 'Loading…'})
                            : shown.content || t('desk.noOutputYet', {defaultValue: '(no output yet)'})}
                </Box>
            </Box>
        </>
    );
};

// AgentDetail is the subagent's brief and its progress: what it was asked,
// what it is doing, its latest steps and what it last said. The full run is
// its card in the conversation.
const AgentDetail = ({task, running}: {task: BackgroundTask; running: boolean}) => {
    const {t} = useTranslation();
    const usage = task.usage;
    return (
        <>
            <Stack direction="row" spacing={2} sx={{flexWrap: 'wrap', rowGap: 0.5}}>
                {(task.subagentType || task.input?.subagent_type) && (
                    <Typography variant="caption" sx={{color: 'text.secondary'}}>
                        {t('desk.agentType', {defaultValue: 'Type'})}: <Box component="span" sx={{color: 'text.primary'}}>{task.subagentType || task.input?.subagent_type}</Box>
                    </Typography>
                )}
                {usage && (
                    <Typography variant="caption" sx={{color: 'text.secondary'}}>
                        {t('desk.usage', {defaultValue: 'Used'})}: <Box component="span" sx={{color: 'text.primary'}}>
                            {usage.tool_uses} {t('desk.tools', {defaultValue: 'tools'})} · {formatTokens(usage.total_tokens)} {t('desk.tokens', {defaultValue: 'tokens'})}
                        </Box>
                    </Typography>
                )}
            </Stack>
            {task.input?.prompt && (
                <Box>
                    <Label>{t('desk.agentPrompt', {defaultValue: 'Asked to'})}</Label>
                    <Typography variant="body2" sx={{color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 96, overflow: 'auto'}}>
                        {task.input.prompt}
                    </Typography>
                </Box>
            )}
            {running && task.activity && (
                <Box>
                    <Label>{t('desk.agentNow', {defaultValue: 'Now'})}</Label>
                    <Typography variant="body2" sx={{color: 'text.primary'}}>{task.activity}</Typography>
                </Box>
            )}
            {task.recent && task.recent.length > 0 && (
                <Box>
                    <Label>{t('desk.agentRecent', {defaultValue: 'Recent steps'})}</Label>
                    <Stack spacing={0.25}>
                        {task.recent.map((step, i) => (
                            <Stack key={i} direction="row" spacing={1} sx={{minWidth: 0, alignItems: 'baseline'}}>
                                <Typography variant="body2" sx={{fontWeight: 600, color: 'text.primary', flexShrink: 0}}>{step.name}</Typography>
                                <Typography variant="body2" noWrap sx={{...mono, color: 'text.secondary'}}>{step.summary}</Typography>
                            </Stack>
                        ))}
                    </Stack>
                </Box>
            )}
            {(task.reply || (!running && task.summary)) && (
                <Box>
                    <Label>{running ? t('desk.agentLatest', {defaultValue: 'Latest reply'}) : t('desk.agentReport', {defaultValue: 'Report'})}</Label>
                    <Typography variant="body2" sx={{color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 160, overflow: 'auto'}}>
                        {task.reply || task.summary}
                    </Typography>
                </Box>
            )}
        </>
    );
};

const formatBytes = (n: number): string => (n >= 1024 ? `${(n / 1024).toFixed(n >= 10240 ? 0 : 1)} KB` : `${n} B`);

export default BackgroundTasksPanel;
