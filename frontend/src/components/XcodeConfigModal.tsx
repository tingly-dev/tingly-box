import { Box, Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography, Stack } from '@mui/material';
import React from 'react';
import xcodeImage from '../assets/images/xcode.png';

interface XcodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    token: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

const XcodeConfigModal: React.FC<XcodeConfigModalProps> = ({
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
                    Configure Xcode
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ pt: 1 }}>
                <Stack spacing={2}>
                    <Box sx={{ bgcolor: 'background.paper', p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5 }}>
                            <strong>1.</strong> Open <strong>Xcode</strong> → <strong>Settings</strong> → <strong>Intelligence</strong>
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1.5 }}>
                            <strong>2.</strong> Click <strong>Add a Model Provider</strong>, select <strong>Internet-Hosted</strong>
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1 }}>
                            <strong>3.</strong> Enter:
                        </Typography>
                        <Box sx={{ pl: 2, mb: 0.5 }}>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                URL: <strong>{baseUrl}/tingly/xcode</strong>
                            </Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                API Key: <strong>{token.slice(0, 16)}...</strong>
                            </Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                Description: <strong>Tingly Box</strong>
                            </Typography>
                        </Box>
                    </Box>

                    <Stack direction="row" spacing={1}>
                        <Button
                            variant="outlined"
                            size="small"
                            onClick={() => copyToClipboard(`${baseUrl}/tingly/xcode`, 'URL')}
                            sx={{ flex: 1 }}
                        >
                            Copy URL
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
                    <Box
                        component="img"
                        src={xcodeImage}
                        alt="Xcode Intelligence Settings"
                        sx={{
                            width: '100%',
                            borderRadius: 6,
                            border: 1,
                            borderColor: 'divider',
                            boxShadow: 3,
                        }}
                    />
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

export default XcodeConfigModal;
