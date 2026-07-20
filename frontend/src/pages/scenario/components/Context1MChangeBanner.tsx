import { Alert, AlertTitle, Typography } from '@mui/material';
import React from 'react';

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
const Context1MChangeBanner: React.FC<Context1MChangeBannerProps> = ({ enabled, clientName, requiresApply = true }) => (
    <Alert
        severity={enabled ? 'success' : 'warning'}
        sx={{
            mb: 2,
            borderRadius: 2,
            '& .MuiAlert-icon': { fontSize: 28 },
        }}
    >
        <AlertTitle>{enabled ? '1M Context Window Enabled' : '1M Context Window Disabled'}</AlertTitle>
        <Typography variant="body2" sx={{ mb: 1 }}>
            {enabled
                ? 'Model names have been updated with [1m] suffix for extended context support.'
                : 'Model names have been updated to remove [1m] suffix.'}
        </Typography>
        <Typography variant="caption" sx={{
            color: "text.secondary"
        }}>
            {requiresApply
                ? `Please apply the configuration below and restart ${clientName} for changes to take effect.`
                : `Please restart ${clientName} and re-select the model for changes to take effect.`}
        </Typography>
    </Alert>
);

export default Context1MChangeBanner;
