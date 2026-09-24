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
    // While a turn runs the send button becomes a stop button.
    onStop?: () => void;
    autoFocus?: boolean;
    minRows?: number;
}

// Composer is the one input for both starting a session and continuing it:
// Enter sends, Shift+Enter adds a line, and an IME composition's Enter
// (confirming a candidate) never sends.
const Composer = ({placeholder, onSubmit, context, canSubmit = true, disabled, onStop, autoFocus, minRows = 2}: ComposerProps) => {
    const {t} = useTranslation();
    const [text, setText] = useState('');
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
                    sx={{fontSize: '0.95rem', lineHeight: 1.6}}
                />
                <Box sx={{flexShrink: 0, pb: 0.25}}>
                    {onStop ? (
                        <Tooltip title={t('desk.interrupt', {defaultValue: 'Stop the current turn'})}>
                            <IconButton size="small" onClick={onStop} aria-label={t('desk.interruptShort', {defaultValue: 'Stop'})} sx={{bgcolor: 'action.selected'}}>
                                <PlayerStop fontSize="small"/>
                            </IconButton>
                        </Tooltip>
                    ) : (
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
                </Box>
            </Stack>
        </Paper>
    );
};

export default Composer;
