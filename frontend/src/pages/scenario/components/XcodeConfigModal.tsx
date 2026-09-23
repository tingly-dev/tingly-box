import { Box, Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography, Stack } from '@mui/material';
import React from 'react';
import xcodeImage from '@/assets/images/xcode.png';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';
import { CopyUrlKeyButtons } from './config/CopyUrlKeyButtons';
import { InstructionSteps } from './config/InstructionSteps';

interface XcodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

const XcodeConfigModal: React.FC<XcodeConfigModalProps> = ({
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
                    Configure Xcode
                </Typography>
            </DialogTitle>
            <DialogContent sx={{ pt: 1 }}>
                <Stack spacing={2}>
                    <InstructionSteps
                        steps={[
                            <><strong>1.</strong> Open <strong>Xcode</strong> → <strong>Settings</strong> → <strong>Intelligence</strong></>,
                            <><strong>2.</strong> Click <strong>Add a Model Provider</strong>, select <strong>Internet-Hosted</strong></>,
                            <><strong>3.</strong> Enter:</>,
                        ]}
                        values={
                            <>
                                <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                    URL: <strong>{baseUrl}/tingly/xcode</strong>
                                </Typography>
                                <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                    API Key: <strong>{token.slice(0, 16)}...</strong>
                                </Typography>
                                <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                    Description: <strong>Tingly Box</strong>
                                </Typography>
                            </>
                        }
                    />

                    <CopyUrlKeyButtons
                        url={`${baseUrl}/tingly/xcode`}
                        token={token}
                        copyToClipboard={copyToClipboard}
                    />
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
