import React, { useCallback, useEffect, useRef, useState } from 'react';
import { CircularProgress, IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import {
    Bolt as BoltIcon,
    CheckCircle as CheckIcon,
    Error as ErrorIcon,
} from '@/components/icons';
import type { ProbeResult } from '@/types/probe';
import { formatLatency, runProbe } from './runProbe';
import { ProbeDialog } from './ProbeDialog';

interface ModelTestButtonProps {
    providerUuid: string;
    providerName: string;
    model: string;
}

// ModelTestButton: compact one-click streaming probe for a single model card,
// living in the card's hover control-bar next to Edit/Delete. Click the bolt,
// get an inline success/fail glyph in place — no pre-selecting the model, no
// separate toolbar button. Click the glyph to open the full ProbeDialog
// (journey, tokens, raw response) pre-loaded with this result.
export const ModelTestButton: React.FC<ModelTestButtonProps> = ({ providerUuid, providerName, model }) => {
    const { t } = useTranslation();
    const [running, setRunning] = useState(false);
    const [result, setResult] = useState<ProbeResult | null>(null);
    const [dialogOpen, setDialogOpen] = useState(false);
    const mounted = useRef(true);

    useEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);

    const run = useCallback(async (e: React.MouseEvent) => {
        e.stopPropagation();
        e.preventDefault();
        setRunning(true);
        setResult(null);
        const res = await runProbe({
            target_type: 'provider',
            provider_uuid: providerUuid,
            model,
            test_mode: 'streaming',
        });
        if (!mounted.current) return;
        setResult(res);
        setRunning(false);
    }, [providerUuid, model]);

    const handleOpenDialog = (e: React.MouseEvent) => {
        e.stopPropagation();
        e.preventDefault();
        setDialogOpen(true);
    };

    if (running) {
        return <CircularProgress size={14} thickness={5} sx={{ mx: '3px' }} />;
    }

    if (result) {
        const ok = result.success;
        const tooltip = ok
            ? (result.data ? formatLatency(result.data.latency_ms) : t('probe.success'))
            : (result.error?.message || t('probe.failed'));

        return (
            <>
                <Tooltip title={tooltip}>
                    <IconButton
                        size="small"
                        onClick={handleOpenDialog}
                        sx={{
                            p: 0.3,
                            color: ok ? 'success.main' : 'error.main',
                            '&:hover': {
                                backgroundColor: ok ? 'rgba(46, 125, 50, 0.08)' : 'rgba(211, 47, 47, 0.08)',
                            },
                        }}
                    >
                        {ok ? <CheckIcon sx={{ fontSize: 14 }} /> : <ErrorIcon sx={{ fontSize: 14 }} />}
                    </IconButton>
                </Tooltip>
                <ProbeDialog
                    open={dialogOpen}
                    onClose={() => setDialogOpen(false)}
                    targetType="provider"
                    targetId={providerUuid}
                    targetName={providerName}
                    model={model}
                    initialResult={result}
                />
            </>
        );
    }

    return (
        <Tooltip title={t('probe.quickTest')}>
            <IconButton
                size="small"
                onClick={run}
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

export default ModelTestButton;
