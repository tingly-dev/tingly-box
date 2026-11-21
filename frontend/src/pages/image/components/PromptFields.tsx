import { memo } from 'react';
import { InputAdornment, TextField } from '@mui/material';
import type { SxProps, TextFieldProps, Theme } from '@mui/material';
import { CopyIconButton } from '@tingly/ui/components/CopyIconButton';
import { usePhoneShell } from '@tingly/ui/hooks/usePhoneShell';
import { usePromptText, type PromptStore } from './usePromptStore';

// The prompt's input surfaces. They subscribe to the store themselves, so a
// keystroke re-renders these and nothing else on the playground card.

interface PanelFieldProps {
    store: PromptStore;
    inputRef: React.Ref<HTMLTextAreaElement>;
    label?: string;
    placeholder: string;
    ariaLabel: string;
    disabled: boolean;
    onKeyDown: (event: React.KeyboardEvent) => void;
    onDrop: (event: React.DragEvent) => void;
    // The buttons after the copy button (snippets, open file, expand).
    actions: React.ReactNode;
    copyLabel: string;
    copiedLabel: string;
    sx?: SxProps<Theme>;
}

export const PromptPanelField = memo(function PromptPanelField({
    store, inputRef, label, placeholder, ariaLabel, disabled, onKeyDown, onDrop, actions, copyLabel, copiedLabel, sx,
}: PanelFieldProps) {
    const value = usePromptText(store);
    // Phone shell: the buttons (44px each there) take a row above the text
    // instead of a column beside it that leaves the prompt a few characters wide.
    const phone = usePhoneShell();
    return (
        <TextField
            inputRef={inputRef}
            multiline
            minRows={3}
            fullWidth
            label={label}
            placeholder={placeholder}
            value={value}
            onChange={(event) => store.set(event.target.value)}
            onKeyDown={onKeyDown}
            onDragOver={(event) => event.preventDefault()}
            onDrop={onDrop}
            disabled={disabled}
            sx={sx}
            slotProps={{
                htmlInput: { 'aria-label': ariaLabel },
                input: {
                    ...(phone && { sx: { display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', pt: 0.5 } }),
                    endAdornment: (
                        <InputAdornment
                            position="end"
                            sx={phone
                                ? { order: -1, height: 'auto', maxHeight: 'none', justifyContent: 'flex-end', mr: -1, ml: 0, gap: 0 }
                                : { alignSelf: 'flex-start', mt: 0.5, mr: -0.5, gap: 0.25 }}
                        >
                            {/* A prompt is text the user goes on to reuse elsewhere —
                                it should never have to be selected by hand. */}
                            {value.trim() && (
                                <CopyIconButton value={value} label={copyLabel} copiedLabel={copiedLabel} iconSize={16} />
                            )}
                            {actions}
                        </InputAdornment>
                    ),
                },
            }}
        />
    );
});

// Same text as the panel's field — a bigger window onto the prompt, not a second one.
export const PromptEditorField = memo(function PromptEditorField({
    store, onKeyDown, placeholder, helperText,
}: { store: PromptStore; onKeyDown: (event: React.KeyboardEvent) => void; placeholder: string; helperText: TextFieldProps['helperText'] }) {
    const value = usePromptText(store);
    return (
        <TextField
            autoFocus
            multiline
            minRows={12}
            maxRows={28}
            fullWidth
            value={value}
            onChange={(event) => store.set(event.target.value)}
            onKeyDown={onKeyDown}
            placeholder={placeholder}
            helperText={helperText}
        />
    );
});

export const PromptEditorCopyButton = memo(function PromptEditorCopyButton({
    store, label, copiedLabel,
}: { store: PromptStore; label: string; copiedLabel: string }) {
    const value = usePromptText(store);
    return <CopyIconButton value={value} label={label} copiedLabel={copiedLabel} size="medium" />;
});
