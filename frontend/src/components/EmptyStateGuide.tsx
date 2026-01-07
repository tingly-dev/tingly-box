import { Add, VpnKey } from '@mui/icons-material';
import { Box, Button, Stack, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';

interface EmptyStateGuideProps {
    title?: string;
    description?: string;
    showOAuthButton?: boolean;
    onAddApiKeyClick?: () => void;
    onAddOAuthClick?: () => void;
}

const EmptyStateGuide: React.FC<EmptyStateGuideProps> = ({
    title = "No API Keys Configured",
    description = "Get started by adding your first API key or OAuth provider to access AI services",
    showOAuthButton = true,
    onAddApiKeyClick,
    onAddOAuthClick,
}) => {
    const navigate = useNavigate();

    const handleAddApiKeyClick = () => {
        if (onAddApiKeyClick) {
            onAddApiKeyClick();
        }
    };

    const handleAddOAuthClick = () => {
        if (onAddOAuthClick) {
            onAddOAuthClick();
        } else {
            navigate('/oauth');
        }
    };

    return (
        <Box textAlign="center" py={8} width="100%">
            <Button
                variant="contained"
                startIcon={<Add />}
                onClick={handleAddApiKeyClick}
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
                    onClick={handleAddApiKeyClick}
                    size="large"
                >
                    Add API Key
                </Button>
                {showOAuthButton && (
                    <Button
                        variant="outlined"
                        startIcon={<VpnKey />}
                        onClick={handleAddOAuthClick}
                        size="large"
                    >
                        Add OAuth
                    </Button>
                )}
            </Stack>
        </Box>
    );
};

export default EmptyStateGuide;
