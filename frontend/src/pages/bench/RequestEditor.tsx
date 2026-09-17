import React, { useState } from 'react';
import { Box, Button, ListItemText, Menu, MenuItem, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { ProbeProtocol } from '@/types/probe';
import { BLANK_REQUEST, type RawRequest } from './benchState';

// RequestEditor: what the client sends. Protocol and Request mode (preset /
// custom) are chosen in Compose now, not here (.design/bench.md §1, §6.3) —
// this component only renders the content for whichever mode is active. The
// preset request keeps the probe's own request and exposes only its message
// override; the custom request is the request itself, written by hand in
// Compose's chosen protocol — exactly what a client speaking it would send,
// with the model filled in by the probe.

// Templates carry the shapes that fixed fixtures cannot express — the
// harness's test subjects, surfaced. One set per protocol; no model field
// (the probe fills it). Each one is a content preset — the same kind of
// thing the probe's own Tool / Vision axes are (.design/bench.md §1), just
// whole-body instead of a single fragment.
const TEMPLATES: Record<ProbeProtocol, { id: string; body: object }[]> = {
    anthropic_v1: [
        { id: 'multi', body: { messages: [
            { role: 'user', content: "What's in ./src?" },
            { role: 'assistant', content: 'Three files: main.go, helper.go, types.go.' },
            { role: 'user', content: 'Summarize helper.go in one line.' },
        ] } },
        { id: 'tool', body: { tools: [{ name: 'get_weather', description: 'Current weather for a city', input_schema: { type: 'object', properties: { city: { type: 'string' } }, required: ['city'] } }], messages: [
            { role: 'user', content: "What's the weather in Tokyo?" },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'toolu_01', name: 'get_weather', input: { city: 'Tokyo' } }] },
            { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'toolu_01', content: '24°C, clear' }] },
        ] } },
        { id: 'image', body: { messages: [
            { role: 'user', content: [{ type: 'text', text: 'What colour is this image?' }, { type: 'image', source: { type: 'url', url: 'https://upload.wikimedia.org/wikipedia/commons/thumb/4/47/PNG_transparency_demonstration_1.png/280px-PNG_transparency_demonstration_1.png' } }] },
        ] } },
        { id: 'midsys', body: { system: 'You are a test agent.', messages: [
            { role: 'user', content: "What's in ./src?" },
            { role: 'assistant', content: 'Three files: main.go, helper.go, types.go.' },
            { role: 'system', content: 'From now on answer only in JSON.' },
            { role: 'user', content: 'List them again.' },
        ] } },
    ],
    openai_chat: [
        { id: 'multi', body: { messages: [
            { role: 'system', content: 'You are a test agent.' },
            { role: 'user', content: "What's in ./src?" },
            { role: 'assistant', content: 'Three files: main.go, helper.go, types.go.' },
            { role: 'user', content: 'Summarize helper.go in one line.' },
        ] } },
        { id: 'tool', body: { tools: [{ type: 'function', function: { name: 'get_weather', parameters: { type: 'object', properties: { city: { type: 'string' } } } } }], messages: [
            { role: 'user', content: "What's the weather in Tokyo?" },
            { role: 'assistant', tool_calls: [{ id: 'call_01', type: 'function', function: { name: 'get_weather', arguments: '{"city":"Tokyo"}' } }] },
            { role: 'tool', tool_call_id: 'call_01', content: '24°C, clear' },
        ] } },
        { id: 'image', body: { messages: [
            { role: 'user', content: [{ type: 'text', text: 'What colour is this image?' }, { type: 'image_url', image_url: { url: 'https://upload.wikimedia.org/wikipedia/commons/thumb/4/47/PNG_transparency_demonstration_1.png/280px-PNG_transparency_demonstration_1.png' } }] },
        ] } },
    ],
    openai_responses: [
        { id: 'multi', body: { instructions: 'You are a test agent.', input: [
            { role: 'user', content: "What's in ./src?" },
            { role: 'assistant', content: 'Three files: main.go, helper.go, types.go.' },
            { role: 'user', content: 'Summarize helper.go in one line.' },
        ] } },
        { id: 'tool', body: { tools: [{ type: 'function', name: 'get_weather', parameters: { type: 'object', properties: { city: { type: 'string' } } } }], input: [
            { role: 'user', content: "What's the weather in Tokyo?" },
            { type: 'function_call', call_id: 'call_01', name: 'get_weather', arguments: '{"city":"Tokyo"}' },
            { type: 'function_call_output', call_id: 'call_01', output: '24°C, clear' },
        ] } },
        { id: 'image', body: { input: [
            { role: 'user', content: [{ type: 'input_text', text: 'What colour is this image?' }, { type: 'input_image', image_url: 'https://upload.wikimedia.org/wikipedia/commons/thumb/4/47/PNG_transparency_demonstration_1.png/280px-PNG_transparency_demonstration_1.png' }] },
        ] } },
    ],
};

// startingPointItems: the menu content for "what should the custom
// request's body start as" — a content preset either way (blank, a copy of
// the preset request, or a Template), so there's exactly one mechanism, not
// three (.design/bench.md §6.3).
function StartingPointMenu({ label, protocol, seedBody, onPick }: {
    label: string;
    protocol: ProbeProtocol;
    /** The preset request's current body, if there's a target to build one from. */
    seedBody?: string;
    onPick: (body: string) => void;
}) {
    const { t } = useTranslation();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const pick = (body: string) => {
        setAnchor(null);
        onPick(body);
    };
    const item = (id: string, body: string, primary: string, secondary: string) => (
        <MenuItem key={id} onClick={() => pick(body)} sx={{ maxWidth: 380, whiteSpace: 'normal' }}>
            <ListItemText
                primary={primary}
                secondary={secondary}
                slotProps={{ primary: { sx: { fontSize: '0.85rem', fontWeight: 600 } }, secondary: { sx: { fontSize: '0.72rem' } } }}
            />
        </MenuItem>
    );
    return (
        <>
            <Button size="small" variant="outlined" onClick={(e) => setAnchor(e.currentTarget)} sx={{ fontSize: '0.72rem' }}>
                {label} ▾
            </Button>
            <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}>
                {seedBody &&
                    item(
                        'preset',
                        seedBody,
                        t('bench.startFromPreset', { defaultValue: 'Copy the preset request' }),
                        t('bench.startFromPresetHint', { defaultValue: "What the probe's own request would send right now." }),
                    )}
                {item(
                    'blank',
                    JSON.stringify(BLANK_REQUEST[protocol], null, 2),
                    t('bench.template.blank', { defaultValue: 'Blank' }),
                    t('bench.template.blankDesc', { defaultValue: "An empty request in this protocol's shape." }),
                )}
                {(TEMPLATES[protocol] ?? []).map((tpl) =>
                    item(tpl.id, JSON.stringify(tpl.body, null, 2), t(`bench.template.${tpl.id}`), t(`bench.template.${tpl.id}Desc`)),
                )}
            </Menu>
        </>
    );
}

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
                    {t('bench.presetHint', { defaultValue: "The preset request: one message, shaped by the Tool / Vision / Thinking knobs — it's the probe itself, materialized. To send anything else — multi-turn, images, tool results, provider-specific fields — switch Request mode to Custom." })}
                </Typography>
            </Stack>
        );
    }

    return (
        <Stack spacing={1.25}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                <Box sx={{ flex: 1 }} />
                <StartingPointMenu
                    label={t('bench.changeStartingPoint', { defaultValue: 'Change starting point' })}
                    protocol={raw.protocol}
                    seedBody={seedBody}
                    onPick={(body) => onRawChange({ ...raw, body })}
                />
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
                {t('bench.rawHint', { defaultValue: "Exactly what a client speaking this protocol would send. The probe fills in the model (and max_tokens for Anthropic); Stream still applies; tools, images and thinking are yours to set here — the Tool / Vision / Thinking knobs only shape the preset request." })}
            </Typography>
        </Stack>
    );
};
