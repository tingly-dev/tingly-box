import {Close, ContentCopy, Launch, OpenInNew} from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import {Claude, Gemini, Google, OpenAI, Qwen} from '@lobehub/icons';
import {useEffect, useState} from 'react';
import api from "@/services/api.ts";
import {getOAuthRedirectPath} from "@/utils/protocol";

interface OAuthProvider {
    id: string;
    name: string;
    displayName: string;
    description: string;
    icon: React.ReactNode;
    color: string;
    enabled?: boolean;
    dev?: boolean;
    deviceCodeFlow?: boolean;
}

// Fallback hardcoded providers for development or when API is unavailable
const FALLBACK_OAUTH_PROVIDERS: OAuthProvider[] = [
    {
        id: 'claude_code',
        name: 'Claude Code',
        displayName: 'Anthropic Claude Code',
        description: 'Access Claude Code models via OAuth',
        icon: <Claude size={32}/>,
        color: '#D97757',
        enabled: true,
    },
    {
        id: 'gemini',
        name: 'Google Gemini CLI',
        displayName: 'Google Gemini CLI',
        description: 'Access Gemini CLI models via OAuth',
        icon: <Gemini size={32}/>,
        color: '#4285F4',
        enabled: false,
    },
    {
        id: 'antigravity',
        name: 'Antigravity',
        displayName: 'Antigravity (Experimental)',
        description: 'Access Antigravity services via Google OAuth',
        icon: <Google size={32}/>,
        color: '#7B1FA2',
        enabled: true,
    },
    {
        id: 'qwen_code',
        name: 'Qwen Code',
        displayName: 'Qwen Code',
        description: 'Access Qwen Code via device code flow',
        icon: <Qwen size={32}/>,
        color: '#00A8E1',
        enabled: true,
        deviceCodeFlow: true,
    },
    {
        id: 'codex',
        name: 'Codex',
        displayName: 'OpenAI Codex',
        description: 'Access OpenAI Codex via OAuth',
        icon: <OpenAI size={32}/>,
        color: '#10A37F',
        enabled: true,
    },
    {
        id: 'mock',
        name: 'Mock',
        displayName: 'Mock OAuth',
        description: 'Test OAuth flow with mock provider',
        icon: <Box sx={{fontSize: 32}}>🧪</Box>,
        color: '#9E9E9E',
        enabled: true,
        dev: true,
    },
    // Add more providers as needed
];

interface OAuthAuthorizationData {
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

interface OAuthDialogProps {
    open: boolean;
    onClose: () => void;
    onSuccess?: () => void;
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
    const [pollingIntervalId, setPollingIntervalId] = useState<NodeJS.Timeout | null>(null);

    // Cleanup OAuth session when dialog closes without success
    const cleanupOnClose = async () => {
        if (authData?.session_id && !opened) {
            try {
                const {oauthApi} = await api.instances();
                await oauthApi.apiV1OauthCancelPost({session_id: authData.session_id});
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

        let intervalId: NodeJS.Timeout | null = null;
        let currentPollCount = 0;

        const doPoll = async () => {
            currentPollCount++;
            setPollCount(currentPollCount);

            try {
                const {oauthApi} = await api.instances();
                const response = await oauthApi.apiV1OauthStatusGet(sessionId);

                if (response.data.data.status === 'success') {
                    // Success - stop polling and notify
                    if (intervalId) {
                        clearInterval(intervalId);
                        setPollingIntervalId(null);
                    }
                    onSuccess?.();
                    return;
                } else if (response.data.data.status === 'failed') {
                    // Failed - stop polling and show error
                    if (intervalId) {
                        clearInterval(intervalId);
                        setPollingIntervalId(null);
                    }
                    const error = response.data.data.error || 'Authorization failed';
                    setErrorMessage(error);
                    onError?.(error);
                    return;
                } else if (response.data.data.status === 'pending') {
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
                <Stack direction="row" alignItems="center" justifyContent="space-between">
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
                            <Typography variant="subtitle2" color="text.secondary" gutterBottom>
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
                            <Typography variant="subtitle2" color="text.secondary" gutterBottom>
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
                        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                            {isDeviceCode
                                ? `Step ${authData.user_code ? '3' : '2'}: Complete authorization`
                                : 'Step 1: Complete authorization'}
                        </Typography>
                        <Box sx={{display: 'flex', alignItems: 'center', gap: 2}}>
                            <CircularProgress size={20} aria-label="Checking authorization status"/>
                            <Typography variant="body2" color="text.secondary">
                                {isDeviceCode
                                    ? 'Waiting for you to complete the authorization...'
                                    : 'Waiting for authorization to complete...'}
                            </Typography>
                            <Typography variant="caption" color="text.secondary" sx={{ml: 'auto'}}>
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
                    <Typography variant="body2" color="text.secondary">
                        If you've already completed the authorization in the other window, click "Yes, I'm done" below.
                        The system will continue to verify the authorization status.
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
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
                    <Typography variant="body2" color="text.secondary">
                        The system couldn't confirm that the authorization was completed. This could mean:
                    </Typography>
                    <ul style={{margin: 0, paddingLeft: '1.5rem'}}>
                        <li>The authorization window was closed without completing</li>
                        <li>There was a delay in the authorization process</li>
                        <li>The authorization was denied</li>
                    </ul>
                    <Typography variant="body2" color="text.secondary">
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

const OAuthDialog = ({open, onClose, onSuccess}: OAuthDialogProps) => {
    const [authorizing, setAuthorizing] = useState<string | null>(null);
    const [authDialogOpen, setAuthDialogOpen] = useState(false);
    const [authData, setAuthData] = useState<OAuthAuthorizationData | null>(null);
    const [oauthProviders, setOAuthProviders] = useState<OAuthProvider[]>(FALLBACK_OAUTH_PROVIDERS);
    const [initError, setInitError] = useState<string | null>(null);
    const [proxyUrl, setProxyUrl] = useState('');
    const [autoDetectedProxy, setAutoDetectedProxy] = useState('');
    const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);

    // Load saved proxy URL from localStorage on mount
    useEffect(() => {
        const savedProxy = localStorage.getItem('oauth_proxy_url');
        if (savedProxy) {
            setProxyUrl(savedProxy);
        }
    }, []);

    // Save proxy URL to localStorage when it changes
    const handleProxyUrlChange = (value: string) => {
        setProxyUrl(value);
        localStorage.setItem('oauth_proxy_url', value);
    };

    // Fetch existing providers to detect proxy URLs when dialog opens
    useEffect(() => {
        if (open) {
            setInitError(null);
            // Try to auto-detect proxy from existing providers
            detectProxyFromProviders();
        }
    }, [open]);

    // Cleanup callback server when dialog closes
    useEffect(() => {
        return () => {
            // When dialog unmounts or closes, cleanup callback server if there's an active session
            if (currentSessionId) {
                cleanupOAuthSession(currentSessionId);
                setCurrentSessionId(null);
            }
        };
    }, [currentSessionId]);

    // Cleanup OAuth session and callback server
    const cleanupOAuthSession = async (sessionId: string) => {
        try {
            const {oauthApi} = await api.instances();
            await oauthApi.apiV1OauthCancelPost({session_id: sessionId});
        } catch (error) {
            console.error('[OAuth] Failed to cleanup session:', error);
        }
    };

    const handleClose = () => {
        // Cleanup callback server before closing
        if (currentSessionId) {
            cleanupOAuthSession(currentSessionId);
            setCurrentSessionId(null);
        }
        setAuthDialogOpen(false);
        setAuthData(null);
        onClose();
    };

    // Auto-detect proxy URL from existing providers
    const detectProxyFromProviders = async () => {
        try {
            const {providersApi} = await api.instances();
            const response = await providersApi.apiV1ProvidersGet();
            if (response.data.success && response.data.data) {
                const providers = response.data.data;
                // Find OpenAI providers with proxy
                const openaiProvider = providers.find((p: any) =>
                    p.provider_base === 'openai' && p.proxy_url
                );
                if (openaiProvider?.proxy_url) {
                    setAutoDetectedProxy(openaiProvider.proxy_url);
                    setProxyUrl(openaiProvider.proxy_url); // Pre-fill the input
                } else {
                    setAutoDetectedProxy('');
                }
            }
        } catch (error) {
            console.error('Failed to fetch providers:', error);
        }
    };

    const handleAuthorizationCompleted = () => {
        // Clear session ID on success (callback server already stopped by backend)
        setCurrentSessionId(null);
        // Refresh data, close both dialogs
        onSuccess?.();
        setAuthDialogOpen(false);
        onClose();
    };

    const handleAuthorizationError = (error: string) => {
        // Keep dialog open to show error
        console.error('OAuth authorization failed:', error);
    };

    const handleProviderClick = async (provider: OAuthProvider) => {
        if (provider.enabled === false) return;

        setAuthorizing(provider.id);
        setInitError(null); // Clear any previous errors

        try {
            const {oauthApi} = await api.instances()
            const redirectUri = await getOAuthRedirectPath();
            const response = await oauthApi.apiV1OauthAuthorizePost(
                {
                    name: "",
                    redirect: redirectUri,
                    user_id: "",
                    provider: provider.id,
                    response_type: 'json',
                    proxy_url: proxyUrl || undefined
                } as any,
            );

            if (response.data.success) {
                const data = response.data.data as any;

                // Determine flow type and set auth data
                let flowType: 'standard' | 'device_code' = 'standard';

                if (data.user_code) {
                    flowType = 'device_code';
                }

                setAuthData({
                    auth_url: data.auth_url,
                    user_code: data.user_code,
                    verification_uri: data.verification_uri,
                    verification_uri_complete: data.verification_uri_complete,
                    expires_in: data.expires_in,
                    interval: data.interval,
                    provider: provider.name,
                    flow_type: flowType,
                    session_id: data.session_id, // Session ID for status tracking
                });
                setCurrentSessionId(data.session_id || null); // Store for cleanup
                setAuthDialogOpen(true);
            } else {
                // Handle API error response
                const errorMsg = response.data?.error || response.data?.message || 'Unknown error';
                setInitError(`OAuth authorization failed: ${errorMsg}`);
                console.error('OAuth authorization failed:', errorMsg);
            }

        } catch (error: any) {
            // Handle network or other errors
            const errorMsg = error?.response?.data?.error || error?.response?.data?.message || error?.message || 'Failed to initiate OAuth flow';
            setInitError(`OAuth authorization failed: ${errorMsg}`);
            console.error('OAuth authorization failed:', error);
        } finally {
            setAuthorizing(null);
        }
    };

    return (
        <>
            <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
                <DialogTitle>
                    <Stack direction="row" alignItems="center" justifyContent="space-between">
                        <Typography variant="h6">Add OAuth Provider</Typography>
                        <IconButton onClick={handleClose} size="small">
                            <Close/>
                        </IconButton>
                    </Stack>
                </DialogTitle>
                <DialogContent>
                    <Box sx={{mb: 3}}>
                        <Typography variant="body2" color="text.secondary">
                            Select a provider to authorize access via OAuth. You will be redirected to the
                            provider&apos;s
                            authorization page.
                        </Typography>
                    </Box>

                    {/* Error alert */}
                    {initError && (
                        <Alert severity="error" sx={{mb: 3}} onClose={() => setInitError(null)}>
                            {initError}
                        </Alert>
                    )}

                    {/* Proxy URL input */}
                    <Box sx={{mb: 3}}>
                        <TextField
                            fullWidth
                            label="HTTP/SOCKS Proxy URL (Optional)"
                            placeholder="http://127.0.0.1:7890 or socks5://127.0.0.1:7890"
                            value={proxyUrl}
                            onChange={(e) => handleProxyUrlChange(e.target.value)}
                            helperText={
                                autoDetectedProxy
                                    ? `Auto-detected from existing OpenAI provider. You can override it if needed.`
                                    : "Optional: Use a proxy to bypass region restrictions (e.g., for OpenAI Codex). Saved for future use."
                            }
                            size="small"
                            color={autoDetectedProxy ? "success" : "primary"}
                            focused={autoDetectedProxy ? true : undefined}
                        />
                        {autoDetectedProxy && (
                            <Alert severity="success" sx={{mt: 1}} icon={<Launch fontSize="small"/>}>
                                <Typography variant="caption">
                                    Proxy auto-detected from your OpenAI provider configuration. OAuth requests will use this proxy.
                                </Typography>
                            </Alert>
                        )}
                    </Box>

                    {/* Dev Mode Debug Buttons */}
                    {import.meta.env.DEV && (
                        <Box sx={{mb: 3}}>
                            <Alert severity="info" sx={{mb: 2}}>
                                <Typography variant="caption" color="text.secondary">
                                    Dev Mode: Quick test OAuth authorization flows
                                </Typography>
                            </Alert>
                            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                                {/* Quick provider tests */}
                                <Button
                                    variant="outlined"
                                    size="small"
                                    onClick={() => handleProviderClick(oauthProviders[0])}
                                    disabled={!oauthProviders[0]?.enabled}
                                >
                                    Test {oauthProviders[0]?.displayName || 'Claude'}
                                </Button>
                                <Button
                                    variant="outlined"
                                    size="small"
                                    onClick={() => handleProviderClick(oauthProviders[3])}
                                    disabled={!oauthProviders[3]?.enabled}
                                >
                                    Test {oauthProviders[3]?.displayName || 'Qwen'}
                                </Button>

                                {/* Mock UI tests */}
                                <Button
                                    variant="outlined"
                                    size="small"
                                    color="info"
                                    onClick={() => {
                                        setAuthData({
                                            flow_type: 'standard',
                                            auth_url: 'https://example.com/oauth',
                                            provider: 'Test Standard',
                                            session_id: 'test-confirm',
                                        });
                                        setAuthDialogOpen(true);
                                    }}
                                >
                                    Test Confirm Dialog (3s)
                                </Button>
                                <Button
                                    variant="outlined"
                                    size="small"
                                    color="warning"
                                    onClick={() => {
                                        setAuthData({
                                            flow_type: 'device_code',
                                            user_code: 'TEST-1234',
                                            verification_uri: 'https://example.com/verify',
                                            expires_in: 600,
                                            interval: 5,
                                            provider: 'Test Device',
                                            session_id: 'test-timeout',
                                        });
                                        setAuthDialogOpen(true);
                                    }}
                                >
                                    Test Timeout (0.5s)
                                </Button>
                                <Button
                                    variant="outlined"
                                    size="small"
                                    color="error"
                                    onClick={() => {
                                        setAuthData({
                                            flow_type: 'standard',
                                            auth_url: '',
                                            provider: 'Test Error',
                                            session_id: 'test-fail',
                                        });
                                        setAuthDialogOpen(true);
                                    }}
                                >
                                    Test Error State (0.5s)
                                </Button>
                            </Stack>
                        </Box>
                    )}

                    <Box
                        sx={{
                            display: 'grid',
                            gridTemplateColumns: {
                                xs: '1fr',
                                sm: 'repeat(2, 1fr)',
                                md: 'repeat(3, 1fr)',
                            },
                            gap: 2,
                        }}
                    >
                        {oauthProviders.filter((provider) => {
                            if (provider.enabled === false) return false;
                            if (provider.dev && !import.meta.env.DEV) return false;
                            return true;
                        }).map((provider) => {
                            return (
                                <Box key={provider.id}>
                                    <Card
                                        sx={{
                                            height: '100%',
                                            display: 'flex',
                                            flexDirection: 'column',
                                            cursor: 'pointer',
                                            transition: 'all 0.2s',
                                            border: '1px solid',
                                            borderColor: 'divider',
                                            '&:hover': {
                                                borderColor: provider.color,
                                                boxShadow: 2,
                                            },
                                        }}
                                        onClick={() => handleProviderClick(provider)}
                                    >
                                        <CardContent sx={{flex: 1, display: 'flex', flexDirection: 'column'}}>
                                            <Stack direction="row" alignItems="center" spacing={2} sx={{mb: 2}}>
                                                <Box
                                                    sx={{
                                                        fontSize: 32,
                                                        width: 48,
                                                        height: 48,
                                                        display: 'flex',
                                                        alignItems: 'center',
                                                        justifyContent: 'center',
                                                        bgcolor: `${provider.color}15`,
                                                        borderRadius: 2,
                                                    }}
                                                >
                                                    {provider.icon}
                                                </Box>
                                                <Box sx={{flex: 1}}>
                                                    <Typography variant="subtitle1" sx={{fontWeight: 600}}>
                                                        {provider.displayName}
                                                    </Typography>
                                                    <Typography variant="caption" color="text.secondary">
                                                        {provider.name}
                                                    </Typography>
                                                </Box>
                                            </Stack>

                                            <Typography variant="body2" color="text.secondary" sx={{mb: 2}}>
                                                {provider.description}
                                            </Typography>

                                            <Box sx={{mt: 'auto'}}>
                                                <Button
                                                    variant="outlined"
                                                    size="small"
                                                    startIcon={<Launch/>}
                                                    disabled={authorizing === provider.id}
                                                    fullWidth
                                                >
                                                    {authorizing === provider.id ? 'Authorizing...' : 'Authorize'}
                                                </Button>
                                            </Box>
                                        </CardContent>
                                    </Card>
                                </Box>
                            );
                        })}
                    </Box>

                    {/* Empty state for future providers */}
                    {oauthProviders.filter((provider) => provider.enabled !== false && (!provider.dev || import.meta.env.DEV)).length === 0 && (
                        <Box textAlign="center" py={4}>
                            <Typography variant="body2" color="text.secondary">
                                No OAuth providers configured yet.
                            </Typography>
                        </Box>
                    )}
                </DialogContent>
            </Dialog>

            {/* OAuth Authorization Dialog */}
            <OAuthAuthorizationDialog
                open={authDialogOpen}
                onClose={() => setAuthDialogOpen(false)}
                authData={authData}
                onSuccess={handleAuthorizationCompleted}
                onError={handleAuthorizationError}
            />
        </>
    );
};

export default OAuthDialog;
