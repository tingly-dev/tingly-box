import { Close } from '@/components/icons';
import { Box, DialogTitle, IconButton } from '@mui/material';
import { type ReactNode } from 'react';

interface DialogHeaderProps {
    title: ReactNode;
    titleId: string;
    closeLabel: string;
    onClose: () => void;
    closeDisabled?: boolean;
}

const DialogHeader = ({
    title,
    titleId,
    closeLabel,
    onClose,
    closeDisabled = false,
}: DialogHeaderProps) => {
    return (
        <Box sx={{ position: 'relative' }}>
            <DialogTitle id={titleId} sx={{ pr: 7 }}>
                {title}
            </DialogTitle>
            <IconButton
                aria-label={closeLabel}
                onClick={onClose}
                disabled={closeDisabled}
                size="small"
                sx={{
                    position: 'absolute',
                    top: 12,
                    right: 12,
                }}
            >
                <Close />
            </IconButton>
        </Box>
    );
};

export default DialogHeader;
