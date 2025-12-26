import { Close, Launch } from '@mui/icons-material';
import {
    Box,
    Button,
    Card,
    CardContent,
    Dialog,
    DialogContent,
    DialogTitle,
    Grid,
    IconButton,
    Stack,
    Typography,
} from '@mui/material';
import { Anthropic, Google } from '@lobehub/icons';
import { useState } from 'react';
import api from "@/services/api.ts";

interface OAuthProvider {
    id: string;
    name: string;
    displayName: string;
    description: string;
    icon: React.ReactNode;
    color: string;
}

const OAUTH_PROVIDERS: OAuthProvider[] = [
    {
        id: 'mock',
        name: 'Mock',
        displayName: 'Mock OAuth Provider',
        description: 'Test OAuth flow with mock provider',
        icon: <Box sx={{ fontSize: 32 }}>🧪</Box>,
        color: '#9E9E9E',
    },
    {
        id: 'anthropic',
        name: 'Anthropic',
        displayName: 'Anthropic Claude',
        description: 'Access Claude models via OAuth',
        icon: <Anthropic size={32} />,
        color: '#D97757',
    },
    {
        id: 'google',
        name: 'Google',
        displayName: 'Google AI Studio',
        description: 'Access Gemini models via OAuth',
        icon: <Google size={32} />,
        color: '#4285F4',
    },
    // Add more providers as needed
];

interface OAuthDialogProps {
    open: boolean;
    onClose: () => void;
}

const OAuthDialog = ({ open, onClose }: OAuthDialogProps) => {
    const [authorizing, setAuthorizing] = useState<string | null>(null);

    const handleProviderClick = async (provider: OAuthProvider) => {
        setAuthorizing(provider.id);

        try {
            const { oauthApi } = await api.instances()
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
                        <Close />
                    </IconButton>
                </Stack>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mb: 3 }}>
                    <Typography variant="body2" color="text.secondary">
                        Select a provider to authorize access via OAuth. You will be redirected to the provider&apos;s authorization page.
                    </Typography>
                </Box>

                <Grid container spacing={2}>
                    {OAUTH_PROVIDERS.map((provider) => (
                        <Grid item xs={12} sm={6} key={provider.id}>
                            <Card
                                sx={{
                                    height: '100%',
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
                                <CardContent>
                                    <Stack spacing={2}>
                                        <Stack direction="row" alignItems="center" spacing={2}>
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
                                            <Box sx={{ flex: 1 }}>
                                                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                                    {provider.displayName}
                                                </Typography>
                                                <Typography variant="caption" color="text.secondary">
                                                    {provider.name}
                                                </Typography>
                                            </Box>
                                        </Stack>

                                        <Typography variant="body2" color="text.secondary">
                                            {provider.description}
                                        </Typography>

                                        <Button
                                            variant="outlined"
                                            size="small"
                                            startIcon={<Launch />}
                                            disabled={authorizing === provider.id}
                                            fullWidth
                                        >
                                            {authorizing === provider.id ? 'Authorizing...' : 'Authorize'}
                                        </Button>
                                    </Stack>
                                </CardContent>
                            </Card>
                        </Grid>
                    ))}
                </Grid>

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
