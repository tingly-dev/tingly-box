import { useEffect, useMemo, useRef, useState } from 'react';
import { Box, Button, ButtonBase, Chip, Collapse, Paper, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { useTranslation } from 'react-i18next';
import type { Attachment, Thread, ThreadEvent } from '../../client';
import { ImSourceBadge } from '../im/ImSourceBadge';
import { AttachmentChips, uploadAttachments } from '../attachments';
import { ThreadComposer } from '../ThreadComposer';
import { Markdown } from '../Markdown';
import { useMessageDetail } from '../../hooks/useMessageDetail';
import { MessageDetailControl } from '../MessageDetailControl';
import { useTranscriptHost } from '../../transcript/context';
import { Check, ChevronRight, DiffIcon, ExpandLess, ExpandMore, PlayArrow, Settings } from '../icons';
import { DiffDialog } from './DiffDialog';
import { LoadEarlier } from './LoadEarlier';
import { ThreadStateMark, ThreadBadges, ThreadSubtitle } from './threadStatus';
import { Transcript, buildTranscript, type AttachmentsComponent, type TranscriptEntry } from '../../transcript';
import { threadTranscript } from './threadTranscript';
import { radius } from '@tingly/ui/theme/radius';
import { readLocal, writeLocal } from '../../utils/storage';

/** The cards' payloads arrive parsed (the transcript adapter reads them); accept both shapes. */
function parse<T>(payload: unknown): T | null {
    if (!payload) return null;
    if (typeof payload === 'object') return payload as T;
    try {
        return JSON.parse(String(payload)) as T;
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
export function ThreadCard({ thread, onOpen, onTasks }: { thread: Thread; onOpen: (id: string) => void; /** Opens the thread's running tasks from its count pill. */ onTasks?: (threads: Thread[], anchor: HTMLElement) => void }) {
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
                <Box component="span" title={t(`thread.status.${thread.status === 'working' && thread.queued ? 'queued' : thread.status}`)} sx={{ display: 'inline-flex', flexShrink: 0 }}>
                    <ThreadStateMark status={thread.status} queued={thread.queued} />
                </Box>
                <Typography noWrap sx={{ flex: { xs: 1, sm: '0 1 auto' }, minWidth: 0, maxWidth: { sm: '45%' }, fontSize: fontSizes.base, fontWeight: 600, color: 'text.primary' }}>{thread.title}</Typography>
                {/* On a phone the line keeps the title; the thread holds the rest. */}
                <Typography noWrap component="div" sx={{ display: { xs: 'none', sm: 'block' }, flex: 1, minWidth: 0, fontSize: fontSizes.sm, color: 'text.secondary' }}>
                    <ThreadSubtitle thread={thread} />
                </Typography>
                <ThreadBadges thread={thread} onTasks={onTasks} />
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
 * Concise detail drops the finished notifications, like a thread's transcript
 * folds its noise away.
 */
export function ThreadUpdateRow({ message, onOpen }: { message: TranscriptEntry; onOpen: (id: string) => void }) {
    const { t } = useTranslation('projects');
    const { detail } = useTranscriptHost();
    const [open, setOpen] = useState(false);
    const pl = parse<UpdatePayload>(message.payload);
    if (!pl) return null;
    if (detail === 'concise' && pl.kind === 'finished') return null;
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

function SuggestionsCard({ message, threads, onStart, onOpen, onTasks, disabled }: {
    message: TranscriptEntry;
    threads: Map<string, Thread>;
    onStart: (messageId: string, indexes: number[]) => Promise<void>;
    onOpen: (id: string) => void;
    /** Opens a suggested thread's running tasks from its count pill. */
    onTasks?: (threads: Thread[], anchor: HTMLElement) => void;
    disabled: boolean;
}) {
    const { t } = useTranslation('projects');
    const [busy, setBusy] = useState(false);
    const suggestions = parse<{ suggestions: Suggestion[] }>(message.payload)?.suggestions ?? [];
    const remaining = suggestions.filter((s) => !s.thread_id).length;
    const start = async (indexes: number[]) => {
        setBusy(true);
        try {
            await onStart(message.id!, indexes);
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
                    if (th) return <ThreadCard key={i} thread={th} onOpen={onOpen} onTasks={onTasks} />;
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
    message: TranscriptEntry;
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
            await onChoose(message.id!, index);
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
    /** Opens a thread's running tasks from its card's count pill. */
    onOpenTasks?: (threads: Thread[], anchor: HTMLElement) => void;
    onStartSuggestions: (messageId: string, indexes: number[]) => Promise<void>;
    onChooseDecision: (messageId: string, index: number) => Promise<void>;
    /** Interrupts the running coordinator turn, like a thread's stop. */
    onStop?: () => Promise<void>;
}

export function ConversationPane({ projectId, messages, more = false, onLoadEarlier, threads, busy, disabled, disabledReason, onSend, threadId, model, onModelChange, modelControl, effort, onEffortChange, composerContext, onOpenThread, onOpenTasks, onStartSuggestions, onChooseDecision, onStop }: Props) {
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

    // The conversation IS a thread's transcript: the same adapter and the
    // same shared renderer a thread's panel uses. Only its own cards differ,
    // and they render through the transcript's host hook.
    const blocks = useMemo(
        () => buildTranscript(threadTranscript(messages, busy, (e) => e.source === 'thread' ? t('thread.fromCoordinator') : undefined).entries),
        [messages, busy, t],
    );
    const renderCustom = useMemo(() => (message: TranscriptEntry) => {
        switch (message.kind) {
            case 'thread_started': {
                const ids = parse<{ thread_ids: string[] }>(message.payload)?.thread_ids ?? [];
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75 }}>
                        {ids.map((id) => {
                            const th = threads.get(id);
                            return th ? <ThreadCard key={id} thread={th} onOpen={onOpenThread} onTasks={onOpenTasks} /> : null;
                        })}
                    </Box>
                );
            }
            case 'suggestions':
                return <SuggestionsCard message={message} threads={threads} onStart={onStartSuggestions} onOpen={onOpenThread} onTasks={onOpenTasks} disabled={disabled} />;
            case 'decision':
                return <DecisionCard message={message} onChoose={onChooseDecision} disabled={disabled} />;
            case 'thread_update':
                return <ThreadUpdateRow message={message} onOpen={onOpenThread} />;
            case 'system':
                // The transcript shows system notes as they read; the settings
                // notice is the conversation's own word for it.
                if (parse<{ settings_changed?: boolean }>(message.payload)?.settings_changed) {
                    return <Typography variant="caption" color="text.secondary" sx={{ display: 'block', my: 1.5 }}>{t('conversation.settingsChanged')}</Typography>;
                }
                return null;
            default:
                return null;
        }
    }, [threads, disabled, onOpenThread, onOpenTasks, onStartSuggestions, onChooseDecision, t]);
    // A user message's files, fetched against the project like a thread's.
    const attachments = useMemo<AttachmentsComponent>(() => ({ items, align }) => <AttachmentChips owner={{ projectId }} items={items} align={align} />, [projectId]);

    useEffect(() => {
        const el = scrollRef.current;
        if (el && stick.current) el.scrollTop = el.scrollHeight;
    }, [messages, busy, detail]);

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0, minWidth: 0 }}>
            <Box
                ref={scrollRef}
                data-testid="conversation-scroll"
                onScroll={() => {
                    const el = scrollRef.current;
                    if (el) {
                        stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
                        setAwayFromBottom(!stick.current);
                    }
                }}
                sx={{ flex: 1, overflowY: 'auto', overflowX: 'hidden', px: 2, py: 2, overscrollBehavior: 'contain' }}
            >
                <Box sx={{ maxWidth: 800, mx: 'auto', width: '100%' }}>
                    {more && onLoadEarlier && <LoadEarlier scrollRef={scrollRef} onLoad={onLoadEarlier} />}
                    {messages.length === 0 && (
                        <Box sx={{ py: { xs: 3, md: 6 }, maxWidth: 650 }}>
                            <Typography variant="h3" sx={{ mb: 2 }}>{t('conversation.emptyTitle')}</Typography>
                            {[1, 2, 3].map((i) => (
                                <Box key={i} sx={{ display: 'flex', gap: 1.5, mb: 1.5 }}>
                                    <Box sx={{ width: 22, height: 22, borderRadius: '50%', bgcolor: 'primary.main', color: 'primary.contrastText', fontSize: fontSizes.sm, fontWeight: 600, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, mt: 0.25 }}>{i}</Box>
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
                    {messages.length > 0 && (
                        <Transcript
                            blocks={blocks}
                            detail={detail}
                            working={busy}
                            expandAll={false}
                            onRespond={async () => false}
                            attachments={attachments}
                            messageSource={(message) => message.imSource ? <ImSourceBadge platform={message.imSource.platform} chatId={message.imSource.chatId} /> : null}
                            renderCustom={renderCustom}
                        />
                    )}
                </Box>
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
                    allowClear={false}
                    autoFocus
                    disabled={disabled}
                    seed={seed}
                    streaming={busy}
                    onStop={onStop}
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
