import { useEffect, useState, type MouseEvent } from 'react';
import { Box, CircularProgress, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { getReadableAccent } from '@tingly/ui/theme/status';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { Subagent, Thread } from '../../client';
import { Block, Cancel, CheckCircle, Robot, Terminal } from '../icons';
import { timeAgo } from '../../utils/format';
import { ListRow } from '../ListRow';
import { loadThreadActivity, type ThreadActivity } from './threadActivity';

interface Props {
    /** The threads whose running work the list carries (one per thread that has tasks). */
    targets: Thread[];
    /** Opens a subagent on its own in its thread. */
    onOpenAgent: (threadId: string, call: string) => void;
    /** Opens a background task in its thread. */
    onOpenTask: (threadId: string, taskId: string) => void;
    /** Opens the thread itself (its section header, where several threads are listed). */
    onOpenThread?: (threadId: string) => void;
    onClose: () => void;
}

/** A subagent's state as the status area reads it: the list does not know whether the turn is still live. */
export const agentState = (agent: Subagent) =>
    ({ running: 'tasks.statusRunning', completed: 'tasks.statusDone', stopped: 'tasks.statusStopped', failed: 'tasks.statusFailed' } as const)[agent.status as 'running' | 'completed' | 'stopped' | 'failed'] ?? agent.status;

/** The state's mark, in a 16px slot. */
export const agentIcon = (agent: Subagent) => {
    const sx = { fontSize: 16 };
    if (agent.status === 'running') return <CircularProgress size={14} />;
    if (agent.status === 'completed') return <CheckCircle sx={{ ...sx, color: 'text.secondary' }} />;
    if (agent.status === 'failed') return <Cancel sx={{ ...sx, color: (theme) => getReadableAccent(theme, 'error') }} />;
    return <Block sx={{ ...sx, color: 'text.secondary' }} />;
};

/**
 * What the status area's counts stand for, one click away: the running
 * background tasks and the subagents of the thread (or of each thread that
 * has tasks). Every row carries the id that opens its detail, so a count
 * drills down to the one thing it counted.
 */
export function ThreadActivityPopover({ targets, onOpenAgent, onOpenTask, onOpenThread, onClose }: Props) {
    const { t, i18n } = useTranslation('projects');
    const [loaded, setLoaded] = useState<Record<string, ThreadActivity>>({});
    const [error, setError] = useState<string | null>(null);
    useEffect(() => {
        let live = true;
        setError(null);
        setLoaded({});
        Promise.all(targets.map((th) => loadThreadActivity(th.id).then((a) => [th.id, a] as const)))
            .then((pairs) => { if (live) setLoaded(Object.fromEntries(pairs)); })
            .catch((e: Error) => { if (live) setError(e.message); });
        return () => { live = false; };
    }, [targets]);

    if (error) {
        return (
            <Box sx={{ width: 480, maxWidth: '92vw', p: 2 }}>
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>{error}</Typography>
            </Box>
        );
    }
    if (Object.keys(loaded).length < targets.length) {
        return (
            <Box sx={{ width: 480, maxWidth: '92vw', p: 2, display: 'flex', alignItems: 'center', gap: 1, color: 'text.secondary' }}>
                <CircularProgress size={14} />
                <Typography variant="body2" sx={{ color: 'inherit' }}>{t('subagents.loading')}</Typography>
            </Box>
        );
    }
    const total = Object.values(loaded).reduce((n, a) => n + a.tasks.length + a.subagents.length, 0);
    return (
        <Box sx={{ width: 480, maxWidth: '92vw', maxHeight: '75vh', overflowY: 'auto', py: 0.5 }}>
            {total === 0 && (
                <Box sx={{ p: 2 }}>
                    <Typography variant="body2" sx={{ color: 'text.primary', fontWeight: 600, mb: 0.5 }}>{t('tasks.none')}</Typography>
                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>{t('tasks.hint')}</Typography>
                </Box>
            )}
            {targets.map((th) => {
                const activity = loaded[th.id];
                if (!activity || (activity.tasks.length === 0 && activity.subagents.length === 0)) return null;
                return (
                    <Box key={th.id} sx={targets.length > 1 ? { borderBottom: targets.indexOf(th) < targets.length - 1 ? 1 : 0, borderColor: 'divider', pb: 0.5, mb: 0.5 } : undefined}>
                        {targets.length > 1 && (
                            <Typography variant="caption" component="div"
                                {...(onOpenThread && { role: 'button', tabIndex: 0, onClick: (e: MouseEvent<HTMLElement>) => { e.stopPropagation(); onClose(); onOpenThread(th.id); } })}
                                title={th.title}
                                sx={{ px: 1.5, pt: 1.25, pb: 0.5, fontWeight: 600, color: 'text.primary', fontSize: fontSizes.sm, cursor: onOpenThread ? 'pointer' : 'default', '&:hover': onOpenThread ? { textDecoration: 'underline' } : undefined }}>
                                {th.title}
                            </Typography>
                        )}
                        {activity.tasks.length > 0 && (
                            <>
                                <GroupHeader label={t('session.backgroundTasks')} count={activity.tasks.length} />
                                {activity.tasks.map((task) => (
                                    <ListRow key={task.task_id} data-activity-task={task.task_id} data-activity-thread={th.id}
                                        onClick={() => { onClose(); onOpenTask(th.id, task.task_id); }}
                                        mark={task.task_type === 'local_agent' ? <Robot sx={{ fontSize: 16 }} /> : <Terminal sx={{ fontSize: 16 }} />}
                                        title={task.description || task.task_id}
                                        subtitle={t('tasks.statusRunning')} end={<CircularProgress size={14} />} />
                                ))}
                            </>
                        )}
                        {activity.subagents.length > 0 && (
                            <>
                                <GroupHeader label={t('subagents.title')} count={activity.subagents.length} />
                                {activity.subagents.map((agent) => (
                                    <ListRow key={agent.call} data-activity-agent={agent.call} data-activity-thread={th.id}
                                        onClick={() => { onClose(); onOpenAgent(th.id, agent.call); }}
                                        mark={<Robot sx={{ fontSize: 16 }} />}
                                        title={<>{agent.description || t('subagents.title')}{agent.subagent_type && <Box component="span" sx={{ ml: 0.75, fontSize: fontSizes.sm, fontWeight: 400, color: 'text.secondary' }}>{agent.subagent_type}</Box>}</>}
                                        subtitle={[t(agentState(agent)), t('subagents.steps', { count: agent.steps }), timeAgo(agent.created_at, i18n.language)].join(' · ')}
                                        end={agentIcon(agent)} />
                                ))}
                            </>
                        )}
                    </Box>
                );
            })}
        </Box>
    );
}

function GroupHeader({ label, count }: { label: string; count: number }) {
    return (
        <Typography variant="caption" component="div"
            sx={{ px: 1.5, pt: 1, pb: 0.5, color: 'text.secondary', fontWeight: 600, letterSpacing: 0.3, textTransform: 'uppercase', fontSize: fontSizes.xs }}>
            {label} · {count}
        </Typography>
    );
}
