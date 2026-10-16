import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    Alert, Box, Button, Collapse, Divider, IconButton, Menu, MenuItem,
    Skeleton, Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread, ThreadEvent } from '../../client';
import { buildTranscript, Transcript, type Respond } from '../../transcript';
import { Composer } from '../Composer';
import { CodeBlock, Markdown } from '../Markdown';
import { CopyIconButton } from '@tingly/ui';
import { ArrowBack, Branch, DiffIcon, FolderOpen, Merge, MoreVert } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { folderName, formatTokens } from '../../utils/format';
import { DiffDialog } from './DiffDialog';
import { ThreadStatusChip, toneColor } from './threadStatus';
import { threadTranscript } from './threadTranscript';
import { projectThreadOps, sessionOps } from './threadOps';
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
}

export function ThreadPanel({ projectId, threadId, thread, events, running, disabled, onBack, hideTitle, actions, composerContext, belowComposer }: Props) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [diffOpen, setDiffOpen] = useState(false);
    const [menu, setMenu] = useState<HTMLElement | null>(null);
    const [taskOpen, setTaskOpen] = useState(false);
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

    const respond: Respond = async (requestId, allow, answer, remember) => {
        try {
            await ops.respond(requestId, allow, !!remember, answer);
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
        } catch (e) {
            notify((e as Error).message, 'error');
        }
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
                    {actions}
                    <IconButton size="small" onClick={(e) => setMenu(e.currentTarget)} aria-label={t('common.more')}><MoreVert fontSize="small" /></IconButton>
                    <Menu anchorEl={menu} open={!!menu} onClose={() => setMenu(null)} anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }} transformOrigin={{ vertical: 'top', horizontal: 'right' }}>
                        <MenuItem onClick={() => { setMenu(null); setTaskOpen((v) => !v); }}>{taskOpen ? t('thread.hideTask') : t('thread.showTask')}</MenuItem>
                        {thread.status !== 'resolved'
                            ? <MenuItem onClick={() => { setMenu(null); void act(ops.finish); }}>{t(session ? 'session.archive' : 'thread.resolve')}</MenuItem>
                            : <MenuItem onClick={() => { setMenu(null); void act(ops.reopen); }}>{t(session ? 'session.reopen' : 'thread.reopen')}</MenuItem>}
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
                    : <Transcript blocks={blocks} pendingRequestId={pendingRequestId} working={running} onRespond={respond} codeBlock={CodeBlock} rememberLabel={rememberLabel} />}
            </Box>

            <Box sx={{ px: 2, pb: 2 }}>
                <Composer
                    context={composerContext}
                    minRows={1}
                    keepUntilSent
                    streaming={running}
                    sendWhileStreaming
                    onStop={() => act(ops.stop)}
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
        </Box>
    );
}
