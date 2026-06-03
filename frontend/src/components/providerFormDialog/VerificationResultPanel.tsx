import {Check, Close} from '@/components/icons';
import {Box, Button, IconButton, Stack, Typography} from '@mui/material';
import React from 'react';
import type {VerificationResult} from './probe';

interface VerificationResultPanelProps {
    result: VerificationResult;
    onClose: () => void;
    /** When the probe fails on an OpenAI URL without /v1, suggest appending it. */
    v1Hint?: { show: boolean; onApply: () => void };
}

const VerificationResultPanel: React.FC<VerificationResultPanelProps> = ({result, onClose, v1Hint}) => {
    const details = (result.details ?? '').split(' • ').filter(d => d.trim());

    return (
        <Box
            sx={{
                mt: 1,
                p: 1.5,
                borderRadius: 1.5,
                bgcolor: 'background.default',
                border: '1px solid',
                borderColor: 'divider',
            }}
        >
            <Box sx={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1}}>
                <Typography variant="caption" color="text.secondary">
                    Connection Test (for reference)
                </Typography>
                <IconButton
                    aria-label="close"
                    size="small"
                    onClick={onClose}
                    sx={{ml: -0.5}}
                >
                    <Close fontSize="small"/>
                </IconButton>
            </Box>

            <Stack spacing={0.75}>
                {details.map((detail, index) => {
                    const isSuccess = detail.includes('✓');
                    const label = detail.replace(/^[✓✗]\s*/, '');

                    return (
                        <Stack key={index} direction="row" spacing={1} alignItems="center" sx={{minHeight: 24}}>
                            {isSuccess ? (
                                <Box
                                    sx={{
                                        width: 18,
                                        height: 18,
                                        borderRadius: '50%',
                                        bgcolor: 'success.main',
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                        flexShrink: 0,
                                    }}
                                >
                                    <Check
                                        fontSize="small"
                                        sx={{
                                            color: 'common.white',
                                            fontSize: '12px',
                                            fontWeight: 'bold',
                                        }}
                                    />
                                </Box>
                            ) : (
                                <Box
                                    sx={{
                                        width: 18,
                                        height: 18,
                                        borderRadius: '50%',
                                        bgcolor: 'warning.main',
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                        flexShrink: 0,
                                    }}
                                >
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            color: 'common.white',
                                            fontSize: '12px',
                                            fontWeight: 'bold',
                                        }}
                                    >
                                        !
                                    </Typography>
                                </Box>
                            )}
                            <Typography variant="body2" sx={{fontSize: '0.8rem', flex: 1}}>
                                {label}
                            </Typography>
                        </Stack>
                    );
                })}
            </Stack>

            {v1Hint?.show && (
                <Stack
                    direction="row"
                    alignItems="center"
                    spacing={1}
                    sx={{mt: 1.5, p: 1, borderRadius: 1, bgcolor: 'warning.lighter', border: '1px solid', borderColor: 'warning.light'}}
                >
                    <Typography variant="caption" sx={{flex: 1}}>
                        Some OpenAI-compatible APIs require a <code>/v1</code> suffix on the base URL.
                    </Typography>
                    <Button size="small" variant="outlined" color="warning" onClick={v1Hint.onApply} sx={{whiteSpace: 'nowrap', minWidth: 'auto'}}>
                        Try /v1
                    </Button>
                </Stack>
            )}

            <Typography
                variant="caption"
                color="text.secondary"
                sx={{display: 'block', mt: 1.5, pt: 1, borderTop: '1px solid', borderColor: 'divider'}}
            >
                Test results are for reference only - you can add the key even if some tests fail
            </Typography>
        </Box>
    );
};

export default VerificationResultPanel;
