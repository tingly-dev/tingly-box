import {Block, Cancel, CheckCircle, PlayerStop, Refresh, Robot, Terminal} from '@/components/icons';
import {useNotify} from '@/hooks/useNotify';
import * as deskApi from '@/services/deskApi';
import type {TaskOutput} from '@/services/deskApi';
import {Box, Button, CircularProgress, IconButton, Stack, Tooltip, Typography} from '@mui/material';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import type {BackgroundTask} from './deskUtils';
import {formatTokens} from './deskUtils';

interface BackgroundTasksPanelProps {
    sessionId: string;
    tasks: BackgroundTask[];
    // Called after a task was stopped, to pick up its final state.
    onChanged: () => void;
}

const mono = {fontFamily: 'monospace', fontSize: '0.75rem'};

// BackgroundTasksPanel lists what the session runs in the background —
// shell commands and subagents started with run_in_background — the work
// that outlives the turn that started it. Each can be stopped on its own,
// and a command's output read while it runs.
const BackgroundTasksPanel = ({sessionId, tasks, onChanged}: BackgroundTasksPanelProps) => {
    const {t} = useTranslation();
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
        <Stack sx={{width: 440, maxWidth: '90vw', maxHeight: '70vh', overflowY: 'auto', py: 0.5}} divider={<Box sx={{borderTop: 1, borderColor: 'divider'}}/>}>
            {tasks.map((task) => <TaskRow key={task.taskId} sessionId={sessionId} task={task} onChanged={onChanged}/>)}
        </Stack>
    );
};

const TaskRow = ({sessionId, task, onChanged}: {sessionId: string; task: BackgroundTask; onChanged: () => void}) => {
    const {t} = useTranslation();
    const notify = useNotify();
    const [stopping, setStopping] = useState(false);
    const [output, setOutput] = useState<TaskOutput | null>(null);
    const [loading, setLoading] = useState(false);
    const running = task.status === 'running' && !task.ended;
    const isAgent = task.taskType === 'local_agent';

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

    const loadOutput = async () => {
        setLoading(true);
        try {
            setOutput(await deskApi.getTaskOutput(sessionId, task.taskId, 16 * 1024));
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.outputFailed', {defaultValue: 'Failed to read the output'}));
        } finally {
            setLoading(false);
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
    const usage = task.usage
        ? `${task.usage.tool_uses} ${t('desk.tools', {defaultValue: 'tools'})} · ${formatTokens(task.usage.total_tokens)} ${t('desk.tokens', {defaultValue: 'tokens'})}`
        : '';
    const icon = running
        ? <CircularProgress size={14}/>
        : task.status === 'completed'
            ? <CheckCircle sx={{fontSize: 16, color: 'success.main'}}/>
            : task.status === 'failed'
                ? <Cancel sx={{fontSize: 16, color: 'error.main'}}/>
                : <Block sx={{fontSize: 16, color: 'text.secondary'}}/>;

    return (
        <Box sx={{px: 1.5, py: 1}}>
            <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                {isAgent ? <Robot sx={{fontSize: 16, color: 'text.secondary'}}/> : <Terminal sx={{fontSize: 16, color: 'text.secondary'}}/>}
                <Box sx={{flex: 1, minWidth: 0}}>
                    <Typography variant="body2" noWrap sx={{color: 'text.primary', fontWeight: 500}}>
                        {task.description || task.taskId}
                    </Typography>
                    <Typography variant="caption" noWrap component="div" sx={{color: task.status === 'failed' ? 'error.main' : 'text.secondary'}}>
                        {[state, usage || (isAgent ? task.subagentType : '')].filter(Boolean).join(' · ')}
                    </Typography>
                </Box>
                <Box sx={{display: 'flex', alignItems: 'center', flexShrink: 0}} title={state}>{icon}</Box>
                {!isAgent && task.outputFile && (
                    <Button size="small" onClick={() => (output ? setOutput(null) : void loadOutput())} disabled={loading} sx={{minWidth: 0, flexShrink: 0}}>
                        {output ? t('desk.hideOutput', {defaultValue: 'Hide'}) : t('desk.output', {defaultValue: 'Output'})}
                    </Button>
                )}
                {running && (
                    <Tooltip title={t('desk.stopTask', {defaultValue: 'Stop this task'})}>
                        <span>
                            <IconButton size="small" onClick={() => void stop()} disabled={stopping} aria-label={t('desk.stopTask', {defaultValue: 'Stop this task'})}>
                                {stopping ? <CircularProgress size={14}/> : <PlayerStop fontSize="small"/>}
                            </IconButton>
                        </span>
                    </Tooltip>
                )}
            </Stack>
            {output && (
                <Box sx={{mt: 1, position: 'relative'}}>
                    {output.truncated && (
                        <Typography variant="caption" sx={{color: 'text.secondary'}}>
                            {t('desk.outputTail', {defaultValue: 'Last {{size}} of {{total}}', size: formatBytes(output.content.length), total: formatBytes(output.size)})}
                        </Typography>
                    )}
                    <Box component="pre" sx={{...mono, m: 0, p: 1, borderRadius: 1, bgcolor: 'action.hover', color: 'text.primary', maxHeight: 240, overflow: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>
                        {output.content || t('desk.noOutputYet', {defaultValue: '(no output yet)'})}
                    </Box>
                    {running && (
                        <IconButton size="small" onClick={() => void loadOutput()} aria-label={t('common.refresh', {defaultValue: 'Refresh'})} sx={{position: 'absolute', top: output.truncated ? 20 : 2, right: 2}}>
                            <Refresh sx={{fontSize: 16}}/>
                        </IconButton>
                    )}
                </Box>
            )}
        </Box>
    );
};

const formatBytes = (n: number): string => (n >= 1024 ? `${(n / 1024).toFixed(n >= 10240 ? 0 : 1)} KB` : `${n} B`);

export default BackgroundTasksPanel;
