import { Add } from '@mui/icons-material';
import { Box, Button, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';

interface EmptyStateGuideProps {
    title?: string;
    description?: string;
    buttonText?: string;
    onButtonClick?: () => void;
}

const EmptyStateGuide: React.FC<EmptyStateGuideProps> = ({
    title = "No API Keys Configured",
    description = "Get started by adding your first API key to access AI services",
    buttonText = "Add API Key",
    onButtonClick,
}) => {
    const navigate = useNavigate();

    const handleClick = () => {
        if (onButtonClick) {
            onButtonClick();
        } else {
            navigate('/api-keys');
        }
    };

    return (
        <Box textAlign="center" py={8} width="100%">
            <Button
                variant="contained"
                startIcon={<Add />}
                onClick={handleClick}
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
            <Button
                variant="contained"
                startIcon={<Add />}
                onClick={handleClick}
                size="large"
            >
                {buttonText}
            </Button>
        </Box>
    );
};

export default EmptyStateGuide;
