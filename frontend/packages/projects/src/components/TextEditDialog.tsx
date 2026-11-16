import { useEffect, useState, type ReactNode } from 'react';
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { formatTokens } from '../utils/format';
import { estimateTokens } from '../utils/tokens';
import { fontSizes } from '@tingly/ui/theme/fonts';

interface Props {
    open: boolean;
    title: string;
    /** Explains what the text is for; shown above the editor. */
    intro?: ReactNode;
    initialText: string;
    /** Shows a name field (e.g. "Add text content"). */
    nameField?: { label: string; initial: string };
    placeholder?: string;
    submitLabel?: string;
    readOnly?: boolean;
    onClose: () => void;
    onSubmit?: (v: { text: string; name: string }) => Promise<void>;
}

/** Large text editor used for instructions, memory and knowledge text. Shows
 * the live token estimate so the cost of what is being written is visible. */
export function TextEditDialog({ open, title, intro, initialText, nameField, placeholder, submitLabel, readOnly, onClose, onSubmit }: Props) {
    const { t } = useTranslation('projects');
    const [text, setText] = useState(initialText);
    const [name, setName] = useState(nameField?.initial ?? '');
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (open) {
            setText(initialText);
            setName(nameField?.initial ?? '');
        }
    }, [open, initialText]);

    const submit = async () => {
        if (!onSubmit) return;
        setBusy(true);
        try {
            await onSubmit({ text, name: name.trim() });
            onClose();
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle>{title}</DialogTitle>
            <DialogContent>
                <Stack spacing={2} sx={{ pt: 0.5 }}>
                    {intro && <Typography variant="body2" color="text.secondary">{intro}</Typography>}
                    {nameField && (
                        <TextField label={nameField.label} value={name} onChange={(e) => setName(e.target.value)} fullWidth autoFocus disabled={readOnly} />
                    )}
                    <TextField
                        value={text}
                        onChange={(e) => setText(e.target.value)}
                        placeholder={placeholder}
                        multiline
                        minRows={12}
                        maxRows={28}
                        fullWidth
                        autoFocus={!nameField}
                        slotProps={{ input: { readOnly }, htmlInput: { style: { fontSize: fontSizes.base, lineHeight: 1.6 } } }}
                    />
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Box sx={{ flex: 1, pl: 1 }}>
                    <Typography variant="caption" color="text.secondary">
                        {t('common.tokensApprox', { value: formatTokens(estimateTokens(text)) })}
                    </Typography>
                </Box>
                <Button onClick={onClose} color="inherit">{readOnly ? t('common.close') : t('common.cancel')}</Button>
                {!readOnly && onSubmit && (
                    <Button variant="contained" onClick={() => void submit()} disabled={busy || (!!nameField && !name.trim())}>
                        {submitLabel ?? t('common.save')}
                    </Button>
                )}
            </DialogActions>
        </Dialog>
    );
}
