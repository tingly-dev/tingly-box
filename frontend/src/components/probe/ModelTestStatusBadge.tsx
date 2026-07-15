import React from 'react';
import { Box, Tooltip } from '@mui/material';
import { alpha } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import { CheckCircle as CheckIcon, Error as ErrorIcon } from '@/components/icons';
import type { ProbeResult } from '@/types/probe';
import { formatLatency } from './runProbe';

interface ModelTestStatusBadgeProps {
    result: ProbeResult;
    onOpen: () => void;
}

// ModelTestStatusBadge: the bottom-left "status" corner — persistent once a
// test has run, unlike the hover-only trigger/edit/delete controls in the
// bottom-right "actions" corner. Testing produces a verdict worth keeping
// visible without re-hovering; click it to reopen the card's ProbeDialog.
export const ModelTestStatusBadge: React.FC<ModelTestStatusBadgeProps> = ({ result, onOpen }) => {
    const { t } = useTranslation();
    const ok = result.success;
    const color = ok ? 'success.main' : 'error.main';
    const tooltip = ok
        ? (result.data ? formatLatency(result.data.latency_ms) : t('probe.success'))
        : (result.error?.message || t('probe.failed'));

    return (
        <Tooltip title={tooltip}>
            <Box
                onClick={(e) => {
                    e.stopPropagation();
                    e.preventDefault();
                    onOpen();
                }}
                onMouseDown={(e) => e.stopPropagation()}
                sx={{
                    position: 'absolute',
                    bottom: 4,
                    left: 4,
                    width: 16,
                    height: 16,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    borderRadius: '50%',
                    cursor: 'pointer',
                    bgcolor: (theme) => alpha(theme.palette[ok ? 'success' : 'error'].main, theme.palette.mode === 'dark' ? 0.24 : 0.14),
                    zIndex: 5,
                }}
            >
                {ok ? <CheckIcon sx={{ fontSize: 12, color }} /> : <ErrorIcon sx={{ fontSize: 12, color }} />}
            </Box>
        </Tooltip>
    );
};

export default ModelTestStatusBadge;
