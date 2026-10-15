import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { Box, CircularProgress, IconButton, InputBase, Paper, Tooltip, Typography, useMediaQuery, useTheme } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { ArrowUpward, Stop } from './icons';
import { ModelPicker } from './ModelPicker';

export interface ComposerProps {
    /** Returns false to keep the text in the box (e.g. the send failed). */
    onSend: (text: string) => Promise<boolean | void> | boolean | void;
    /** A turn runs: Stop shows when onStop is set, and Send waits unless sendWhileStreaming. */
    streaming?: boolean;
    onStop?: () => void | Promise<void>;
    /** Send stays available while a turn runs (the host queues or delivers it after the turn). */
    sendWhileStreaming?: boolean;
    disabled?: boolean;
    /** False holds Send while the text can still be edited (e.g. no folder picked yet). */
    canSubmit?: boolean;
    placeholder?: string;
    autoFocus?: boolean;
    /** Shows the model picker when onModelChange is set. */
    model?: string;
    onModelChange?: (m: string) => void;
    /** Controls above the text (a host's profile, model and permission pickers). */
    context?: ReactNode;
    /** Controls shown at the bottom-left (project / memory chips). */
    leftSlot?: ReactNode;
    minRows?: number;
    /** Text to place in the box when it changes (e.g. a refused first message). */
    seed?: string;
    /**
     * Keep the text in the box until onSend accepts it, instead of clearing
     * it at once. For a send that is a quick request (a page reload while it
     * is in flight keeps the text); a streamed reply clears at once instead.
     */
    keepUntilSent?: boolean;
    /** Controlled text, for hosts that keep drafts; uncontrolled otherwise. */
    text?: string;
    onTextChange?: (text: string) => void;
    /** Called with what was sent once it was accepted, instead of clearing the box (a host clears its own draft). */
    onAccepted?: (sent: string) => void;
    /** Show the send keys under the box (desktop only). */
    keysHint?: boolean;
}

/**
 * The message box every Projects and Desk surface writes in. Enter sends on
 * desktop; on a phone Enter adds a line and Ctrl/Cmd+Enter sends. An IME's
 * Enter (confirming a candidate) never sends, and a send never runs twice.
 */
export function Composer({
    onSend, streaming, onStop, sendWhileStreaming, disabled, canSubmit = true, placeholder, autoFocus, model, onModelChange,
    context, leftSlot, minRows = 2, seed, keepUntilSent, text: controlled, onTextChange, onAccepted, keysHint,
}: ComposerProps) {
    const { t } = useTranslation('projects');
    const mobile = useMediaQuery(useTheme().breakpoints.down('sm'));
    const [own, setOwn] = useState('');
    const text = controlled ?? own;
    const textRef = useRef(text);
    useLayoutEffect(() => { textRef.current = text; }, [text]);
    const setText = (v: string) => {
        textRef.current = v;
        if (onTextChange) onTextChange(v);
        else setOwn(v);
    };
    const [sending, setSending] = useState(false);
    const sendingRef = useRef(false);
    const [stopping, setStopping] = useState(false);
    const stoppingRef = useRef(false);
    const inputRef = useRef<HTMLTextAreaElement>(null);

    useEffect(() => {
        if (autoFocus) inputRef.current?.focus();
    }, [autoFocus]);

    useEffect(() => {
        if (seed) setText(seed);
        // Only a new seed puts text in the box.
    }, [seed]);

    const blockedByTurn = streaming && !sendWhileStreaming;
    const canSend = text.trim() !== '' && canSubmit && !blockedByTurn && !sending && !disabled;

    const submit = async () => {
        if (!canSend || sendingRef.current) return;
        const value = text;
        sendingRef.current = true;
        setSending(true);
        if (!keepUntilSent) setText('');
        try {
            const ok = (await onSend(value.trim())) !== false;
            if (ok && keepUntilSent) {
                if (onAccepted) onAccepted(value);
                else if (textRef.current === value) setText('');
            } else if (ok) {
                onAccepted?.(value);
            } else if (!keepUntilSent && textRef.current === '') {
                setText(value);
            }
        } finally {
            sendingRef.current = false;
            setSending(false);
            inputRef.current?.focus();
        }
    };

    const stop = async () => {
        if (!onStop || stoppingRef.current) return;
        stoppingRef.current = true;
        setStopping(true);
        try {
            await onStop();
        } finally {
            stoppingRef.current = false;
            setStopping(false);
        }
    };

    const showStop = streaming && onStop;
    // While a turn runs, Send shows next to Stop once there is something to send.
    const showSend = !showStop || (sendWhileStreaming && text.trim() !== '');

    return (
        <Paper
            variant="outlined"
            sx={{
                borderRadius: 4,
                px: 2,
                pt: 1.5,
                pb: 1,
                bgcolor: 'background.paper',
                boxShadow: '0 2px 12px rgba(0,0,0,0.04)',
                '&:focus-within': { borderColor: 'text.disabled' },
            }}
        >
            {context && (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap', rowGap: 0.5, mb: 1 }}>{context}</Box>
            )}
            <InputBase
                inputRef={inputRef}
                multiline
                minRows={minRows}
                maxRows={mobile ? 6 : 14}
                fullWidth
                value={text}
                disabled={disabled}
                placeholder={placeholder ?? t('composer.placeholder')}
                inputProps={{ 'aria-label': placeholder ?? t('composer.placeholder') }}
                onChange={(e) => setText(e.target.value)}
                onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing && (!mobile || e.ctrlKey || e.metaKey)) {
                        e.preventDefault();
                        void submit();
                    }
                }}
                sx={{ fontSize: '0.875rem', lineHeight: 1.65, color: 'text.primary' }}
            />
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 1 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, minWidth: 0, flexWrap: 'wrap' }}>
                    {leftSlot}
                    {keysHint && !mobile && !leftSlot && (
                        <Typography variant="caption" color="text.secondary">{t('composer.keys')}</Typography>
                    )}
                </Box>
                {onModelChange && <ModelPicker value={model ?? ''} onChange={onModelChange} />}
                {showSend && (
                    <Tooltip title={t('composer.send')}>
                        <span>
                            <IconButton
                                onClick={() => void submit()}
                                disabled={!canSend}
                                size="small"
                                aria-label={t('composer.sendShort')}
                                sx={{
                                    bgcolor: 'primary.main',
                                    color: 'primary.contrastText',
                                    '&:hover': { bgcolor: 'primary.dark' },
                                    '&.Mui-disabled': { bgcolor: 'action.selected', color: 'text.disabled' },
                                }}
                            >
                                {sending ? <CircularProgress size={18} color="inherit" /> : <ArrowUpward fontSize="small" />}
                            </IconButton>
                        </span>
                    </Tooltip>
                )}
                {showStop && (
                    <Tooltip title={t('composer.stopTurn')}>
                        <span>
                            <IconButton
                                onClick={() => void stop()}
                                disabled={stopping}
                                size="small"
                                aria-label={t('composer.stop')}
                                sx={{ bgcolor: 'text.primary', color: 'background.paper', '&:hover': { bgcolor: 'text.secondary' } }}
                            >
                                {stopping ? <CircularProgress size={18} color="inherit" /> : <Stop fontSize="small" />}
                            </IconButton>
                        </span>
                    </Tooltip>
                )}
            </Box>
        </Paper>
    );
}
