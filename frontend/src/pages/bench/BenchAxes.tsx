import React, { useMemo } from 'react';
import { HelpOutline } from '@/components/icons';
import { Box, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Provider } from '@/types/provider';
import type { ProbeProtocol } from '@/types/probe';
import type { FlagSpec, RuleFlags } from '@/components/RoutingGraphTypes';
import RulePluginsCard from '@/components/rule-card/RulePluginsCard';
import { Axis, AxisGroup, ExclusiveToggle, ThinkingSlider, PROTOCOL_META } from '@/components/probe/AxisPrimitives';
import { protocolAvailability, visionAvailable, type ProbeAxes } from '@/components/probe/probeConfig';
import type { BenchTarget } from './benchState';

// BenchAxes: every probe axis resident, no Advanced fold — the page exists
// so that all knobs are visible and composable (.design/bench.md §2). Not a
// reuse of the probe dialog's layout: Compose reads top to bottom as one
// sequence — Protocol (the coordinate system) → Request mode (a formal
// preset/custom choice, not a door you stumble on) → Scope (transport) →
// Parameters → Presets → Plugins — instead of Probe's frequency-ordered
// Shape/Scope-then-Advanced (.design/bench.md §1 "四种归类" and the redesign
// note). Plugins stays its own final group because it configures TB's handling,
// not the request body.

export type RequestMode = 'preset' | 'custom';

export interface AxisAvailability {
    protocol: { value: ProbeProtocol | ''; options: ProbeProtocol[]; locked: boolean; disabled: boolean; lockHint?: string };
    scopeDisabled: boolean;
    scopeHint: string;
    visionDisabled: boolean;
    visionHint: string;
}

export function useAxisAvailability(target: BenchTarget | null, provider: Provider | null, axes: ProbeAxes): AxisAvailability {
    const { t } = useTranslation();
    return useMemo(() => {
        const avail = protocolAvailability(provider);
        const protocol =
            provider?.api_style === 'google'
                ? { value: axes.protocol, options: [], locked: true, disabled: true, lockHint: t('probe.protocolGoogle') }
                : { value: axes.protocol, options: avail.options, locked: avail.locked, disabled: false, lockHint: t('probe.protocolLockedProvider') };
        const visionDisabled = !visionAvailable(provider);
        return {
            protocol,
            scopeDisabled: !target,
            scopeHint: t('probe.scopeHint'),
            visionDisabled,
            visionHint: visionDisabled ? t('probe.visionGoogle') : t('probe.visionHint'),
        };
    }, [target, provider, axes.protocol, t]);
}

export const BenchAxes: React.FC<{
    axes: ProbeAxes;
    onChange: (axes: ProbeAxes) => void;
    availability: AxisAvailability;
    mode: RequestMode;
    onModeChange: (mode: RequestMode) => void;
    /** Protocols a hand-written request may be in — only consulted in custom mode (whatever the provider speaks). */
    customProtocolOptions: ProbeProtocol[];
    /** The unified Protocol control's current value, already resolved for whichever mode is active. */
    protocolValue: ProbeProtocol;
    onProtocolChange: (protocol: ProbeProtocol) => void;
    pluginFlags: RuleFlags;
    pluginRegistry: FlagSpec[];
    pluginsActive: boolean;
    onOpenPlugins: () => void;
}> = ({
    axes,
    onChange,
    availability,
    mode,
    onModeChange,
    customProtocolOptions,
    protocolValue,
    onProtocolChange,
    pluginFlags,
    pluginRegistry,
    pluginsActive,
    onOpenPlugins,
}) => {
    const { t } = useTranslation();
    const isCustom = mode === 'custom';
    const set = (patch: Partial<ProbeAxes>) => onChange({ ...axes, ...patch });
    const { protocol: presetProtocol } = availability;
    const presetProtocolOptions = presetProtocol.options.length ? presetProtocol.options : presetProtocol.value ? [presetProtocol.value] : [];
    const protocolOptions = isCustom ? customProtocolOptions : presetProtocolOptions;
    const protocolLocked = isCustom ? protocolOptions.length <= 1 : presetProtocol.locked || presetProtocol.disabled;
    const protocolHint = isCustom
        ? t('bench.protocolCustomHint', { defaultValue: 'Chosen once, at the start of the custom request — switching it replaces the body with the new protocol’s starting template.' })
        : presetProtocol.locked || presetProtocol.disabled
          ? presetProtocol.lockHint
          : `${PROTOCOL_META[protocolValue]?.full || ''} · ${t('probe.protocolHint')}`;

    return (
        <Stack spacing={1.5}>
            {/* Protocol first: the coordinate system everything below is
                expressed in — not a peer parameter (.design/bench.md §1). */}
            <Axis label={t('probe.protocol')} hint={protocolHint}>
                <ExclusiveToggle
                    value={protocolValue}
                    onChange={onProtocolChange}
                    options={protocolOptions.map((p) => ({
                        value: p,
                        label: (protocolOptions.length === 1 ? PROTOCOL_META[p]?.full : PROTOCOL_META[p]?.short) || p,
                    }))}
                    disabled={protocolLocked || protocolOptions.length === 0}
                />
            </Axis>

            {/* Request mode second: a formal choice, not a door discovered
                halfway down a knob list — everything from here down changes
                meaning depending on it (.design/bench.md §1, §6.3). */}
            <Axis label={t('bench.requestMode', { defaultValue: 'Request mode' })} hint={t('bench.requestModeHint', { defaultValue: 'Preset: the probe builds it from the knobs below. Custom: you write it — Parameters and Presets stop applying.' })}>
                <ExclusiveToggle<RequestMode>
                    value={mode}
                    onChange={onModeChange}
                    options={[
                        { value: 'preset', label: t('bench.modePreset', { defaultValue: 'Preset' }) },
                        { value: 'custom', label: t('bench.modeCustom', { defaultValue: 'Custom' }) },
                    ]}
                />
            </Axis>

            <Axis label={t('probe.scope')} hint={availability.scopeHint}>
                <ExclusiveToggle
                    value={axes.direct ? 'direct' : 'tb'}
                    onChange={(v) => set({ direct: v === 'direct' })}
                    options={[
                        { value: 'tb', label: t('probe.throughTB') },
                        { value: 'direct', label: t('probe.direct') },
                    ]}
                    disabled={availability.scopeDisabled}
                />
            </Axis>

            {/* Parameters: real, independently-valued request fields — turning
                one doesn't inject or remove content (.design/bench.md §1).
                Stream applies in both modes (a real body field either way);
                Thinking only shapes the preset builder, so it's not shown
                once the body is yours to write. */}
            <AxisGroup label={t('probe.groupParameters', { defaultValue: 'Parameters' })}>
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
                {!isCustom && (
                    <Axis label={t('probe.thinking')} hint={t('probe.thinkingHint')}>
                        <ThinkingSlider value={axes.thinking} onChange={(v) => set({ thinking: v })} />
                    </Axis>
                )}
            </AxisGroup>

            {/* Presets: fixed, unparametrized blobs toggled on/off — the
                same canned content Templates offer in the custom request
                editor, just body-fragment-sized (.design/bench.md §1). Only
                meaningful in preset mode — a custom request owns its own
                content, this isn't disabled for it, it's simply not
                rendered (.design/bench.md §6.3). */}
            {!isCustom && (
                <AxisGroup label={t('bench.groupPresets', { defaultValue: 'Presets' })}>
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
                    <Axis label={t('probe.vision')} hint={availability.visionHint}>
                        <ExclusiveToggle
                            value={axes.vision}
                            onChange={(v) => set({ vision: v })}
                            options={[
                                { value: 'none', label: t('probe.visionNone') },
                                { value: 'user', label: t('probe.visionUser') },
                                { value: 'tool', label: t('probe.visionTool') },
                            ]}
                            disabled={availability.visionDisabled}
                        />
                    </Axis>
                </AxisGroup>
            )}

            {/* Plugins is transport configuration rather than request content.
                It remains the last, independent Compose group in both request
                modes, and reuses the routing graph's exact summary card. */}
            <AxisGroup
                label={
                    <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
                        {t('bench.plugins', { defaultValue: 'Plugins' })}
                        <Tooltip title={t('bench.pluginsQ', { defaultValue: 'flag overlay · this request only' })}>
                            <HelpOutline tabIndex={0} sx={{ fontSize: 12, color: 'text.disabled', cursor: 'help' }} />
                        </Tooltip>
                    </Box>
                }
            >
                {!pluginsActive && (
                    <Typography variant="caption" sx={{ color: 'warning.main', display: 'block', lineHeight: 1.4, mb: 1 }}>
                        {t('bench.pluginsDirect', { defaultValue: 'Direct bypasses TB. Flags are TB middleware, so they cannot apply here — switch Scope to “Through TB” to test flags.' })}
                    </Typography>
                )}
                <Box sx={{ display: 'flex', justifyContent: 'center', pointerEvents: pluginsActive ? 'auto' : 'none' }}>
                    <RulePluginsCard
                        flags={pluginFlags}
                        registry={pluginRegistry}
                        active={pluginsActive}
                        onOpenCatalog={onOpenPlugins}
                    />
                </Box>
            </AxisGroup>
        </Stack>
    );
};
