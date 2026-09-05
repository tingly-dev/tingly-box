import React, { memo } from 'react';
import { Box, Typography, Tooltip, ToggleButton, ToggleButtonGroup, Slider } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { toggleButtonGroupStyle } from '@/styles/toggleStyles';
import type { ProbeThinking, ProbeProtocol } from '@/types/probe';

// AxisPrimitives: the instrument-panel vocabulary shared by the probe dialog's
// control rail and the Playground's compose column — one axis logic, two
// layouts (.design/playground.md §4). Axis = label + control row,
// ExclusiveToggle = single-select group, ThinkingSlider = the effort ladder.

// Rail-short label + full name (hover tooltip) per protocol — one map so the
// two never drift out of sync.
export const PROTOCOL_META: Partial<Record<ProbeProtocol | '', { short: string; full: string }>> = {
    openai_chat: { short: 'O Chat', full: 'OpenAI Chat Completions' },
    openai_responses: { short: 'O Resp.', full: 'OpenAI Responses API' },
    anthropic_v1: { short: 'A', full: 'Anthropic Messages' },
};

// THINKING_LADDER orders the effort steps for the slider control bar. Mirrors
// the rule flag's thinking_effort options (.design/rule-flags.md) minus the
// "By Client"/"Off" states, which don't apply to a probe that always builds
// its own request.
export const THINKING_LADDER: ProbeThinking[] = ['none', 'low', 'medium', 'high', 'max'];

// Full-width group with equal-width options — the alignment primitive of the
// rail. Minimal deltas over the shared theme style (width + flex); all
// visual styling (padding, colors, selected state, shape) stays themed.
export const railGroupStyle = {
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

export const Axis = memo(({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) => {
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
export function ExclusiveToggle<T extends string>({
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


// ThinkingSlider: the effort ladder as one stepped control bar. End-mark
// labels center on their ticks and would stick out of the rail, so the
// slider is inset and the wrapper clips the rest.
export const ThinkingSlider: React.FC<{ value: ProbeThinking; onChange: (v: ProbeThinking) => void }> = ({ value, onChange }) => {
    const { t } = useTranslation();
    return (
        <Box sx={{ px: 1.25, overflowX: 'hidden' }}>
            <Slider
                size="small"
                value={THINKING_LADDER.indexOf(value)}
                min={0}
                max={THINKING_LADDER.length - 1}
                step={null}
                marks={THINKING_LADDER.map((lvl, i) => ({
                    value: i,
                    label: t(`probe.thinking${lvl.charAt(0).toUpperCase()}${lvl.slice(1)}`),
                }))}
                onChange={(_, v) => onChange(THINKING_LADDER[v as number])}
                sx={{
                    '& .MuiSlider-markLabel': { fontSize: '0.7rem' },
                }}
            />
        </Box>
    );
};
