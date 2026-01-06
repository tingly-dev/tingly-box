import {Close, Launch, ContentCopy, OpenInNew, CheckCircle} from '@mui/icons-material';
import {
    Box,
    Button,
    Card,
    CardContent,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    Typography,
    Alert,
    CircularProgress,
} from '@mui/material';
import {Claude, Google, Qwen, Gemini} from '@lobehub/icons';
import {useState, useEffect} from 'react';
import api from "@/services/api.ts";

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
        displayName: 'Antigravity',
        description: 'Access Antigravity services via Google OAuth',
        icon: <Google size={32}/>,
        color: '#7B1FA2',
        enabled: false,
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
    onSuccess
}: {
    open: boolean;
    onClose: () => void;
    authData: OAuthAuthorizationData | null;
    onSuccess?: () => void;
}) => {
    const [opened, setOpened] = useState(false);

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
        }
        if (!open) {
            setOpened(false);
        }
    }, [open, authData, opened]);

    const copyUserCode = () => {
        if (authData?.user_code) {
            void navigator.clipboard.writeText(authData.user_code);
        }
    };

    const handleCompleted = () => {
        // Call onSuccess callback to let parent handle refresh logic
        onSuccess?.();
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

    if (!authData) return null;

    const isDeviceCode = authData.flow_type === 'device_code';

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Typography variant="h6">
                        {isDeviceCode ? 'Device Code Authorization' : 'OAuth Authorization'}
                    </Typography>
                    <IconButton onClick={onClose} size="small">
                        <Close/>
                    </IconButton>
                </Stack>
            </DialogTitle>
            <DialogContent>
                <Stack spacing={3}>
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
                            >
                                <Typography variant="h4" sx={{fontFamily: 'monospace', letterSpacing: 2}}>
                                    {authData.user_code || '------'}
                                </Typography>
                                {authData.user_code && (
                                    <IconButton onClick={copyUserCode} size="small">
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
                            <CircularProgress size={20} />
                            <Typography variant="body2" color="text.secondary">
                                {isDeviceCode
                                    ? 'Waiting for you to complete the authorization...'
                                    : 'Waiting for authorization to complete...'}
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
                        >
                            Open Authorization Page Again
                        </Button>
                    )}

                    {/* Completion button - user can click when done */}
                    <Button
                        variant="contained"
                        color="primary"
                        startIcon={<CheckCircle/>}
                        onClick={handleCompleted}
                        fullWidth
                        sx={{mt: 3}}
                    >
                        I've Completed Authorization
                    </Button>
                </Stack>
            </DialogContent>
        </Dialog>
    );
};

const OAuthDialog = ({open, onClose, onSuccess}: OAuthDialogProps) => {
    const [authorizing, setAuthorizing] = useState<string | null>(null);
    const [authDialogOpen, setAuthDialogOpen] = useState(false);
    const [authData, setAuthData] = useState<OAuthAuthorizationData | null>(null);
    const [oauthProviders, setOAuthProviders] = useState<OAuthProvider[]>(FALLBACK_OAUTH_PROVIDERS);

    const handleAuthorizationCompleted = () => {
        // Refresh data, close both dialogs
        onSuccess?.();
        setAuthDialogOpen(false);
        onClose();
    };

    const handleProviderClick = async (provider: OAuthProvider) => {
        if (provider.enabled === false) return;

        setAuthorizing(provider.id);

        try {
            const {oauthApi} = await api.instances()
            const response = await oauthApi.apiV1OauthAuthorizePost(
                {
                    name: "",
                    redirect: "",
                    user_id: "",
                    provider: provider.id,
                    response_type: 'json'
                },
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
                    flow_type: flowType
                });
                setAuthDialogOpen(true);
            }

            console.log('Authorize OAuth for:', provider.id);
        } catch (error) {
            console.error('OAuth authorization failed:', error);
        } finally {
            setAuthorizing(null);
        }
    };

    return (
        <>
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Typography variant="h6">Add OAuth Provider</Typography>
                    <IconButton onClick={onClose} size="small">
                        <Close/>
                    </IconButton>
                </Stack>
            </DialogTitle>
            <DialogContent>
                <Box sx={{mb: 3}}>
                    <Typography variant="body2" color="text.secondary">
                        Select a provider to authorize access via OAuth. You will be redirected to the provider&apos;s
                        authorization page.
                    </Typography>
                </Box>

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
        />
    </>
    );
};

export default OAuthDialog;
