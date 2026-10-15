import { useEffect, useRef, useState } from 'react';
import {
    Alert, Box, Button, Collapse, Dialog, DialogActions, DialogContent, DialogTitle, IconButton,
    Paper, Skeleton, TextField, Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, ApiError, unwrap, type Diff, type Thread, type ThreadEvent } from '../../client';
import { Composer } from '../Composer';
import { Markdown } from '../Markdown';
import { CopyIconButton } from '@tingly/ui';
import { ArrowBack, Block, Branch, Build, Check, DiffIcon, ExpandMore, Merge, Question, Stop, Terminal } from '../icons';
import { useCopyFeedback } from '../../hooks/useCopyFeedback';
import { useNotify } from '../../hooks/useNotify';
import { formatTokens, timeAgo } from '../../utils/format';
import { InlineCode, ThreadStatusChip } from './threadStatus';

const mono = { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' } as const;

interface RequestPayload {
    request_id: string;
    tool?: string;
    input?: Record<string, unknown>;
    state: 'pending' | 'allowed' | 'denied' | 'answered' | 'expired';
    scope?: string;
    answer?: string;
}

function parse<T>(s?: string): T | null {
    if (!s) return null;
    try {
        return JSON.parse(s) as T;
    } catch {
        return null;
    }
}

/** The concrete thing a tool call does, for a one-line summary. */
function toolSummary(input: Record<string, unknown> | null): string {
    if (!input) return '';
    for (const k of ['command', 'file_path', 'path', 'pattern', 'url', 'query', 'description']) {
        const v = input[k];
        if (typeof v === 'string' && v) return v.split('\n')[0];
    }
    return '';
}

/** How a settled permission request ended, as a short tag on its tool call. */
function RequestOutcome({ pl }: { pl: RequestPayload }) {
    const { t } = useTranslation('projects');
    const ok = pl.state === 'allowed' || pl.state === 'answered';
    return (
        <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.25, flexShrink: 0, fontSize: 12, color: ok ? 'success.main' : pl.state === 'denied' ? 'error.main' : 'text.disabled' }}>
            {ok ? <Check sx={{ fontSize: 13 }} /> : <Block sx={{ fontSize: 13 }} />}
            {t(`thread.req.${pl.state}`)}{pl.scope === 'thread' ? ` · ${t('thread.req.forThread')}` : ''}
        </Box>
    );
}

function ToolRow({ ev, result, approval }: { ev: ThreadEvent; result?: ThreadEvent; approval?: RequestPayload }) {
    const [open, setOpen] = useState(false);
    const input = parse<Record<string, unknown>>(ev.payload);
    const summary = toolSummary(input);
    const isErr = parse<{ is_error?: boolean }>(result?.payload)?.is_error;
    return (
        <Box sx={{ my: 0.5 }}>
            <Box
                role="button"
                tabIndex={0}
                onClick={() => setOpen((v) => !v)}
                sx={{ display: 'flex', alignItems: 'center', gap: 0.75, color: isErr ? 'error.main' : 'text.secondary', fontSize: 13, py: 0.25, borderRadius: 1, '&:hover': { bgcolor: 'action.hover' } }}
            >
                <Build sx={{ fontSize: 14 }} />
                <Box component="span" sx={{ fontWeight: 600 }}>{ev.content}</Box>
                <Box component="span" sx={{ ...mono, fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0, flex: 1 }}>{summary}</Box>
                {approval && <RequestOutcome pl={approval} />}
                <ExpandMore sx={{ fontSize: 14, transform: open ? 'rotate(180deg)' : 'none', transition: 'transform .15s' }} />
            </Box>
            <Collapse in={open} unmountOnExit>
                <Box component="pre" sx={{ ...mono, fontSize: 12, m: 0, mt: 0.5, p: 1, bgcolor: 'action.hover', borderRadius: 1.5, maxHeight: 280, overflow: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                    {JSON.stringify(input, null, 2)}
                    {result?.content ? `\n\n— output —\n${result.content}` : ''}
                </Box>
            </Collapse>
        </Box>
    );
}

function RequestCard({ ev, projectId, threadId, active }: { ev: ThreadEvent; projectId: string; threadId: string; active: boolean }) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [answer, setAnswer] = useState('');
    const [busy, setBusy] = useState(false);
    const pl = parse<RequestPayload>(ev.payload);
    if (!pl) return null;
    const pending = pl.state === 'pending' && active;
    const respond = async (allow: boolean, scope?: 'once' | 'thread') => {
        setBusy(true);
        try {
            await unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/respond', { params: { path: { id: projectId, tid: threadId } }, body: { request_id: pl.request_id, allow, scope, answer: answer || undefined } }));
        } catch (e) {
            notify((e as Error).message, 'error');
        } finally {
            setBusy(false);
        }
    };
    const isAsk = ev.kind === 'ask';
    // The literal command when there is one: that is what the user approves.
    const command = typeof pl.input?.command === 'string' ? pl.input.command : '';

    if (!pending) {
        // Settled: one quiet line, so the thread reads as a log again. A
        // request left pending by a turn that ended can no longer be answered.
        const settled = pl.state === 'pending' ? { ...pl, state: 'expired' as const } : pl;
        return (
            <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.75, my: 0.5, fontSize: 13, color: 'text.secondary' }}>
                {isAsk ? <Question sx={{ fontSize: 14, mt: 0.25 }} /> : <Terminal sx={{ fontSize: 14, mt: 0.25 }} />}
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Box sx={{ ...(isAsk ? {} : mono), fontSize: isAsk ? 13 : 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{command || ev.content}</Box>
                    {pl.answer && <Box sx={{ color: 'text.primary' }}>↳ {pl.answer}</Box>}
                </Box>
                <RequestOutcome pl={settled} />
            </Box>
        );
    }

    return (
        <Paper variant="outlined" sx={{ my: 1.25, p: 1.5, borderRadius: 2.5, borderColor: 'warning.main' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                {isAsk ? <Question sx={{ fontSize: 16 }} color="warning" /> : <Terminal sx={{ fontSize: 16 }} color="warning" />}
                <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.primary' }}>
                    {isAsk ? t('thread.question') : t('thread.approvalTool', { tool: pl.tool || t('thread.approvalAny') })}
                </Typography>
            </Box>
            {command ? (
                <Box component="pre" sx={{ ...mono, fontSize: 12.5, m: 0, p: 1, bgcolor: 'action.hover', borderRadius: 1.5, color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 200, overflow: 'auto' }}>{command}</Box>
            ) : (
                <Typography sx={{ fontSize: 13.5, color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}><InlineCode text={ev.content} /></Typography>
            )}
            <Box sx={{ mt: 1.25 }}>
                {isAsk ? (
                    <Box sx={{ display: 'flex', gap: 1 }}>
                        <TextField size="small" fullWidth autoFocus value={answer} onChange={(e) => setAnswer(e.target.value)} placeholder={t('thread.answerPlaceholder')}
                            onKeyDown={(e) => e.key === 'Enter' && answer.trim() && void respond(true)} />
                        <Button variant="contained" disabled={busy || !answer.trim()} onClick={() => void respond(true)}>{t('thread.answer')}</Button>
                    </Box>
                ) : (
                    <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'center' }}>
                        <Button variant="contained" size="small" disabled={busy} onClick={() => void respond(true, 'once')}>{t('thread.allowOnce')}</Button>
                        <Button variant="outlined" size="small" disabled={busy} onClick={() => void respond(true, 'thread')}>{t('thread.allowThread', { tool: pl.tool })}</Button>
                        <Button size="small" color="inherit" disabled={busy} onClick={() => void respond(false)}>{t('thread.deny')}</Button>
                    </Box>
                )}
            </Box>
        </Paper>
    );
}

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
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const [diffOpen, setDiffOpen] = useState(false);
    const [taskOpen, setTaskOpen] = useState(false);
    const scrollRef = useRef<HTMLDivElement>(null);
    const stick = useRef(true);

    useEffect(() => {
        const el = scrollRef.current;
        if (el && stick.current) el.scrollTop = el.scrollHeight;
    }, [events]);

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
    const results = new Map<number, ThreadEvent>();
    const list = events ?? [];
    list.forEach((e, i) => {
        if (e.kind === 'tool_result') {
            for (let j = i - 1; j >= 0; j--) {
                if (list[j].kind === 'tool_use' && !results.has(j)) {
                    results.set(j, e);
                    break;
                }
            }
        }
    });
    // The report repeats the turn's last message; show it once, as the report.
    const repeated = new Set<string>();
    list.forEach((e, i) => {
        if (e.kind !== 'report') return;
        for (let j = i - 1; j >= 0; j--) {
            if (list[j].kind === 'assistant') {
                if (list[j].content.trim() === e.content.trim()) repeated.add(list[j].id);
                break;
            }
        }
    });
    // A settled approval right after its own tool call becomes a tag on that
    // call instead of a second line repeating the same command.
    const approvals = new Map<number, RequestPayload>();
    const absorbed = new Set<string>();
    list.forEach((e, i) => {
        const next = list[i + 1];
        if (e.kind !== 'tool_use' || next?.kind !== 'approval') return;
        const pl = parse<RequestPayload>(next.payload);
        if (pl && pl.tool === e.content && !(pl.state === 'pending' && running)) {
            approvals.set(i, pl.state === 'pending' ? { ...pl, state: 'expired' } : pl);
            absorbed.add(next.id);
        }
    });
    const pendingRequest = running && list.some((e) => (e.kind === 'approval' || e.kind === 'ask') && parse<RequestPayload>(e.payload)?.state === 'pending');
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
                {thread.status === 'waiting' && thread.wait_reason && !list.some((e) => (e.kind === 'approval' || e.kind === 'ask') && e.payload?.includes('"state":"pending"')) && (
                    <Alert severity="warning" sx={{ mt: 1, py: 0 }}>{thread.wait_reason}</Alert>
                )}
            </Box>

            <Box ref={scrollRef} onScroll={() => {
                const el = scrollRef.current;
                if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
            }} sx={{ flex: 1, overflowY: 'auto', px: 2, py: 1.5, overscrollBehavior: 'contain' }}>
                {events === null && <Skeleton variant="rounded" height={120} />}
                {list.map((e, i) => {
                    switch (e.kind) {
                        case 'user':
                            return (
                                <Box key={e.id} sx={{ my: 1.5 }}>
                                    <Typography variant="caption" color="text.secondary">{e.from === 'coordinator' ? t('thread.fromCoordinator') : t('thread.fromYou')} · {timeAgo(e.created_at, i18n.language)}</Typography>
                                    <Box sx={{ mt: 0.25, bgcolor: 'action.hover', borderRadius: 2, px: 1.5, py: 1, color: 'text.primary', fontSize: 14, whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 240, overflow: 'auto' }}>{e.content}</Box>
                                </Box>
                            );
                        case 'assistant':
                            if (repeated.has(e.id)) return null;
                            return <Box key={e.id} sx={{ my: 1, fontSize: 14 }}><Markdown text={e.content} /></Box>;
                        case 'tool_use':
                            return <ToolRow key={e.id} ev={e} result={results.get(i)} approval={approvals.get(i)} />;
                        case 'tool_result':
                            return null;
                        case 'approval':
                        case 'ask':
                            if (absorbed.has(e.id)) return null;
                            return <RequestCard key={e.id} ev={e} projectId={projectId} threadId={threadId} active={running} />;
                        case 'report': {
                            const pl = parse<{ commits_ahead?: number; files_changed?: number }>(e.payload);
                            return (
                                <Paper key={e.id} variant="outlined" sx={{ my: 1.5, p: 1.5, borderRadius: 2.5, borderColor: 'success.main' }}>
                                    <Typography variant="overline" color="success.main">{t('thread.report')}{pl?.commits_ahead ? ` · ${t('thread.commits', { count: pl.commits_ahead })}` : ''}</Typography>
                                    <Markdown text={e.content} />
                                </Paper>
                            );
                        }
                        case 'error':
                            return <Alert key={e.id} severity="error" sx={{ my: 1, '& .MuiAlert-message': { minWidth: 0, wordBreak: 'break-word' } }}>{e.content}</Alert>;
                        default:
                            return <Typography key={e.id} variant="caption" color="text.secondary" sx={{ display: 'block', textAlign: 'center', my: 1 }}>{e.content}</Typography>;
                    }
                })}
                {/* A pending request card is already the call to action. */}
                {running && !pendingRequest && (
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, color: 'text.secondary', fontSize: 13, py: 1 }}>
                        <Box sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: thread.status === 'waiting' ? 'warning.main' : 'info.main', animation: 'pulse 1.2s infinite', '@keyframes pulse': { '0%, 100%': { opacity: 0.3 }, '50%': { opacity: 1 } } }} />
                        {thread.status === 'waiting' ? t('thread.waitingOnYou') : t('thread.working')}
                    </Box>
                )}
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
