import {Close, Launch, ContentCopy, OpenInNew, CheckCircle, Refresh} from '@mui/icons-material';
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
import {useState} from 'react';
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

const OAUTH_PROVIDERS: OAuthProvider[] = [
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

interface DeviceCodeData {
    device_code: string;
    user_code: string;
    verification_uri: string;
    verification_uri_complete?: string;
    expires_in: number;
    interval: number;
    provider: string;
}

interface OAuthDialogProps {
    open: boolean;
    onClose: () => void;
    onProviderAdded?: () => void; // Callback when provider is added
}

// Device Code Flow Dialog
const DeviceCodeDialog = ({open, onClose, deviceCodeData, onProviderAdded}: {
    open: boolean;
    onClose: () => void;
    deviceCodeData: DeviceCodeData | null;
    onProviderAdded?: () => void;
}) => {
    const [status, setStatus] = useState<'pending' | 'verifying' | 'success'>('pending');

    const handleVerificationComplete = async () => {
        setStatus('verifying');
        // Trigger refresh callback
        if (onProviderAdded) {
            await onProviderAdded();
        }
        setStatus('success');
        // Close dialog after showing success state
        setTimeout(() => {
            onClose();
            setStatus('pending');
        }, 2000);
    };

    const copyUserCode = () => {
        if (deviceCodeData) {
            navigator.clipboard.writeText(deviceCodeData.user_code);
        }
    };

    if (!deviceCodeData) return null;

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Typography variant="h6">Device Code Authorization</Typography>
                    <IconButton onClick={onClose} size="small" disabled={status === 'verifying'}>
                        <Close/>
                    </IconButton>
                </Stack>
            </DialogTitle>
            <DialogContent>
                {status === 'success' ? (
                    <Stack spacing={3} alignItems="center" py={4}>
                        <CheckCircle sx={{fontSize: 64, color: 'success.main'}} />
                        <Typography variant="h6" color="success.main">
                            Provider Added Successfully!
                        </Typography>
                        <Typography variant="body2" color="text.secondary" textAlign="center">
                            The {deviceCodeData.provider} provider has been added to your account.
                        </Typography>
                    </Stack>
                ) : (
                    <Stack spacing={3}>
                        <Alert severity="info">
                            Follow these steps to authorize {deviceCodeData.provider}:
                        </Alert>

                        <Box>
                            <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                                Step 1: Visit the authorization page
                            </Typography>
                            <Button
                                variant="outlined"
                                startIcon={<OpenInNew/>}
                                href={deviceCodeData.verification_uri_complete || deviceCodeData.verification_uri}
                                target="_blank"
                                rel="noopener noreferrer"
                                fullWidth
                                disabled={status === 'verifying'}
                            >
                                Open {deviceCodeData.verification_uri_complete ? 'Auto-filled Link' : 'Authorization Page'}
                            </Button>
                        </Box>

                        <Box>
                            <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                                Step 2: Enter this code
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
                                    {deviceCodeData.user_code}
                                </Typography>
                                <IconButton onClick={copyUserCode} size="small">
                                    <ContentCopy/>
                                </IconButton>
                            </Box>
                        </Box>

                        <Box>
                            <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                                Step 3: Complete authorization
                            </Typography>
                            {status === 'verifying' ? (
                                <Box sx={{display: 'flex', alignItems: 'center', gap: 2, py: 1}}>
                                    <CircularProgress size={20} />
                                    <Typography variant="body2" color="text.secondary">
                                        Verifying...
                                    </Typography>
                                </Box>
                            ) : (
                                <Box sx={{display: 'flex', flexDirection: 'column', gap: 2}}>
                                    <Box sx={{display: 'flex', alignItems: 'center', gap: 2}}>
                                        <CircularProgress size={20} />
                                        <Typography variant="body2" color="text.secondary">
                                            Waiting for you to complete the authorization...
                                        </Typography>
                                    </Box>

                                    <Button
                                        variant="contained"
                                        color="primary"
                                        startIcon={<CheckCircle/>}
                                        onClick={handleVerificationComplete}
                                        fullWidth
                                        sx={{mt: 1}}
                                    >
                                        I've Completed Authorization
                                    </Button>
                                </Box>
                            )}
                        </Box>

                        <Alert severity="warning">
                            This code expires in {Math.floor(deviceCodeData.expires_in / 60)} minutes.
                            Click the button above after you complete the authorization.
                        </Alert>
                    </Stack>
                )}
            </DialogContent>
        </Dialog>
    );
};

const OAuthDialog = ({open, onClose, onProviderAdded}: OAuthDialogProps) => {
    const [authorizing, setAuthorizing] = useState<string | null>(null);
    const [deviceCodeData, setDeviceCodeData] = useState<DeviceCodeData | null>(null);
    const [deviceCodeDialogOpen, setDeviceCodeDialogOpen] = useState(false);

    const handleProviderAdded = async () => {
        // Call the parent callback if provided
        if (onProviderAdded) {
            await onProviderAdded();
        }
        // Also refresh the page to ensure provider list is updated
        window.location.reload();
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

                // Check if this is device code flow (has user_code) or standard auth code flow (has auth_url)
                if (data.user_code) {
                    // Device code flow
                    setDeviceCodeData(data);
                    setDeviceCodeDialogOpen(true);
                    setAuthorizing(null);
                    return;
                } else if (data.auth_url) {
                    // Standard auth code flow - open URL in new window
                    window.open(data.auth_url, '_blank');
                }
            }

            console.log('Authorize OAuth for:', provider.id);

            // For standard flow, close dialog after authorization initiated
            if (!provider.deviceCodeFlow) {
                await new Promise(resolve => setTimeout(resolve, 1000));
                onClose();
            }
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
                    {OAUTH_PROVIDERS.filter((provider) => {
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
                {OAUTH_PROVIDERS.length === 0 && (
                    <Box textAlign="center" py={4}>
                        <Typography variant="body2" color="text.secondary">
                            No OAuth providers configured yet.
                        </Typography>
                    </Box>
                )}
            </DialogContent>
        </Dialog>

        {/* Device Code Flow Dialog */}
        <DeviceCodeDialog
            open={deviceCodeDialogOpen}
            onClose={() => setDeviceCodeDialogOpen(false)}
            deviceCodeData={deviceCodeData}
            onProviderAdded={handleProviderAdded}
        />
    </>
    );
};

export default OAuthDialog;
