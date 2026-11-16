import {
    Box,
    Button,
    CircularProgress,
    Popover,
    Typography,
    type ButtonProps,
} from '@mui/material';
import { type ReactNode, useId } from 'react';

export interface ConfirmPopoverProps {
    /** The element the popover opens next to (usually the button that was pressed). */
    anchorEl: HTMLElement | null;
    title: ReactNode;
    description?: ReactNode;
    confirmLabel?: ReactNode;
    confirmingLabel?: ReactNode;
    cancelLabel?: ReactNode;
    confirmColor?: ButtonProps['color'];
    loading?: boolean;
    onClose: () => void;
    onConfirm: () => void;
}

/**
 * A confirm step that opens at the control that triggered it, so the eye and
 * the pointer stay where they are. Use it for a destructive or costly action
 * on one row/card; keep `ConfirmDialog` for confirmations that have no single
 * anchor (page-level actions, long explanations).
 *
 * Open state is `anchorEl != null`. Esc / outside click cancels, Enter on the
 * focused confirm button confirms; focus starts on Cancel so a stray Enter is safe.
 */
const ConfirmPopover = ({
    anchorEl,
    title,
    description,
    confirmLabel = 'Confirm',
    confirmingLabel = 'Working…',
    cancelLabel = 'Cancel',
    confirmColor = 'primary',
    loading = false,
    onClose,
    onConfirm,
}: ConfirmPopoverProps) => {
    const titleId = useId();
    const descriptionId = useId();

    return (
        <Popover
            open={Boolean(anchorEl)}
            anchorEl={anchorEl}
            onClose={loading ? undefined : onClose}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            transformOrigin={{ vertical: 'top', horizontal: 'right' }}
            slotProps={{
                paper: {
                    role: 'alertdialog',
                    'aria-labelledby': titleId,
                    'aria-describedby': description ? descriptionId : undefined,
                    'aria-busy': loading,
                    sx: { mt: 0.5, p: 2, width: 300, maxWidth: 'calc(100vw - 32px)' },
                },
            }}
            onClick={(e) => e.stopPropagation()}
        >
            <Typography id={titleId} variant="subtitle2" sx={{ fontWeight: 600 }}>
                {title}
            </Typography>
            {description && (
                <Typography
                    id={descriptionId}
                    component="div"
                    variant="body2"
                    color="text.secondary"
                    sx={{ mt: 0.75, wordBreak: 'break-word' }}
                >
                    {description}
                </Typography>
            )}
            <Box sx={{ display: 'flex', justifyContent: 'flex-end', gap: 1, mt: 2 }}>
                <Button size="small" autoFocus onClick={onClose} disabled={loading}>
                    {cancelLabel}
                </Button>
                <Button
                    size="small"
                    variant="contained"
                    color={confirmColor}
                    onClick={onConfirm}
                    disabled={loading}
                >
                    {loading && (
                        <CircularProgress size={14} color="inherit" aria-hidden="true" sx={{ mr: 1 }} />
                    )}
                    {loading ? confirmingLabel : confirmLabel}
                </Button>
            </Box>
        </Popover>
    );
};

export default ConfirmPopover;
