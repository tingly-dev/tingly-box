import { Alert, IconButton, Link } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Close, GitHub, Star } from '@/components/icons';

const REPO_URL = 'https://github.com/tingly-dev/tingly-box';
const DISMISS_KEY = 'layout.githubStarBanner.dismissed';

// Shows once per app run (sessionStorage clears on restart, so the banner
// reappears next launch) and asks the user to star the repo. Dismissing it
// only hides it for the current run — there's no permanent opt-out, in line
// with keeping this low-friction rather than adding another setting.
export const GitHubStarBanner = () => {
    const { t } = useTranslation();
    const [dismissed, setDismissed] = useState(() => sessionStorage.getItem(DISMISS_KEY) === '1');

    if (dismissed) return null;

    const handleDismiss = () => {
        sessionStorage.setItem(DISMISS_KEY, '1');
        setDismissed(true);
    };

    return (
        <Alert
            icon={<Star sx={{ fontSize: 20 }} />}
            severity="info"
            variant="outlined"
            sx={{
                mx: 2,
                mt: 2,
                borderRadius: 2,
                alignItems: 'center',
                '& .MuiAlert-message': { flexGrow: 1 },
            }}
            action={
                <IconButton
                    size="small"
                    aria-label={t('common.dismiss')}
                    onClick={handleDismiss}
                >
                    <Close sx={{ fontSize: 18 }} />
                </IconButton>
            }
        >
            {t('layout.githubStarBanner.text')}{' '}
            <Link
                href={REPO_URL}
                target="_blank"
                rel="noopener noreferrer"
                sx={{ fontWeight: 600, display: 'inline-flex', alignItems: 'center', gap: 0.5, verticalAlign: 'middle' }}
            >
                <GitHub sx={{ fontSize: 16 }} />
                {t('layout.githubStarBanner.cta')}
            </Link>
        </Alert>
    );
};
