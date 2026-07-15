import React from 'react';
import { Box, Tooltip } from '@mui/material';
import { alpha } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import { CheckCircle as CheckIcon, Error as ErrorIcon } from '@/components/icons';
import type { ProbeResult } from '@/types/probe';
import { formatLatency } from './runProbe';
import { ProbeDialog } from './ProbeDialog';

interface ModelTestStatusBadgeProps {
    result: ProbeResult;
    providerUuid: string;
    providerName: string;
    model: string;
    dialogOpen: boolean;
    onOpenDialog: (e: React.MouseEvent) => void;
    onCloseDialog: () => void;
}

// ModelTestStatusBadge: the bottom-left "status" corner — persistent once a
// test has run, unlike the hover-only trigger/edit/delete controls in the
// bottom-right "actions" corner. Testing produces a verdict worth keeping
// visible without re-hovering; click it to open the full ProbeDialog.
export const ModelTestStatusBadge: React.FC<ModelTestStatusBadgeProps> = ({
    result,
    providerUuid,
    providerName,
    model,
    dialogOpen,
    onOpenDialog,
    onCloseDialog,
}) => {
    const { t } = useTranslation();
    const ok = result.success;
    const color = ok ? 'success.main' : 'error.main';
    const tooltip = ok
        ? (result.data ? formatLatency(result.data.latency_ms) : t('probe.success'))
        : (result.error?.message || t('probe.failed'));

    return (
        <>
            <Tooltip title={tooltip}>
                <Box
                    onClick={(e) => {
                        e.stopPropagation();
                        e.preventDefault();
                        onOpenDialog(e);
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
            <ProbeDialog
                open={dialogOpen}
                onClose={onCloseDialog}
                targetType="provider"
                targetId={providerUuid}
                targetName={providerName}
                model={model}
                initialResult={result}
            />
        </>
    );
};

export default ModelTestStatusBadge;
