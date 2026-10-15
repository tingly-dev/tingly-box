import { useCallback, useEffect, useRef, useState } from 'react';
import { Alert, Box, Breadcrumbs, Button, IconButton, Skeleton, Tooltip, Typography } from '@mui/material';
import { Link as RouterLink, useLocation, useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, ApiError, unwrap, type ConversationDetail, type Message, type StreamEvent } from '../client';
import { regenerate, sendMessage } from '../client/stream';
import { ChatActionsMenu } from '../components/ChatActionsMenu';
import { Composer } from '../components/Composer';
import { Markdown } from '../components/Markdown';
import { Check, ContentCopy, ExpandMore, Refresh } from '../components/icons';
import { useCopyFeedback } from '../hooks/useCopyFeedback';
import { useNotify } from '../hooks/useNotify';
import type { PendingFirstMessage } from '../hooks/useStartChat';
import { useChosenModel } from '../hooks/useStartChat';
import { useAppData } from '../contexts/AppDataContext';
import { formatTokens } from '../utils/format';
import { fontMono } from '@tingly/ui/theme/fonts';

const COLUMN = { maxWidth: 780, mx: 'auto', width: '100%', px: { xs: 2, md: 3 } } as const;

function UserBubble({ text }: { text: string }) {
    return (
        <Box sx={{ display: 'flex', justifyContent: 'flex-end', my: 2 }}>
            <Box sx={{ maxWidth: '85%', bgcolor: 'action.hover', borderRadius: 1.5, px: 1.75, py: 1, whiteSpace: 'pre-wrap', wordBreak: 'break-word', color: 'text.primary', fontSize: '0.875rem', lineHeight: 1.65 }}>
                {text}
            </Box>
        </Box>
    );
}

function AssistantActions({ msg, isLast, onRegenerate, disabled }: { msg: Message; isLast: boolean; onRegenerate: () => void; disabled: boolean }) {
    const { t } = useTranslation('projects');
    const { copied, copy } = useCopyFeedback();
    const meta = [msg.model, msg.input_tokens ? `${formatTokens(msg.input_tokens)} in` : '', msg.output_tokens ? `${formatTokens(msg.output_tokens)} out` : '']
        .filter(Boolean)
        .join(' · ');
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.5, color: 'text.secondary', '& .meta': { opacity: 0 }, '&:hover .meta': { opacity: 1 } }}>
            {msg.content && (
                <Tooltip title={copied ? t('common.copied') : t('common.copy')}>
                    <IconButton size="small" onClick={() => copy(msg.content)} aria-label={t('common.copy')}>
                        {copied ? <Check fontSize="small" color="success" /> : <ContentCopy fontSize="small" />}
                    </IconButton>
                </Tooltip>
            )}
            {isLast && (
                <Tooltip title={t('chat.regenerate')}>
                    <span>
                        <IconButton size="small" onClick={onRegenerate} disabled={disabled} aria-label={t('chat.regenerate')}>
                            <Refresh fontSize="small" />
                        </IconButton>
                    </span>
                </Tooltip>
            )}
            {msg.stop_reason === 'cancelled' && <Typography variant="caption">{t('chat.stopped')}</Typography>}
            {(msg.stop_reason === 'max_tokens' || msg.stop_reason === 'length') && (
                <Typography variant="caption" color="warning.main">{t('chat.truncated')}</Typography>
            )}
            <Typography className="meta" variant="caption" sx={{ ml: 1, fontFamily: fontMono, transition: 'opacity .15s' }}>
                {meta}
            </Typography>
        </Box>
    );
}

export default function ChatPage() {
    const { id = '' } = useParams();
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const location = useLocation();
    const notify = useNotify();
    const { refresh } = useAppData();

    const [detail, setDetail] = useState<ConversationDetail | null>(null);
    const [loadError, setLoadError] = useState<string | null>(null);
    const { model, setModel, adoptModel } = useChosenModel();
    const [streaming, setStreaming] = useState(false);
    const [draftUser, setDraftUser] = useState<string | null>(null);
    const [draftReply, setDraftReply] = useState('');
    // retryable: the failure happened mid-turn (saved), so regenerate re-runs it.
    // A refused turn saved nothing and its text is back in the composer.
    const [turnError, setTurnError] = useState<{ message: string; retryable: boolean } | null>(null);
    const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
    const [seed, setSeed] = useState('');

    const abortRef = useRef<AbortController | null>(null);
    const scrollRef = useRef<HTMLDivElement>(null);
    const stickRef = useRef(true);
    const firstSentRef = useRef<string | null>(null);

    const load = useCallback(async () => {
        try {
            const d = await unwrap(api.GET('/api/v1/conversations/{id}', { params: { path: { id } } }));
            setDetail(d);
            setLoadError(null);
            if (d.conversation.model) adoptModel(d.conversation.model);
            return d;
        } catch (e) {
            setLoadError((e as Error).message);
            return null;
        }
    }, [id]);

    // Auto-scroll only while the user is at the bottom of this thread.
    const onScroll = () => {
        const el = scrollRef.current;
        if (!el) return;
        stickRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    };
    useEffect(() => {
        const el = scrollRef.current;
        if (el && stickRef.current) el.scrollTop = el.scrollHeight;
    }, [detail, draftReply, draftUser, turnError]);

    const handleEvent = useCallback((ev: StreamEvent) => {
        switch (ev.type) {
            case 'user_message':
                setDraftUser(null);
                if (ev.message) {
                    const m = ev.message;
                    setDetail((d) => (d ? { ...d, messages: [...d.messages, m] } : d));
                }
                break;
            case 'delta':
                setDraftReply((r) => r + (ev.text ?? ''));
                break;
            case 'assistant_message':
                setDraftReply('');
                if (ev.message) {
                    const m = ev.message;
                    setDetail((d) => (d ? { ...d, conversation: { ...d.conversation, model: m.model ?? d.conversation.model }, messages: [...d.messages, m] } : d));
                }
                break;
            case 'title':
                setDetail((d) => (d ? { ...d, conversation: { ...d.conversation, title: ev.title ?? '' } } : d));
                refresh();
                break;
            case 'error':
                setTurnError({ message: ev.error ?? 'error', retryable: true });
                break;
        }
    }, [refresh]);

    const run = useCallback(async (kind: 'send' | 'regenerate', text?: string, modelOverride?: string): Promise<boolean> => {
        const ctrl = new AbortController();
        abortRef.current = ctrl;
        stickRef.current = true;
        setStreaming(true);
        setTurnError(null);
        setDraftReply('');
        if (kind === 'send') setDraftUser(text ?? '');
        const m = modelOverride ?? model;
        try {
            if (kind === 'send') {
                await sendMessage(id, text ?? '', m, { signal: ctrl.signal, onEvent: handleEvent });
            } else {
                await regenerate(id, m, { signal: ctrl.signal, onEvent: handleEvent });
            }
            refresh();
            return true;
        } catch (e) {
            if ((e as Error).name === 'AbortError') {
                await load();
                refresh();
                return true;
            }
            setDraftUser(null);
            // Nothing was saved: the turn was refused before it started.
            if (e instanceof ApiError) {
                setTurnError({ message: e.message, retryable: false });
                return false;
            }
            notify((e as Error).message, 'error');
            await load();
            return true;
        } finally {
            setDraftReply('');
            setStreaming(false);
            abortRef.current = null;
        }
    }, [id, model, handleEvent, load, notify, refresh]);

    // Load the conversation, then send the first message handed over by the
    // page that created it (once, even under StrictMode).
    useEffect(() => {
        setDetail(null);
        setTurnError(null);
        void load().then((d) => {
            const pending = (location.state as PendingFirstMessage | null)?.pending;
            if (!d || !pending || firstSentRef.current === id || d.messages.length > 0) return;
            firstSentRef.current = id;
            navigate(location.pathname, { replace: true, state: null });
            void run('send', pending.text, pending.model).then((ok) => {
                if (!ok) setSeed(pending.text);
            });
        });
        return () => abortRef.current?.abort();
    }, [id]);

    if (loadError) {
        return (
            <Box sx={{ ...COLUMN, pt: 8 }}>
                <Alert severity="error" action={<Button color="inherit" component={RouterLink} to="/chat">{t('nav.newChat')}</Button>}>{loadError}</Alert>
            </Box>
        );
    }

    const conv = detail?.conversation;
    const messages = detail?.messages ?? [];
    const lastAssistant = [...messages].reverse().find((m) => m.role === 'assistant');
    const lastMsg = messages[messages.length - 1];

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
            {/* Header: project breadcrumb + title menu */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: { xs: 6, md: 3 }, py: 1.25, minHeight: 52, borderBottom: 1, borderColor: 'divider' }}>
                <Breadcrumbs separator="/" sx={{ minWidth: 0, '& ol': { flexWrap: 'nowrap' } }}>
                    {conv ? (
                        <Button
                            color="inherit"
                            size="small"
                            endIcon={<ExpandMore fontSize="small" />}
                            onClick={(e) => setMenuAnchor(e.currentTarget)}
                            sx={{ fontWeight: 600, fontSize: '0.875rem', maxWidth: { xs: 200, md: 420 }, color: 'text.primary' }}
                        >
                            <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                {conv.title || t('chat.untitled')}
                            </Box>
                        </Button>
                    ) : (
                        <Skeleton width={160} />
                    )}
                </Breadcrumbs>
                <Box sx={{ flex: 1 }} />
            </Box>
            {conv && (
                <ChatActionsMenu
                    chat={conv}
                    anchorEl={menuAnchor}
                    onClose={() => setMenuAnchor(null)}
                    onChanged={(c) => setDetail((d) => (d ? { ...d, conversation: c } : d))}
                    onDeleted={() => navigate('/chats')}
                />
            )}

            {/* Thread */}
            <Box ref={scrollRef} onScroll={onScroll} sx={{ flex: 1, overflowY: 'auto', overscrollBehavior: 'contain' }}>
                <Box sx={{ ...COLUMN, pt: 2, pb: 4 }}>
                    {!detail && [0, 1].map((i) => <Skeleton key={i} variant="rounded" height={64} sx={{ my: 2 }} />)}
                    {messages.map((m) =>
                        m.role === 'user' ? (
                            <UserBubble key={m.id} text={m.content} />
                        ) : (
                            <Box key={m.id} sx={{ my: 2 }}>
                                {m.content && <Markdown text={m.content} />}
                                {m.error && !(streaming || turnError) && (
                                    <Alert severity="error" variant="outlined" sx={{ mt: 1 }}>{m.error}</Alert>
                                )}
                                <AssistantActions msg={m} isLast={m.id === lastAssistant?.id && lastMsg?.id === m.id} disabled={streaming} onRegenerate={() => void run('regenerate')} />
                            </Box>
                        ),
                    )}
                    {draftUser !== null && <UserBubble text={draftUser} />}
                    {streaming && (
                        <Box sx={{ my: 2 }}>
                            {draftReply ? (
                                <Markdown text={draftReply} />
                            ) : (
                                <Box sx={{ display: 'flex', gap: 0.75, py: 1.5 }} aria-label={t('chat.thinking')}>
                                    {[0, 1, 2].map((i) => (
                                        <Box key={i} sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: 'primary.main', animation: 'pulse 1.2s infinite', animationDelay: `${i * 0.15}s`, '@keyframes pulse': { '0%, 80%, 100%': { opacity: 0.25 }, '40%': { opacity: 1 } } }} />
                                    ))}
                                </Box>
                            )}
                        </Box>
                    )}
                    {turnError && (
                        <Alert
                            severity="error"
                            sx={{ mt: 2, '& .MuiAlert-message': { minWidth: 0, wordBreak: 'break-word' } }}
                            action={
                                <Box sx={{ display: 'flex', gap: 1 }}>
                                    {turnError.retryable && (
                                        <Button color="inherit" size="small" onClick={() => void run('regenerate')}>{t('chat.retry')}</Button>
                                    )}
                                    <Button color="inherit" size="small" component={RouterLink} to="/settings">{t('connection.open')}</Button>
                                </Box>
                            }
                        >
                            {turnError.message}
                        </Alert>
                    )}
                    {detail && messages.length === 0 && !streaming && draftUser === null && (
                        <Typography color="text.secondary" sx={{ textAlign: 'center', mt: 8 }}>{t('chat.empty')}</Typography>
                    )}
                </Box>
            </Box>

            {/* Composer */}
            <Box sx={{ ...COLUMN, pb: 2 }}>
                <Composer
                    autoFocus
                    model={model}
                    onModelChange={setModel}
                    streaming={streaming}
                    disabled={!detail}
                    onStop={() => abortRef.current?.abort()}
                    onSend={(text) => run('send', text)}
                    placeholder={t('chat.replyPlaceholder')}
                    minRows={1}
                    seed={seed}
                />
                <Typography variant="caption" color="text.disabled" sx={{ display: 'block', textAlign: 'center', mt: 0.75 }}>
                    {t('chat.disclaimer')}
                </Typography>
            </Box>
        </Box>
    );
}
