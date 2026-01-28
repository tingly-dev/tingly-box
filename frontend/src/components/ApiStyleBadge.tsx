import {Box, useTheme, alpha} from '@mui/material';
import type {SxProps, Theme} from '@mui/material';

interface ApiStyleBadgeProps {
    apiStyle: string;
    compact?: boolean;
    sx?: SxProps<Theme>;
}

// Helper function to render API style badge with icon and colored background
export const ApiStyleBadge = ({apiStyle, sx = {}, compact = false}: ApiStyleBadgeProps) => {
    const theme = useTheme();
    const isOpenAI = apiStyle === 'openai';
    const isAnthropic = apiStyle === 'anthropic';
    const isGoogle = apiStyle === 'google';

    if (!isOpenAI && !isAnthropic && !isGoogle) {
        return null; // Don't show badge for unknown styles
    }

    // Use theme colors for better consistency
    const getBadgeStyles = () => {
        if (isOpenAI) {
            return {
                backgroundColor: alpha(theme.palette.info.main, 0.1),
                color: theme.palette.info.main,
                borderColor: alpha(theme.palette.info.main, 0.3),
            };
        } else if (isAnthropic) {
            // Warm orange, between red and terracotta
            return {
                backgroundColor: alpha('#E07A5F', 0.12),
                color: '#E07A5F',
                borderColor: alpha('#E07A5F', 0.4),
            };
        } else if (isGoogle) {
            // Google blue
            return {
                backgroundColor: alpha('#4285F4', 0.1),
                color: '#4285F4',
                borderColor: alpha('#4285F4', 0.3),
            };
        }
        return {
            backgroundColor: alpha(theme.palette.grey[500], 0.1),
            color: theme.palette.text.secondary,
            borderColor: alpha(theme.palette.grey[500], 0.3),
        };
    };

    const label = isOpenAI ? 'OpenAI' : isAnthropic ? 'Anthropic' : 'Google';
    const badgeStyles = getBadgeStyles();

    return (
        <Box
            sx={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 1,
                px: 1.5,
                py: 0.75,
                borderRadius: theme.shape.borderRadius,
                fontSize: '11px',
                fontWeight: 600,
                height: '24px',
                minWidth: '80px',
                border: `1px solid ${badgeStyles.borderColor}`,
                backgroundColor: badgeStyles.backgroundColor,
                color: badgeStyles.color,
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
            {compact ? (<span>{label}</span>) : (<span>{label} Style</span>)}
        </Box>
    );
};