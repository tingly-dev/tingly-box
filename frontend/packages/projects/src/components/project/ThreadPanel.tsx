import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    Alert, Box, Button, Collapse, Divider, IconButton, Menu, MenuItem,
    Popover, Skeleton, Tooltip, Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { BackgroundTask, Thread, ThreadEvent } from '../../client';
import { buildTranscript, Trajectory, Transcript, type Respond, type TrajectoryAnchor } from '../../transcript';
import { Composer } from '../Composer';
import { CodeBlock, Markdown } from '../Markdown';
import { ConfirmDialog, CopyIconButton } from '@tingly/ui';
import { ArrowBack, Branch, CheckCircle, DiffIcon, FolderOpen, Merge, MoreVert, Stream, Terminal, Undo } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { folderName, formatTokens } from '../../utils/format';
import { DiffDialog } from './DiffDialog';
import { ThreadStatusChip, toneColor } from './threadStatus';
import { threadTranscript } from './threadTranscript';
import { projectThreadOps, sessionOps } from './threadOps';
import { BackgroundTasksPanel } from '../workdir/BackgroundTasksPanel';
import { backgroundTasks } from '../workdir/backgroundTasks';
import { readLocal, removeLocal, writeLocal } from '../../utils/storage';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';

const mono = { fontFamily: fontMono } as const;

/** Facts about a thread on one line, separated by dots; empty ones drop out. */
function MetaLine({ items }: { items: React.ReactNode[] }) {
    const shown = items.filter(Boolean);
    if (shown.length === 0) return null;
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: 0.75, rowGap: 0.25, color: 'text.secondary', fontSize: fontSizes.md }}>
            {shown.map((x, i) => (
                <Box key={i} component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75 }}>
                    {i > 0 && <Box component="span" sx={{ color: 'text.disabled' }}>·</Box>}
                    {x}
                </Box>
            ))}
        </Box>
    );
}

interface Props {
    /** The project the thread belongs to; empty for a folder session. */
    projectId: string;
    threadId: string;
    thread?: Thread;
    events: ThreadEvent[] | null;
    running: boolean;
    disabled: boolean;
    onBack: () => void;
    /** In a window the window's title bar names the thread: leave the title and the back arrow out. */
    hideTitle?: boolean;
    /** Buttons before the thread's menu: pop out, dock. */
    actions?: React.ReactNode;
    /** Pickers shown above the message box (a folder session's permission mode, a host's own). */
    composerContext?: React.ReactNode;
    /** Under the message box (a host's status line). */
    belowComposer?: React.ReactNode;
    /** What a folder session's process runs in the background now. */
    liveTasks?: BackgroundTask[];
    /** Re-reads the session, e.g. after a task was stopped. */
    onRefresh?: () => void;
}

type View = 'chat' | 'trajectory';

const anchorSelector = (anchor: TrajectoryAnchor): string => {
    if ('call' in anchor) return `[data-call-ids~="${CSS.escape(anchor.call)}"]`;
    if ('request' in anchor) return `[data-request-id="${CSS.escape(anchor.request)}"]`;
    return `[data-block="${anchor.block}"]`;
};

export function ThreadPanel({ projectId, threadId, thread, events, running, disabled, onBack, hideTitle, actions, composerContext, belowComposer, liveTasks = [], onRefresh }: Props) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [diffOpen, setDiffOpen] = useState(false);
    const [menu, setMenu] = useState<HTMLElement | null>(null);
    const [taskOpen, setTaskOpen] = useState(false);
    // Chat and Trajectory read the same transcript; the choice and whether
    // every tool call starts open are remembered per browser.
    const [view, setViewState] = useState<View>(() => readLocal('thread.view') === 'trajectory' ? 'trajectory' : 'chat');
    const setView = (next: View) => {
        setViewState(next);
        writeLocal('thread.view', next);
    };
    // What is typed but not sent survives leaving the thread or a reload.
    const draftKey = `draft.${threadId}`;
    const [draft, setDraftState] = useState(() => readLocal(draftKey) ?? '');
    const setDraft = (text: string) => {
        setDraftState(text);
        if (text) writeLocal(draftKey, text);
        else removeLocal(draftKey);
    };
    const [expandAll, setExpandAll] = useState(() => readLocal('thread.expandTools') === '1');
    const toggleExpand = () => {
        writeLocal('thread.expandTools', expandAll ? '0' : '1');
        setExpandAll(!expandAll);
    };
    // What came in since the last visit sits under a "New" line. The mark is
    // read once per visit and moved to the latest event while the thread is
    // open, so the line stays put while the user reads.
    const seenKey = `seen.${threadId}`;
    const [newSince, setNewSince] = useState(() => ({ threadId, at: readLocal(seenKey) ?? undefined }));
    if (newSince.threadId !== threadId) setNewSince({ threadId, at: readLocal(seenKey) ?? undefined });
    const latestAt = events?.at(-1)?.created_at;
    useEffect(() => {
        if (latestAt) writeLocal(seenKey, latestAt);
    }, [seenKey, latestAt]);
    const [target, setTarget] = useState<TrajectoryAnchor | null>(null);
    const [tasksAnchor, setTasksAnchor] = useState<HTMLElement | null>(null);
    // The handoff command stays on screen with its own copy button, since
    // copying right after the request can be refused by the browser.
    const [handoff, setHandoff] = useState<string | null>(null);
    const [confirm, setConfirm] = useState<'archive' | 'handoff' | null>(null);
    const [acting, setActing] = useState(false);
    const scrollRef = useRef<HTMLDivElement>(null);
    const stick = useRef(true);
    // A folder session has no project: no branch, review or merge.
    const session = !projectId;
    const ops = useMemo(() => session ? sessionOps(threadId) : projectThreadOps(projectId, threadId), [session, projectId, threadId]);

    useEffect(() => {
        const el = scrollRef.current;
        if (el && stick.current) el.scrollTop = el.scrollHeight;
    }, [events]);

    const coordinator = t('thread.fromCoordinator');
    const { blocks, pendingRequestId } = useMemo(() => {
        const { entries, pendingRequestId } = threadTranscript(events ?? [], running, (from) => from === 'coordinator' ? coordinator : undefined);
        return { blocks: buildTranscript(entries), pendingRequestId };
    }, [events, running, coordinator]);

    const tasks = useMemo(() => session
        ? backgroundTasks(threadTranscript(events ?? [], running, () => undefined).entries, liveTasks)
        : [], [session, events, running, liveTasks]);

    // Scroll the conversation to a message and flash it: "where did this
    // come from" is one click from the trajectory and the tasks panel.
    const flash = (anchor: TrajectoryAnchor) => {
        const el = scrollRef.current?.querySelector<HTMLElement>(anchorSelector(anchor));
        if (!el) return;
        stick.current = false;
        el.scrollIntoView({ block: 'center' });
        el.animate?.([{ outline: '2px solid transparent' }, { outline: '2px solid var(--mui-palette-primary-main, #1976d2)' }, { outline: '2px solid transparent' }], { duration: 1600 });
    };
    const openInChat = (anchor: TrajectoryAnchor) => {
        setTasksAnchor(null);
        if (view === 'chat') flash(anchor);
        else {
            setView('chat');
            setTarget(anchor);
        }
    };
    useEffect(() => {
        if (!target || view !== 'chat') return;
        // After the chat has rendered in place of the trajectory.
        const frame = requestAnimationFrame(() => {
            flash(target);
            setTarget(null);
        });
        return () => cancelAnimationFrame(frame);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [target, view]);

    const respond: Respond = async (requestId, allow, answer, remember, answers) => {
        try {
            await ops.respond(requestId, allow, !!remember, answer, answers);
            return true;
        } catch (e) {
            notify((e as Error).message, 'error');
            return false;
        }
    };
    const rememberLabel = useCallback((tool: string) => t('thread.allowThread', { tool }), [t]);

    const act = async (run: () => Promise<unknown>) => {
        try {
            await run();
            return true;
        } catch (e) {
            notify((e as Error).message, 'error');
            return false;
        }
    };
    const runConfirmed = async (what: 'archive' | 'handoff') => {
        setActing(true);
        const done = what === 'archive'
            ? await act(ops.finish)
            : await act(async () => setHandoff(await ops.handoff()));
        setActing(false);
        if (done) setConfirm(null);
    };
    // Archiving or handing off ends the process and every background task it
    // runs: ask first while any is running.
    const guarded = (what: 'archive' | 'handoff') => {
        setMenu(null);
        if (liveTasks.length > 0) setConfirm(what);
        else void runConfirmed(what);
    };

    if (!thread) {
        return <Box sx={{ p: 2 }}><Skeleton height={40} /><Skeleton variant="rounded" height={200} /></Box>;
    }
    const canReview = !!thread.branch && thread.commits_ahead > 0 && !thread.merged_at;

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Box sx={{ px: 1.5, pt: 1, pb: 1, borderBottom: 1, borderColor: 'divider' }}>
                {/* One row to name the thread and act on it, one quiet row of facts. */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                    {!hideTitle && <IconButton size="small" onClick={onBack} aria-label={t('thread.back')}><ArrowBack fontSize="small" /></IconButton>}
                    {hideTitle
                        ? <Box sx={{ flex: 1 }} />
                        : <Typography noWrap sx={{ flex: 1, minWidth: 0, fontSize: '0.9375rem', fontWeight: 650, color: 'text.primary' }} title={thread.title}>{thread.title}</Typography>}
                    {canReview && <Button size="small" variant="contained" startIcon={<DiffIcon />} onClick={() => setDiffOpen(true)} sx={{ flexShrink: 0, py: 0.25 }}>{t('thread.review')}</Button>}
                    {/* While work runs in the background it names itself. */}
                    {session && liveTasks.length > 0 && (
                        <Tooltip title={t('session.backgroundTasks')}>
                            <Button size="small" variant="outlined" color="inherit" startIcon={<Stream sx={{ fontSize: '16px !important' }} />} onClick={(e) => setTasksAnchor(e.currentTarget)}
                                sx={{ borderRadius: 4, borderColor: 'divider', color: 'text.secondary', py: 0, px: 1.25, minWidth: 0, whiteSpace: 'nowrap', flexShrink: 0 }}>
                                {t('session.tasksRunning', { count: liveTasks.length })}
                            </Button>
                        </Tooltip>
                    )}
                    {actions}
                    {/* Done with it is one click; reopening is the same place. */}
                    {thread.status !== 'resolved'
                        ? (
                            <Tooltip title={t(session ? 'session.archive' : 'thread.resolve')}>
                                <IconButton size="small" disabled={acting} onClick={() => session ? guarded('archive') : void act(ops.finish)} aria-label={t(session ? 'session.archive' : 'thread.resolve')}><CheckCircle fontSize="small" /></IconButton>
                            </Tooltip>
                        )
                        : (
                            <Tooltip title={t(session ? 'session.reopen' : 'thread.reopen')}>
                                <IconButton size="small" disabled={acting} onClick={() => void act(ops.reopen)} aria-label={t(session ? 'session.reopen' : 'thread.reopen')}><Undo fontSize="small" /></IconButton>
                            </Tooltip>
                        )}
                    <IconButton size="small" onClick={(e) => setMenu(e.currentTarget)} aria-label={t('common.more')}><MoreVert fontSize="small" /></IconButton>
                    <Menu anchorEl={menu} open={!!menu} onClose={() => setMenu(null)} anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }} transformOrigin={{ vertical: 'top', horizontal: 'right' }}>
                        <MenuItem onClick={() => { setMenu(null); setTaskOpen((v) => !v); }}>{taskOpen ? t('thread.hideTask') : t('thread.showTask')}</MenuItem>
                        <MenuItem onClick={() => { setMenu(null); setView(view === 'chat' ? 'trajectory' : 'chat'); }}>{view === 'chat' ? t('thread.showTrajectory') : t('thread.showChat')}</MenuItem>
                        {view === 'chat' && <MenuItem onClick={() => { setMenu(null); toggleExpand(); }}>{expandAll ? t('thread.collapseTools') : t('thread.expandTools')}</MenuItem>}
                        {/* The panel opens where the menu was, under the More button. */}
                        {session && <MenuItem onClick={() => { setTasksAnchor(menu); setMenu(null); }}>{t('session.backgroundTasks')}</MenuItem>}
                        {session && thread.status !== 'resolved' && (
                            <MenuItem disabled={running || acting} onClick={() => guarded('handoff')}>{running ? t('session.handoffBusy') : t('session.handoff')}</MenuItem>
                        )}
                        {/* Run details, for when they matter: kept off the header. */}
                        {(thread.model || thread.input_tokens > 0 || thread.output_tokens > 0) && <Divider />}
                        {(thread.model || thread.input_tokens > 0 || thread.output_tokens > 0) && (
                            <Box sx={{ px: 2, py: 0.75, fontSize: fontSizes.sm, color: 'text.secondary' }}>
                                {thread.model && <Box sx={{ ...mono }}>{thread.model}</Box>}
                                {(thread.input_tokens > 0 || thread.output_tokens > 0) && <Box>{formatTokens(thread.input_tokens)} in / {formatTokens(thread.output_tokens)} out</Box>}
                            </Box>
                        )}
                    </Menu>
                </Box>
                <Box sx={{ pl: hideTitle ? 0.5 : 4.5 }}>
                    <MetaLine items={[
                        <ThreadStatusChip thread={thread} />,
                        thread.branch
                            ? <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                                <Branch sx={{ fontSize: 14, flexShrink: 0 }} />
                                <Box component="span" sx={{ ...mono, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0 }} title={t('thread.fromBase', { base: thread.base_branch })}>{thread.branch}</Box>
                                <CopyIconButton value={`cd ${thread.worktree}`} label={t('thread.copyCd')} copiedLabel={t('common.copied')} iconSize={13} />
                            </Box>
                            : session
                                ? <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                                    <FolderOpen sx={{ fontSize: 14, flexShrink: 0 }} />
                                    <Box component="span" sx={{ ...mono, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0 }} title={thread.worktree}>{folderName(thread.worktree)}</Box>
                                    <CopyIconButton value={`cd ${thread.worktree}`} label={t('thread.copyCd')} copiedLabel={t('common.copied')} iconSize={13} />
                                </Box>
                                : t('thread.noRepo'),
                        thread.merged_at
                            ? <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, color: toneColor('success') }}><Merge sx={{ fontSize: 13 }} />{t('thread.mergedInto', { base: thread.base_branch })}</Box>
                            : thread.commits_ahead > 0 && <>{t('thread.commits', { count: thread.commits_ahead })} · {t('thread.files', { count: thread.files_changed })}</>,
                        thread.uncommitted && <Box component="span" sx={{ color: toneColor('warning') }}>{t('thread.uncommitted')}</Box>,
                    ]} />
                </Box>
                <Collapse in={taskOpen}>
                    <Box sx={{ mt: 1, p: 1.25, bgcolor: 'action.hover', borderRadius: 1, maxHeight: 220, overflow: 'auto' }}><Markdown text={thread.task} /></Box>
                </Collapse>
                {/* A pending request card is already the call to action. */}
                {thread.status === 'waiting' && thread.wait_reason && !pendingRequestId && (
                    <Alert severity="warning" sx={{ mt: 1, py: 0 }}>{thread.wait_reason}</Alert>
                )}
            </Box>

            <Box ref={scrollRef} onScroll={() => {
                const el = scrollRef.current;
                if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
            }} sx={{ flex: 1, overflowY: 'auto', overflowX: 'hidden', px: 2, py: 2, overscrollBehavior: 'contain' }}>
                {events === null
                    ? <Skeleton variant="rounded" height={120} />
                    : view === 'chat'
                        ? <Transcript blocks={blocks} pendingRequestId={pendingRequestId} working={running} newSince={newSince.at} expandAll={expandAll} onRespond={respond} codeBlock={CodeBlock} rememberLabel={rememberLabel} />
                        : <Trajectory blocks={blocks} working={running} pendingRequestId={pendingRequestId} project={thread.worktree} onOpen={openInChat} />}
            </Box>

            <Box sx={{ px: 2, pb: 2 }}>
                {handoff && (
                    <Alert severity="info" variant="outlined" icon={<Terminal fontSize="small" />} onClose={() => setHandoff(null)}
                        sx={{ mb: 1, '& .MuiAlert-message': { minWidth: 0, flex: 1 } }}>
                        <Typography variant="body2" sx={{ color: 'text.primary', mb: 0.5 }}>{t('session.handoffReady')}</Typography>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                            <Box component="code" sx={{ ...mono, flex: 1, minWidth: 0, overflowX: 'auto', whiteSpace: 'nowrap', fontSize: fontSizes.md, px: 1, py: 0.5, borderRadius: 1, bgcolor: 'action.hover', color: 'text.primary' }}>
                                {handoff}
                            </Box>
                            <CopyIconButton value={handoff} label={t('common.copy')} copiedLabel={t('common.copied')} />
                        </Box>
                    </Alert>
                )}
                <Composer
                    context={composerContext}
                    minRows={1}
                    keepUntilSent
                    text={draft}
                    onTextChange={setDraft}
                    // Clear only what was sent: typing may have gone on meanwhile.
                    onAccepted={(sent) => setDraftState((prev) => {
                        if (prev !== sent) return prev;
                        removeLocal(draftKey);
                        return '';
                    })}
                    streaming={running}
                    sendWhileStreaming
                    onStop={() => { void act(ops.stop); }}
                    disabled={disabled || thread.status === 'resolved'}
                    placeholder={thread.status === 'resolved' ? t(session ? 'session.archivedPlaceholder' : 'thread.resolvedPlaceholder') : running ? t(session ? 'session.queuePlaceholder' : 'thread.queuePlaceholder') : t(session ? 'session.placeholder' : 'thread.placeholder')}
                    onSend={async (text) => {
                        try {
                            await ops.send(text);
                            stick.current = true;
                            return true;
                        } catch (e) {
                            notify((e as Error).message, 'error');
                            return false;
                        }
                    }}
                />
                {belowComposer}
            </Box>
            <DiffDialog thread={thread} open={diffOpen} onClose={() => setDiffOpen(false)} />
            {session && (
                <Popover open={!!tasksAnchor} anchorEl={tasksAnchor} onClose={() => setTasksAnchor(null)}
                    anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }} transformOrigin={{ vertical: 'top', horizontal: 'right' }}>
                    <BackgroundTasksPanel sessionId={threadId} tasks={tasks} onChanged={() => onRefresh?.()} onReveal={(call) => openInChat({ call })} />
                </Popover>
            )}
            <ConfirmDialog
                open={confirm !== null}
                title={confirm === 'handoff' ? t('session.handoffConfirm') : t('session.archiveConfirm')}
                description={t('session.tasksWillStop', { count: liveTasks.length })}
                loading={acting}
                cancelLabel={t('common.cancel')}
                confirmLabel={confirm === 'handoff' ? t('session.handoff') : t('session.archive')}
                confirmColor="warning"
                onClose={() => setConfirm(null)}
                onConfirm={() => confirm && void runConfirmed(confirm)}
            />
        </Box>
    );
}
