import {PlayerStop, Send} from '@/components/icons';
import {Box, CircularProgress, IconButton, InputBase, Paper, Stack, Tooltip, Typography, useMediaQuery, useTheme} from '@mui/material';
import type {ReactNode} from 'react';
import {useLayoutEffect, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';

interface ComposerProps {
    placeholder: string;
    // Resolves true once the text was accepted; the input is cleared only
    // then, so a failed send keeps what the user typed.
    onSubmit: (text: string) => Promise<boolean>;
    // Context chips shown above the input (folder, permission mode).
    context?: ReactNode;
    canSubmit?: boolean;
    disabled?: boolean;
    // Set while a turn runs: a Stop button shows, and Send stays available
    // for whatever onSubmit does meanwhile (the session view queues it).
    onStop?: () => void | Promise<void>;
    autoFocus?: boolean;
    minRows?: number;
    // Optional control of the text, for callers that put text back into the
    // input (a queued message taken back); uncontrolled otherwise.
    text?: string;
    onTextChange?: (text: string) => void;
    // Atomically clear a persisted draft only if it still matches this submission.
    onAccepted?: (submittedDraft: string) => void;
}

// Composer is the one input for both starting a session and continuing it:
// Desktop Enter sends; mobile Enter and Shift+Enter add a line. Ctrl/Cmd+Enter
// sends on either. An IME composition's Enter
// (confirming a candidate) never sends.
const Composer = ({
    placeholder, onSubmit, context, canSubmit = true, disabled, onStop, autoFocus, minRows = 2, text: controlled, onTextChange, onAccepted,
}: ComposerProps) => {
    const {t} = useTranslation();
    const isMobile = useMediaQuery(useTheme().breakpoints.down('sm'));
    const [stopping, setStopping] = useState(false);
    const stoppingRef = useRef(false);
    const stop = async () => {
        if (!onStop || stoppingRef.current) return;
        stoppingRef.current = true;
        setStopping(true);
        try { await onStop(); } finally { stoppingRef.current = false; setStopping(false); }
    };
    const [own, setOwn] = useState('');
    const text = controlled ?? own;
    const setText = (v: string) => {
        textRef.current = v;
        if (onTextChange) onTextChange(v);
        else setOwn(v);
    };
    const [submitting, setSubmitting] = useState(false);
    const submittingRef = useRef(false);
    const textRef = useRef(text);
    useLayoutEffect(() => { textRef.current = text; }, [text]);

    const ready = canSubmit && !disabled && !submitting && text.trim() !== '';

    const submit = async () => {
        if (!ready || submittingRef.current) return;
        const submitted = text;
        submittingRef.current = true;
        setSubmitting(true);
        try {
            if (await onSubmit(submitted.trim())) {
                if (onAccepted) onAccepted(submitted);
                else if (textRef.current === submitted) setText('');
            }
        } finally {
            submittingRef.current = false;
            setSubmitting(false);
        }
    };

    return (
        <Paper
            variant="outlined"
            sx={{
                borderRadius: 3,
                px: 1.5,
                pt: context ? 1 : 1.25,
                pb: 1,
                bgcolor: 'background.paper',
                '&:focus-within': {borderColor: 'text.secondary'},
            }}
        >
            {context && (
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', flexWrap: 'wrap', rowGap: 0.5, mb: 0.75}}>
                    {context}
                </Stack>
            )}
            <Stack direction="row" spacing={1} sx={{alignItems: 'flex-end'}}>
                <InputBase
                    multiline
                    fullWidth
                    minRows={minRows}
                    maxRows={isMobile ? 5 : 10}
                    autoFocus={autoFocus}
                    inputProps={{'aria-label': placeholder}}
                    placeholder={placeholder}
                    value={text}
                    disabled={disabled}
                    onChange={(e) => setText(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing && (!isMobile || e.ctrlKey || e.metaKey)) {
                            e.preventDefault();
                            void submit();
                        }
                    }}
                    sx={{fontSize: '0.9375rem', lineHeight: 1.6, color: 'text.primary'}}
                />
                <Stack direction="row" spacing={0.5} sx={{flexShrink: 0, pb: 0.25}}>
                    {(!onStop || text.trim() !== '') && (
                        <IconButton
                            size="small"
                            sx={{minWidth: {xs: 40, md: 28}, minHeight: {xs: 40, md: 28}}}
                            color="primary"
                            disabled={!ready}
                            onClick={submit}
                            aria-label={t('desk.send', {defaultValue: 'Send'})}
                        >
                            {submitting ? <CircularProgress size={18}/> : <Send fontSize="small"/>}
                        </IconButton>
                    )}
                    {onStop && (
                        <Tooltip title={t('desk.interrupt', {defaultValue: 'Stop the current turn'})}>
                            <IconButton size="small" disabled={stopping} onClick={() => void stop()} aria-label={t('desk.interruptShort', {defaultValue: 'Stop'})} sx={{bgcolor: 'action.selected', minWidth: {xs: 40, md: 28}, minHeight: {xs: 40, md: 28}}}>
                                <>{stopping ? <CircularProgress size={18}/> : <PlayerStop fontSize="small"/>}</>
                            </IconButton>
                        </Tooltip>
                    )}
                </Stack>
            </Stack>
            {!isMobile && <Typography variant="caption" sx={{display: 'block', color: 'text.secondary', mt: 0.25}}>{t('desk.sendKeys', {defaultValue: 'Enter to send · Shift+Enter for a new line'})}</Typography>}
        </Paper>
    );
};

export default Composer;
