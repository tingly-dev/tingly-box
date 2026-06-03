import { Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography, Stack, Link } from '@mui/material';
import React from 'react';

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
    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="sm"
            fullWidth
            PaperProps={{
                sx: {
                    borderRadius: 3,
                }
            }}
        >
            <DialogTitle sx={{ pb: 1 }}>
                <Typography variant="h6" fontWeight={600}>
                    Configure VS Code
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ pt: 1 }}>
                <Stack spacing={2}>
                    <Typography variant="body2" color="text.secondary">
                        Install the Tingly Box extension, then follow the setup guide inside VS Code.
                        The extension handles the required endpoint and API key configuration for you.
                    </Typography>

                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                        <Button
                            component={Link}
                            href={VSCODE_INSTALL_URL}
                            variant="contained"
                            sx={{ flex: 1 }}
                        >
                            Install in VS Code
                        </Button>
                        <Button
                            component={Link}
                            href={MARKETPLACE_URL}
                            target="_blank"
                            rel="noopener noreferrer"
                            variant="outlined"
                            sx={{ flex: 1 }}
                        >
                            View Marketplace
                        </Button>
                    </Stack>
                </Stack>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1 }}>
                <Button onClick={onClose} variant="contained">
                    Done
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default VSCodeConfigModal;
