import {PlayerStop, Send} from '@/components/icons';
import {Box, CircularProgress, IconButton, InputBase, Paper, Stack, Tooltip} from '@mui/material';
import type {ReactNode} from 'react';
import {useState} from 'react';
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
    onStop?: () => void;
    autoFocus?: boolean;
    minRows?: number;
    // Optional control of the text, for callers that put text back into the
    // input (a queued message taken back); uncontrolled otherwise.
    text?: string;
    onTextChange?: (text: string) => void;
}

// Composer is the one input for both starting a session and continuing it:
// Enter sends, Shift+Enter adds a line, and an IME composition's Enter
// (confirming a candidate) never sends.
const Composer = ({
    placeholder, onSubmit, context, canSubmit = true, disabled, onStop, autoFocus, minRows = 2, text: controlled, onTextChange,
}: ComposerProps) => {
    const {t} = useTranslation();
    const [own, setOwn] = useState('');
    const text = controlled ?? own;
    const setText = (v: string) => (onTextChange ? onTextChange(v) : setOwn(v));
    const [submitting, setSubmitting] = useState(false);

    const ready = canSubmit && !disabled && !submitting && text.trim() !== '';

    const submit = async () => {
        if (!ready) return;
        setSubmitting(true);
        try {
            if (await onSubmit(text.trim())) setText('');
        } finally {
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
                    maxRows={12}
                    autoFocus={autoFocus}
                    placeholder={placeholder}
                    value={text}
                    disabled={disabled}
                    onChange={(e) => setText(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
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
                            <IconButton size="small" onClick={onStop} aria-label={t('desk.interruptShort', {defaultValue: 'Stop'})} sx={{bgcolor: 'action.selected'}}>
                                <PlayerStop fontSize="small"/>
                            </IconButton>
                        </Tooltip>
                    )}
                </Stack>
            </Stack>
        </Paper>
    );
};

export default Composer;
