import { useEffect, useState, type ReactNode } from 'react';
import { Box, Button, CircularProgress, Collapse, IconButton, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { getReadableAccent } from '@tingly/ui/theme/status';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { api, unwrap, type Subagent, type Thread } from '../../client';
import { Cancel, CheckCircle, ChevronRight, DiffIcon, PlayerStop, Robot, Terminal } from '../icons';
import { DisclosureRow } from '../common/Disclosure';
import { useNotify } from '../../hooks/useNotify';
import { formatDuration, useNow } from '../workdir/BackgroundTasksPanel';
import type { BackgroundTaskRow } from '../workdir/backgroundTasks';
import { timeAgo } from '../../utils/format';
import { CopyValue, Fact, InfoRailFrame, RailCard, StatusCard, UsageCard, shortSession } from './infoRail';
import { agentIcon, agentState } from './ThreadActivityPopover';
import { ThreadLinksCard } from './LinksCard';

const isRunning = (task: BackgroundTaskRow) => task.status === 'running' && !task.ended;

function TaskLine({ threadId, task, onChanged, onReveal }: { threadId: string; task: BackgroundTaskRow; onChanged: () => void; onReveal: (callId: string) => void }) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [stopping, setStopping] = useState(false);
    const running = isRunning(task);
    const now = useNow(running);
    const started = task.startedAt ? Date.parse(task.startedAt) : NaN;
    const elapsed = task.usage?.duration_ms ?? (Number.isNaN(started) ? undefined : (running ? now : Date.parse(task.finishedAt ?? '') || now) - started);
    const label = task.description || task.input?.description || task.taskId;
    const stop = async () => {
        setStopping(true);
        try {
            await unwrap(api.POST('/api/v1/threads/{id}/tasks/{task_id}/stop', { params: { path: { id: threadId, task_id: task.taskId } } }));
            onChanged();
        } catch (e) {
            notify((e as Error).message, 'error');
            setStopping(false);
        }
    };
    const end = running
        ? (
            <Tooltip title={t('tasks.stopTask')}>
                <span>
                    <IconButton size="small" disabled={stopping} onClick={() => void stop()} aria-label={t('tasks.stopTask')} sx={{ p: 0.25 }}>
                        {stopping ? <CircularProgress size={14} /> : <PlayerStop sx={{ fontSize: 16 }} />}
                    </IconButton>
                </span>
            </Tooltip>
        )
        : task.status === 'completed'
            ? <CheckCircle sx={{ fontSize: 16, color: 'text.secondary' }} />
            : <Cancel sx={{ fontSize: 16, color: (theme) => (task.status === 'failed' ? getReadableAccent(theme, 'error') : theme.palette.text.secondary) }} />;
    return (
        <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', minWidth: 0, py: 0.5, borderTop: 1, borderColor: 'divider' }}>
            {task.taskType === 'local_agent' ? <Robot sx={{ fontSize: 16, color: 'text.secondary' }} /> : <Terminal sx={{ fontSize: 16, color: 'text.secondary' }} />}
            <Typography noWrap title={label} onClick={() => task.callId && onReveal(task.callId)}
                sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.sm, color: 'text.primary', cursor: task.callId ? 'pointer' : 'default', '&:hover': task.callId ? { textDecoration: 'underline' } : {} }}>
                {label}
            </Typography>
            {elapsed !== undefined && <Typography component="span" sx={{ fontSize: fontSizes.sm, color: 'text.secondary', flexShrink: 0 }}>{formatDuration(elapsed)}</Typography>}
            <Box sx={{ display: 'flex', alignItems: 'center', flexShrink: 0, minWidth: 20, justifyContent: 'center' }}>{end}</Box>
        </Stack>
    );
}

/** The thread's subagents (its Agent calls, oldest first), read once the card
 * shows: every run of the thread, not just this turn's. A row opens the run
 * on its own, where its steps read out. */
function SubagentsCard({ subagents, onOpenAgent, onShowAgent }: { subagents: { list: () => Promise<Subagent[]> }; onOpenAgent: (agent: Subagent) => void; onShowAgent: (agent: Subagent) => void }) {
    const { t, i18n } = useTranslation('projects');
    const [agents, setAgents] = useState<Subagent[] | null>(null);
    useEffect(() => {
        let live = true;
        subagents.list().then((list) => { if (live) setAgents(list); }).catch(() => { if (live) setAgents([]); });
        return () => { live = false; };
    }, [subagents]);
    if (agents != null && agents.length === 0) return null;
    return (
        <RailCard id="agents" title={t('subagents.title')} aside={agents != null && agents.length > 0 ? String(agents.length) : undefined}>
            {agents === null
                ? <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, color: 'text.secondary', fontSize: fontSizes.sm }}><CircularProgress size={14} />{t('subagents.loading')}</Box>
                : (
                    <Box sx={{ mx: -0.75, maxHeight: 320, overflowY: 'auto' }}>
                        {[...agents].reverse().map((agent) => (
                            <Stack key={agent.call} direction="row" spacing={0.75} sx={{ alignItems: 'center', minWidth: 0, py: 0.5, borderTop: 1, borderColor: 'divider' }}>
                                <Robot sx={{ fontSize: 16, color: 'text.secondary' }} />
                                <Typography noWrap title={agent.description} onClick={() => onOpenAgent(agent)}
                                    sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.sm, color: 'text.primary', cursor: 'pointer', '&:hover': { textDecoration: 'underline' } }}>
                                    {agent.description || t('subagents.title')}
                                </Typography>
                                {agent.subagent_type && (
                                    <Typography component="span" sx={{ fontSize: fontSizes.sm, color: 'text.secondary', flexShrink: 0 }}>{agent.subagent_type}</Typography>
                                )}
                                <Typography component="span" title={[t(agentState(agent)), t('subagents.steps', { count: agent.steps }), timeAgo(agent.created_at, i18n.language)].join(' · ')}
                                    sx={{ fontSize: fontSizes.sm, color: 'text.secondary', flexShrink: 0 }}>
                                    {t(agentState(agent))}
                                </Typography>
                                <Box sx={{ display: 'flex', alignItems: 'center', flexShrink: 0, minWidth: 20, justifyContent: 'center' }}>{agentIcon(agent)}</Box>
                                <Tooltip title={t('subagents.show')}>
                                    <IconButton size="small" onClick={() => onShowAgent(agent)} aria-label={t('subagents.show')} sx={{ p: 0.25 }}>
                                        <ChevronRight sx={{ fontSize: 16 }} />
                                    </IconButton>
                                </Tooltip>
                            </Stack>
                        ))}
                    </Box>
                )}
        </RailCard>
    );
}

interface Props {
    thread: Thread;
    /** A working folder's session: no worktree branch to review; its background tasks always have a card (a project thread gets one while it has some). */
    folder: boolean;
    tasks: BackgroundTaskRow[];
    canReview: boolean;
    onReview: () => void;
    /** After a task was stopped, to pick up its final state. */
    onChanged: () => void;
    /** Scrolls the conversation to the call that started a task. */
    onReveal: (callId: string) => void;
    /** The permission mode's display name (the picker sits at the message box). */
    permission?: string;
    configuredModel?: string;
    /** A card above the rest, folded into the rail from the page (a folder's sessions). */
    top?: ReactNode;
    /** Cards the host adds (SessionSlots.railCards), after Usage. */
    extra?: ReactNode;
    /** The cards in a menu under the Details button, not docked beside the conversation. */
    popover?: boolean;
    /** Lists the thread's subagents (its Agent calls) for the rail's own card. */
    subagents?: { list: () => Promise<Subagent[]> };
    /** Opens a subagent on its own, in place of the conversation. */
    onOpenAgent?: (agent: Subagent) => void;
    /** Shows a subagent where it was called, in the conversation. */
    onShowAgent?: (agent: Subagent) => void;
}

/**
 * What the open thread is, beside the conversation: where it runs, what it
 * used, what it changed and what it runs in the background. Each card
 * answers one question and folds; the numbers are the ones the thread
 * already carries.
 */
export function ThreadInfoRail({ thread, folder, tasks, canReview, onReview, onChanged, onReveal, permission, configuredModel, top, extra, subagents, onOpenAgent, onShowAgent, popover }: Props) {
    const { t } = useTranslation('projects');
    const [showFinished, setShowFinished] = useState(false);
    const running = tasks.filter(isRunning);
    const finished = tasks.filter((x) => !isRunning(x));
    const hasChanges = !!thread.branch || thread.commits_ahead > 0 || thread.uncommitted || !!thread.merged_at;
    const branch = thread.branch && (thread.base_branch ? `${thread.base_branch} → ${thread.branch}` : thread.branch);
    return (
        <InfoRailFrame label={t('rail.title')} popover={popover}>
            {top != null && (
                <RailCard id="sessions" title={t('session.sessions')}>
                    <Box sx={{ mx: -0.75, maxHeight: 320, overflowY: 'auto' }}>{top}</Box>
                </RailCard>
            )}
            <StatusCard thread={thread} />
            <RailCard id="env" title={t('rail.environment')}>
                <Fact label={t('rail.folder')} labelWidth={64}><CopyValue value={thread.cwd} /></Fact>
                {branch && <Fact label={t('rail.branch')} labelWidth={64}><CopyValue value={branch} /></Fact>}
                {configuredModel && <Fact label={t('rail.model')} labelWidth={64}><CopyValue value={configuredModel} /></Fact>}
                {folder && thread.model && <Fact label={t('rail.model')} labelWidth={64}><CopyValue value={[thread.profile, thread.model].filter(Boolean).join(' · ')} /></Fact>}
                {thread.reported_model && <Fact label={t('rail.lastModel')} labelWidth={64}><CopyValue value={thread.reported_model} /></Fact>}
                {permission && <Fact label={t('rail.permissions')} labelWidth={64}><CopyValue value={permission} /></Fact>}
                {thread.session_id && <Fact label={t('rail.session')} labelWidth={64}><CopyValue value={shortSession(thread.session_id)} copy={thread.session_id} /></Fact>}
            </RailCard>
            <UsageCard thread={thread} />
            {extra}
            <ThreadLinksCard thread={thread} />
            {hasChanges && (
                <RailCard id="changes" title={t('rail.changes')}>
                    {thread.merged_at
                        ? <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('thread.mergedInto', { base: thread.base_branch })}</Typography>
                        : thread.commits_ahead > 0
                            ? <Typography sx={{ fontSize: fontSizes.sm, color: 'text.primary' }}>{t('thread.commits', { count: thread.commits_ahead })} · {t('thread.files', { count: thread.files_changed })}</Typography>
                            : <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('rail.noCommits')}</Typography>}
                    {thread.uncommitted && <Typography sx={{ fontSize: fontSizes.sm, color: 'text.primary' }}>{t('thread.uncommitted')}</Typography>}
                    {canReview && <Button size="small" variant="outlined" startIcon={<DiffIcon />} onClick={onReview} sx={{ mt: 0.75 }}>{t('thread.review')}</Button>}
                </RailCard>
            )}
            {tasks.length > 0 && (
                <RailCard id="tasks" title={t('session.backgroundTasks')} aside={running.length > 0 ? t('session.tasksRunning', { count: running.length }) : undefined}>
                    {running.map((x) => <TaskLine key={x.taskId} threadId={thread.id} task={x} onChanged={onChanged} onReveal={onReveal} />)}
                    {finished.length > 0 && (
                        <>
                            <DisclosureRow open={showFinished} onToggle={() => setShowFinished(!showFinished)} sx={{ mx: -1.5, borderTop: 1, borderColor: 'divider', fontSize: fontSizes.sm }}>
                                <Box component="span" sx={{ flex: 1 }}>{t('rail.finished', { count: finished.length })}</Box>
                            </DisclosureRow>
                            <Collapse in={showFinished} unmountOnExit>
                                {finished.map((x) => <TaskLine key={x.taskId} threadId={thread.id} task={x} onChanged={onChanged} onReveal={onReveal} />)}
                            </Collapse>
                        </>
                    )}
                </RailCard>
            )}
            {subagents && onOpenAgent && onShowAgent && (
                <SubagentsCard subagents={subagents} onOpenAgent={onOpenAgent} onShowAgent={onShowAgent} />
            )}
        </InfoRailFrame>
    );
}
