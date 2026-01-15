import { Box, useTheme, alpha } from '@mui/material';
import type { SxProps, Theme } from '@mui/material';

interface AuthTypeBadgeProps {
    authType: string;
    sx?: SxProps<Theme>;
}

// Helper function to render auth type badge with colored background
export const AuthTypeBadge = ({ authType, sx = {} }: AuthTypeBadgeProps) => {
    const theme = useTheme();

    // Define styles for each auth type
    const getBadgeStyles = () => {
        switch (authType) {
            case 'oauth':
                return {
                    backgroundColor: alpha(theme.palette.success.main, 0.1),
                    color: theme.palette.success.main,
                    borderColor: alpha(theme.palette.success.main, 0.3),
                    label: 'OAuth',
                };
            case 'api_key':
                return {
                    backgroundColor: alpha(theme.palette.success.main, 0.1),
                    color: theme.palette.success.main,
                    borderColor: alpha(theme.palette.success.main, 0.3),
                    label: 'API Key',
                };
            case 'bearer_token':
                return {
                    backgroundColor: alpha(theme.palette.success.main, 0.1),
                    color: theme.palette.success.main,
                    borderColor: alpha(theme.palette.success.main, 0.3),
                    label: 'Bearer',
                };
            case 'basic_auth':
                return {
                    backgroundColor: alpha(theme.palette.success.main, 0.1),
                    color: theme.palette.success.main,
                    borderColor: alpha(theme.palette.success.main, 0.3),
                    label: 'Basic',
                };
            default:
                return {
                    backgroundColor: alpha(theme.palette.success.main, 0.1),
                    color: theme.palette.success.main,
                    borderColor: alpha(theme.palette.success.main, 0.3),
                    label: authType || 'Unknown',
                };
        }
    };

    const badgeStyles = getBadgeStyles();

    return (
        <Box
            sx={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                px: 1,
                py: 0.5,
                // borderRadius: theme.shape.borderRadius,
                fontSize: '10px',
                fontWeight: 600,
                textTransform: 'uppercase',
                height: '20px',
                minWidth: '60px',
                // border: `1px solid ${badgeStyles.borderColor}`,
                // backgroundColor: badgeStyles.backgroundColor,
                // color: badgeStyles.color,
                transition: theme.transitions.create(['background-color', 'color', 'border-color'], {
                    duration: theme.transitions.duration.shorter,
                }),
                '&:hover': {
                    backgroundColor: alpha(badgeStyles.color, 0.15),
                    borderColor: alpha(badgeStyles.color, 0.5),
                },
                ...sx,
            }}
        >
            {badgeStyles.label}
        </Box>
    );
};
