import React from 'react';
import { CircularProgress, IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Bolt as BoltIcon } from '@/components/icons';

interface ModelTestTriggerProps {
    running: boolean;
    onRun: (e: React.MouseEvent) => void;
}

// ModelTestTrigger: the bolt icon that kicks off a model test. Lives in the
// card's hover-only control-bar (bottom-right, alongside Edit/Delete) — an
// action, not a status, so it only shows up when the user is looking at this
// card. Re-clickable even after a result exists, to re-run the test.
export const ModelTestTrigger: React.FC<ModelTestTriggerProps> = ({ running, onRun }) => {
    const { t } = useTranslation();

    if (running) {
        return <CircularProgress size={14} thickness={5} sx={{ mx: '3px' }} />;
    }

    return (
        <Tooltip title={t('probe.quickTest')}>
            <IconButton
                size="small"
                onClick={onRun}
                sx={{
                    p: 0.3,
                    color: 'text.secondary',
                    '&:hover': {
                        backgroundColor: 'rgba(0, 0, 0, 0.04)',
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
