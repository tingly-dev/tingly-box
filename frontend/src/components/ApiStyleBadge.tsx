import {Anthropic, OpenAI} from '@lobehub/icons';
import {Box} from '@mui/material';
import type {SxProps, Theme} from '@mui/material';

interface ApiStyleBadgeProps {
    apiStyle: string;
    sx?: SxProps<Theme>;
}

// Helper function to render API style badge with icon and colored background
export const ApiStyleBadge = ({apiStyle, sx = {}}: ApiStyleBadgeProps) => {
    const isOpenAI = apiStyle === 'openai';
    const isAnthropic = apiStyle === 'anthropic';

    if (!isOpenAI && !isAnthropic) {
        return null; // Don't show badge for unknown styles
    }

    const backgroundColor = isOpenAI ? '#1578FF' : '#E97B37';
    const label = isOpenAI ? 'OpenAI' : 'Anthropic';

    return (
        <Box
            sx={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 2,
                px: 1,
                py: 0.25,
                borderRadius: 1,
                backgroundColor,
                color: 'white',
                fontSize: '11px',
                fontWeight: 600,
                height: '20px',
                minWidth: '110px',
                ...sx,
            }}
        >
            {/*<Icon size={10} />*/}
            <span>{label} Style</span>
        </Box>
    );
};