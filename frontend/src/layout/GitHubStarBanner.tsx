import { IconButton, Link, Stack, Typography } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Close, GitHub, Star } from '@/components/icons';

const REPO_URL = 'https://github.com/tingly-dev/tingly-box';
const DISMISS_KEY = 'layout.githubStarBanner.dismissed';

// Shows once per app run (sessionStorage clears on restart, so the banner
// reappears next launch) and asks the user to star the repo. Dismissing it
// only hides it for the current run — there's no permanent opt-out, in line
// with keeping this low-friction rather than adding another setting.
//
// Styled as a plain surface card (paper bg + divider border) rather than a
// MUI Alert, so it reads as part of the app chrome instead of a status/info
// message with its own fixed hue.
export const GitHubStarBanner = () => {
    const { t } = useTranslation();
    const [dismissed, setDismissed] = useState(() => sessionStorage.getItem(DISMISS_KEY) === '1');

    if (dismissed) return null;

    const handleDismiss = () => {
        sessionStorage.setItem(DISMISS_KEY, '1');
        setDismissed(true);
    };

    return (
        <Stack
            direction="row"
            spacing={1.5}
            sx={{
                alignItems: 'center',
                mx: 2,
                mt: 2,
                px: 2,
                py: 1,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                bgcolor: 'background.paper',
            }}
        >
            <Star sx={{ fontSize: 18, color: 'primary.main', flexShrink: 0 }} />
            <Typography variant="body2" sx={{ color: 'text.secondary', flexGrow: 1 }}>
                {t('layout.githubStarBanner.text')}{' '}
                <Link
                    href={REPO_URL}
                    target="_blank"
                    rel="noopener noreferrer"
                    color="primary"
                    sx={{ fontWeight: 600, display: 'inline-flex', alignItems: 'center', gap: 0.5, verticalAlign: 'middle' }}
                >
                    <GitHub sx={{ fontSize: 15 }} />
                    {t('layout.githubStarBanner.cta')}
                </Link>
            </Typography>
            <IconButton
                size="small"
                aria-label={t('common.dismiss')}
                onClick={handleDismiss}
                sx={{ color: 'text.secondary' }}
            >
                <Close sx={{ fontSize: 18 }} />
            </IconButton>
        </Stack>
    );
};
