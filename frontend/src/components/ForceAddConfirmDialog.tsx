import { WarningAmber, Info } from '@mui/icons-material';
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
    Chip,
} from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';

interface ForceAddConfirmDialogProps {
    open: boolean;
    error: {
        message: string;
        details?: string;
    } | null;
    providerInfo?: {
        name: string;
        apiBase: string;
        apiStyle: 'openai' | 'anthropic' | undefined;
        hasToken: boolean;
    };
    onConfirm: () => void;
    onCancel: () => void;
}

const ForceAddConfirmDialog: React.FC<ForceAddConfirmDialogProps> = ({
    open,
    error,
    providerInfo,
    onConfirm,
    onCancel,
}) => {
    const { t } = useTranslation();

    const handleConfirm = () => {
        console.log('ForceAddConfirmDialog onConfirm called');
        onConfirm();
    };

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
                <Stack spacing={2.5}>
                    {/* Provider Info Summary */}
                    {providerInfo && (
                        <Box sx={{
                            p: 2,
                            bgcolor: 'action.hover',
                            borderRadius: 1,
                            border: '1px solid',
                            borderColor: 'divider',
                        }}>
                            <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                                {t('providerDialog.forceAdd.providerInfo')}
                            </Typography>
                            <Stack spacing={1}>
                                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                    <Typography variant="body2" color="text.secondary">
                                        {t('providerDialog.keyName.label')}:
                                    </Typography>
                                    <Typography variant="body2" fontWeight="medium">
                                        {providerInfo.name || '-'}
                                    </Typography>
                                </Box>
                                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                                    <Typography variant="body2" color="text.secondary" sx={{ minWidth: 80 }}>
                                        {t('providerDialog.providerOrUrl.label')}:
                                    </Typography>
                                    <Typography variant="body2" fontWeight="medium" sx={{ textAlign: 'right', ml: 2, wordBreak: 'break-all' }}>
                                        {providerInfo.apiBase || '-'}
                                    </Typography>
                                </Box>
                                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                    <Typography variant="body2" color="text.secondary">
                                        {t('providerDialog.apiKey.label')}:
                                    </Typography>
                                    <Chip
                                        size="small"
                                        label={providerInfo.hasToken ? '••••••••' : t('providerDialog.forceAdd.noKey')}
                                        variant="outlined"
                                    />
                                </Box>
                            </Stack>
                        </Box>
                    )}

                    {/* Warning Message */}
                    <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start' }}>
                        <Info color="info" sx={{ mt: 0.5, flexShrink: 0 }} />
                        <Typography variant="body2" color="text.secondary">
                            {t('providerDialog.forceAdd.message')}
                        </Typography>
                    </Box>

                    {/* Error Details */}
                    {error && (
                        <Alert severity="error" variant="outlined">
                            <Typography variant="body2" fontWeight="bold" gutterBottom>
                                {t('providerDialog.forceAdd.whyFailed')}
                            </Typography>
                            <Typography variant="body2" sx={{ mb: 1 }}>
                                {error.message}
                            </Typography>
                            {error.details && (
                                <Typography variant="caption" color="text.secondary">
                                    {t('providerDialog.forceAdd.errorDetails')}: {error.details}
                                </Typography>
                            )}
                        </Alert>
                    )}

                    {/* Confirmation Note */}
                    <Box sx={{
                        p: 2,
                        bgcolor: 'warning.50',
                        borderRadius: 1,
                        border: '1px solid',
                        borderColor: 'warning.200',
                    }}>
                        <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start' }}>
                            <WarningAmber sx={{ color: 'warning.main', fontSize: 20, flexShrink: 0, mt: 0.25 }} />
                            <Stack spacing={0.5}>
                                <Typography variant="subtitle2" fontWeight="medium" color="warning.dark">
                                    {t('providerDialog.forceAdd.confirmNoteTitle')}
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    {t('providerDialog.forceAdd.confirmNote')}
                                </Typography>
                            </Stack>
                        </Box>
                    </Box>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, gap: 1 }}>
                <Button onClick={onCancel} color="inherit">
                    {t('providerDialog.forceAdd.cancel')}
                </Button>
                <Button
                    onClick={handleConfirm}
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
