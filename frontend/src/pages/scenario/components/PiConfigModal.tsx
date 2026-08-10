import { Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography, Stack, Link } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';

interface PiConfigModalProps {
    open: boolean;
    onClose: () => void;
}

const PI_REPO_URL = 'https://github.com/earendil-works/pi';

const PiConfigModal: React.FC<PiConfigModalProps> = ({
    open,
    onClose,
}) => {
    const { t } = useTranslation();
    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="sm"
            fullWidth
            slotProps={{
                paper: {
                    sx: {
                        borderRadius: 3,
                    }
                }
            }}
        >
            <DialogTitle sx={{ pb: 1 }}>
                <Typography variant="h6" sx={{
                    fontWeight: 600
                }}>
                    {t('scenarioPage.pi.configTitle')}
                </Typography>
            </DialogTitle>
            <DialogContent sx={{ pt: 1 }}>
                <Stack spacing={2}>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('scenarioPage.pi.modalDescription')}
                    </Typography>
                    <Button
                        component={Link}
                        href={PI_REPO_URL}
                        target="_blank"
                        rel="noopener noreferrer"
                        variant="outlined"
                    >
                        {t('scenarioPage.pi.viewRepo')}
                    </Button>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, pt: 1 }}>
                <Button onClick={onClose} variant="contained">
                    {t('common.done')}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default PiConfigModal;
