import { Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography, Stack, Link } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';

interface VSCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
}

const MARKETPLACE_URL = 'https://marketplace.visualstudio.com/items?itemName=Tingly-Dev.vscode-tingly-box';
const VSCODE_INSTALL_URL = 'vscode:extension/Tingly-Dev.vscode-tingly-box';

const VSCodeConfigModal: React.FC<VSCodeConfigModalProps> = ({
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
                    {t('scenarioPage.vscode.configTitle')}
                </Typography>
            </DialogTitle>
            <DialogContent sx={{ pt: 1 }}>
                <Stack spacing={2}>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('scenarioPage.vscode.modalDescription')}
                    </Typography>

                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                        <Button
                            component={Link}
                            href={VSCODE_INSTALL_URL}
                            variant="contained"
                            sx={{ flex: 1 }}
                        >
                            {t('scenarioPage.vscode.installInVSCode')}
                        </Button>
                        <Button
                            component={Link}
                            href={MARKETPLACE_URL}
                            target="_blank"
                            rel="noopener noreferrer"
                            variant="outlined"
                            sx={{ flex: 1 }}
                        >
                            {t('scenarioPage.vscode.viewMarketplace')}
                        </Button>
                    </Stack>
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

export default VSCodeConfigModal;
