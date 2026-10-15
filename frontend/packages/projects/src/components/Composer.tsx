import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Box, IconButton, InputBase, Paper, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { ArrowUpward, Stop } from './icons';
import { ModelPicker } from './ModelPicker';

interface Props {
    /** Returns false to keep the text in the box (e.g. the send failed). */
    onSend: (text: string) => Promise<boolean | void> | boolean | void;
    onStop?: () => void;
    streaming?: boolean;
    disabled?: boolean;
    placeholder?: string;
    autoFocus?: boolean;
    /** Shows the model picker when onModelChange is set. */
    model?: string;
    onModelChange?: (m: string) => void;
    /** Controls shown at the bottom-left (project / memory chips). */
    leftSlot?: ReactNode;
    minRows?: number;
    /** Text to place in the box when it changes (e.g. a refused first message). */
    seed?: string;
}

export function Composer({ onSend, onStop, streaming, disabled, placeholder, autoFocus, model, onModelChange, leftSlot, minRows = 2, seed }: Props) {
    const { t } = useTranslation();
    const [text, setText] = useState('');
    const [sending, setSending] = useState(false);
    const inputRef = useRef<HTMLTextAreaElement>(null);

    useEffect(() => {
        if (autoFocus) inputRef.current?.focus();
    }, [autoFocus]);

    useEffect(() => {
        if (seed) setText(seed);
    }, [seed]);

    const canSend = text.trim().length > 0 && !streaming && !sending && !disabled;
    const submit = async () => {
        if (!canSend) return;
        const value = text;
        setText('');
        setSending(true);
        try {
            const ok = await onSend(value);
            if (ok === false) setText(value);
        } finally {
            setSending(false);
            inputRef.current?.focus();
        }
    };

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
            <InputBase
                inputRef={inputRef}
                multiline
                minRows={minRows}
                maxRows={14}
                fullWidth
                value={text}
                placeholder={placeholder ?? t('composer.placeholder')}
                onChange={(e) => setText(e.target.value)}
                onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                        e.preventDefault();
                        void submit();
                    }
                }}
                sx={{ fontSize: 15, lineHeight: 1.6, color: 'text.primary' }}
            />
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 1 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, minWidth: 0, flexWrap: 'wrap' }}>{leftSlot}</Box>
                {onModelChange && <ModelPicker value={model ?? ''} onChange={onModelChange} />}
                {streaming ? (
                    <Tooltip title={t('composer.stop')}>
                        <IconButton onClick={onStop} sx={{ bgcolor: 'text.primary', color: 'background.paper', '&:hover': { bgcolor: 'text.secondary' } }} size="small" aria-label={t('composer.stop')}>
                            <Stop fontSize="small" />
                        </IconButton>
                    </Tooltip>
                ) : (
                    <Tooltip title={t('composer.send')}>
                        <span>
                            <IconButton
                                onClick={() => void submit()}
                                disabled={!canSend}
                                size="small"
                                aria-label={t('composer.send')}
                                sx={{
                                    bgcolor: 'primary.main',
                                    color: 'primary.contrastText',
                                    '&:hover': { bgcolor: 'primary.dark' },
                                    '&.Mui-disabled': { bgcolor: 'action.selected', color: 'text.disabled' },
                                }}
                            >
                                <ArrowUpward fontSize="small" />
                            </IconButton>
                        </span>
                    </Tooltip>
                )}
            </Box>
        </Paper>
    );
}
