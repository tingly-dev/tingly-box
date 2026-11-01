import { useEffect, useRef, useState } from 'react';
import { Box, Button, ButtonBase, Chip, CircularProgress, Collapse, Paper, Stack, Typography } from '@mui/material';
import { getReadableAccent } from '@tingly/ui/theme/status';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { Attachment, Thread, ThreadEvent } from '../../client';
import { ImSourceBadge } from '../im/ImSourceBadge';
import { AttachmentChips, uploadAttachments } from '../attachments';
import { ThreadComposer } from '../ThreadComposer';
import { Markdown } from '../Markdown';
import { useMessageDetail } from '../../hooks/useMessageDetail';
import { MessageDetailControl } from '../MessageDetailControl';
import { showManagerMessage } from './messageDetail';
import { Check, ChevronRight, DiffIcon, ErrorOutline, ExpandLess, ExpandMore, PlayArrow, Settings } from '../icons';
import { DiffDialog } from './DiffDialog';
import { LoadEarlier } from './LoadEarlier';
import { ThreadStateMark, ThreadBadges, ThreadSubtitle } from './threadStatus';
import { radius } from '@tingly/ui/theme/radius';
import { conversationText, messageSpacing, userBubble, userMessageWidth } from '../../transcript/conversationStyles';
import { readLocal, writeLocal } from '../../utils/storage';

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
                <Typography noWrap component="div" sx={{ display: { xs: 'none', sm: 'block' }, flex: 1, minWidth: 0, fontSize: fontSizes.sm, color: 'text.secondary' }}>
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
        <Box>
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
        <Paper variant="outlined" sx={{ borderRadius: radius.card, p: 1.5 }}>
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

interface DecisionPayload {
    question: string;
    context?: string;
    options: { label: string; consequence?: string }[];
    recommended: number;
    reason?: string;
    chosen?: number;
}

/**
 * A question the coordinator needs answered: a choice that changes the goal
 * or a step that cannot be undone. The recommended option leads; an answered
 * card keeps the pick and drops the buttons.
 */
function DecisionCard({ message, onChoose, disabled }: {
    message: ThreadEvent;
    onChoose: (messageId: string, index: number) => Promise<void>;
    disabled: boolean;
}) {
    const { t } = useTranslation('projects');
    const [busy, setBusy] = useState(false);
    const d = parse<DecisionPayload>(message.payload);
    if (!d) return null;
    const answered = d.chosen !== undefined && d.chosen !== null;
    const choose = async (index: number) => {
        setBusy(true);
        try {
            await onChoose(message.id, index);
        } finally {
            setBusy(false);
        }
    };
    return (
        <Paper variant="outlined" sx={{ borderRadius: radius.card, p: 1.5 }}>
            <Typography variant="overline" color="text.secondary">{t('conversation.decision')}</Typography>
            <Typography sx={{ fontWeight: 600, fontSize: fontSizes.base, color: 'text.primary' }}>{d.question}</Typography>
            {d.context && <Typography variant="body2" color="text.secondary" sx={{ mt: 0.25, whiteSpace: 'pre-wrap' }}>{d.context}</Typography>}
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75, mt: 1 }}>
                {d.options.map((o, i) => {
                    const picked = answered && d.chosen === i;
                    const recommended = !answered && d.recommended === i;
                    return (
                        <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 1, p: 1, borderRadius: radius.row, bgcolor: 'action.hover', opacity: answered && !picked ? 0.55 : 1 }}>
                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography sx={{ fontWeight: 600, fontSize: fontSizes.base, color: 'text.primary' }}>
                                    {o.label}
                                    {recommended && <Chip size="small" label={t('conversation.recommended')} sx={{ ml: 1, height: 18 }} />}
                                </Typography>
                                {o.consequence && <Typography variant="body2" color="text.secondary">{o.consequence}</Typography>}
                                {recommended && d.reason && <Typography variant="caption" color="text.secondary">{d.reason}</Typography>}
                            </Box>
                            {picked ? (
                                <Check fontSize="small" color="primary" aria-label={t('conversation.chosen')} />
                            ) : !answered && (
                                <Button size="small" variant={recommended ? 'contained' : 'outlined'} disabled={busy || disabled} onClick={() => void choose(i)}>{t('conversation.choose')}</Button>
                            )}
                        </Box>
                    );
                })}
            </Box>
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
    modelControl?: React.ReactNode;
    /** The coordinator's thinking effort, beside its model picker. */
    effort?: string;
    onEffortChange?: (effort: string) => void;
    /** Pickers above the message box (what the project's threads start with). */
    composerContext?: React.ReactNode;
    onOpenThread: (id: string) => void;
    onStartSuggestions: (messageId: string, indexes: number[]) => Promise<void>;
    onChooseDecision: (messageId: string, index: number) => Promise<void>;
    /** A bound IM chat's name for a message's source line; unknown chats show their id. */
    chatTitle?: (platform: string, chatId: string) => string | undefined;
}

export function ConversationPane({ projectId, messages, more = false, onLoadEarlier, threads, busy, disabled, disabledReason, onSend, threadId, model, onModelChange, modelControl, effort, onEffortChange, composerContext, onOpenThread, onStartSuggestions, onChooseDecision, chatTitle }: Props) {
    const { t } = useTranslation('projects');
    const scrollRef = useRef<HTMLDivElement>(null);
    const stick = useRef(true);
    const [awayFromBottom, setAwayFromBottom] = useState(false);
    const [seed, setSeed] = useState('');
    const [detail, setDetail] = useMessageDetail('conversation');
    const [settingsOpen, setSettingsOpen] = useState(() => readLocal('conversation.threadDefaultsOpen') === '1');
    const toggleSettings = () => {
        writeLocal('conversation.threadDefaultsOpen', settingsOpen ? '0' : '1');
        setSettingsOpen(!settingsOpen);
    };

    useEffect(() => {
        const el = scrollRef.current;
        if (el && stick.current) el.scrollTop = el.scrollHeight;
    }, [messages, busy, detail]);

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0, minWidth: 0 }}>
            <Box
                ref={scrollRef}
                onScroll={() => {
                    const el = scrollRef.current;
                    if (el) {
                        stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
                        setAwayFromBottom(!stick.current);
                    }
                }}
                sx={{ flex: 1, overflowY: 'auto', overflowX: 'hidden', px: 2, py: 2, overscrollBehavior: 'contain' }}
            >
                <Stack spacing={messageSpacing} sx={{ maxWidth: 800, mx: 'auto', width: '100%' }}>
                    {more && onLoadEarlier && <LoadEarlier scrollRef={scrollRef} onLoad={onLoadEarlier} />}
                    {messages.length === 0 && (
                        <Box sx={{ py: { xs: 3, md: 6 }, maxWidth: 650 }}>
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
                        if (m.kind !== 'user' && !showManagerMessage(m, detail)) return null;
                        switch (m.kind) {
                            case 'user': {
                                const attachments = parse<{ attachments?: Attachment[] }>(m.payload)?.attachments ?? [];
                                const showText = showManagerMessage(m, detail);
                                if (!showText && attachments.length === 0) return null;
                                return (
                                    <Box key={m.id} sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end' }}>
                                        <Box sx={{ maxWidth: userMessageWidth }}>
                                            <AttachmentChips owner={{ projectId }} items={attachments} />
                                        </Box>
                                        {showText && m.content.trim() !== '' && (
                                            <Box sx={userBubble}>
                                                <Typography variant="body1" sx={conversationText}>{m.content}</Typography>
                                            </Box>
                                        )}
                                        {m.source === 'im' && m.source_platform
                                            ? <ImSourceBadge platform={m.source_platform} chat={chatTitle?.(m.source_platform, m.source_chat_id ?? '') ?? m.source_chat_id ?? ''} />
                                            : null}
                                    </Box>
                                );
                            }
                            case 'assistant':
                                return <Box key={m.id}><Markdown text={m.content} conversation /></Box>;
                            case 'thread_started': {
                                const ids = parse<{ thread_ids: string[] }>(m.payload)?.thread_ids ?? [];
                                return (
                                    <Box key={m.id} sx={{ display: 'flex', flexDirection: 'column', gap: 0.75 }}>
                                        {ids.map((id) => {
                                            const th = threads.get(id);
                                            return th ? <ThreadCard key={id} thread={th} onOpen={onOpenThread} /> : null;
                                        })}
                                    </Box>
                                );
                            }
                            case 'suggestions':
                                return <SuggestionsCard key={m.id} message={m} threads={threads} onStart={onStartSuggestions} onOpen={onOpenThread} disabled={disabled} />;
                            case 'decision':
                                return <DecisionCard key={m.id} message={m} onChoose={onChooseDecision} disabled={disabled} />;
                            // A thread's line above is its live state; each report is also its
                            // own folded entry here, in the order things happened.
                            case 'thread_update':
                                return <ThreadUpdateRow key={m.id} message={m} onOpen={onOpenThread} />;
                            case 'system':
                                return <Typography key={m.id} variant="caption" color="text.secondary" sx={{ display: 'block', my: 1.5 }}>{parse<{ settings_changed?: boolean }>(m.payload)?.settings_changed ? t('conversation.settingsChanged') : m.content}</Typography>;
                            case 'error':
                                return (
                                    <Stack key={m.id} role="alert" direction="row" spacing={0.75} sx={{ alignItems: 'flex-start', color: (theme) => getReadableAccent(theme, 'error') }}>
                                        <ErrorOutline sx={{ fontSize: 18, mt: 0.25 }} />
                                        <Box sx={{ minWidth: 0, flex: 1 }}>
                                            <Typography variant="body2" sx={{ color: 'inherit', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{m.content}</Typography>
                                            <Button color="inherit" size="small" component={RouterLink} to="/settings" sx={{ px: 0 }}>{t('connection.open')}</Button>
                                        </Box>
                                    </Stack>
                                );
                            default:
                                return null;
                        }
                    })}
                    {busy && (
                        <Stack role="status" direction="row" spacing={1} sx={{ alignItems: 'center', color: 'text.secondary' }}>
                            <CircularProgress size={12} color="inherit" />
                            <Typography variant="body2" sx={{ color: 'inherit' }}>{t('chat.thinking')}</Typography>
                        </Stack>
                    )}
                </Stack>
            </Box>
            <Box sx={{ px: 2, pb: 2, position: 'relative' }}>
                {awayFromBottom && (
                    <Button size="small" variant="contained" onClick={() => {
                        stick.current = true;
                        scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' });
                        setAwayFromBottom(false);
                    }} sx={{ position: 'absolute', bottom: '100%', left: '50%', transform: 'translateX(-50%)', mb: 1, borderRadius: 99, boxShadow: 3, whiteSpace: 'nowrap', zIndex: 1 }}>↓ {t('conversation.latest', { defaultValue: 'Latest messages' })}</Button>
                )}
                <Box sx={{ maxWidth: 800, mx: 'auto' }}>
                {disabledReason}
                <MessageDetailControl detail={detail} onChange={setDetail} surface="conversation" />
                {composerContext && (
                    <Box sx={{ mb: 0.75, borderRadius: radius.card, bgcolor: 'action.hover', fontSize: fontSizes.sm, color: 'text.secondary', overflow: 'hidden' }}>
                        <ButtonBase onClick={toggleSettings} aria-expanded={settingsOpen}
                            sx={{ width: '100%', display: 'flex', alignItems: 'center', gap: 1, minHeight: 30, px: 1.25, justifyContent: 'flex-start', font: 'inherit', color: 'inherit', textAlign: 'left', '&:hover': { color: 'text.primary' } }}>
                            <Settings sx={{ fontSize: 15 }} />
                            <Box component="span" sx={{ flex: 1 }}>{t('threadSettings.settings')}</Box>
                            {settingsOpen ? <ExpandMore sx={{ fontSize: 16 }} /> : <ExpandLess sx={{ fontSize: 16 }} />}
                        </ButtonBase>
                        <Collapse in={settingsOpen} unmountOnExit>
                            <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 1, px: 1.25, pb: 1 }}>{composerContext}</Box>
                        </Collapse>
                    </Box>
                )}
                <ThreadComposer
                    // Per conversation: its draft starts fresh.
                    key={threadId}
                    draftKey={threadId}
                    autoFocus
                    disabled={disabled}
                    seed={seed}
                    streaming={busy}
                    leftSlot={modelControl}
                    model={model}
                    onModelChange={onModelChange}
                    effort={effort}
                    onEffortChange={onEffortChange}
                    modelSource="projects"
                    placeholder={t(busy ? 'conversation.queuePlaceholder' : 'conversation.placeholder')}
                    attach={(files) => uploadAttachments({ projectId }, files)}
                    onSend={async (text, attachments) => {
                        stick.current = true;
                        return onSend(text, attachments);
                    }}
                />
                </Box>
            </Box>
        </Box>
    );
}
