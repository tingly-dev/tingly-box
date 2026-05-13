import { Box, Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography, Stack, Link } from '@mui/material';
import React from 'react';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';

interface ClaudeDesktopConfigModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

const ClaudeDesktopConfigModal: React.FC<ClaudeDesktopConfigModalProps> = ({
    open,
    onClose,
    baseUrl,
    copyToClipboard,
}) => {
    // Get token from context
    const { token } = useScenarioPageModal();

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
                    Configure Claude Desktop
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ pt: 1 }}>
                <Stack spacing={2}>
                    <Box sx={{ bgcolor: 'background.paper', p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>
                            Step 1: Enable Developer Mode
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 0.5 }}>
                            Download Claude Desktop from <Link href="https://claude.com/download" target="_blank" underline="hover">claude.com/download</Link>
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1.5 }}>
                            Launch the app, then enable developer mode:
                        </Typography>
                        <Box sx={{ pl: 2, mb: 0.5, bgcolor: 'background.default', p: 1.5, borderRadius: 1 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                Help → Troubleshooting → Enable Developer Mode
                            </Typography>
                        </Box>
                    </Box>

                    <Box sx={{ bgcolor: 'background.paper', p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>
                            Step 2: Configure Third-Party Inference
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1.5 }}>
                            Go to:
                        </Typography>
                        <Box sx={{ pl: 2, mb: 1.5, bgcolor: 'background.default', p: 1.5, borderRadius: 1 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                Developer → Configure third-party inference
                            </Typography>
                        </Box>
                        <Typography variant="subtitle2" sx={{ mb: 1 }}>
                            In the configuration dialog:
                        </Typography>
                        <Box sx={{ pl: 2, mb: 0.5 }}>
                            <Typography variant="subtitle2">
                                <strong>Connection:</strong> Select "Gateway"
                            </Typography>
                            <Typography variant="subtitle2">
                                <strong>Gateway base URL:</strong>
                            </Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace', mb: 1 }}>
                                {baseUrl}/tingly/claude_desktop
                            </Typography>
                            <Typography variant="subtitle2">
                                <strong>Gateway API key:</strong>
                            </Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                {token.slice(0, 16)}...
                            </Typography>
                        </Box>
                    </Box>

                    <Stack direction="row" spacing={1}>
                        <Button
                            variant="outlined"
                            size="small"
                            onClick={() => copyToClipboard(`${baseUrl}/tingly/claude_desktop`, 'URL')}
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

                    <Box sx={{ bgcolor: 'background.paper', p: 2, borderRadius: 1, border: 1, borderColor: 'divider', mt: 1 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                            Step 3: Configure Models
                        </Typography>
                        <Typography variant="body2" sx={{ mb: 1 }}>
                            Add the following models to display them in the interface:
                        </Typography>
                        <Box sx={{ bgcolor: 'background.default', p: 1.5, borderRadius: 1 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.85rem' }}>
                                claude-sonnet-4-6<br />
                                claude-opus-4-6<br />
                                claude-opus-4-7
                            </Typography>
                        </Box>
                        <Typography variant="body2" sx={{ mt: 1, color: 'text.secondary' }}>
                            You can create multiple configurations and switch between them as needed.
                        </Typography>
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

export default ClaudeDesktopConfigModal;
