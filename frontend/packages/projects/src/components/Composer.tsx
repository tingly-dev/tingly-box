import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { Box, CircularProgress, IconButton, InputBase, ListItemText, Menu, MenuItem, Paper, Tooltip, Typography, useMediaQuery, useTheme } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Attachment } from '../client';
import { isLongText, lineCount, pastedTextFile } from './attachments';
import { ArrowUpward, Attach, Close, Description, ExpandMore, Stop } from './icons';
import { ModelPicker } from './ModelPicker';
import { radius } from '@tingly/ui/theme/radius';
import { fontSizes } from '@tingly/ui/theme/fonts';

/** A file on its way into the message: uploading, ready, or refused. */
interface Pending {
    key: number;
    name: string;
    /** Lines of a long paste stored as a file; the chip says so instead of a file name. */
    lines?: number;
    /** Local preview of an image while and after it uploads. */
    preview?: string;
    done?: Attachment;
}

/**
 * How a message meets a running turn. The default (no mode) joins the turn,
 * as in Claude Code; the other two are picked per message from the send menu.
 */
export type SendMode = 'queue' | 'stop_and_send';

export interface ComposerProps {
    /**
     * Returns false to keep the text in the box (e.g. the send failed). The
     * mode is only passed when the user chose one from the send menu.
     */
    onSend: (text: string, attachments: Attachment[], mode?: SendMode) => Promise<boolean | void> | boolean | void;
    /**
     * Stores files for the message and returns what to send with it. Setting
     * it turns on the attach button, dropping files and pasting (a long
     * paste becomes a text file).
     */
    attach?: (files: File[]) => Promise<Attachment[]>;
    /** Why attaching is unavailable right now; shows the button disabled with this hint, and ignores paste and drop of files. */
    attachBlocked?: string;
    /** A turn runs: Stop shows when onStop is set, and Send waits unless sendWhileStreaming. */
    streaming?: boolean;
    onStop?: () => void | Promise<void>;
    /** Send stays available while a turn runs (the host adds it to the turn, or queues it). */
    sendWhileStreaming?: boolean;
    /**
     * While a turn runs, the send button gets a menu to send "after this turn"
     * or "stop and send" instead (Alt+Enter is "after this turn"). The host
     * must honour the mode passed to onSend.
     */
    sendModes?: boolean;
    disabled?: boolean;
    /** False holds Send while the text can still be edited (e.g. no folder picked yet). */
    canSubmit?: boolean;
    placeholder?: string;
    autoFocus?: boolean;
    /** Shows the model picker when onModelChange is set. */
    model?: string;
    onModelChange?: (m: string) => void;
    /** What `model` is for, when other model pickers share the box. */
    modelLabel?: string;
    /** Controls above the text (a host's profile, model and permission pickers). */
    context?: ReactNode;
    /** Controls shown at the bottom-left (project / memory chips). */
    leftSlot?: ReactNode;
    /** Lines the box shows when empty (one by default). */
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
 * The message box every Projects surface writes in. Enter sends on
 * desktop; on a phone Enter adds a line and Ctrl/Cmd+Enter sends. An IME's
 * Enter (confirming a candidate) never sends, and a send never runs twice.
 */
export function Composer({
    onSend, attach, attachBlocked, streaming, onStop, sendWhileStreaming, sendModes, disabled, canSubmit = true, placeholder, autoFocus, model, onModelChange, modelLabel,
    context, leftSlot, minRows = 1, seed, keepUntilSent, text: controlled, onTextChange, onAccepted, keysHint,
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
    const [dragging, setDragging] = useState(false);
    const [files, setFiles] = useState<Pending[]>([]);
    const [attachError, setAttachError] = useState('');
    const fileInput = useRef<HTMLInputElement>(null);
    const nextKey = useRef(0);
    const uploading = files.some((f) => !f.done);
    const ready = files.flatMap((f) => (f.done ? [f.done] : []));
    const filesRef = useRef(files);
    filesRef.current = files;
    useEffect(() => () => filesRef.current.forEach((f) => f.preview && URL.revokeObjectURL(f.preview)), []);

    const addFiles = async (list: File[], pastedLines?: number) => {
        if (!attach || attachBlocked || list.length === 0) return;
        setAttachError('');
        const added: Pending[] = list.map((f) => ({
            key: nextKey.current++,
            name: f.name || 'file',
            lines: pastedLines,
            preview: f.type.startsWith('image/') ? URL.createObjectURL(f) : undefined,
        }));
        setFiles((cur) => [...cur, ...added]);
        const drop = (keys: number[]) => setFiles((cur) => cur.filter((f) => {
            const gone = keys.includes(f.key);
            if (gone && f.preview) URL.revokeObjectURL(f.preview);
            return !gone;
        }));
        try {
            const stored = await attach(list);
            if (stored.length !== added.length) throw new Error(t('composer.attachFailed'));
            setFiles((cur) => cur.map((f) => {
                const i = added.findIndex((a) => a.key === f.key);
                return i < 0 ? f : { ...f, done: stored[i] };
            }));
        } catch (e) {
            drop(added.map((a) => a.key));
            setAttachError((e as Error).message || t('composer.attachFailed'));
        }
    };
    const removeFile = (key: number) => setFiles((cur) => cur.filter((f) => {
        if (f.key === key && f.preview) URL.revokeObjectURL(f.preview);
        return f.key !== key;
    }));

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
    const hasContent = text.trim() !== '' || ready.length > 0;
    const canSend = hasContent && !uploading && canSubmit && !blockedByTurn && !sending && !disabled;

    const submit = async (mode?: SendMode) => {
        if (!canSend || sendingRef.current) return;
        const value = text;
        sendingRef.current = true;
        setSending(true);
        if (!keepUntilSent) setText('');
        try {
            const ok = (await (mode ? onSend(value.trim(), ready, mode) : onSend(value.trim(), ready))) !== false;
            if (ok) setFiles([]);
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

    const [modeMenu, setModeMenu] = useState<HTMLElement | null>(null);
    const showStop = streaming && onStop;
    // While a turn runs, Send shows next to Stop once there is something to send.
    const showSend = !showStop || (sendWhileStreaming && hasContent);

    const sendButton = showSend && (
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
    );
    const modesAvailable = sendModes && streaming && sendWhileStreaming && showSend;
    const modeButton = modesAvailable && (
        <>
            <Tooltip title={t('composer.sendOptions')}>
                <span>
                    <IconButton
                        onClick={(e) => setModeMenu(e.currentTarget)}
                        disabled={!canSend}
                        size="small"
                        aria-label={t('composer.sendOptions')}
                        aria-haspopup="menu"
                        sx={{ color: 'text.secondary', ml: -0.5 }}
                    >
                        <ExpandMore fontSize="small" />
                    </IconButton>
                </span>
            </Tooltip>
            <Menu anchorEl={modeMenu} open={!!modeMenu} onClose={() => setModeMenu(null)} anchorOrigin={{ vertical: 'top', horizontal: 'right' }} transformOrigin={{ vertical: 'bottom', horizontal: 'right' }}>
                <MenuItem onClick={() => { setModeMenu(null); void submit('queue'); }}>
                    <ListItemText primary={t('composer.sendQueue')} secondary={t('composer.sendQueueHint')} slotProps={{ primary: { sx: { fontSize: fontSizes.base } }, secondary: { sx: { fontSize: fontSizes.sm } } }} />
                </MenuItem>
                <MenuItem onClick={() => { setModeMenu(null); void submit('stop_and_send'); }}>
                    <ListItemText primary={t('composer.sendStop')} secondary={t('composer.sendStopHint')} slotProps={{ primary: { sx: { fontSize: fontSizes.base } }, secondary: { sx: { fontSize: fontSizes.sm } } }} />
                </MenuItem>
            </Menu>
        </>
    );
    const stopButton = showStop && (
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
    );
    const hint = keysHint && !mobile && !leftSlot;
    // Nothing under the text: the buttons sit beside it and the box is one
    // line until the text grows.
    const inline = !leftSlot && !onModelChange && !hint;

    const attachButton = attach && (
        <Tooltip title={attachBlocked || t('composer.attach')}>
            <span>
                <IconButton size="small" disabled={disabled || !!attachBlocked} onClick={() => fileInput.current?.click()} aria-label={t('composer.attach')} sx={{ color: 'text.secondary' }}>
                    <Attach fontSize="small" />
                </IconButton>
                <input
                    ref={fileInput}
                    type="file"
                    multiple
                    hidden
                    data-testid="composer-file-input"
                    onChange={(e) => {
                        void addFiles(Array.from(e.target.files ?? []));
                        e.target.value = '';
                    }}
                />
            </span>
        </Tooltip>
    );
    const chips = (files.length > 0 || attachError !== '') && (
        <Box sx={{ mb: 1 }}>
            {files.length > 0 && (
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75 }}>
                    {files.map((f) => (
                        <Box
                            key={f.key}
                            sx={{ display: 'flex', alignItems: 'center', gap: 0.75, pl: f.preview ? 0.5 : 1, pr: 0.5, py: 0.5, borderRadius: radius.row, border: 1, borderColor: 'divider', maxWidth: 240 }}
                        >
                            {f.preview
                                ? <Box component="img" src={f.preview} alt="" sx={{ width: 28, height: 28, borderRadius: radius.chip, objectFit: 'cover' }} />
                                : <Description fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} />}
                            <Typography variant="caption" noWrap sx={{ minWidth: 0 }}>
                                {f.lines !== undefined ? t('composer.pastedText', { count: f.lines }) : f.name}
                            </Typography>
                            {f.done
                                ? <IconButton size="small" onClick={() => removeFile(f.key)} aria-label={t('composer.removeAttachment', { name: f.name })} sx={{ p: 0.25 }}><Close sx={{ fontSize: 14 }} /></IconButton>
                                : <CircularProgress size={14} sx={{ mx: 0.5 }} />}
                        </Box>
                    ))}
                </Box>
            )}
            {attachError !== '' && <Typography variant="caption" color="error" role="alert" sx={{ display: 'block', mt: 0.5 }}>{attachError}</Typography>}
        </Box>
    );

    const input = (
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
            onPaste={attach && !attachBlocked ? (e) => {
                const pasted = Array.from(e.clipboardData.files);
                if (pasted.length > 0) {
                    e.preventDefault();
                    void addFiles(pasted);
                    return;
                }
                const long = e.clipboardData.getData('text/plain');
                if (isLongText(long)) {
                    e.preventDefault();
                    void addFiles([pastedTextFile(long)], lineCount(long));
                }
            } : undefined}
            onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing && (!mobile || e.ctrlKey || e.metaKey)) {
                    e.preventDefault();
                    void submit(e.altKey && modesAvailable ? 'queue' : undefined);
                }
            }}
            sx={{ fontSize: fontSizes.base, lineHeight: 1.65, color: 'text.primary', ...(inline && { py: 0.5 }) }}
        />
    );

    return (
        <Paper
            variant="outlined"
            sx={{
                borderRadius: radius.card,
                px: inline ? 1.5 : 1.75,
                pt: inline ? 0.75 : 1.25,
                pb: 0.75,
                bgcolor: 'background.paper',
                boxShadow: '0 2px 12px rgba(0,0,0,0.04)',
                '&:focus-within': { borderColor: 'text.disabled' },
                ...(dragging && { borderColor: 'primary.main', borderStyle: 'dashed' }),
            }}
            onDragOver={attach && !attachBlocked ? (e) => {
                if (!e.dataTransfer.types.includes('Files')) return;
                e.preventDefault();
                setDragging(true);
            } : undefined}
            onDragLeave={attach && !attachBlocked ? () => setDragging(false) : undefined}
            onDrop={attach && !attachBlocked ? (e) => {
                if (e.dataTransfer.files.length === 0) return;
                e.preventDefault();
                setDragging(false);
                void addFiles(Array.from(e.dataTransfer.files));
            } : undefined}
        >
            {context && (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap', rowGap: 0.5, mb: 1 }}>{context}</Box>
            )}
            {chips}
            {inline ? (
                <Box sx={{ display: 'flex', alignItems: 'flex-end', gap: 1 }}>
                    {attachButton}
                    {input}
                    {sendButton}
                    {modeButton}
                    {stopButton}
                </Box>
            ) : (
                <>
                    {input}
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.75 }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, minWidth: 0, flexWrap: 'wrap' }}>
                            {attachButton}
                            {leftSlot}
                            {hint && <Typography variant="caption" color="text.secondary">{t('composer.keys')}</Typography>}
                        </Box>
                        {onModelChange && <ModelPicker value={model ?? ''} onChange={onModelChange} label={modelLabel} />}
                        {sendButton}
                        {modeButton}
                        {stopButton}
                    </Box>
                </>
            )}
        </Paper>
    );
}
