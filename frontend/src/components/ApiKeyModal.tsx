import {
    ContentCopy as CopyIcon,
} from '@mui/icons-material';
import {
    Box,
    Button,
    Dialog,
    DialogContent,
    DialogTitle,
    Typography
} from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';

interface ApiKeyModalProps {
    open: boolean;
    onClose: () => void;
    token: string;
    onCopy: (text: string, label: string) => void;
}

export const ApiKeyModal: React.FC<ApiKeyModalProps> = ({
    open,
    onClose,
    token,
    onCopy
}) => {
    const { t } = useTranslation();
    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="md"
            fullWidth
        >
            <DialogTitle>{t('apiKeyModal.title')}</DialogTitle>
            <DialogContent>
                <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                        {t('apiKeyModal.description')}
                    </Typography>
                    <Box
                        onClick={() => onCopy(token, 'API Key')}
                        sx={{
                            p: 2,
                            bgcolor: 'grey.100',
                            borderRadius: 1,
                            fontFamily: 'monospace',
                            fontSize: '0.85rem',
                            wordBreak: 'break-all',
                            border: '1px solid',
                            borderColor: 'grey.300',
                            cursor: 'pointer',
                            '&:hover': {
                                backgroundColor: 'grey.200',
                                borderColor: 'primary.main'
                            },
                            transition: 'all 0.2s ease-in-out',
                            title: t('apiKeyModal.clickToCopy')
                        }}
                    >
                        {token}
                    </Box>
                </Box>
                <Box sx={{ display: 'flex', gap: 1 }}>
                    <Button
                        variant="outlined"
                        onClick={() => onCopy(token, 'API Key')}
                        startIcon={<CopyIcon fontSize="small" />}
                    >
                        {t('apiKeyModal.copyButton')}
                    </Button>
                </Box>
            </DialogContent>
        </Dialog>
    );
};

export default ApiKeyModal;
