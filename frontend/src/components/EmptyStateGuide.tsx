import { Add, VpnKey } from '@mui/icons-material';
import { Box, Button, Stack, Typography } from '@mui/material';

interface EmptyStateGuideProps {
    title?: string;
    description?: string;
    showOAuthButton?: boolean;
    showHeroIcon?: boolean;
    primaryButtonLabel?: string;
    secondaryButtonLabel?: string;
    onAddApiKeyClick?: () => void;
    onAddOAuthClick?: () => void;
}

const EmptyStateGuide: React.FC<EmptyStateGuideProps> = ({
    title = "No API Keys Configured",
    description = "Get started by adding your first API key or OAuth provider to access AI services",
    showOAuthButton = true,
    showHeroIcon = true,
    primaryButtonLabel = "Add API Key",
    secondaryButtonLabel = "Add OAuth",
    onAddApiKeyClick,
    onAddOAuthClick,
}) => {
    return (
        <Box textAlign="center" py={8} width="100%">
            {showHeroIcon && (
                <Button
                    variant="contained"
                    startIcon={<Add />}
                    onClick={onAddApiKeyClick}
                    size="large"
                    sx={{
                        backgroundColor: 'primary.main',
                        color: 'white',
                        width: 80,
                        height: 80,
                        borderRadius: 2,
                        mb: 3,
                        '&:hover': {
                            backgroundColor: 'primary.dark',
                            transform: 'scale(1.05)',
                        },
                    }}
                >
                    <Add sx={{ fontSize: 40 }} />
                </Button>
            )}
            <Typography variant="h5" sx={{ fontWeight: 600, mb: 2 }}>
                {title}
            </Typography>
            <Typography variant="body1" color="text.secondary" sx={{ mb: 3, maxWidth: 500, mx: 'auto' }}>
                {description}
            </Typography>
            <Stack direction="row" spacing={2} justifyContent="center">
                <Button
                    variant="contained"
                    startIcon={<Add />}
                    onClick={onAddApiKeyClick}
                    size="large"
                >
                    {primaryButtonLabel}
                </Button>
                {showOAuthButton && onAddOAuthClick && (
                    <Button
                        variant="outlined"
                        startIcon={<VpnKey />}
                        onClick={onAddOAuthClick}
                        size="large"
                    >
                        {secondaryButtonLabel}
                    </Button>
                )}
            </Stack>
        </Box>
    );
};

export default EmptyStateGuide;
