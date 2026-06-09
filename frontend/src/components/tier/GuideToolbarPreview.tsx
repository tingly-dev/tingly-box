import { Box, Button, Stack, Typography } from '@mui/material';
import { alpha, keyframes, useTheme } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import {
    Add as AddIcon,
    BugReport as TroubleshootIcon,
    Key as KeyIcon,
} from '@/components/icons';

export type GuideToolbarButton = 'connectAI' | 'newRule';

interface GuideToolbarPreviewProps {
    // Which toolbar button this step is teaching — gets a pulsing highlight ring.
    highlight: GuideToolbarButton;
}

const pulse = keyframes`
    0%   { box-shadow: 0 0 0 0 var(--guide-ring); }
    70%  { box-shadow: 0 0 0 8px transparent; }
    100% { box-shadow: 0 0 0 0 transparent; }
`;

/**
 * A non-functional replica of the page toolbar (Troubleshoot · Connect AI ·
 * New Rule), shown inside the routing guide so users can recognise the exact
 * button a step is asking them to click. The taught button pulses; a small
 * "Click here" tag points at it.
 */
export const GuideToolbarPreview: React.FC<GuideToolbarPreviewProps> = ({ highlight }) => {
    const { t } = useTranslation();
    const theme = useTheme();
    const ringColor = alpha(theme.palette.primary.main, 0.5);

    const ring = (active: boolean) =>
        active
            ? {
                  position: 'relative' as const,
                  animation: `${pulse} 1.8s ease-out infinite`,
                  '--guide-ring': ringColor,
              }
            : { opacity: 0.45 };

    // A "Click here" chip with a downward caret, floated above the target button.
    // Plain element (not a nested component) so React doesn't remount it.
    const tag = (
        <Typography
            component="span"
            sx={{
                position: 'absolute',
                bottom: 'calc(100% + 5px)',
                left: '50%',
                transform: 'translateX(-50%)',
                fontSize: '0.6rem',
                fontWeight: 700,
                letterSpacing: '0.02em',
                color: 'primary.contrastText',
                bgcolor: 'primary.main',
                px: 0.75,
                py: 0.15,
                borderRadius: 1,
                whiteSpace: 'nowrap',
                boxShadow: 1,
                pointerEvents: 'none',
                // little downward caret pointing at the button
                '&::after': {
                    content: '""',
                    position: 'absolute',
                    top: '100%',
                    left: '50%',
                    transform: 'translateX(-50%)',
                    borderLeft: '4px solid transparent',
                    borderRight: '4px solid transparent',
                    borderTop: '4px solid',
                    borderTopColor: 'primary.main',
                },
            }}
        >
            {t('rule.routing.guide.clickHere', { defaultValue: 'Click here' })}
        </Typography>
    );

    return (
        <Box
            sx={{
                width: '100%',
                maxWidth: 700,
                mb: 1.5,
                px: 1.5,
                pt: 3,
                pb: 1,
                borderRadius: 1,
                border: '1px dashed',
                borderColor: 'divider',
                bgcolor: 'background.paper',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: 1,
            }}
        >
            <Typography variant="caption" color="text.disabled" sx={{ fontStyle: 'italic', flexShrink: 0 }}>
                {t('rule.routing.guide.toolbarLabel', { defaultValue: 'Page toolbar' })}
            </Typography>
            <Stack direction="row" spacing={1} alignItems="center">
                <Button variant="outlined" size="small" startIcon={<TroubleshootIcon />} sx={{ opacity: 0.45, pointerEvents: 'none' }} tabIndex={-1}>
                    {t('templateActions.troubleshoot', { defaultValue: 'Troubleshoot' })}
                </Button>
                <Box sx={{ position: 'relative' }}>
                    {highlight === 'connectAI' && tag}
                    <Button
                        variant="outlined"
                        size="small"
                        startIcon={<KeyIcon />}
                        tabIndex={-1}
                        sx={{ pointerEvents: 'none', ...ring(highlight === 'connectAI') }}
                    >
                        {t('templateActions.connectAI', { defaultValue: 'Connect AI' })}
                    </Button>
                </Box>
                <Box sx={{ position: 'relative' }}>
                    {highlight === 'newRule' && tag}
                    <Button
                        variant="contained"
                        size="small"
                        startIcon={<AddIcon />}
                        tabIndex={-1}
                        sx={{ pointerEvents: 'none', ...ring(highlight === 'newRule') }}
                    >
                        {t('templateActions.newRule', { defaultValue: 'New Rule' })}
                    </Button>
                </Box>
            </Stack>
        </Box>
    );
};

export default GuideToolbarPreview;
