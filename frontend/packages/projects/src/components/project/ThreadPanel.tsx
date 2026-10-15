import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    Alert, Box, Button, Collapse, Dialog, DialogActions, DialogContent, DialogTitle, IconButton,
    Skeleton, Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, ApiError, unwrap, type Diff, type Thread, type ThreadEvent } from '../../client';
import { buildTranscript, Transcript, type Respond } from '../../transcript';
import { Composer } from '../Composer';
import { CodeBlock, Markdown } from '../Markdown';
import { CopyIconButton } from '@tingly/ui';
import { ArrowBack, Branch, DiffIcon, Merge, Stop } from '../icons';
import { useCopyFeedback } from '../../hooks/useCopyFeedback';
import { useNotify } from '../../hooks/useNotify';
import { formatTokens } from '../../utils/format';
import { ThreadStatusChip } from './threadStatus';
import { threadTranscript } from './threadTranscript';

const mono = { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' } as const;

function DiffDialog({ thread, open, onClose }: { thread: Thread; open: boolean; onClose: () => void }) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [diff, setDiff] = useState<Diff | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [blocked, setBlocked] = useState<{ error: string; command: string } | null>(null);
    const [busy, setBusy] = useState(false);
    const { copied, copy } = useCopyFeedback();

    useEffect(() => {
        if (!open) return;
        setDiff(null);
        setError(null);
        setBlocked(null);
        unwrap(api.GET('/api/v1/projects/{id}/threads/{tid}/diff', { params: { path: { id: thread.project_id, tid: thread.id } } })).then(setDiff).catch((e: Error) => setError(e.message));
    }, [open, thread.id]);

    const merge = async () => {
        setBusy(true);
        try {
            await unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/merge', { params: { path: { id: thread.project_id, tid: thread.id } } }));
            notify(t('thread.merged', { branch: thread.branch, base: thread.base_branch }), 'success');
            onClose();
        } catch (e) {
            if (e instanceof ApiError && e.status === 409 && (e.body as { command?: string })?.command) {
                setBlocked(e.body as { error: string; command: string });
            } else {
                notify((e as Error).message, 'error');
            }
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <DiffIcon />
                <Box component="span" sx={{ flex: 1 }}>{thread.title}</Box>
                <Typography component="span" sx={{ ...mono, fontSize: 13, color: 'text.secondary' }}>{thread.base_branch}…{thread.branch}</Typography>
            </DialogTitle>
            <DialogContent dividers>
                {error && <Alert severity="error">{error}</Alert>}
                {!diff && !error && <Skeleton variant="rounded" height={200} />}
                {diff && (
                    <>
                        {diff.uncommitted && <Alert severity="warning" sx={{ mb: 1.5 }}>{t('thread.uncommittedWarn')}<Box component="pre" sx={{ ...mono, fontSize: 12, m: 0, mt: 0.5 }}>{diff.uncommitted}</Box></Alert>}
                        <Box component="pre" sx={{ ...mono, fontSize: 12.5, m: 0, mb: 1.5, color: 'text.secondary', whiteSpace: 'pre-wrap' }}>{diff.stat || t('thread.noChanges')}</Box>
                        <Box component="pre" sx={{ ...mono, fontSize: 12.5, m: 0, p: 1.5, bgcolor: 'action.hover', borderRadius: 2, overflow: 'auto', maxHeight: '55vh' }}>
                            {diff.patch.split('\n').map((line, i) => (
                                <Box key={i} component="div" sx={{
                                    color: line.startsWith('+') && !line.startsWith('+++') ? 'success.main' : line.startsWith('-') && !line.startsWith('---') ? 'error.main' : line.startsWith('@@') ? 'info.main' : 'text.primary',
                                    fontWeight: line.startsWith('diff --git') ? 700 : 400,
                                }}>{line || ' '}</Box>
                            ))}
                        </Box>
                        {diff.truncated && <Typography variant="caption" color="text.secondary">{t('thread.truncated')}</Typography>}
                    </>
                )}
                {blocked && (
                    <Alert severity="warning" sx={{ mt: 1.5, '& .MuiAlert-message': { width: '100%' } }}>
                        <Typography variant="body2" sx={{ color: 'inherit', fontWeight: 600 }}>{t('thread.mergeBlocked')}: {blocked.error}</Typography>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 1, p: 1, bgcolor: 'background.paper', borderRadius: 1 }}>
                            <Box component="code" sx={{ ...mono, fontSize: 12.5, flex: 1, wordBreak: 'break-all' }}>{blocked.command}</Box>
                            <Button size="small" onClick={() => copy(blocked.command)}>{copied ? t('common.copied') : t('common.copy')}</Button>
                        </Box>
                    </Alert>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 1.5 }}>
                <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>{t('thread.mergeHint', { base: thread.base_branch, repo: thread.repo_path })}</Typography>
                <Button onClick={onClose} color="inherit">{t('common.close')}</Button>
                <Button variant="contained" startIcon={<Merge />} disabled={busy || !diff || !diff.patch} onClick={() => void merge()}>{t('thread.mergeIt')}</Button>
            </DialogActions>
        </Dialog>
    );
}

/** Facts about a thread on one line, separated by dots; empty ones drop out. */
function MetaLine({ items }: { items: React.ReactNode[] }) {
    const shown = items.filter(Boolean);
    if (shown.length === 0) return null;
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: 0.75, mt: 0.25, color: 'text.secondary', fontSize: 12.5 }}>
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
    projectId: string;
    threadId: string;
    thread?: Thread;
    events: ThreadEvent[] | null;
    running: boolean;
    disabled: boolean;
    onBack: () => void;
}

export function ThreadPanel({ projectId, threadId, thread, events, running, disabled, onBack }: Props) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [diffOpen, setDiffOpen] = useState(false);
    const [taskOpen, setTaskOpen] = useState(false);
    const scrollRef = useRef<HTMLDivElement>(null);
    const stick = useRef(true);

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
            await unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/respond', {
                params: { path: { id: projectId, tid: threadId } },
                body: { request_id: requestId, allow, scope: remember ? 'thread' : 'once', answer: answer || undefined },
            }));
            return true;
        } catch (e) {
            notify((e as Error).message, 'error');
            return false;
        }
    };
    const rememberLabel = useCallback((tool: string) => t('thread.allowThread', { tool }), [t]);

    const act = async (path: '/api/v1/projects/{id}/threads/{tid}/stop' | '/api/v1/projects/{id}/threads/{tid}/resolve' | '/api/v1/projects/{id}/threads/{tid}/reopen') => {
        try {
            await unwrap(api.POST(path, { params: { path: { id: projectId, tid: threadId } } }));
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
            <Box sx={{ px: 2, pt: 1.5, pb: 1, borderBottom: 1, borderColor: 'divider' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <IconButton size="small" onClick={onBack} aria-label={t('thread.back')}><ArrowBack fontSize="small" /></IconButton>
                    <Typography variant="h5" noWrap sx={{ flex: 1, minWidth: 0 }}>{thread.title}</Typography>
                    <ThreadStatusChip thread={thread} />
                </Box>
                {thread.branch ? (
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.5, color: 'text.secondary', fontSize: 12.5, minWidth: 0 }}>
                        <Branch sx={{ fontSize: 14, flexShrink: 0 }} />
                        <Box component="span" sx={{ ...mono, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0 }}>{thread.branch}</Box>
                        <Box component="span" sx={{ flexShrink: 0 }}>{t('thread.fromBase', { base: thread.base_branch })}</Box>
                        <CopyIconButton value={`cd ${thread.worktree}`} label={t('thread.copyCd')} copiedLabel={t('common.copied')} iconSize={14} />
                    </Box>
                ) : (
                    <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }}>{t('thread.noRepo')}</Typography>
                )}
                <MetaLine items={[
                    thread.merged_at
                        ? <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, color: 'success.main' }}><Merge sx={{ fontSize: 13 }} />{t('thread.mergedInto', { base: thread.base_branch })}</Box>
                        : thread.commits_ahead > 0 && <>{t('thread.commits', { count: thread.commits_ahead })} · {t('thread.files', { count: thread.files_changed })}</>,
                    thread.uncommitted && <Box component="span" sx={{ color: 'warning.main' }}>{t('thread.uncommitted')}</Box>,
                    thread.model && <Box component="span" sx={{ ...mono }}>{thread.model}</Box>,
                    (thread.input_tokens > 0 || thread.output_tokens > 0) && <>{formatTokens(thread.input_tokens)} in / {formatTokens(thread.output_tokens)} out</>,
                ]} />
                <Box sx={{ display: 'flex', gap: 1, mt: 1, flexWrap: 'wrap' }}>
                    {canReview && <Button size="small" variant="contained" startIcon={<DiffIcon />} onClick={() => setDiffOpen(true)}>{t('thread.review')}</Button>}
                    {running && <Button size="small" variant="outlined" color="inherit" startIcon={<Stop />} onClick={() => void act('/api/v1/projects/{id}/threads/{tid}/stop')}>{t('thread.stop')}</Button>}
                    {thread.status !== 'resolved'
                        ? <Button size="small" color="inherit" onClick={() => void act('/api/v1/projects/{id}/threads/{tid}/resolve')}>{t('thread.resolve')}</Button>
                        : <Button size="small" color="inherit" onClick={() => void act('/api/v1/projects/{id}/threads/{tid}/reopen')}>{t('thread.reopen')}</Button>}
                    <Button size="small" color="inherit" onClick={() => setTaskOpen((v) => !v)}>{taskOpen ? t('thread.hideTask') : t('thread.showTask')}</Button>
                </Box>
                <Collapse in={taskOpen}>
                    <Box sx={{ mt: 1, p: 1.25, bgcolor: 'action.hover', borderRadius: 2, maxHeight: 220, overflow: 'auto' }}><Markdown text={thread.task} /></Box>
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
                    minRows={1}
                    disabled={disabled || thread.status === 'resolved'}
                    placeholder={thread.status === 'resolved' ? t('thread.resolvedPlaceholder') : running ? t('thread.queuePlaceholder') : t('thread.placeholder')}
                    onSend={async (text) => {
                        try {
                            await unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/messages', { params: { path: { id: projectId, tid: threadId } }, body: { content: text } }));
                            stick.current = true;
                            return true;
                        } catch (e) {
                            notify((e as Error).message, 'error');
                            return false;
                        }
                    }}
                />
            </Box>
            <DiffDialog thread={thread} open={diffOpen} onClose={() => setDiffOpen(false)} />
        </Box>
    );
}
