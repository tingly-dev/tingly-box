import { Box, useTheme, alpha } from '@mui/material';
import type { SxProps, Theme } from '@mui/material';
import { EMPTY_SX } from '@/constants/defaults';

interface AuthTypeBadgeProps {
    authType: string;
    sx?: SxProps<Theme>;
}

// Helper function to render auth type badge with colored background
export const AuthTypeBadge = ({ authType, sx = EMPTY_SX }: AuthTypeBadgeProps) => {
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
            case 'vmodel':
                return 'Virtual';
            default:
                return authType || 'Unknown';
        }
    };

    const label = getLabel();
    // vmodel is selectable but should not compete visually with real
    // credentials. Render it muted (text.secondary) rather than the regular
    // success-tinted hover treatment.
    const isVirtual = authType === 'vmodel';

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
                color: isVirtual ? theme.palette.text.secondary : undefined,
                transition: theme.transitions.create(['background-color', 'color', 'border-color'], {
                    duration: theme.transitions.duration.shorter,
                }),
                '&:hover': {
                    backgroundColor: isVirtual
                        ? alpha(theme.palette.text.secondary, 0.08)
                        : alpha(theme.palette.success.main, 0.15),
                },
                ...sx,
            }}
        >
            {label}
        </Box>
    );
};
