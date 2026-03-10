import type { DialogProps } from '@mui/material';

export const shouldIgnoreDialogClose = (reason: Parameters<NonNullable<DialogProps['onClose']>>[1]) => {
    return reason === 'backdropClick';
};

