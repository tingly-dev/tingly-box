import React, { useState } from 'react';
import { Box, Button, ListItemText, Menu, MenuItem, Select, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { ProbeProtocol } from '@/types/probe';
import { PROTOCOL_META } from '@/components/probe/AxisPrimitives';
import type { RawRequest } from './benchState';

// RequestEditor: what the client sends. Fixture mode keeps the probe's
// built-in request and exposes only its message override; raw mode is the
// request itself, written by hand in one of the three client protocols —
// exactly what a client speaking that protocol would send, with the model
// filled in by the probe (.design/bench.md §6).

const PROTOCOL_LABEL = (p: ProbeProtocol) => PROTOCOL_META[p]?.full || p;

// Templates carry the shapes that fixed fixtures cannot express — the
// harness's test subjects, surfaced. One set per protocol; no model field
// (the probe fills it).
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

export const RequestEditor: React.FC<{
    message: string;
    onMessageChange: (message: string) => void;
    raw: RawRequest | null;
    onRawChange: (raw: RawRequest | null) => void;
    /** Protocols the current target can be spoken to in (the raw request's protocol must be one). */
    protocolOptions: ProbeProtocol[];
    /** The builder's current body — the natural starting point for a hand-written request. */
    seedBody?: string;
    /** Parse error of the current raw body, if any. */
    error?: string;
    messagePlaceholder: string;
}> = ({ message, onMessageChange, raw, onRawChange, protocolOptions, seedBody, error, messagePlaceholder }) => {
    const { t } = useTranslation();
    const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
    const defaultProtocol = protocolOptions[0] ?? 'openai_chat';

    const startRaw = (body: string, protocol?: ProbeProtocol) => onRawChange({ protocol: protocol ?? defaultProtocol, body });
    const templates = raw ? TEMPLATES[raw.protocol] ?? [] : [];

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
                    {t('bench.fixtureHint', { defaultValue: "The probe's built-in request: one message, shaped by the Tool / Vision / Thinking knobs. To send anything else — multi-turn, images, tool results, provider-specific fields — write the request yourself." })}
                </Typography>
                <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                    <Button size="small" variant="outlined" onClick={() => startRaw(seedBody ?? JSON.stringify(TEMPLATES[defaultProtocol][0].body, null, 2))}>
                        {seedBody
                            ? t('bench.startFromBuilder', { defaultValue: "Edit the builder's request" })
                            : t('bench.writeYourself', { defaultValue: 'Write the request yourself' })}
                    </Button>
                </Box>
            </Stack>
        );
    }

    return (
        <Stack spacing={1.25}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                <Select
                    size="small"
                    value={raw.protocol}
                    onChange={(e) => onRawChange({ ...raw, protocol: e.target.value as ProbeProtocol })}
                    sx={{ fontSize: '0.78rem', minWidth: 190 }}
                    inputProps={{ 'aria-label': t('probe.protocol') }}
                >
                    {protocolOptions.map((p) => (
                        <MenuItem key={p} value={p} sx={{ fontSize: '0.8rem' }}>{PROTOCOL_LABEL(p)}</MenuItem>
                    ))}
                    {!protocolOptions.includes(raw.protocol) && (
                        <MenuItem value={raw.protocol} disabled sx={{ fontSize: '0.8rem' }}>{PROTOCOL_LABEL(raw.protocol)}</MenuItem>
                    )}
                </Select>
                <Box sx={{ flex: 1 }} />
                {seedBody && (
                    <Tooltip title={t('bench.startFromBuilderHint', { defaultValue: "Replace the text with the request the probe's builder would send right now." })}>
                        <Button size="small" onClick={() => onRawChange({ ...raw, body: seedBody })} sx={{ fontSize: '0.72rem' }}>
                            {t('bench.startFromBuilder', { defaultValue: "Edit the builder's request" })}
                        </Button>
                    </Tooltip>
                )}
                <Button size="small" variant="outlined" onClick={(e) => setMenuAnchor(e.currentTarget)} sx={{ fontSize: '0.72rem' }}>
                    {t('bench.templates', { defaultValue: 'Templates' })} ▾
                </Button>
                <Menu anchorEl={menuAnchor} open={!!menuAnchor} onClose={() => setMenuAnchor(null)}>
                    {templates.map((tpl) => (
                        <MenuItem
                            key={tpl.id}
                            onClick={() => { setMenuAnchor(null); onRawChange({ ...raw, body: JSON.stringify(tpl.body, null, 2) }); }}
                            sx={{ maxWidth: 380, whiteSpace: 'normal' }}
                        >
                            <ListItemText
                                primary={t(`bench.template.${tpl.id}`)}
                                secondary={t(`bench.template.${tpl.id}Desc`)}
                                slotProps={{ primary: { sx: { fontSize: '0.85rem', fontWeight: 600 } }, secondary: { sx: { fontSize: '0.72rem' } } }}
                            />
                        </MenuItem>
                    ))}
                </Menu>
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
                {t('bench.rawHint', { protocol: PROTOCOL_LABEL(raw.protocol), defaultValue: 'Exactly what a client speaking {{protocol}} would send. The probe fills in the model (and max_tokens for Anthropic); Stream still applies; tools, images and thinking are yours to set here — the Tool / Vision / Thinking knobs only shape the fixture.' })}
            </Typography>
            <Box>
                <Button size="small" onClick={() => onRawChange(null)} sx={{ fontSize: '0.72rem' }}>
                    {t('bench.backToFixture', { defaultValue: 'Back to the fixture' })}
                </Button>
            </Box>
        </Stack>
    );
};

export default RequestEditor;
