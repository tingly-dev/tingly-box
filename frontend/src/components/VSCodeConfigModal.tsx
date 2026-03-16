import { Box, Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography, Stack, Link } from '@mui/material';
import React from 'react';

interface VSCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    token: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

const VSCodeConfigModal: React.FC<VSCodeConfigModalProps> = ({
    open,
    onClose,
    baseUrl,
    token,
    copyToClipboard,
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
                    <Box sx={{ bgcolor: 'background.paper', p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5 }}>
                            <strong>1.</strong> Install the <strong>Tingly Box</strong> extension for VS Code
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1.5 }}>
                            <strong>2.</strong> Open <strong>Settings</strong> → search for <strong>Tingly Box</strong>
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1 }}>
                            <strong>3.</strong> Configure:
                        </Typography>
                        <Box sx={{ pl: 2, mb: 0.5 }}>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                Base URL: <strong>{baseUrl}/tingly/vscode</strong>
                            </Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                API Key: <strong>{token.slice(0, 16)}...</strong>
                            </Typography>
                        </Box>
                    </Box>

                    <Stack direction="row" spacing={1}>
                        <Button
                            variant="outlined"
                            size="small"
                            onClick={() => copyToClipboard(`${baseUrl}/tingly/vscode`, 'Base URL')}
                            sx={{ flex: 1 }}
                        >
                            Copy Base URL
                        </Button>
                        <Button
                            variant="outlined"
                            size="small"
                            onClick={() => copyToClipboard(token, 'API Key')}
                            sx={{ flex: 1 }}
                        >
                            Copy API Key
                        </Button>
                    </Stack>

                    <Box sx={{ bgcolor: 'info.main', p: 2, borderRadius: 1 }}>
                        <Typography variant="subtitle2" sx={{ color: 'info.contrastText', fontWeight: 600 }}>
                            Get the Extension
                        </Typography>
                        <Typography variant="body2" sx={{ color: 'info.contrastText', mt: 1 }}>
                            Install the Tingly Box extension from the VS Code Marketplace.
                        </Typography>
                        <Stack direction="row" spacing={2} sx={{ mt: 1 }}>
                            <Link
                                href="https://marketplace.visualstudio.com/items?itemName=Tingly-Dev.vscode-tingly-box"
                                target="_blank"
                                rel="noopener noreferrer"
                                sx={{ color: 'white', textDecoration: 'underline' }}
                            >
                                View on Marketplace
                            </Link>
                            <Link
                                href="vscode:extension/Tingly-Dev.vscode-tingly-box"
                                sx={{ color: 'white', textDecoration: 'underline' }}
                            >
                                Install directly
                            </Link>
                        </Stack>
                    </Box>
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
