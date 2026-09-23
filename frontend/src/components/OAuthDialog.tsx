import {Close, ExpandLess, ExpandMore, Launch} from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    Checkbox,
    CircularProgress,
    Collapse,
    Dialog,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    IconButton,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import {useEffect, useRef, useState} from 'react';
import api from "@/services/api.ts";
import {getOAuthRedirectPath} from "@/utils/protocol";
import {FALLBACK_OAUTH_PROVIDERS, type OAuthProvider} from './oauth/fallbackProviders';
import OAuthAuthorizationDialog, {type OAuthAuthorizationData} from './oauth/OAuthAuthorizationDialog';

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
    // Collapsed by default: the port-forwarding note for fixed-callback-port
    // providers (Claude Code, Codex) is only useful for remote deployments.
    const [showPortForwardHelp, setShowPortForwardHelp] = useState(false);

    // Load saved proxy URL and global proxy setting from localStorage/config on mount
    useEffect(() => {
        const savedProxy = localStorage.getItem('oauth_proxy_url');
        if (savedProxy) {
            setProxyUrl(savedProxy);
        }
        // NOTE: divergent storage key on purpose — the provider form uses
        // `provider_use_global_proxy`; this keeps `oauth_use_global_proxy` so
        // existing users' saved preferences don't break.
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
        setShowPortForwardHelp(false);
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

                                {/* Some providers (Claude Code, Codex) redirect the browser to a fixed
                                    loopback port for the callback. On a remote deployment that port lives
                                    on the server, not the browser's machine, so it needs forwarding — shown
                                    on demand rather than guessed at, since there's no reliable way to tell
                                    local and remote access apart from here. */}
                                {provider?.callbackPort && (
                                    <Box>
                                        <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                                            <Typography variant="caption" sx={{color: 'text.secondary'}}>
                                                Uses local port {provider.callbackPort} for the sign-in callback.
                                            </Typography>
                                            <Button
                                                size="small"
                                                onClick={() => setShowPortForwardHelp((v) => !v)}
                                                endIcon={showPortForwardHelp ? <ExpandLess fontSize="small"/> : <ExpandMore fontSize="small"/>}
                                                sx={{minWidth: 0, py: 0, fontSize: 'caption.fontSize', textTransform: 'none'}}
                                            >
                                                Running this remotely?
                                            </Button>
                                        </Stack>
                                        <Collapse in={showPortForwardHelp} timeout="auto" unmountOnExit>
                                            <Box sx={{mt: 1, p: 1.5, bgcolor: 'action.hover', borderRadius: 1}}>
                                                <Typography variant="body2" sx={{color: 'text.secondary'}}>
                                                    If Tingly-Box runs on a remote server, this callback still
                                                    goes to <b>your</b> machine's port {provider.callbackPort} —
                                                    forward it there before continuing, e.g.:
                                                </Typography>
                                                <Box
                                                    component="code"
                                                    sx={{
                                                        display: 'block',
                                                        mt: 1,
                                                        p: 1,
                                                        bgcolor: 'background.paper',
                                                        borderRadius: 1,
                                                        fontFamily: 'monospace',
                                                        fontSize: '0.8rem',
                                                        overflowX: 'auto',
                                                    }}
                                                >
                                                    ssh -L {provider.callbackPort}:localhost:{provider.callbackPort} user@your-server
                                                </Box>
                                            </Box>
                                        </Collapse>
                                    </Box>
                                )}

                                {initError && (
                                    <Alert severity="error" onClose={handleRetry}>{initError}</Alert>
                                )}

                                {/* Proxy URL: hand-rolled instead of provider-form-dialog/ProxyUrlField —
                                    this one has an auto-detect helperText + success highlight and tighter
                                    spacing, which ProxyUrlField's props don't cover. Uses the
                                    `oauth_use_global_proxy` key, not ProxyUrlField's
                                    `provider_use_global_proxy` (see mount effect above). */}
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
