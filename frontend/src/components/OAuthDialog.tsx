import {Close, ContentCopy, Launch, OpenInNew} from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    Checkbox,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    IconButton,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import {Claude, Gemini, Google, Kimi, OpenAI, Qwen} from './BrandIcons';
import {useEffect, useRef, useState} from 'react';
import api from "@/services/api.ts";
import {getOAuthRedirectPath} from "@/utils/protocol";

// Type for timer (browser vs Node.js)
type TimerId = ReturnType<typeof setTimeout>;

export interface OAuthProvider {
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
export const FALLBACK_OAUTH_PROVIDERS: OAuthProvider[] = [
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
        enabled: true,
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
        enabled: false,  // DISABLED: Aliyun WAF blocking OAuth requests
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
        id: 'kimi_code',
        name: 'Kimi Code',
        displayName: 'Kimi Code',
        description: 'Access Kimi Code via device code flow',
        icon: <Kimi size={32}/>,
        color: '#6366F1',
        enabled: true,
        deviceCodeFlow: true,
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
    // When set (and the dialog is open), immediately start the OAuth flow for
    // this provider id, skipping the in-dialog grid. Used by the unified
    // "Connect Provider" picker to route OAuth cards straight into the flow.
    autoStartProviderId?: string | null;
    // When set, the flow re-authenticates this existing provider in place: on
    // success the backend overwrites its credentials on the same UUID instead of
    // creating a new provider, so every rule/service reference stays intact.
    // Pair with autoStartProviderId set to the provider's issuer.
    reauthProviderUuid?: string | null;
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

const OAuthDialog = ({open, onClose, onSuccess, autoStartProviderId, reauthProviderUuid}: OAuthDialogProps) => {
    const isReauth = Boolean(reauthProviderUuid);
    const [authorizing, setAuthorizing] = useState<string | null>(null);
    const [authDialogOpen, setAuthDialogOpen] = useState(false);
    const [authData, setAuthData] = useState<OAuthAuthorizationData | null>(null);
    const [oauthProviders, setOAuthProviders] = useState<OAuthProvider[]>(FALLBACK_OAUTH_PROVIDERS);
    const [initError, setInitError] = useState<string | null>(null);
    const [proxyUrl, setProxyUrl] = useState('');
    const [autoDetectedProxy, setAutoDetectedProxy] = useState('');
    const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);
    const [useGlobalProxy, setUseGlobalProxy] = useState(false);
    const [globalProxyUrl, setGlobalProxyUrl] = useState('');
    // For autoStartProviderId mode: whether user has clicked "Connect & Authorize"
    const [configConfirmed, setConfigConfirmed] = useState(false);
    const [authAttempt, setAuthAttempt] = useState(0);

    // Load saved proxy URL and global proxy setting from localStorage/config on mount
    useEffect(() => {
        const savedProxy = localStorage.getItem('oauth_proxy_url');
        if (savedProxy) {
            setProxyUrl(savedProxy);
        }
        const savedUseGlobal = localStorage.getItem('oauth_use_global_proxy') === 'true';
        setUseGlobalProxy(savedUseGlobal);
        // Fetch global proxy URL from config
        api.getConfig().then((result) => {
            const gp = result?.data?.http_transport?.global_proxy_url ?? '';
            setGlobalProxyUrl(gp);
            if (savedUseGlobal && gp && !savedProxy) {
                setProxyUrl(gp);
            }
        });
    }, []);

    // Save proxy URL to localStorage when it changes
    const handleProxyUrlChange = (value: string) => {
        setProxyUrl(value);
        localStorage.setItem('oauth_proxy_url', value);
    };

    const handleUseGlobalProxyChange = (checked: boolean) => {
        setUseGlobalProxy(checked);
        localStorage.setItem('oauth_use_global_proxy', String(checked));
        if (checked && globalProxyUrl) {
            setProxyUrl(globalProxyUrl);
            localStorage.setItem('oauth_proxy_url', globalProxyUrl);
        } else if (!checked) {
            setProxyUrl('');
            localStorage.setItem('oauth_proxy_url', '');
        }
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
            await api.oauthCancel({ session_id: sessionId });
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
        setConfigConfirmed(false);
        setAuthAttempt(0);
        onClose();
    };

    // Auto-detect proxy URL from existing providers
    const detectProxyFromProviders = async () => {
        try {
            const response = await api.getProviders();
            if (response.success && response.data) {
                const providers = response.data;
                // Find OpenAI-style providers with proxy
                const openaiProvider = providers.find((p: any) =>
                    p.api_style === 'openai' && p.proxy_url
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
            const redirectUri = await getOAuthRedirectPath();
            const response = await api.oauthAuthorize(
                {
                    provider: provider.id,
                    proxy_url: proxyUrl || undefined,
                    redirect: redirectUri,
                    // Re-auth: overwrite this existing provider in place (same UUID).
                    provider_uuid: reauthProviderUuid || undefined,
                } as any,
            );

            if (response?.success) {
                const data = response.data as any;

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
            console.error('[OAuth] Full error object:', error);
            console.error('[OAuth] Error response:', error?.response);
            console.error('[OAuth] Error data:', error?.response?.data);
            const errorMsg = error?.response?.data?.error || error?.response?.data?.message || error?.message || 'Failed to initiate OAuth flow';
            setInitError(`OAuth authorization failed: ${errorMsg}`);
            console.error('OAuth authorization failed:', error);
        } finally {
            setAuthorizing(null);
        }
    };

    // When opened with an autoStartProviderId, kick off that provider's flow
    // only after the user has confirmed config (clicked "Connect & Authorize").
    // Reset on close so the next open re-triggers cleanly.
    const autoStartedRef = useRef<string | null>(null);
    useEffect(() => {
        if (!open) {
            autoStartedRef.current = null;
            setAuthAttempt(0);
            return;
        }
        if (!autoStartProviderId || autoStartedRef.current === `${autoStartProviderId}:${authAttempt}`) return;
        if (!configConfirmed) return;
        const provider = oauthProviders.find((p) => p.id === autoStartProviderId);
        if (provider && provider.enabled !== false) {
            autoStartedRef.current = `${autoStartProviderId}:${authAttempt}`;
            handleProviderClick(provider);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, autoStartProviderId, oauthProviders, configConfirmed, authAttempt]);

    // Direct mode: launched from the unified picker for a single provider.
    // Shows a config screen (proxy settings, etc.) before starting the OAuth
    // flow. Only begins authorization when the user clicks "Connect & Authorize".
    if (autoStartProviderId) {
        const provider = oauthProviders.find((p) => p.id === autoStartProviderId);
        const name = provider?.displayName || provider?.name || 'provider';
        const isConnecting = configConfirmed && !initError;

        const handleRetry = () => {
            setConfigConfirmed(false);
            setInitError(null);
            setAuthAttempt((attempt) => attempt + 1);
            autoStartedRef.current = null;
        };

        return (
            <>
                <Dialog open={open && !authDialogOpen} onClose={handleClose} maxWidth="xs" fullWidth>
                    <DialogTitle>
                        <Stack
                            direction="row"
                            sx={{
                                alignItems: "center",
                                justifyContent: "space-between"
                            }}>
                            <Typography variant="h6">
                                {isReauth ? `Reauthorize ${provider?.name || 'Provider'}` : `Connect ${provider?.name || 'Provider'}`}
                            </Typography>
                            <IconButton onClick={handleClose} size="small"><Close/></IconButton>
                        </Stack>
                    </DialogTitle>
                    <DialogContent>
                        {isConnecting ? (
                            <Stack
                                spacing={2}
                                sx={{
                                    alignItems: "center",
                                    py: 3
                                }}>
                                <CircularProgress size={28}/>
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>
                                    {isReauth ? `Reauthorizing ${name}…` : `Connecting to ${name}…`}
                                </Typography>
                            </Stack>
                        ) : (
                            <Stack spacing={2.5} sx={{py: 1}}>
                                {isReauth && (
                                    <Alert severity="info">
                                        Sign in again to refresh this credential. The provider keeps its
                                        existing name and UUID, so all routing rules and model keys stay intact.
                                    </Alert>
                                )}
                                {/* Provider info */}
                                {provider && (
                                    <Stack direction="row" spacing={2} sx={{
                                        alignItems: "center"
                                    }}>
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
                                                flexShrink: 0,
                                            }}
                                        >
                                            {provider.icon}
                                        </Box>
                                        <Box>
                                            <Typography variant="subtitle1" sx={{
                                                fontWeight: 600
                                            }}>
                                                {provider.displayName}
                                            </Typography>
                                            <Typography variant="body2" sx={{
                                                color: "text.secondary"
                                            }}>
                                                {provider.description}
                                            </Typography>
                                        </Box>
                                    </Stack>
                                )}

                                {initError && (
                                    <Alert severity="error" onClose={handleRetry}>{initError}</Alert>
                                )}

                                {/* Proxy URL */}
                                <TextField
                                    fullWidth
                                    label="HTTP/SOCKS Proxy URL (Optional)"
                                    placeholder="http://127.0.0.1:7890 or socks5://127.0.0.1:7890"
                                    value={proxyUrl}
                                    onChange={(e) => handleProxyUrlChange(e.target.value)}
                                    helperText={
                                        autoDetectedProxy
                                            ? 'Auto-detected from existing provider. You can override if needed.'
                                            : 'Optional: use a proxy to bypass regional restrictions.'
                                    }
                                    size="small"
                                    color={autoDetectedProxy ? 'success' : 'primary'}
                                    focused={autoDetectedProxy ? true : undefined}
                                />
                                <Box sx={{display: 'flex', justifyContent: 'flex-end', mt: -1.5}}>
                                    <FormControlLabel
                                        control={
                                            <Checkbox
                                                size="small"
                                                checked={useGlobalProxy}
                                                disabled={!globalProxyUrl}
                                                onChange={(e) => handleUseGlobalProxyChange(e.target.checked)}
                                            />
                                        }
                                        label={
                                            <Typography variant="body2" color={globalProxyUrl ? 'text.secondary' : 'text.disabled'}>
                                                {globalProxyUrl
                                                    ? `Use quick proxy (${globalProxyUrl})`
                                                    : 'Use quick proxy (not configured)'}
                                            </Typography>
                                        }
                                        labelPlacement="start"
                                    />
                                </Box>

                                <Button
                                    variant="contained"
                                    fullWidth
                                    startIcon={<Launch/>}
                                    disabled={provider?.enabled === false}
                                    onClick={() => {
                                        if (configConfirmed) setAuthAttempt((attempt) => attempt + 1);
                                        else setConfigConfirmed(true);
                                    }}
                                >
                                    {isReauth ? 'Reauthorize' : 'Connect & Authorize'}
                                </Button>
                            </Stack>
                        )}
                    </DialogContent>
                </Dialog>
                <OAuthAuthorizationDialog
                    open={authDialogOpen}
                    onClose={handleClose}
                    authData={authData}
                    onSuccess={handleAuthorizationCompleted}
                    onError={handleAuthorizationError}
                />
            </>
        );
    }

    // No autoStartProviderId: the legacy provider-grid picker has been removed in
    // favor of the unified ConnectProviderDialog. Render nothing; callers are
    // expected to route "add OAuth" through the Connect AI picker, which sets an
    // autoStartProviderId before opening this dialog.
    return null;
};

export default OAuthDialog;
