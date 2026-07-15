import React from 'react';
import { IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Bolt as BoltIcon } from '@/components/icons';

interface ModelTestTriggerProps {
    onOpen: () => void;
}

// ModelTestTrigger: the bolt icon that opens the full ProbeDialog for this
// model. Lives in the card's hover-only control-bar (bottom-right, alongside
// Edit/Delete) — an action, not a status, so it only shows up when the user
// is looking at this card. Deliberately opens the dialog rather than running
// the probe inline — that's the established probe interaction (progress,
// journey, and raw response all live in the dialog), not a quick silent run.
export const ModelTestTrigger: React.FC<ModelTestTriggerProps> = ({ onOpen }) => {
    const { t } = useTranslation();

    return (
        <Tooltip title={t('probe.quickTest')}>
            <IconButton
                size="small"
                onClick={onOpen}
                sx={{
                    p: 0.3,
                    color: 'text.secondary',
                    '&:hover': {
                        backgroundColor: 'action.hover',
                        color: 'primary.main',
                    },
                }}
            >
                <BoltIcon sx={{ fontSize: 14 }} />
            </IconButton>
        </Tooltip>
    );
};

export default ModelTestTrigger;
