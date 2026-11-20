import { Box, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Close } from '@/components/icons';
import { useBackToClose } from '@tingly/ui/hooks/useBackToClose';
import { usePhoneShell } from '@tingly/ui/hooks/usePhoneShell';

interface ModelPickerDialogProps {
    open: boolean;
    onClose: () => void;
    title: ReactNode;
    /** The picker (`ModelSelectDialog`). */
    children: ReactNode;
    /** A footer under the picker (multi-select's count and Add). */
    actions?: ReactNode;
    /** Desktop paper and content styles; the phone sheet ignores them. */
    paperSx?: SxProps<Theme>;
    contentSx?: SxProps<Theme>;
}

const DESKTOP_PAPER_SX = { height: '80vh' };

/**
 * The frame every model picker opens in. Desktop: a large centred dialog.
 * Phone shell: a full-screen sheet with a close button, which the system Back
 * closes too (`.design/mobile-ui.md`, "Model picker").
 */
export function ModelPickerDialog({ open, onClose, title, children, actions, paperSx = DESKTOP_PAPER_SX, contentSx }: ModelPickerDialogProps) {
    const { t } = useTranslation();
    const phone = usePhoneShell();
    useBackToClose(open, onClose, phone);

    if (phone) {
        return (
            <Dialog open={open} onClose={onClose} fullScreen>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, px: 0.5, flexShrink: 0, borderBottom: 1, borderColor: 'divider' }}>
                    <IconButton onClick={onClose} aria-label={t('common.close')}>
                        <Close />
                    </IconButton>
                    <Typography component="h2" variant="subtitle1" noWrap sx={{ flex: 1, minWidth: 0, fontWeight: 600 }}>
                        {title}
                    </Typography>
                </Box>
                <DialogContent sx={{ p: 0, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                    {children}
                </DialogContent>
                {actions && <DialogActions sx={{ px: 2, gap: 1, borderTop: 1, borderColor: 'divider' }}>{actions}</DialogActions>}
            </Dialog>
        );
    }

    return (
        <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth slotProps={{ paper: { sx: paperSx } }}>
            <DialogTitle sx={{ textAlign: 'center' }}>{title}</DialogTitle>
            <DialogContent sx={contentSx}>{children}</DialogContent>
            {actions && <DialogActions sx={{ px: 3, py: 1.5, gap: 1, borderTop: 1, borderColor: 'divider' }}>{actions}</DialogActions>}
        </Dialog>
    );
}

export default ModelPickerDialog;
