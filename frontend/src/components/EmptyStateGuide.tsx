import { Add } from '@/components/icons';
import { Box, Button, Stack, Typography } from '@mui/material';

interface EmptyStateGuideProps {
    title?: string;
    description?: string;
    showHeroIcon?: boolean;
    primaryButtonLabel?: string;
    onAddApiKeyClick?: () => void;
}

const EmptyStateGuide: React.FC<EmptyStateGuideProps> = ({
    title = "No API Keys Configured",
    description = "Get started by connecting your first AI provider to access AI services",
    showHeroIcon = true,
    primaryButtonLabel = "Connect AI",
    onAddApiKeyClick,
}) => {
    return (
        <Box
            sx={{
                textAlign: "center",
                py: 8,
                width: "100%"
            }}>
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
            <Typography
                variant="body1"
                sx={{
                    color: "text.secondary",
                    mb: 3,
                    maxWidth: 500,
                    mx: 'auto'
                }}>
                {description}
            </Typography>
            <Stack direction="row" spacing={2} sx={{
                justifyContent: "center"
            }}>
                <Button
                    variant="contained"
                    startIcon={<Add />}
                    onClick={onAddApiKeyClick}
                    size="large"
                >
                    {primaryButtonLabel}
                </Button>
            </Stack>
        </Box>
    );
};

export default EmptyStateGuide;
