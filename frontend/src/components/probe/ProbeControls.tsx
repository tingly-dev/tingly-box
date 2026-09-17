import React, { useState } from 'react';
import { Box, Typography, TextField, Stack, Collapse } from '@mui/material';
import { ExpandMore as ExpandMoreIcon, ExpandLess as ExpandLessIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import type { ProbeProtocol } from '@/types/probe';
import type { ProbeAxes } from './probeConfig';
import { Axis, AxisGroup, ExclusiveToggle, ThinkingSlider, PROTOCOL_META } from './AxisPrimitives';

// ProbeControls renders the control rail: orthogonal axes stacked vertically,
// one label + control pair per row. Groups fill the rail width and every
// option button is equal-width, so the rail reads as an aligned instrument
// panel instead of ragged inline chips. Adding a future axis = one more row
// here (and a field on ProbeAxes) — in the Parameters AxisGroup if it's a
// real, independently-valued field, in Content if it's a fixed blob toggled
// on/off (.design/bench.md §1). Shape/Scope stay ungrouped above the fold:
// they're the two axes 80% of probes touch, not because of which kind they
// are (they're actually one of each — Stream is a parameter, Scope is
// transport, not even part of the request body).

interface ProbeControlsProps {
    axes: ProbeAxes;
    onAxesChange: (axes: ProbeAxes) => void;
    message: string;
    onMessageChange: (message: string) => void;
    messagePlaceholder: string;
    // Protocol axis rendering, pre-resolved by the dialog (per-target
    // reduction: locked single option, or disabled for Google). '' while the
    // provider record is still loading.
    protocol: {
        value: ProbeProtocol | '';
        options: ProbeProtocol[];
        locked: boolean;
        disabled: boolean;
        lockHint?: string;
    };
    scopeDisabled: boolean;
    scopeHint: string;
    // Vision axis rendering: disabled (with hint) for targets without a probe
    // image mapping (Google's own SDK).
    vision: {
        disabled: boolean;
        hint: string;
    };
}

export const ProbeControls: React.FC<ProbeControlsProps> = ({
    axes,
    onAxesChange,
    message,
    onMessageChange,
    messagePlaceholder,
    protocol,
    scopeDisabled,
    scopeHint,
    vision,
}) => {
    const { t } = useTranslation();
    const [advancedOpen, setAdvancedOpen] = useState(false);

    const set = (patch: Partial<ProbeAxes>) => onAxesChange({ ...axes, ...patch });

    // Protocol options as actually rendered (falls back to a single-item list
    // while the target is still resolving). Short labels exist to fit three
    // buttons in the rail; with only one button there's no room pressure, so
    // show the full protocol name instead of an abbreviation nobody needs.
    const protocolOptions = protocol.options.length ? protocol.options : [protocol.value];

    return (
        <Stack spacing={1.5}>
            {/* Primary axes: what 80% of probes touch. */}
            <Axis label={t('probe.shape')} hint={t('probe.shapeHint')}>
                <ExclusiveToggle
                    value={axes.stream ? 'stream' : 'nonstream'}
                    onChange={(v) => set({ stream: v === 'stream' })}
                    options={[
                        { value: 'nonstream', label: t('probe.nonstream') },
                        { value: 'stream', label: t('probe.stream') },
                    ]}
                />
            </Axis>

            <Axis label={t('probe.scope')} hint={scopeHint}>
                <ExclusiveToggle
                    value={axes.direct ? 'direct' : 'tb'}
                    onChange={(v) => set({ direct: v === 'direct' })}
                    options={[
                        { value: 'tb', label: t('probe.throughTB') },
                        { value: 'direct', label: t('probe.direct') },
                    ]}
                    disabled={scopeDisabled}
                />
            </Axis>


            {/* Everything else is advanced — collapsed out of the way until asked for. */}
            <Box>
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 0.5,
                        cursor: 'pointer',
                        color: 'text.secondary',
                        '&:hover': { color: 'text.primary' },
                    }}
                    onClick={() => setAdvancedOpen(!advancedOpen)}
                >
                    <Typography variant="caption" sx={{ fontWeight: 500 }}>
                        {t('probe.advanced')}
                    </Typography>
                    {advancedOpen ? <ExpandLessIcon sx={{ fontSize: 16 }} /> : <ExpandMoreIcon sx={{ fontSize: 16 }} />}
                </Box>
                <Collapse in={advancedOpen}>
                    <Stack spacing={1.5} sx={{ mt: 0.5 }}>
                        {/* Parameters: real, independently-valued request fields. Turning
                            one doesn't inject or remove content — it configures how the
                            request already being sent gets built (.design/bench.md §1). */}
                        <AxisGroup label={t('probe.groupParameters', { defaultValue: 'Parameters' })}>
                            {/* Thinking as a stepped control bar: the effort is a ladder, and a
                                marked slider reads as one knob instead of four buttons. End-mark
                                labels center on their ticks and would stick out of the rail, so
                                the slider is inset and the wrapper clips the rest. */}
                            <Axis label={t('probe.thinking')} hint={t('probe.thinkingHint')}>
                                <ThinkingSlider value={axes.thinking} onChange={(v) => set({ thinking: v })} />
                            </Axis>

                            <Axis
                                label={t('probe.protocol')}
                                hint={
                                    protocol.locked || protocol.disabled
                                        ? protocol.lockHint
                                        : `${PROTOCOL_META[protocol.value]?.full || ''} · ${t('probe.protocolHint')}`
                                }
                            >
                                <ExclusiveToggle
                                    value={protocol.value}
                                    onChange={(v) => set({ protocol: v as ProbeProtocol })}
                                    options={protocolOptions.map((p) => ({
                                        value: p,
                                        label: (protocolOptions.length === 1 ? PROTOCOL_META[p]?.full : PROTOCOL_META[p]?.short) || p,
                                    }))}
                                    disabled={protocol.locked || protocol.disabled}
                                />
                            </Axis>
                        </AxisGroup>

                        {/* Content: a fixed, unparametrized blob toggled on/off. There's no
                            "which tool" or "which image" dial — the same canned content
                            Bench's Templates menu offers, just body-fragment-sized instead
                            of whole-body (.design/bench.md §1). */}
                        <AxisGroup label={t('probe.groupContent', { defaultValue: 'Content' })}>
                            <Axis label={t('probe.tool')} hint={t('probe.toolHint')}>
                                <ExclusiveToggle
                                    value={axes.tool ? 'on' : 'off'}
                                    onChange={(v) => set({ tool: v === 'on' })}
                                    options={[
                                        { value: 'off', label: t('probe.toolOff') },
                                        { value: 'on', label: t('probe.toolOn') },
                                    ]}
                                />
                            </Axis>

                            {/* Vision: does this route actually deliver images? 'User' puts the
                                probe image in the user message; 'Tool' returns it from a synthetic
                                tool round — the two channels that fail independently (issue #1606). */}
                            <Axis label={t('probe.vision')} hint={vision.disabled ? vision.hint : t('probe.visionHint')}>
                                <ExclusiveToggle
                                    value={axes.vision}
                                    onChange={(v) => set({ vision: v })}
                                    options={[
                                        { value: 'none', label: t('probe.visionNone') },
                                        { value: 'user', label: t('probe.visionUser') },
                                        { value: 'tool', label: t('probe.visionTool') },
                                    ]}
                                    disabled={vision.disabled}
                                />
                            </Axis>

                            {/* Message override lives here — the default per-tool message is
                                right for most probes; custom text is an explicit choice. */}
                            <Axis label={t('probe.message')} hint={t('probe.messageHint')}>
                                <TextField
                                    size="small"
                                    value={message}
                                    onChange={(e) => onMessageChange(e.target.value)}
                                    placeholder={messagePlaceholder}
                                    multiline
                                    maxRows={4}
                                    slotProps={{
                                        htmlInput: { sx: { fontSize: '0.78rem' } },
                                        input: { sx: { py: 0.65 } },
                                    }}
                                    sx={{ width: '100%' }}
                                />
                            </Axis>
                        </AxisGroup>
                    </Stack>
                </Collapse>
            </Box>
        </Stack>
    );
};

export default ProbeControls;
