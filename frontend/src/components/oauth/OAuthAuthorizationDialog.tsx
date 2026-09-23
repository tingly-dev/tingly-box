import {Close, ContentCopy, OpenInNew} from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    Typography,
} from '@mui/material';
import {useEffect, useState} from 'react';
import api from "@/services/api.ts";

// Type for timer (browser vs Node.js)
type TimerId = ReturnType<typeof setTimeout>;

export interface OAuthAuthorizationData {
    auth_url?: string;
    user_code?: string;
    verification_uri?: string;
    verification_uri_complete?: string;
    expires_in?: number;
    interval?: number;
    provider?: string;
    flow_type: 'standard' | 'device_code';
    session_id?: string; // Session ID for status tracking
}

// OAuth Authorization Dialog - unified UI for both standard and device code flow
const OAuthAuthorizationDialog = ({
                                      open,
                                      onClose,
                                      authData,
                                      onSuccess,
                                      onError
                                  }: {
    open: boolean;
    onClose: () => void;
    authData: OAuthAuthorizationData | null;
    onSuccess?: () => void;
    onError?: (error: string) => void;
}) => {
    const [opened, setOpened] = useState(false);
    const [pollCount, setPollCount] = useState(0);
    const [showConfirmDialog, setShowConfirmDialog] = useState(false);
    const [showTimeoutDialog, setShowTimeoutDialog] = useState(false);
    const [errorMessage, setErrorMessage] = useState<string | null>(null);
    const [pollingIntervalId, setPollingIntervalId] = useState<TimerId | null>(null);

    // Cleanup OAuth session when dialog closes without success
    const cleanupOnClose = async () => {
        if (authData?.session_id && !opened) {
            try {
                await api.oauthCancel({ session_id: authData.session_id });
            } catch (error) {
                console.error('[OAuth] Failed to cleanup session:', error);
            }
        }
    };

    // Polling constants
    const POLL_INTERVAL = 2000; // 2 seconds
    const CONFIRM_THRESHOLD = 30; // 1 minute (30 * 2s)
    const MAX_POLL_COUNT = 90; // 3 minutes (90 * 2s)

    // Clean up polling on unmount
    useEffect(() => {
        return () => {
            // Clear any pending polling interval
            if (pollingIntervalId) {
                clearInterval(pollingIntervalId);
            }
        };
    }, [pollingIntervalId]);

    // Auto-open authorization URL when dialog opens
    useEffect(() => {
        if (open && authData && !opened) {
            if (authData.flow_type === 'standard' && authData.auth_url) {
                window.open(authData.auth_url, '_blank');
            } else if (authData.flow_type === 'device_code') {
                const url = authData.verification_uri_complete || authData.verification_uri;
                if (url) {
                    window.open(url, '_blank');
                }
            }
            setOpened(true);
            setPollCount(0);
            setShowConfirmDialog(false);
            setShowTimeoutDialog(false);
            setErrorMessage(null);

            // Start polling
            if (authData.session_id) {
                pollSessionStatus(authData.session_id);
            }
        }
        if (!open) {
            // Cleanup when dialog closes without success
            if (opened && !errorMessage && authData?.session_id) {
                cleanupOnClose();
            }
            setOpened(false);
            setPollCount(0);
            setShowConfirmDialog(false);
            setShowTimeoutDialog(false);
        }
    }, [open, authData, opened]);

    // Polling logic with two-tier timeout
    const pollSessionStatus = async (sessionId: string) => {
        // Dev mode: fast track test sessions
        if (import.meta.env.DEV && sessionId.startsWith('test-')) {
            // Test confirm dialog (triggers after 3 seconds)
            if (sessionId === 'test-confirm') {
                setTimeout(() => {
                    setShowConfirmDialog(true);
                }, 3000);
                return;
            }

            // Test timeout dialog (triggers immediately)
            if (sessionId === 'test-timeout') {
                setTimeout(() => {
                    setShowTimeoutDialog(true);
                }, 500);
                return;
            }

            // Test error state (triggers immediately)
            if (sessionId === 'test-fail') {
                setTimeout(() => {
                    setErrorMessage('Test authorization failed - this is a simulated error');
                    onError?.('Test authorization failed');
                }, 500);
                return;
            }
        }

        let intervalId: TimerId | null = null;
        let currentPollCount = 0;

        const doPoll = async () => {
            currentPollCount++;
            setPollCount(currentPollCount);

            try {
                const response = await api.oauthStatus(sessionId);

                if (response.data.status === 'success') {
                    // Success - stop polling and notify
                    if (intervalId) {
                        clearInterval(intervalId);
                        setPollingIntervalId(null);
                    }
                    onSuccess?.();
                    return;
                } else if (response.data.status === 'failed') {
                    // Failed - stop polling and show error
                    if (intervalId) {
                        clearInterval(intervalId);
                        setPollingIntervalId(null);
                    }
                    const error = response.data.error || 'Authorization failed';
                    setErrorMessage(error);
                    onError?.(error);
                    return;
                } else if (response.data.status === 'pending') {
                    // Still pending - check thresholds
                    if (currentPollCount >= MAX_POLL_COUNT) {
                        // Max timeout reached
                        if (intervalId) {
                            clearInterval(intervalId);
                            setPollingIntervalId(null);
                        }
                        setShowTimeoutDialog(true);
                    } else if (currentPollCount === CONFIRM_THRESHOLD) {
                        // Show confirmation dialog
                        setShowConfirmDialog(true);
                    }
                }
            } catch (error) {
                console.error('Failed to poll OAuth status:', error);
                // Continue polling on transient errors
            }
        };

        // Initial poll
        doPoll();

        // Set up interval
        intervalId = setInterval(doPoll, POLL_INTERVAL);
        setPollingIntervalId(intervalId);
    };

    const copyUserCode = () => {
        if (authData?.user_code) {
            void navigator.clipboard.writeText(authData.user_code);
        }
    };

    const handleCompleted = () => {
        // User confirms completion - let polling continue to verify
        setShowConfirmDialog(false);
    };

    const handleOpenAuthPage = () => {
        if (authData?.flow_type === 'standard' && authData.auth_url) {
            window.open(authData.auth_url, '_blank');
        } else if (authData?.flow_type === 'device_code') {
            const url = authData.verification_uri_complete || authData.verification_uri;
            if (url) {
                window.open(url, '_blank');
            }
        }
    };

    // Calculate remaining time
    const getRemainingTime = () => {
        const remaining = (MAX_POLL_COUNT - pollCount) * POLL_INTERVAL / 1000;
        if (remaining < 60) {
            return `${Math.ceil(remaining)} seconds`;
        }
        return `${Math.ceil(remaining / 60)} minutes`;
    };

    if (!authData) return null;

    const isDeviceCode = authData.flow_type === 'device_code';

    // Handle dialog close - cleanup before closing
    const handleClose = () => {
        // Stop polling
        if (pollingIntervalId) {
            clearInterval(pollingIntervalId);
            setPollingIntervalId(null);
        }
        // Cleanup OAuth session
        cleanupOnClose();
        // Call parent onClose
        onClose();
    };

    return (
        <>
            <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth aria-labelledby="oauth-auth-title">
                <DialogTitle id="oauth-auth-title">
                    <Stack
                        direction="row"
                        sx={{
                            alignItems: "center",
                            justifyContent: "space-between"
                        }}>
                        <Typography variant="h6">
                            {isDeviceCode ? 'Device Code Authorization' : 'OAuth Authorization'}
                        </Typography>
                        <IconButton onClick={handleClose} size="small" aria-label="Close dialog">
                            <Close/>
                        </IconButton>
                    </Stack>
                </DialogTitle>
                <DialogContent>
                    <Stack spacing={3}>
                        {/* Error message */}
                        {errorMessage && (
                            <Alert severity="error" aria-live="polite">
                                Authorization failed: {errorMessage}
                            </Alert>
                        )}

                        <Alert severity="info">
                            {isDeviceCode
                                ? `Follow these steps to authorize ${authData.provider}:`
                                : `Complete the authorization in the opened window for ${authData.provider}.`
                            }
                        </Alert>

                        {isDeviceCode && authData.user_code && (
                            <Box>
                                <Typography variant="subtitle2" gutterBottom sx={{
                                    color: "text.secondary"
                                }}>
                                    Step 1: Visit the authorization page
                                </Typography>
                                <Button
                                    variant="outlined"
                                    startIcon={<OpenInNew/>}
                                    onClick={handleOpenAuthPage}
                                    fullWidth
                                    aria-label="Open authorization page in new tab"
                                >
                                    Open Authorization Page
                                </Button>
                            </Box>
                        )}

                        {isDeviceCode && (
                            <Box>
                                <Typography variant="subtitle2" gutterBottom sx={{
                                    color: "text.secondary"
                                }}>
                                    Step {authData.user_code ? '2: Enter this code' : '1: Enter the code'}
                                </Typography>
                                <Box
                                    sx={{
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                        gap: 2,
                                        p: 2,
                                        bgcolor: 'action.hover',
                                        borderRadius: 1,
                                        border: '2px dashed',
                                        borderColor: 'primary.main',
                                    }}
                                    role="region"
                                    aria-label="User code for device authorization"
                                >
                                    <Typography variant="h4" sx={{fontFamily: 'monospace', letterSpacing: 2}} aria-label={`User code is ${authData.user_code || '------'}`}>
                                        {authData.user_code || '------'}
                                    </Typography>
                                    {authData.user_code && (
                                        <IconButton onClick={copyUserCode} size="small" aria-label="Copy user code to clipboard">
                                            <ContentCopy/>
                                        </IconButton>
                                    )}
                                </Box>
                            </Box>
                        )}

                        <Box>
                            <Typography variant="subtitle2" gutterBottom sx={{
                                color: "text.secondary"
                            }}>
                                {isDeviceCode
                                    ? `Step ${authData.user_code ? '3' : '2'}: Complete authorization`
                                    : 'Step 1: Complete authorization'}
                            </Typography>
                            <Box sx={{display: 'flex', alignItems: 'center', gap: 2}}>
                                <CircularProgress size={20} aria-label="Checking authorization status"/>
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>
                                    {isDeviceCode
                                        ? 'Waiting for you to complete the authorization...'
                                        : 'Waiting for authorization to complete...'}
                                </Typography>
                                <Typography
                                    variant="caption"
                                    sx={{
                                        color: "text.secondary",
                                        ml: 'auto'
                                    }}>
                                    {getRemainingTime()} remaining
                                </Typography>
                            </Box>
                        </Box>

                        {authData.expires_in && (
                            <Alert severity="warning">
                                {isDeviceCode
                                    ? `This code expires in ${Math.floor(authData.expires_in / 60)} minutes.`
                                    : 'Please complete the authorization promptly.'}
                                {isDeviceCode && ' Once authorized, the provider will be automatically added.'}
                            </Alert>
                        )}

                        {!isDeviceCode && (
                            <Button
                                variant="outlined"
                                startIcon={<OpenInNew/>}
                                onClick={handleOpenAuthPage}
                                fullWidth
                                aria-label="Open authorization page again in new tab"
                            >
                                Open Authorization Page Again
                            </Button>
                        )}
                    </Stack>
                </DialogContent>
            </Dialog>
            {/* Confirmation Dialog */}
            <Dialog open={showConfirmDialog} onClose={() => setShowConfirmDialog(false)} maxWidth="sm" fullWidth aria-labelledby="oauth-confirm-title">
                <DialogTitle id="oauth-confirm-title">Still Waiting for Authorization</DialogTitle>
                <DialogContent>
                    <Stack spacing={2}>
                        <Alert severity="info">
                            We've been waiting for about a minute. Have you completed the authorization?
                        </Alert>
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>
                            If you've already completed the authorization in the other window, click "Yes, I'm done" below.
                            The system will continue to verify the authorization status.
                        </Typography>
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>
                            If you haven't completed it yet, you can continue. The system will keep checking for up to 3 minutes.
                        </Typography>
                        <Stack direction="row" spacing={2} sx={{mt: 2}}>
                            <Button
                                variant="contained"
                                onClick={handleCompleted}
                                fullWidth
                                aria-label="Yes, I have completed the authorization"
                            >
                                Yes, I'm Done
                            </Button>
                            <Button
                                variant="outlined"
                                onClick={() => setShowConfirmDialog(false)}
                                fullWidth
                                aria-label="Continue waiting for authorization"
                            >
                                Still Working on It
                            </Button>
                        </Stack>
                    </Stack>
                </DialogContent>
            </Dialog>
            {/* Timeout Dialog */}
            <Dialog open={showTimeoutDialog} onClose={onClose} maxWidth="sm" fullWidth aria-labelledby="oauth-timeout-title">
                <DialogTitle id="oauth-timeout-title">Authorization Timeout</DialogTitle>
                <DialogContent>
                    <Stack spacing={2}>
                        <Alert severity="warning">
                            Authorization check has timed out after 3 minutes.
                        </Alert>
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>
                            The system couldn't confirm that the authorization was completed. This could mean:
                        </Typography>
                        <ul style={{margin: 0, paddingLeft: '1.5rem'}}>
                            <li>The authorization window was closed without completing</li>
                            <li>There was a delay in the authorization process</li>
                            <li>The authorization was denied</li>
                        </ul>
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>
                            If you did complete the authorization successfully, the provider may have been added.
                            Please check your provider list and try again if needed.
                        </Typography>
                        <Button
                            variant="contained"
                            onClick={onClose}
                            fullWidth
                            sx={{mt: 2}}
                            aria-label="Close authorization dialog"
                        >
                            Close
                        </Button>
                    </Stack>
                </DialogContent>
            </Dialog>
        </>
    );
};

export default OAuthAuthorizationDialog;
