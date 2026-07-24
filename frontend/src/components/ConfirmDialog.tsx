import {
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
    type ButtonProps,
} from '@mui/material';
import { type ReactNode, useId } from 'react';

export interface ConfirmDialogProps {
    open: boolean;
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

const ConfirmDialog = ({
    open,
    title,
    description,
    confirmLabel = 'Confirm',
    confirmingLabel = 'Working…',
    cancelLabel = 'Cancel',
    confirmColor = 'primary',
    loading = false,
    onClose,
    onConfirm,
}: ConfirmDialogProps) => {
    const titleId = useId();
    const descriptionId = useId();

    return (
        <Dialog
            open={open}
            onClose={loading ? undefined : onClose}
            aria-labelledby={titleId}
            aria-describedby={description ? descriptionId : undefined}
            slotProps={{
                paper: {
                    'aria-busy': loading,
                },
            }}
            fullWidth
            maxWidth="xs"
        >
            <DialogTitle id={titleId}>{title}</DialogTitle>
            {description && (
                <DialogContent>
                    <DialogContentText id={descriptionId} component="div">
                        {description}
                    </DialogContentText>
                </DialogContent>
            )}
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} disabled={loading}>
                    {cancelLabel}
                </Button>
                <Button
                    variant="contained"
                    color={confirmColor}
                    onClick={onConfirm}
                    disabled={loading}
                >
                    {loading && (
                        <CircularProgress
                            size={16}
                            color="inherit"
                            aria-hidden="true"
                            sx={{ mr: 1 }}
                        />
                    )}
                    {loading ? confirmingLabel : confirmLabel}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ConfirmDialog;
