import {Close, Launch} from '@mui/icons-material';
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
        description: 'Access Qwen Code models via OAuth',
        icon: <Qwen size={32}/>,
        color: '#00A8E1',
        enabled: false,
    },
    {
        id: 'mock',
        name: 'Mock',
        displayName: 'Mock OAuth',
        description: 'Test OAuth flow with mock provider',
        icon: <Box sx={{fontSize: 32}}>🧪</Box>,
        color: '#9E9E9E',
        enabled: true,
    },
    // Add more providers as needed
];

interface OAuthDialogProps {
    open: boolean;
    onClose: () => void;
}

const OAuthDialog = ({open, onClose}: OAuthDialogProps) => {
    const [authorizing, setAuthorizing] = useState<string | null>(null);

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

            // For now, open the auth URL in a new window
            if (response.data.success) {
                window.open(response.data.data.auth_url, '_blank');
            }

            console.log('Authorize OAuth for:', provider.id);

            // Simulate API call - replace with actual implementation
            await new Promise(resolve => setTimeout(resolve, 1000));

            // Close dialog after authorization initiated
            onClose();
        } catch (error) {
            console.error('OAuth authorization failed:', error);
        } finally {
            setAuthorizing(null);
        }
    };

    return (
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
                    {OAUTH_PROVIDERS.map((provider) => {
                        const isDisabled = provider.enabled === false;

                        return (
                            <Box key={provider.id}>
                                <Card
                                    sx={{
                                        height: '100%',
                                        display: 'flex',
                                        flexDirection: 'column',
                                        cursor: isDisabled ? 'not-allowed' : 'pointer',
                                        transition: 'all 0.2s',
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        opacity: isDisabled ? 0.5 : 1,
                                        filter: isDisabled ? 'grayscale(100%)' : 'none',
                                        ...(isDisabled ? {} : {
                                            '&:hover': {
                                                borderColor: provider.color,
                                                boxShadow: 2,
                                            },
                                        }),
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
                                                    opacity: isDisabled ? 0.5 : 1,
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
                                                disabled={isDisabled || authorizing === provider.id}
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
    );
};

export default OAuthDialog;
