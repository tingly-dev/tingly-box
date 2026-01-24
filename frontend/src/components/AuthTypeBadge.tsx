import { Box, useTheme, alpha } from '@mui/material';
import type { SxProps, Theme } from '@mui/material';

interface AuthTypeBadgeProps {
    authType: string;
    sx?: SxProps<Theme>;
}

// Helper function to render auth type badge with colored background
export const AuthTypeBadge = ({ authType, sx = {} }: AuthTypeBadgeProps) => {
    const theme = useTheme();

    // Define label for each auth type
    const getLabel = () => {
        switch (authType) {
            case 'oauth':
                return 'OAuth';
            case 'api_key':
                return 'API Key';
            case 'bearer_token':
                return 'Bearer';
            case 'basic_auth':
                return 'Basic';
            default:
                return authType || 'Unknown';
        }
    };

    const label = getLabel();

    return (
        <Box
            sx={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                px: 1,
                py: 0.5,
                fontSize: '10px',
                fontWeight: 600,
                textTransform: 'uppercase',
                height: '20px',
                minWidth: '60px',
                transition: theme.transitions.create(['background-color', 'color', 'border-color'], {
                    duration: theme.transitions.duration.shorter,
                }),
                '&:hover': {
                    backgroundColor: alpha(theme.palette.success.main, 0.15),
                },
                ...sx,
            }}
        >
            {label}
        </Box>
    );
};
