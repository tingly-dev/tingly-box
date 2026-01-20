import { WarningAmber } from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Stack,
    Typography,
} from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';

interface ForceAddConfirmDialogProps {
    open: boolean;
    error: {
        message: string;
        details?: string;
    } | null;
    onConfirm: () => void;
    onCancel: () => void;
}

const ForceAddConfirmDialog: React.FC<ForceAddConfirmDialogProps> = ({
    open,
    error,
    onConfirm,
    onCancel,
}) => {
    const { t } = useTranslation();

    return (
        <Dialog open={open} onClose={onCancel} maxWidth="sm" fullWidth>
            <DialogTitle>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <WarningAmber color="warning" />
                    <Typography variant="h6" component="span">
                        {t('providerDialog.forceAdd.title')}
                    </Typography>
                </Box>
            </DialogTitle>
            <DialogContent>
                <Stack spacing={2}>
                    <Typography variant="body1">
                        {t('providerDialog.forceAdd.message')}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        {t('providerDialog.forceAdd.explanation')}
                    </Typography>

                    {error && (
                        <Alert severity="error" sx={{ mt: 2 }}>
                            <Typography variant="body2" fontWeight="bold">
                                {error.message}
                            </Typography>
                            {error.details && (
                                <Typography variant="caption" display="block" sx={{ mt: 1 }}>
                                    {t('providerDialog.forceAdd.errorDetails')}: {error.details}
                                </Typography>
                            )}
                        </Alert>
                    )}
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onCancel} color="primary">
                    {t('providerDialog.forceAdd.cancel')}
                </Button>
                <Button
                    onClick={onConfirm}
                    variant="contained"
                    color="warning"
                    autoFocus
                >
                    {t('providerDialog.forceAdd.confirm')}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ForceAddConfirmDialog;
