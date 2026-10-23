import { useEffect, useRef, useState } from 'react';
import { Alert, Box, Button, Chip, Collapse, Paper, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { Attachment, Thread, ThreadEvent } from '../../client';
import { ImSourceBadge } from '../im/ImSourceBadge';
import { AttachmentChips, uploadAttachments } from '../attachments';
import { ThreadComposer } from '../ThreadComposer';
import { Markdown } from '../Markdown';
import { readLocal, writeLocal } from '../../utils/storage';
import { ChevronRight, DiffIcon, ExpandMore, PlayArrow } from '../icons';
import { DiffDialog } from './DiffDialog';
import { LoadEarlier } from './LoadEarlier';
import { ThreadStateMark, ThreadBadges, ThreadSubtitle, stateColor } from './threadStatus';
import { radius } from '@tingly/ui/theme/radius';

const REPORTS_KEY = 'conversation.threadReports';
const COLUMN = { maxWidth: 760, mx: 'auto', width: '100%', px: { xs: 2, md: 3 } } as const;

function parse<T>(payload?: string): T | null {
    if (!payload) return null;
    try {
        return JSON.parse(payload) as T;
    } catch {
        return null;
    }
}

/**
 * A thread as one line in the conversation: its status, title, what it last
 * did and its work. The line is the thread's live state; it changes in place
 * rather than the conversation adding a note. A thread with changes to
 * review offers "Review" right here.
 */
export function ThreadCard({ thread, onOpen }: { thread: Thread; onOpen: (id: string) => void }) {
    const { t } = useTranslation('projects');
    const [reviewing, setReviewing] = useState(false);
    const canReview = thread.status === 'ready' && !!thread.branch && thread.commits_ahead > 0 && !thread.merged_at;
    return (
        <>
            <Box
                role="button"
                tabIndex={0}
                onClick={() => onOpen(thread.id)}
                onKeyDown={(e) => e.key === 'Enter' && onOpen(thread.id)}
                sx={{
                    display: 'flex', alignItems: 'center', gap: 1, minHeight: 40, px: 1.25, py: 0.5, borderRadius: radius.row, cursor: 'pointer',
                    border: 1, borderColor: 'divider', bgcolor: 'background.paper', outline: 'none',
                    '&:hover': { borderColor: 'text.disabled' },
                    '&:focus-visible': { boxShadow: (th) => `inset 0 0 0 2px ${th.palette.primary.main}` },
                }}
            >
                <Box component="span" title={t(`thread.status.${thread.status}`)} sx={{ display: 'inline-flex', flexShrink: 0 }}>
                    <ThreadStateMark status={thread.status} />
                </Box>
                <Typography noWrap sx={{ flex: { xs: 1, sm: '0 1 auto' }, minWidth: 0, maxWidth: { sm: '45%' }, fontSize: fontSizes.base, fontWeight: 600, color: 'text.primary' }}>{thread.title}</Typography>
                {/* On a phone the line keeps the title; the thread holds the rest. */}
                <Typography noWrap component="div" sx={{ display: { xs: 'none', sm: 'block' }, flex: 1, minWidth: 0, fontSize: fontSizes.sm, color: thread.status === 'waiting' ? stateColor('waiting') : 'text.secondary' }}>
                    <ThreadSubtitle thread={thread} />
                </Typography>
                <ThreadBadges thread={thread} />
                {canReview && (
                    <Button size="small" variant="outlined" startIcon={<DiffIcon />} sx={{ flexShrink: 0, py: 0.125 }}
                        onClick={(e) => {
                            e.stopPropagation();
                            setReviewing(true);
                        }}>
                        {t('thread.review')}
                    </Button>
                )}
            </Box>
            {canReview && <DiffDialog thread={thread} open={reviewing} onClose={() => setReviewing(false)} />}
        </>
    );
}

interface UpdatePayload {
    thread_id: string;
    title: string;
    kind: string;
    status: string;
}

/**
 * What a thread reported to the coordinator, as its own quiet entry where it
 * happened: the thread, how the turn ended, and the report one click away.
 * The thread's line above keeps the live state; this keeps the history.
 */
export function ThreadUpdateRow({ message, onOpen }: { message: ThreadEvent; onOpen: (id: string) => void }) {
    const { t } = useTranslation('projects');
    const [open, setOpen] = useState(false);
    const pl = parse<UpdatePayload>(message.payload);
    if (!pl) return null;
    const hasText = message.content.trim() !== '';
    const toggle = () => hasText && setOpen((v) => !v);
    return (
        <Box sx={{ my: 0.75 }}>
            <Box
                role="button"
                tabIndex={0}
                aria-expanded={hasText ? open : undefined}
                onClick={toggle}
                onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        toggle();
                    }
                }}
                sx={{
                    display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0, py: 0.25, px: 0.5, mx: -0.5, borderRadius: radius.row,
                    cursor: 'default', userSelect: 'none', color: 'text.secondary', fontSize: fontSizes.sm, outline: 'none',
                    '&:hover': { bgcolor: 'action.hover', color: 'text.primary' },
                    '&:focus-visible': { boxShadow: (th) => `inset 0 0 0 2px ${th.palette.primary.main}` },
                }}
            >
                <Box component="span" title={t(`thread.status.${pl.status}`)} sx={{ display: 'inline-flex', flexShrink: 0 }}>
                    <ThreadStateMark status={pl.status} />
                </Box>
                <Typography noWrap component="span" sx={{ minWidth: 0, fontSize: 'inherit', fontWeight: 600, color: 'text.primary' }}>{pl.title}</Typography>
                <Typography noWrap component="span" sx={{ flexShrink: 0, fontSize: 'inherit' }}>{t(`conversation.updateKind.${pl.kind}`, { defaultValue: pl.kind })}</Typography>
                {hasText && (open ? <ExpandMore sx={{ fontSize: 16, flexShrink: 0 }} /> : <ChevronRight sx={{ fontSize: 16, flexShrink: 0 }} />)}
                <Box sx={{ flex: 1 }} />
                <Box component="button" type="button" onClick={(e) => { e.stopPropagation(); onOpen(pl.thread_id); }}
                    sx={{ flexShrink: 0, p: 0, border: 0, bgcolor: 'transparent', font: 'inherit', color: 'text.disabled', cursor: 'pointer', '&:hover': { color: 'text.primary', textDecoration: 'underline' } }}>
                    {t('conversation.openThread')}
                </Box>
            </Box>
            <Collapse in={open} unmountOnExit>
                <Box sx={{ mt: 0.5, ml: 0.5, pl: 1.5, borderLeft: 2, borderColor: 'divider', color: 'text.secondary', fontSize: fontSizes.sm }}>
                    <Markdown text={message.content} />
                </Box>
            </Collapse>
        </Box>
    );
}

interface Suggestion {
    title: string;
    task: string;
    repo?: string;
    thread_id?: string;
}

function SuggestionsCard({ message, threads, onStart, onOpen, disabled }: {
    message: ThreadEvent;
    threads: Map<string, Thread>;
    onStart: (messageId: string, indexes: number[]) => Promise<void>;
    onOpen: (id: string) => void;
    disabled: boolean;
}) {
    const { t } = useTranslation('projects');
    const [busy, setBusy] = useState(false);
    const suggestions = parse<{ suggestions: Suggestion[] }>(message.payload)?.suggestions ?? [];
    const remaining = suggestions.filter((s) => !s.thread_id).length;
    const start = async (indexes: number[]) => {
        setBusy(true);
        try {
            await onStart(message.id, indexes);
        } finally {
            setBusy(false);
        }
    };
    return (
        <Paper variant="outlined" sx={{ borderRadius: radius.card, p: 1.5, my: 1.5 }}>
            <Typography variant="overline" color="text.secondary">{t('conversation.suggested')}</Typography>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, mt: 0.5 }}>
                {suggestions.map((s, i) => {
                    const th = s.thread_id ? threads.get(s.thread_id) : undefined;
                    if (th) return <ThreadCard key={i} thread={th} onOpen={onOpen} />;
                    return (
                        <Box key={i} sx={{ display: 'flex', alignItems: 'flex-start', gap: 1, p: 1.25, borderRadius: radius.row, bgcolor: 'action.hover' }}>
                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography sx={{ fontWeight: 600, fontSize: fontSizes.base, color: 'text.primary' }}>{s.title}</Typography>
                                <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'pre-wrap', display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{s.task}</Typography>
                            </Box>
                            <Button size="small" startIcon={<PlayArrow />} disabled={busy || disabled} onClick={() => void start([i])}>{t('conversation.start')}</Button>
                        </Box>
                    );
                })}
            </Box>
            {remaining > 1 && (
                <Button variant="contained" size="small" sx={{ mt: 1.25 }} disabled={busy || disabled} onClick={() => void start([])}>
                    {t('conversation.startAll', { count: remaining })}
                </Button>
            )}
        </Paper>
    );
}

const EXAMPLES = ['conversation.example1', 'conversation.example2', 'conversation.example3'];

interface Props {
    projectId: string;
    messages: ThreadEvent[];
    /** Older messages exist than the loaded ones. */
    more?: boolean;
    onLoadEarlier?: () => Promise<void>;
    threads: Map<string, Thread>;
    busy: boolean;
    disabled: boolean;
    disabledReason?: React.ReactNode;
    onSend: (text: string, attachments: Attachment[]) => Promise<boolean>;
    /** The conversation's own thread: its draft is kept under this id. */
    threadId: string;
    /** The coordinator's model, switched from the message box; the picker is left out without onModelChange. */
    model?: string;
    onModelChange?: (model: string) => void;
    /** Pickers above the message box (what the project's threads start with). */
    composerContext?: React.ReactNode;
    onOpenThread: (id: string) => void;
    onStartSuggestions: (messageId: string, indexes: number[]) => Promise<void>;
    /** A bound IM chat's name for a message's source line; unknown chats show their id. */
    chatTitle?: (platform: string, chatId: string) => string | undefined;
}

export function ConversationPane({ projectId, messages, more = false, onLoadEarlier, threads, busy, disabled, disabledReason, onSend, threadId, model, onModelChange, composerContext, onOpenThread, onStartSuggestions, chatTitle }: Props) {
    const { t } = useTranslation('projects');
    const scrollRef = useRef<HTMLDivElement>(null);
    const stick = useRef(true);
    const [seed, setSeed] = useState('');
    // Thread reports are history, not news: shown by default, hideable per browser.
    const [showReports, setShowReports] = useState(() => readLocal(REPORTS_KEY) !== '0');
    const hasReports = messages.some((m) => m.kind === 'thread_update');

    useEffect(() => {
        const el = scrollRef.current;
        if (el && stick.current) el.scrollTop = el.scrollHeight;
    }, [messages, busy]);

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0, minWidth: 0 }}>
            <Box
                ref={scrollRef}
                onScroll={() => {
                    const el = scrollRef.current;
                    if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
                }}
                sx={{ flex: 1, overflowY: 'auto', overscrollBehavior: 'contain' }}
            >
                <Box sx={{ ...COLUMN, pt: 2, pb: 3 }}>
                    {more && onLoadEarlier && <LoadEarlier scrollRef={scrollRef} onLoad={onLoadEarlier} />}
                    {messages.length === 0 && (
                        <Box sx={{ py: { xs: 3, md: 6 } }}>
                            <Typography variant="h3" sx={{ mb: 2 }}>{t('conversation.emptyTitle')}</Typography>
                            {[1, 2, 3].map((i) => (
                                <Box key={i} sx={{ display: 'flex', gap: 1.5, mb: 1.5 }}>
                                    <Box sx={{ width: 22, height: 22, borderRadius: '50%', bgcolor: 'primary.main', color: 'primary.contrastText', fontSize: fontSizes.sm, fontWeight: 700, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, mt: 0.25 }}>{i}</Box>
                                    <Typography color="text.secondary">{t(`conversation.step${i}`)}</Typography>
                                </Box>
                            ))}
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 3, mb: 1 }}>{t('conversation.tryExample')}</Typography>
                            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                                {EXAMPLES.map((k) => (
                                    <Chip key={k} label={t(k)} variant="outlined" onClick={() => setSeed(t(k))} sx={{ height: 'auto', py: 0.75, '& .MuiChip-label': { whiteSpace: 'normal' } }} />
                                ))}
                            </Box>
                        </Box>
                    )}
                    {messages.map((m) => {
                        switch (m.kind) {
                            case 'user':
                                return (
                                    <Box key={m.id} sx={{ display: 'flex', justifyContent: 'flex-end', my: 2 }}>
                                        <Box sx={{ maxWidth: '85%', display: 'flex', flexDirection: 'column', alignItems: 'flex-end' }}>
                                            <AttachmentChips owner={{ projectId }} items={parse<{ attachments?: Attachment[] }>(m.payload)?.attachments ?? []} />
                                            {m.content.trim() !== '' && (
                                                <Box sx={{ bgcolor: 'action.hover', borderRadius: radius.card, px: 1.75, py: 1, whiteSpace: 'pre-wrap', wordBreak: 'break-word', color: 'text.primary', fontSize: fontSizes.base, lineHeight: 1.65 }}>{m.content}</Box>
                                            )}
                                            {m.source === 'im' && m.source_platform
                                                ? <ImSourceBadge platform={m.source_platform} chat={chatTitle?.(m.source_platform, m.source_chat_id ?? '') ?? m.source_chat_id ?? ''} />
                                                : null}
                                        </Box>
                                    </Box>
                                );
                            case 'assistant':
                                return <Box key={m.id} sx={{ my: 2 }}><Markdown text={m.content} /></Box>;
                            case 'thread_started': {
                                const ids = parse<{ thread_ids: string[] }>(m.payload)?.thread_ids ?? [];
                                return (
                                    <Box key={m.id} sx={{ display: 'flex', flexDirection: 'column', gap: 0.75, my: 1.5 }}>
                                        {ids.map((id) => {
                                            const th = threads.get(id);
                                            return th ? <ThreadCard key={id} thread={th} onOpen={onOpenThread} /> : null;
                                        })}
                                    </Box>
                                );
                            }
                            case 'suggestions':
                                return <SuggestionsCard key={m.id} message={m} threads={threads} onStart={onStartSuggestions} onOpen={onOpenThread} disabled={disabled} />;
                            // A thread's line above is its live state; each report is also its
                            // own folded entry here, in the order things happened.
                            case 'thread_update':
                                return showReports ? <ThreadUpdateRow key={m.id} message={m} onOpen={onOpenThread} /> : null;
                            case 'error':
                                return (
                                    <Alert key={m.id} severity="error" sx={{ my: 1.5, '& .MuiAlert-message': { minWidth: 0, wordBreak: 'break-word' } }}
                                        action={<Button color="inherit" size="small" component={RouterLink} to="/settings">{t('connection.open')}</Button>}>
                                        {m.content}
                                    </Alert>
                                );
                            default:
                                return null;
                        }
                    })}
                    {busy && (
                        <Box sx={{ display: 'flex', gap: 0.75, py: 1.5 }} aria-label={t('chat.thinking')}>
                            {[0, 1, 2].map((i) => (
                                <Box key={i} sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: 'primary.main', animation: 'pulse 1.2s infinite', animationDelay: `${i * 0.15}s`, '@keyframes pulse': { '0%, 80%, 100%': { opacity: 0.25 }, '40%': { opacity: 1 } } }} />
                            ))}
                        </Box>
                    )}
                </Box>
            </Box>
            <Box sx={{ ...COLUMN, pb: 2 }}>
                {disabledReason}
                {hasReports && (
                    <Box sx={{ display: 'flex', justifyContent: 'flex-end', pb: 0.5 }}>
                        <Button size="small" color="inherit" aria-pressed={showReports}
                            sx={{ minWidth: 0, py: 0, fontSize: fontSizes.sm, fontWeight: 400, color: 'text.secondary' }}
                            onClick={() => {
                                const next = !showReports;
                                setShowReports(next);
                                writeLocal(REPORTS_KEY, next ? '1' : '0');
                            }}>
                            {t(showReports ? 'conversation.hideReports' : 'conversation.showReports')}
                        </Button>
                    </Box>
                )}
                <ThreadComposer
                    // Per conversation: its draft starts fresh.
                    key={threadId}
                    draftKey={threadId}
                    autoFocus
                    disabled={disabled}
                    seed={seed}
                    context={composerContext}
                    streaming={busy}
                    model={model}
                    onModelChange={onModelChange}
                    modelLabel={t('conversation.coordinatorModel')}
                    placeholder={t(busy ? 'conversation.queuePlaceholder' : 'conversation.placeholder')}
                    attach={(files) => uploadAttachments({ projectId }, files)}
                    onSend={async (text, attachments) => {
                        stick.current = true;
                        return onSend(text, attachments);
                    }}
                />
            </Box>
        </Box>
    );
}
