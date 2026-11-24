import { Button, Dialog, DialogActions, DialogContent, DialogTitle, InputAdornment, TextField } from '@mui/material';
import { type ReactNode, useEffect, useId, useState } from 'react';

export interface CustomValueDialogProps {
    open: boolean;
    title: ReactNode;
    /** The text the field starts with: the current custom value, empty otherwise. */
    initial: string;
    inputLabel?: string;
    helperText?: ReactNode;
    /** Shown after the number ("min", "rows"). */
    unit?: string;
    /** 'number' opens a numeric keypad on phones; the text is still validated by `validate`. */
    inputMode?: 'numeric' | 'decimal' | 'text';
    placeholder?: string;
    confirmLabel?: ReactNode;
    cancelLabel?: ReactNode;
    /** Returns the error to show, or null when the text is acceptable. */
    validate: (raw: string) => string | null;
    onSubmit: (raw: string) => void;
    onCancel: () => void;
}

/**
 * The small "type your own" prompt behind a "Custom…" menu entry. Enter
 * confirms, Esc or Cancel leaves the setting as it was; the error shows only
 * after the first edit or attempt so an empty field is not shouted at.
 */
export default function CustomValueDialog({
    open, title, initial, inputLabel, helperText, unit, inputMode = 'text', placeholder,
    confirmLabel = 'Apply', cancelLabel = 'Cancel', validate, onSubmit, onCancel,
}: CustomValueDialogProps) {
    const [text, setText] = useState(initial);
    const [touched, setTouched] = useState(false);
    const titleId = useId();

    useEffect(() => {
        if (open) { setText(initial); setTouched(false); }
    }, [open, initial]);

    const error = validate(text.trim());
    const showError = touched && error;
    const submit = () => {
        setTouched(true);
        if (!error) onSubmit(text.trim());
    };

    return (
        <Dialog open={open} onClose={onCancel} fullWidth maxWidth="xs" aria-labelledby={titleId}
            onClick={(e) => e.stopPropagation()}>
            <DialogTitle id={titleId}>{title}</DialogTitle>
            <form onSubmit={(e) => { e.preventDefault(); submit(); }}>
                <DialogContent sx={{ pt: 1 }}>
                    <TextField
                        autoFocus fullWidth size="small"
                        label={inputLabel}
                        placeholder={placeholder}
                        value={text}
                        onChange={(e) => { setText(e.target.value); setTouched(true); }}
                        error={!!showError}
                        helperText={showError ? error : helperText}
                        slotProps={{
                            htmlInput: { inputMode, 'aria-label': typeof inputLabel === 'string' ? inputLabel : undefined },
                            input: unit ? { endAdornment: <InputAdornment position="end">{unit}</InputAdornment> } : undefined,
                        }}
                    />
                </DialogContent>
                <DialogActions>
                    <Button color="inherit" onClick={onCancel}>{cancelLabel}</Button>
                    <Button type="submit" variant="contained" disabled={!!error && touched}>{confirmLabel}</Button>
                </DialogActions>
            </form>
        </Dialog>
    );
}
