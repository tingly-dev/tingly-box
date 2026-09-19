import React from 'react';
import { Box, Button, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { RawRequest } from './benchState';

// RequestEditor: what the client sends. Protocol and Content (which body,
// Message vs a whole-body preset) are chosen in Compose now, not here
// (.design/bench.md §1, §6.3) — this component only renders the content for
// whichever Content is active. The Message view keeps the probe's own
// request and exposes only its message override; any other Content is the
// request itself, written by hand in Compose's chosen protocol — exactly
// what a client speaking it would send, with the model filled in by the
// probe.

export const RequestEditor: React.FC<{
    message: string;
    onMessageChange: (message: string) => void;
    raw: RawRequest | null;
    onRawChange: (raw: RawRequest | null) => void;
    /** The builder's current body — the natural starting point for a hand-written request. */
    seedBody?: string;
    /** Parse error of the current raw body, if any. */
    error?: string;
    messagePlaceholder: string;
}> = ({ message, onMessageChange, raw, onRawChange, seedBody, error, messagePlaceholder }) => {
    const { t } = useTranslation();

    if (!raw) {
        return (
            <Stack spacing={1.5}>
                <TextField
                    label={t('probe.message')}
                    size="small"
                    multiline
                    minRows={2}
                    maxRows={6}
                    value={message}
                    onChange={(e) => onMessageChange(e.target.value)}
                    placeholder={messagePlaceholder}
                    slotProps={{ htmlInput: { sx: { fontSize: '0.82rem' } }, inputLabel: { shrink: true } }}
                />
                <Typography variant="caption" sx={{ color: 'text.secondary', lineHeight: 1.4 }}>
                    {t('bench.presetHint', { defaultValue: "One message, shaped by the Tool / Vision / Thinking knobs — it's the probe itself, materialized. To send anything else — multi-turn, images, tool results, provider-specific fields — pick a different Content above." })}
                </Typography>
            </Stack>
        );
    }

    return (
        <Stack spacing={1.25}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                <Box sx={{ flex: 1 }} />
                {seedBody && (
                    <Tooltip title={t('bench.startFromPresetHint', { defaultValue: "What the probe's own request would send right now." })}>
                        <Button size="small" variant="outlined" onClick={() => onRawChange({ ...raw, body: seedBody })} sx={{ fontSize: '0.72rem' }}>
                            {t('bench.startFromPreset', { defaultValue: 'Copy the preset request' })}
                        </Button>
                    </Tooltip>
                )}
            </Box>
            <TextField
                multiline
                minRows={12}
                maxRows={36}
                value={raw.body}
                onChange={(e) => onRawChange({ ...raw, body: e.target.value })}
                error={!!error}
                helperText={error ? t('bench.rawInvalid', { error, defaultValue: 'Not valid JSON: {{error}}' }) : undefined}
                slotProps={{ htmlInput: { sx: { fontFamily: 'monospace', fontSize: '0.74rem', lineHeight: 1.5 }, spellCheck: false } }}
            />
            <Typography variant="caption" sx={{ color: 'text.secondary', lineHeight: 1.4 }}>
                {t('bench.rawHint', { defaultValue: "Exactly what a client speaking this protocol would send. The probe fills in the model (and max_tokens for Anthropic); Stream still applies; tools, images and thinking are yours to set here — the Tool / Vision / Thinking knobs above only shape Content: Message." })}
            </Typography>
        </Stack>
    );
};
