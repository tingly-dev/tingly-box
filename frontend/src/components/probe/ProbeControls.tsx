import React, { memo, useState } from 'react';
import { Box, Typography, Tooltip, ToggleButton, ToggleButtonGroup, TextField, Stack, Collapse, Slider } from '@mui/material';
import { ExpandMore as ExpandMoreIcon, ExpandLess as ExpandLessIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import { toggleButtonGroupStyle } from '@/styles/toggleStyles';
import type { ProbeThinking, ProbeProtocol } from '@/types/probe';
import type { ProbeAxes } from './probeConfig';

// ProbeControls renders the control rail: orthogonal axes stacked vertically,
// one label + control pair per row. Groups fill the rail width and every
// option button is equal-width, so the rail reads as an aligned instrument
// panel instead of ragged inline chips. Adding a future axis = one more row
// here (and a field on ProbeAxes).

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

// Rail-short label + full name (hover tooltip) per protocol — one map so the
// two never drift out of sync.
const PROTOCOL_META: Partial<Record<ProbeProtocol | '', { short: string; full: string }>> = {
    openai_chat: { short: 'O Chat', full: 'OpenAI Chat Completions' },
    openai_responses: { short: 'O Resp.', full: 'OpenAI Responses API' },
    anthropic_v1: { short: 'A', full: 'Anthropic Messages' },
};

// THINKING_LADDER orders the effort steps for the slider control bar. Mirrors
// the rule flag's thinking_effort options (.design/rule-flags.md) minus the
// "By Client"/"Off" states, which don't apply to a probe that always builds
// its own request.
const THINKING_LADDER: ProbeThinking[] = ['none', 'low', 'medium', 'high', 'max'];

// Full-width group with equal-width options — the alignment primitive of the
// rail. Minimal deltas over the shared theme style (width + flex); all
// visual styling (padding, colors, selected state, shape) stays themed.
const railGroupStyle = {
    ...toggleButtonGroupStyle,
    width: '100%',
    '& .MuiToggleButton-root': {
        ...((toggleButtonGroupStyle as Record<string, any>)['& .MuiToggleButton-root'] ?? {}),
        flex: 1,
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
};

const Axis = memo(({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) => {
    const head = (
        <Typography
            variant="caption"
            sx={{
                color: 'text.secondary',
                cursor: hint ? 'help' : 'default',
                fontWeight: 500,
            }}
        >
            {label}
        </Typography>
    );
    return (
        <Box sx={{ minWidth: 0 }}>
            {hint ? <Tooltip title={hint}>{head}</Tooltip> : head}
            <Box sx={{ mt: 0.5 }}>{children}</Box>
        </Box>
    );
});

// ExclusiveToggle: the rail's one two-state-or-more control primitive — every
// axis (shape, scope, tool, protocol) is a single-select ToggleButtonGroup
// with the same rail styling and the same "ignore the null-deselect click"
// guard. Centralizing it here means railGroupStyle is wired once, not once
// per axis.
function ExclusiveToggle<T extends string>({
    value,
    onChange,
    options,
    disabled,
}: {
    value: T;
    onChange: (value: T) => void;
    options: { value: T; label: React.ReactNode }[];
    disabled?: boolean;
}) {
    return (
        <ToggleButtonGroup
            size="small"
            exclusive
            value={value}
            onChange={(_, v) => v && onChange(v)}
            sx={railGroupStyle}
            disabled={disabled}
        >
            {options.map((o) => (
                <ToggleButton key={o.value} value={o.value}>
                    {o.label}
                </ToggleButton>
            ))}
        </ToggleButtonGroup>
    );
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

                        {/* Thinking as a stepped control bar: the effort is a ladder, and a
                            marked slider reads as one knob instead of four buttons. End-mark
                            labels center on their ticks and would stick out of the rail, so
                            the slider is inset and the wrapper clips the rest. */}
                        <Axis label={t('probe.thinking')} hint={t('probe.thinkingHint')}>
                            <Box sx={{ px: 1.25, overflowX: 'hidden' }}>
                                <Slider
                                    size="small"
                                    value={THINKING_LADDER.indexOf(axes.thinking)}
                                    min={0}
                                    max={THINKING_LADDER.length - 1}
                                    step={null}
                                    marks={THINKING_LADDER.map((lvl, i) => ({
                                        value: i,
                                        label: t(`probe.thinking${lvl.charAt(0).toUpperCase()}${lvl.slice(1)}`),
                                    }))}
                                    onChange={(_, v) => set({ thinking: THINKING_LADDER[v as number] })}
                                    sx={{
                                        '& .MuiSlider-markLabel': { fontSize: '0.7rem' },
                                    }}
                                />
                            </Box>
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
                    </Stack>
                </Collapse>
            </Box>
        </Stack>
    );
};

export default ProbeControls;
