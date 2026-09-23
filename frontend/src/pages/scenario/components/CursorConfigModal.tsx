import { Alert, Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography, Stack } from '@mui/material';
import React from 'react';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';
import { CopyUrlKeyButtons } from './config/CopyUrlKeyButtons';
import { InstructionSteps } from './config/InstructionSteps';

interface CursorConfigModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

// Cursor calls "Override OpenAI Base URL" from its own cloud backend
// (api2.cursor.sh), never from the local Cursor app — so a localhost or
// private-network URL is unreachable from there and the chat just hangs
// with no local traffic to debug. See .design/cursor.md.
const looksUnreachableFromCursorCloud = (url: string): boolean => {
    try {
        const { hostname, protocol } = new URL(url);
        if (protocol !== 'https:') return true;
        if (hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '::1') return true;
        if (/^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(hostname)) return true;
        return false;
    } catch {
        return true;
    }
};

const CursorConfigModal: React.FC<CursorConfigModalProps> = ({
    open,
    onClose,
    baseUrl,
    copyToClipboard,
}) => {
    // Get token from context
    const { token } = useScenarioPageModal();
    const unreachable = looksUnreachableFromCursorCloud(baseUrl);
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
                    Configure Cursor
                </Typography>
            </DialogTitle>
            <DialogContent sx={{ pt: 1 }}>
                <Stack spacing={2}>
                    <Alert severity={unreachable ? 'warning' : 'info'} variant="outlined">
                        Cursor calls this Base URL from <strong>its own cloud servers</strong>, not from
                        the Cursor app on your machine — so it must be reachable over the public
                        internet via <strong>HTTPS</strong>.{' '}
                        {unreachable ? (
                            <>This URL looks local or private, so it <strong>won't work as-is</strong> —
                                expose this server publicly first (e.g. a Cloudflare Tunnel or ngrok), or
                                use a publicly deployed Tingly Box instance.</>
                        ) : (
                            <>Double-check this address is actually reachable from the internet before
                                pasting it into Cursor.</>
                        )}
                    </Alert>
                    <InstructionSteps
                        steps={[
                            <><strong>1.</strong> Open <strong>Cursor</strong> → <strong>Settings</strong> → <strong>Models</strong></>,
                            <><strong>2.</strong> Under <strong>OpenAI API Key</strong>, enable <strong>Override OpenAI Base URL</strong></>,
                            <><strong>3.</strong> Enter:</>,
                        ]}
                        values={
                            <>
                                <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                    Base URL: <strong>{baseUrl}/tingly/cursor</strong>
                                </Typography>
                                <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                    API Key: <strong>{token.slice(0, 16)}...</strong>
                                </Typography>
                            </>
                        }
                        trailingStep={
                            <><strong>4.</strong> Click <strong>Verify</strong> to save</>
                        }
                    />

                    <CopyUrlKeyButtons
                        url={`${baseUrl}/tingly/cursor`}
                        token={token}
                        copyToClipboard={copyToClipboard}
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

export default CursorConfigModal;
