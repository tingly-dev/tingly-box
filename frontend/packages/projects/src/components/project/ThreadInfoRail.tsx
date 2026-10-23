import { useState, type ReactNode } from 'react';
import { Box, Button, CircularProgress, Collapse, IconButton, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { getReadableAccent } from '@tingly/ui/theme/status';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { api, unwrap, type Thread } from '../../client';
import { Cancel, CheckCircle, ChevronRight, DiffIcon, ExpandMore, PlayerStop, Robot, Terminal } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { formatTokens } from '../../utils/format';
import { readLocal, writeLocal } from '../../utils/storage';
import { formatDuration, useNow } from '../workdir/BackgroundTasksPanel';
import type { BackgroundTaskRow } from '../workdir/backgroundTasks';

/** How wide a thread panel must be to carry the rail beside the conversation. */
export const RAIL_MIN_PANEL = 880;
export const RAIL_WIDTH = 288;

const mono = { fontFamily: fontMono, fontSize: fontSizes.sm } as const;

/** A card of the rail: a title the user can fold, remembered per browser. */
function RailCard({ id, title, aside, defaultOpen = true, children }: { id: string; title: string; aside?: ReactNode; defaultOpen?: boolean; children: ReactNode }) {
    const key = `thread.rail.${id}`;
    const [open, setOpen] = useState(() => (readLocal(key) ?? (defaultOpen ? '1' : '0')) === '1');
    const toggle = () => {
        writeLocal(key, open ? '0' : '1');
        setOpen(!open);
    };
    return (
        <Box component="section" aria-label={title} sx={{ border: 1, borderColor: 'divider', borderRadius: 2, bgcolor: 'background.paper' }}>
            <Box role="button" tabIndex={0} aria-expanded={open} onClick={toggle} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggle(); } }}
                sx={{ display: 'flex', alignItems: 'center', gap: 0.5, px: 1.5, py: 1, cursor: 'pointer', userSelect: 'none' }}>
                <Typography sx={{ fontSize: fontSizes.md, fontWeight: 600, color: 'text.primary' }}>{title}</Typography>
                {open ? <ExpandMore sx={{ fontSize: 16, color: 'text.secondary' }} /> : <ChevronRight sx={{ fontSize: 16, color: 'text.secondary' }} />}
                <Box sx={{ flex: 1 }} />
                {aside != null && <Typography component="span" sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{aside}</Typography>}
            </Box>
            <Collapse in={open}>
                <Box sx={{ px: 1.5, pb: 1.25 }}>{children}</Box>
            </Collapse>
        </Box>
    );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
    return (
        <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 1.5, py: 0.25, fontSize: fontSizes.md }}>
            <Box component="span" sx={{ color: 'text.secondary', flexShrink: 0 }}>{label}</Box>
            <Box component="span" sx={{ ...mono, color: 'text.primary', minWidth: 0, textAlign: 'right', overflowWrap: 'anywhere' }}>{children}</Box>
        </Box>
    );
}

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
                sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.md, color: 'text.primary', cursor: task.callId ? 'pointer' : 'default', '&:hover': task.callId ? { textDecoration: 'underline' } : {} }}>
                {label}
            </Typography>
            {elapsed !== undefined && <Typography component="span" sx={{ fontSize: fontSizes.sm, color: 'text.secondary', flexShrink: 0 }}>{formatDuration(elapsed)}</Typography>}
            <Box sx={{ display: 'flex', alignItems: 'center', flexShrink: 0, minWidth: 20, justifyContent: 'center' }}>{end}</Box>
        </Stack>
    );
}

interface Props {
    thread: Thread;
    /** A working folder's session: no worktree branch to review; background tasks live here. */
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
    /** A card above the rest, folded into the rail from the page (a folder's sessions). */
    top?: ReactNode;
}

/**
 * What the open thread is, beside the conversation: where it runs, what it
 * used, what it changed and what it runs in the background. Each card
 * answers one question and folds; the numbers are the ones the thread
 * already carries.
 */
export function ThreadInfoRail({ thread, folder, tasks, canReview, onReview, onChanged, onReveal, permission, top }: Props) {
    const { t } = useTranslation('projects');
    const [showFinished, setShowFinished] = useState(false);
    const running = tasks.filter(isRunning);
    const finished = tasks.filter((x) => !isRunning(x));
    const hasTokens = thread.input_tokens > 0 || thread.output_tokens > 0;
    const hasChanges = !!thread.branch || thread.commits_ahead > 0 || thread.uncommitted || !!thread.merged_at;
    const branch = thread.branch && (thread.base_branch ? `${thread.base_branch} → ${thread.branch}` : thread.branch);
    return (
        <Box component="aside" aria-label={t('rail.title')} sx={{ width: RAIL_WIDTH, flexShrink: 0, borderLeft: 1, borderColor: 'divider', bgcolor: 'background.default', p: 1.5, display: 'flex', flexDirection: 'column', gap: 1.25, overflowY: 'auto', minHeight: 0 }}>
            {top != null && (
                <RailCard id="sessions" title={t('session.sessions')}>
                    <Box sx={{ mx: -0.75, maxHeight: 320, overflowY: 'auto' }}>{top}</Box>
                </RailCard>
            )}
            <RailCard id="env" title={t('rail.environment')}>
                <Fact label={t('rail.folder')}>{thread.cwd}</Fact>
                {branch && <Fact label={t('rail.branch')}>{branch}</Fact>}
                {thread.model && <Fact label={t('rail.model')}>{[thread.profile, thread.model].filter(Boolean).join(' · ')}</Fact>}
                {permission && <Fact label={t('rail.permissions')}>{permission}</Fact>}
            </RailCard>
            {(hasTokens || thread.turns > 0) && (
                <RailCard id="usage" title={t('rail.usage')} defaultOpen={false} aside={hasTokens ? `${formatTokens(thread.input_tokens + thread.output_tokens)}` : undefined}>
                    {hasTokens && <Fact label={t('rail.tokens')}>{formatTokens(thread.input_tokens)} / {formatTokens(thread.output_tokens)}</Fact>}
                    {thread.turns > 0 && <Fact label={t('rail.turns')}>{thread.turns}</Fact>}
                </RailCard>
            )}
            {hasChanges && (
                <RailCard id="changes" title={t('rail.changes')}>
                    {thread.merged_at
                        ? <Typography sx={{ fontSize: fontSizes.md, color: 'text.secondary' }}>{t('thread.mergedInto', { base: thread.base_branch })}</Typography>
                        : thread.commits_ahead > 0
                            ? <Typography sx={{ fontSize: fontSizes.md, color: 'text.primary' }}>{t('thread.commits', { count: thread.commits_ahead })} · {t('thread.files', { count: thread.files_changed })}</Typography>
                            : <Typography sx={{ fontSize: fontSizes.md, color: 'text.secondary' }}>{t('rail.noCommits')}</Typography>}
                    {thread.uncommitted && <Typography sx={{ fontSize: fontSizes.md, color: 'text.primary' }}>{t('thread.uncommitted')}</Typography>}
                    {canReview && <Button size="small" variant="contained" startIcon={<DiffIcon />} onClick={onReview} sx={{ mt: 0.75 }}>{t('thread.review')}</Button>}
                </RailCard>
            )}
            {folder && (
                <RailCard id="tasks" title={t('session.backgroundTasks')} aside={running.length > 0 ? t('session.tasksRunning', { count: running.length }) : undefined}>
                    {tasks.length === 0 && <Typography sx={{ fontSize: fontSizes.md, color: 'text.secondary' }}>{t('tasks.none')}</Typography>}
                    {running.map((x) => <TaskLine key={x.taskId} threadId={thread.id} task={x} onChanged={onChanged} onReveal={onReveal} />)}
                    {finished.length > 0 && (
                        <>
                            <Box role="button" tabIndex={0} aria-expanded={showFinished} onClick={() => setShowFinished(!showFinished)}
                                onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setShowFinished(!showFinished); } }}
                                sx={{ display: 'flex', alignItems: 'center', gap: 0.5, py: 0.5, borderTop: running.length > 0 ? 1 : 0, borderColor: 'divider', fontSize: fontSizes.md, color: 'text.secondary', cursor: 'pointer', userSelect: 'none' }}>
                                {showFinished ? <ExpandMore sx={{ fontSize: 16 }} /> : <ChevronRight sx={{ fontSize: 16 }} />}
                                {t('rail.finished', { count: finished.length })}
                            </Box>
                            <Collapse in={showFinished} unmountOnExit>
                                {finished.map((x) => <TaskLine key={x.taskId} threadId={thread.id} task={x} onChanged={onChanged} onReveal={onReveal} />)}
                            </Collapse>
                        </>
                    )}
                </RailCard>
            )}
        </Box>
    );
}
