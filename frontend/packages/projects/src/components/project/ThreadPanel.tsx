import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    Alert, Box, Button, Collapse, Divider, IconButton, Menu, MenuItem,
    Popover, Skeleton, Tooltip, Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Attachment, BackgroundTask, Subagent, Thread, ThreadEvent } from '../../client';
import { buildTranscript, SubagentFocus, Trajectory, Transcript, type Respond, type TrajectoryAnchor } from '../../transcript';
import type { TranscriptSubagents } from '../../hooks/useTranscript';
import { ThreadComposer, type ThreadComposerHandle } from '../ThreadComposer';
import { HeldMessages, heldMessages } from './HeldMessages';
import { AttachmentChips, uploadAttachments, type AttachOwner } from '../attachments';
import { CodeBlock, Markdown } from '../Markdown';
import { ConfirmDialog, CopyIconButton } from '@tingly/ui';
import { ArrowBack, DiffIcon, Merge, MoreVert, Robot, Stream, Terminal } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { formatTokens } from '../../utils/format';
import { DiffDialog } from './DiffDialog';
import { ComposerStrip } from './ComposerStrip';
import { RAIL_MIN_PANEL, ThreadInfoRail } from './ThreadInfoRail';
import { LoadEarlier } from './LoadEarlier';
import { needsUser, ThreadState, ThreadStateMark } from './threadStatus';
import { threadTranscript } from './threadTranscript';
import { threadOps } from './threadOps';
import { BackgroundTasksPanel } from '../workdir/BackgroundTasksPanel';
import { backgroundTasks } from '../workdir/backgroundTasks';
import { SubagentsPanel } from '../workdir/SubagentsPanel';
import { readLocal, writeLocal } from '../../utils/storage';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

const mono = { fontFamily: fontMono } as const;

/** Facts about a thread on one line, separated by dots; empty ones drop out. */
function MetaLine({ items }: { items: React.ReactNode[] }) {
    const shown = items.filter(Boolean);
    if (shown.length === 0) return null;
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: 0.75, rowGap: 0.25, color: 'text.secondary', fontSize: fontSizes.sm }}>
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
    /** The project (or folder) the thread belongs to. */
    projectId: string;
    /** The thread is a working folder's: no branch, review or merge, and it can be handed off to a terminal. */
    folder?: boolean;
    threadId: string;
    thread?: Thread;
    events: ThreadEvent[] | null;
    /** Older entries exist than the loaded ones. */
    more?: boolean;
    onLoadEarlier?: () => Promise<void>;
    /** Reads a subagent's steps when its card opens (they are not in a page of the transcript), and finds subagents. */
    subagents?: TranscriptSubagents;
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
    /** A card the rail carries above its own (a folder's session list); the rail then stays on, from `railMinPanel` wide. */
    railTop?: React.ReactNode;
    railMinPanel?: number;
}

type View = 'chat' | 'trajectory';

const anchorSelector = (anchor: TrajectoryAnchor): string => {
    if ('call' in anchor) return `[data-call-ids~="${CSS.escape(anchor.call)}"]`;
    if ('request' in anchor) return `[data-request-id="${CSS.escape(anchor.request)}"]`;
    return `[data-block="${anchor.block}"]`;
};

export function ThreadPanel({ projectId, folder = false, threadId, thread, events, more = false, onLoadEarlier, subagents, running, disabled, onBack, hideTitle, actions, composerContext, belowComposer, liveTasks = [], onRefresh, railTop, railMinPanel = RAIL_MIN_PANEL }: Props) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [diffOpen, setDiffOpen] = useState(false);
    const [menu, setMenu] = useState<HTMLElement | null>(null);
    const [taskOpen, setTaskOpen] = useState(false);
    // The info rail sits beside the conversation when the panel is wide enough; the user can hide it.
    const panelRef = useRef<HTMLDivElement>(null);
    const [wide, setWide] = useState(false);
    const [railOn, setRailOn] = useState(() => readLocal('thread.rail') !== '0');
    useEffect(() => {
        const el = panelRef.current;
        if (!el || typeof ResizeObserver === 'undefined') return;
        const ro = new ResizeObserver(([entry]) => setWide(entry.contentRect.width >= railMinPanel));
        ro.observe(el);
        return () => ro.disconnect();
        // The panel's box exists once the thread has loaded.
    }, [!!thread, railMinPanel]);
    const toggleRail = () => {
        writeLocal('thread.rail', railOn ? '0' : '1');
        setRailOn(!railOn);
    };
    const rail = wide && (railOn || railTop != null);
    // Chat and Trajectory read the same transcript; the choice and whether
    // every tool call starts open are remembered per browser.
    const [view, setViewState] = useState<View>(() => readLocal('thread.view') === 'trajectory' ? 'trajectory' : 'chat');
    const setView = (next: View) => {
        setViewState(next);
        writeLocal('thread.view', next);
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
    // A subagent opened on its own, in place of the conversation.
    const [focus, setFocus] = useState<{ threadId: string; call: string } | null>(null);
    const focused = focus?.threadId === threadId ? focus.call : null;
    const [agentsAnchor, setAgentsAnchor] = useState<HTMLElement | null>(null);
    // The opened subagent's call and result, read when they are not on a loaded page.
    const [own, setOwn] = useState<{ call: string; events: ThreadEvent[] } | null>(null);
    const [tasksAnchor, setTasksAnchor] = useState<HTMLElement | null>(null);
    // The handoff command stays on screen with its own copy button, since
    // copying right after the request can be refused by the browser.
    const [handoff, setHandoff] = useState<string | null>(null);
    const [confirm, setConfirm] = useState<'archive' | 'handoff' | null>(null);
    const [acting, setActing] = useState(false);
    const scrollRef = useRef<HTMLDivElement>(null);
    const stick = useRef(true);
    // A folder thread has no branch to review or merge; its uploads are its own.
    const session = folder;
    const owner = useMemo<AttachOwner>(() => (session ? { threadId } : { projectId }), [session, projectId, threadId]);
    const Attachments = useMemo(() => ({ items }: { items: Attachment[] }) => <AttachmentChips owner={owner} items={items} />, [owner]);
    const ops = useMemo(() => threadOps(threadId), [threadId]);
    const composer = useRef<ThreadComposerHandle>(null);
    // Messages held for after the running turn.
    const held = useMemo(() => heldMessages(events), [events]);
    // Taking a held message back puts its text in the box to edit; the draft
    // keeps whatever was typed meanwhile.
    const takeBack = async (ids: string[], intoDraft: boolean) => {
        const taken = held.filter((h) => ids.includes(h.id));
        if (intoDraft && taken.length > 0) {
            const text = taken.map((h) => h.content).filter((c) => c.trim() !== '').join('\n\n');
            if (text) composer.current?.addToDraft(text);
        }
        await Promise.all(taken.map((h) => ops.unqueue(h.id).catch(() => undefined)));
    };

    useEffect(() => {
        const el = scrollRef.current;
        if (el && stick.current) el.scrollTop = el.scrollHeight;
    }, [events]);

    // Any thread's subagent can be opened on its own: its steps are read apart.
    const host = useMemo(() => subagents && { ...subagents, focus: (call: string) => setFocus({ threadId, call }) }, [subagents, threadId]);

    const coordinator = t('thread.fromCoordinator');
    const { blocks, pendingRequestId } = useMemo(() => {
        const { entries, pendingRequestId } = threadTranscript(events ?? [], running, (e) => e.source === 'thread' ? coordinator : undefined);
        return { blocks: buildTranscript(entries), pendingRequestId };
    }, [events, running, coordinator]);

    const focusCallLoaded = !!focused && !!events?.some((e) => !e.parent && e.kind === 'tool_use' && e.request_id === focused);
    useEffect(() => {
        if (!focused || focusCallLoaded || !subagents || own?.call === focused) return;
        subagents.own(focused).then((evs) => setOwn({ call: focused, events: evs })).catch((e: Error) => notify(e.message, 'error'));
    }, [focused, focusCallLoaded, subagents, own, notify]);
    // The opened subagent as its card would show it: its call and result
    // (loaded, or read on their own) and its loaded steps.
    const focusBlock = useMemo(() => {
        if (!focused) return null;
        const all = events ?? [];
        const loaded = all.filter((e) => !e.parent && e.request_id === focused);
        const ids = new Set(loaded.map((e) => e.id));
        const extra = focusCallLoaded || own?.call !== focused ? [] : own.events.filter((e) => !ids.has(e.id));
        if (!focusCallLoaded && extra.length === 0) return null;
        const list = [...extra, ...loaded, ...all.filter((e) => e.parent === focused)]
            .sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime());
        const b = buildTranscript(threadTranscript(list, running, () => undefined).entries).find((x) => x.type === 'agent' && x.call.id === focused);
        return b?.type === 'agent' ? b : null;
    }, [focused, events, own, focusCallLoaded, running]);
    useEffect(() => {
        // An opened subagent starts at what it was asked.
        if (!focused || !scrollRef.current) return;
        stick.current = false;
        scrollRef.current.scrollTop = 0;
    }, [focused]);
    const focusSteps = focused ? host?.steps(focused) : undefined;
    const focusType = focusBlock?.task?.subagentType || (focusBlock?.call.input as { subagent_type?: string } | undefined)?.subagent_type;

    // The waiting question's own deadline, for the strip's folded line.
    const pendingAutoAt = useMemo(() => {
        const b = blocks.find((x) => x.type === 'request' && !!pendingRequestId && x.message.request_id === pendingRequestId);
        return b?.type === 'request' ? b.message.autoAt : undefined;
    }, [blocks, pendingRequestId]);

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
    // Shows a message in the conversation, reading the conversation back to
    // it first when it is older than the loaded page.
    const openInChat = async (anchor: TrajectoryAnchor, eventId?: string) => {
        setTasksAnchor(null);
        setAgentsAnchor(null);
        if (!focused && view === 'chat' && scrollRef.current?.querySelector(anchorSelector(anchor))) {
            flash(anchor);
            return;
        }
        if ('call' in anchor && subagents) {
            try {
                await subagents.locate(anchor.call, eventId);
            } catch (e) {
                notify((e as Error).message, 'error');
            }
        }
        setFocus(null);
        setView('chat');
        setTarget(anchor);
    };
    const openAgent = (agent: Subagent) => {
        setAgentsAnchor(null);
        setView('chat');
        setFocus({ threadId, call: agent.call });
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
        <Box ref={panelRef} sx={{ display: 'flex', height: '100%', minHeight: 0 }}>
        <Box sx={{ display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0, height: '100%', minHeight: 0 }}>
            <Box sx={{ px: 1.5, pt: 1, pb: 1, borderBottom: 1, borderColor: 'divider' }}>
                {/* One row to name the thread and act on it, one quiet row of facts. */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                    {!hideTitle && <IconButton size="small" onClick={onBack} aria-label={t('thread.back')}><ArrowBack fontSize="small" /></IconButton>}
                    {hideTitle
                        ? <Box sx={{ flex: 1 }} />
                        : <Typography noWrap sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.base, fontWeight: 650, color: 'text.primary' }} title={thread.title}>{thread.title}</Typography>}
                    {canReview && <Button size="small" variant="contained" startIcon={<DiffIcon />} onClick={() => setDiffOpen(true)} sx={{ flexShrink: 0, py: 0.25 }}>{t('thread.review')}</Button>}
                    {/* While work runs in the background it names itself. */}
                    {session && liveTasks.length > 0 && !rail && (
                        <Tooltip title={t('session.backgroundTasks')}>
                            <Button size="small" variant="outlined" color="inherit" startIcon={<Stream sx={{ fontSize: '16px !important' }} />} onClick={(e) => setTasksAnchor(e.currentTarget)}
                                sx={{ borderRadius: radius.pill, borderColor: 'divider', color: 'text.secondary', py: 0, px: 1.25, minWidth: 0, whiteSpace: 'nowrap', flexShrink: 0 }}>
                                {t('session.tasksRunning', { count: liveTasks.length })}
                            </Button>
                        </Tooltip>
                    )}
                    {actions}
                    <IconButton size="small" onClick={(e) => setMenu(e.currentTarget)} aria-label={t('common.more')}><MoreVert fontSize="small" /></IconButton>
                    <Menu anchorEl={menu} open={!!menu} onClose={() => setMenu(null)} anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }} transformOrigin={{ vertical: 'top', horizontal: 'right' }}>
                        <MenuItem onClick={() => { setMenu(null); setTaskOpen((v) => !v); }}>{taskOpen ? t('thread.hideTask') : t('thread.showTask')}</MenuItem>
                        <MenuItem onClick={() => { setMenu(null); setFocus(null); setView(view === 'chat' ? 'trajectory' : 'chat'); }}>{view === 'chat' ? t('thread.showTrajectory') : t('thread.showChat')}</MenuItem>
                        {view === 'chat' && <MenuItem onClick={() => { setMenu(null); toggleExpand(); }}>{expandAll ? t('thread.collapseTools') : t('thread.expandTools')}</MenuItem>}
                        {/* The panel opens where the menu was, under the More button. */}
                        {session && !rail && <MenuItem onClick={() => { setTasksAnchor(menu); setMenu(null); }}>{t('session.backgroundTasks')}</MenuItem>}
                        {wide && railTop == null && <MenuItem onClick={() => { setMenu(null); toggleRail(); }}>{railOn ? t('rail.hide') : t('rail.show')}</MenuItem>}
                        {subagents && <MenuItem onClick={() => { setAgentsAnchor(menu); setMenu(null); }}>{t('subagents.title')}</MenuItem>}
                        {session && thread.status !== 'resolved' && (
                            <MenuItem disabled={running || acting} onClick={() => guarded('handoff')}>{running ? t('session.handoffBusy') : t('session.handoff')}</MenuItem>
                        )}
                        {/* Finishing sits in the menu, apart from the buttons used all the time: a stray click cannot archive. */}
                        <Divider />
                        {thread.status !== 'resolved'
                            ? <MenuItem disabled={acting} onClick={() => { setMenu(null); if (session) guarded('archive'); else void act(ops.finish); }}>{t(session ? 'session.archive' : 'thread.resolve')}</MenuItem>
                            : <MenuItem disabled={acting} onClick={() => { setMenu(null); void act(ops.reopen); }}>{t(session ? 'session.reopen' : 'thread.reopen')}</MenuItem>}
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
                        <ThreadState status={thread.status} />,
                        // The folder and branch float above the message box.
                        !thread.branch && !session && t('thread.noRepo'),
                        // The rail's Changes card carries these.
                        !rail && (thread.merged_at
                            ? <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}><Merge sx={{ fontSize: 13 }} />{t('thread.mergedInto', { base: thread.base_branch })}</Box>
                            : thread.commits_ahead > 0 && <>{t('thread.commits', { count: thread.commits_ahead })} · {t('thread.files', { count: thread.files_changed })}</>),
                        !rail && thread.uncommitted && t('thread.uncommitted'),
                    ]} />
                </Box>
                <Collapse in={taskOpen}>
                    <Box sx={{ mt: 1, p: 1.25, bgcolor: 'action.hover', borderRadius: radius.row, maxHeight: 220, overflow: 'auto' }}><Markdown text={thread.task} /></Box>
                </Collapse>
                {/* A pending request card is already the call to action. */}
                {needsUser(thread.status) && thread.wait_reason && !pendingRequestId && (
                    <Box sx={{ mt: 1, display: 'flex', alignItems: 'flex-start', gap: 1, px: 1.25, py: 0.75, borderRadius: radius.row, bgcolor: 'action.hover', fontSize: fontSizes.md, color: 'text.primary', overflowWrap: 'anywhere' }}>
                        <Box sx={{ pt: '3px' }}><ThreadStateMark status={thread.status} /></Box>
                        <Box>{thread.wait_reason}</Box>
                    </Box>
                )}
            </Box>

            {focused && (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.5, borderBottom: 1, borderColor: 'divider', bgcolor: 'action.hover' }}>
                    <Button size="small" color="inherit" startIcon={<ArrowBack sx={{ fontSize: '16px !important' }} />} onClick={() => setFocus(null)} sx={{ color: 'text.secondary', flexShrink: 0 }}>
                        {t('subagents.back')}
                    </Button>
                    <Robot sx={{ fontSize: 16, color: 'text.secondary', flexShrink: 0 }} />
                    <Typography variant="body2" noWrap sx={{ flex: 1, minWidth: 0, fontWeight: 600, color: 'text.primary' }}>
                        {(focusBlock?.call.input as { description?: string } | undefined)?.description || t('subagents.title')}
                    </Typography>
                    {focusType && <Typography variant="caption" sx={{ color: 'text.secondary', flexShrink: 0 }}>{focusType}</Typography>}
                    <Button size="small" onClick={() => void openInChat({ call: focused }, own?.call === focused ? own.events.find((e) => e.kind === 'tool_use')?.id : undefined)} sx={{ flexShrink: 0, whiteSpace: 'nowrap' }}>{t('subagents.show')}</Button>
                </Box>
            )}
            <Box ref={scrollRef} onScroll={() => {
                const el = scrollRef.current;
                if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
            }} sx={{ flex: 1, overflowY: 'auto', overflowX: 'hidden', px: 2, py: 2, overscrollBehavior: 'contain' }}>
                {!focused && events !== null && more && onLoadEarlier && <LoadEarlier scrollRef={scrollRef} onLoad={onLoadEarlier} />}
                {focused && focusSteps?.loaded && focusSteps.more && host && <LoadEarlier scrollRef={scrollRef} label={t('subagents.earlier')} onLoad={() => host.load(focused, true)} />}
                {events === null || (focused && !focusBlock)
                    ? <Skeleton variant="rounded" height={120} />
                    : focused && focusBlock
                        ? <SubagentFocus block={focusBlock} subagents={host} working={running} pendingRequestId={pendingRequestId} expandAll={expandAll} onRespond={respond} codeBlock={CodeBlock} attachments={Attachments} rememberLabel={rememberLabel} />
                    : view === 'chat'
                        ? <Transcript blocks={blocks} subagents={host} pendingRequestId={pendingRequestId} working={running} newSince={newSince.at} expandAll={expandAll} onRespond={respond} codeBlock={CodeBlock} attachments={Attachments} rememberLabel={rememberLabel} />
                        : <Trajectory blocks={blocks} working={running} pendingRequestId={pendingRequestId} project={thread.cwd} onOpen={(anchor) => void openInChat(anchor)} />}
            </Box>

            <Box sx={{ px: 2, pb: 2 }}>
                {handoff && (
                    <Alert severity="info" variant="outlined" icon={<Terminal fontSize="small" />} onClose={() => setHandoff(null)}
                        sx={{ mb: 1, '& .MuiAlert-message': { minWidth: 0, flex: 1 } }}>
                        <Typography variant="body2" sx={{ color: 'text.primary', mb: 0.5 }}>{t('session.handoffReady')}</Typography>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                            <Box component="code" sx={{ ...mono, flex: 1, minWidth: 0, overflowX: 'auto', whiteSpace: 'nowrap', fontSize: fontSizes.sm, px: 1, py: 0.5, borderRadius: radius.row, bgcolor: 'action.hover', color: 'text.primary' }}>
                                {handoff}
                            </Box>
                            <CopyIconButton value={handoff} label={t('common.copy')} copiedLabel={t('common.copied')} />
                        </Box>
                    </Alert>
                )}
                <ComposerStrip thread={thread} folder={session} autoAt={pendingAutoAt} onAutoAnswer={(mode) => { void act(() => ops.setAutoAnswer(mode)).then((ok) => ok && onRefresh?.()); }} />
                <HeldMessages held={held} onEdit={(h) => void takeBack([h.id], true)} onRemove={(h) => void takeBack([h.id], false)} />
                <ThreadComposer
                    ref={composer}
                    draftKey={threadId}
                    context={composerContext}
                    streaming={running}
                    sendModes
                    onStop={() => {
                        // Stop drops what was held: hand it back to the box rather than lose it.
                        if (held.length > 0) {
                            const text = held.map((h) => h.content).filter((c) => c.trim() !== '').join('\n\n');
                            if (text) composer.current?.addToDraft(text);
                            notify(t('queue.returned'), 'info');
                        }
                        void act(ops.stop);
                    }}
                    disabled={disabled || thread.status === 'resolved'}
                    placeholder={thread.status === 'resolved' ? t(session ? 'session.archivedPlaceholder' : 'thread.resolvedPlaceholder') : running ? t(session ? 'session.queuePlaceholder' : 'thread.queuePlaceholder') : t(session ? 'session.placeholder' : 'thread.placeholder')}
                    attach={(files) => uploadAttachments(owner, files)}
                    onSend={async (text, attachments, mode) => {
                        try {
                            await ops.send(text, attachments, mode);
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
                    <BackgroundTasksPanel sessionId={threadId} tasks={tasks} onChanged={() => onRefresh?.()} onReveal={(call) => void openInChat({ call })} />
                </Popover>
            )}
            {subagents && (
                <Popover open={!!agentsAnchor} anchorEl={agentsAnchor} onClose={() => setAgentsAnchor(null)}
                    anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }} transformOrigin={{ vertical: 'top', horizontal: 'right' }}>
                    <SubagentsPanel load={subagents.list} turnLive={running} onOpen={openAgent} onShow={(agent) => void openInChat({ call: agent.call }, agent.event_id)} />
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
        {rail && (
            <ThreadInfoRail
                thread={thread}
                folder={session}
                tasks={tasks}
                canReview={canReview}
                onReview={() => setDiffOpen(true)}
                onChanged={() => onRefresh?.()}
                onReveal={(call) => void openInChat({ call })}
                top={railTop}
                permission={session ? (thread.permission_mode || t('session.permissionDefault')) : undefined}
            />
        )}
        </Box>
    );
}
