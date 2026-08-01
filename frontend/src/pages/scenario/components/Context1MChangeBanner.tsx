import { Alert, AlertTitle, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';

interface Context1MChangeBannerProps {
    enabled: boolean;
    // Client name used in the call-to-action line, e.g. "Claude Code".
    clientName: string;
    // Whether the user must re-apply the generated config (launcher-based
    // clients). Desktop picks renamed models straight from /v1/models, so it
    // only needs a restart / re-pick.
    requiresApply?: boolean;
}

// Context1MChangeBanner is the shared pending-change notice the scenario
// config modals show after the user toggles 1M context on a rule, explaining
// what changed and what the user must do for the client to pick it up.
const Context1MChangeBanner: React.FC<Context1MChangeBannerProps> = ({ enabled, clientName, requiresApply = true }) => {
    const { t } = useTranslation();
    return (
        <Alert
            severity={enabled ? 'success' : 'warning'}
            sx={{
                mb: 2,
                borderRadius: 2,
                '& .MuiAlert-icon': { fontSize: 28 },
            }}
        >
            <AlertTitle>{enabled ? t('context1M.enabledTitle') : t('context1M.disabledTitle')}</AlertTitle>
            <Typography variant="body2" sx={{ mb: 1 }}>
                {enabled ? t('context1M.enabledBody') : t('context1M.disabledBody')}
            </Typography>
            <Typography variant="caption" sx={{
                color: "text.secondary"
            }}>
                {requiresApply
                    ? t('context1M.requiresApplyHint', { client: clientName })
                    : t('context1M.restartOnlyHint', { client: clientName })}
            </Typography>
        </Alert>
    );
};

export default Context1MChangeBanner;
