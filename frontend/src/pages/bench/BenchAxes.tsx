import React, { useMemo } from 'react';
import { Stack } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Provider } from '@/types/provider';
import type { ProbeProtocol, ProbeRouting } from '@/types/probe';
import { Axis, AxisGroup, ExclusiveToggle, ThinkingSlider, PROTOCOL_META } from '@/components/probe/AxisPrimitives';
import { protocolAvailability, scopeAvailable, visionAvailable, type ProbeAxes } from '@/components/probe/probeConfig';
import { ruleProtocolForScenario } from '@/components/probe/ResultSections';
import type { BenchTarget } from './benchLink';

// BenchAxes: every probe axis resident, no Advanced fold — the page exists
// so that all knobs are visible and composable (.design/bench.md §2). Not a
// reuse of the probe dialog's layout: Compose reads top to bottom as one
// sequence — Protocol (the coordinate system) → Request mode (a formal
// preset/custom choice, not a door you stumble on) → Scope (transport) →
// Parameters → Presets — instead of Probe's frequency-ordered Shape/Scope-
// then-Advanced (.design/bench.md §1 "四种归类" and the redesign note).

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
        const isRule = target?.kind === 'rule';
        const avail = protocolAvailability(provider);
        const protocol = isRule
            ? (() => {
                  const value = ruleProtocolForScenario(target.scenario);
                  return { value, options: [value], locked: true, disabled: false, lockHint: t('probe.protocolLockedRule') };
              })()
            : provider?.api_style === 'google'
              ? { value: axes.protocol, options: [], locked: true, disabled: true, lockHint: t('probe.protocolGoogle') }
              : { value: axes.protocol, options: avail.options, locked: avail.locked, disabled: false, lockHint: t('probe.protocolLockedProvider') };
        const scopeDisabled = !target || !scopeAvailable(target.kind);
        const visionDisabled = !visionAvailable(provider);
        return {
            protocol,
            scopeDisabled,
            scopeHint: scopeDisabled ? t('probe.scopeRuleLocked') : t('probe.scopeHint'),
            visionDisabled,
            visionHint: visionDisabled ? t('probe.visionGoogle') : t('probe.visionHint'),
        };
    }, [target, provider, axes.protocol, t]);
}

// Scope is one axis with three values, reduced per target (like Protocol):
// a rule target chooses between the full production chain (TB matches the
// rule from the request model) and pinning the rule; a provider target
// chooses between going through TB (service pinned, middleware intact) and
// calling the upstream directly. Same question — "how much of TB is in the
// path?" — so it stays one control (.design/bench.md §3).
type ScopeValue = 'natural' | 'pinned' | 'tb' | 'direct';

export const BenchAxes: React.FC<{
    axes: ProbeAxes;
    onChange: (axes: ProbeAxes) => void;
    availability: AxisAvailability;
    targetKind: 'rule' | 'provider' | null;
    routing: ProbeRouting;
    onRoutingChange: (routing: ProbeRouting) => void;
    mode: RequestMode;
    onModeChange: (mode: RequestMode) => void;
    /** Protocols a hand-written request may be in — only consulted in custom mode (a rule's scenario family, or whatever the provider speaks). */
    customProtocolOptions: ProbeProtocol[];
    /** The unified Protocol control's current value, already resolved for whichever mode is active. */
    protocolValue: ProbeProtocol;
    onProtocolChange: (protocol: ProbeProtocol) => void;
}> = ({ axes, onChange, availability, targetKind, routing, onRoutingChange, mode, onModeChange, customProtocolOptions, protocolValue, onProtocolChange }) => {
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

    const isRule = targetKind === 'rule';
    const scopeValue: ScopeValue = isRule ? routing : axes.direct ? 'direct' : 'tb';
    const scopeOptions: { value: ScopeValue; label: string }[] = isRule
        ? [
              { value: 'natural', label: t('bench.scopeFull', { defaultValue: 'Full chain' }) },
              { value: 'pinned', label: t('bench.scopePinned', { defaultValue: 'Pinned rule' }) },
          ]
        : [
              { value: 'tb', label: t('probe.throughTB') },
              { value: 'direct', label: t('probe.direct') },
          ];
    const scopeHint = isRule
        ? routing === 'pinned'
            ? t('bench.scopePinnedHint', { defaultValue: 'X-Tingly-Probe-Rule forces this rule; only rule matching is skipped. Use it when the request model collides with another rule or the rule is inactive.' })
            : t('bench.scopeFullHint', { defaultValue: 'Exactly the production path: TB matches the rule from the request model. The Journey shows which rule actually matched.' })
        : availability.scopeHint;

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

            <Axis label={t('probe.scope')} hint={scopeHint}>
                <ExclusiveToggle<ScopeValue>
                    value={scopeValue}
                    onChange={(v) => {
                        if (v === 'natural' || v === 'pinned') onRoutingChange(v);
                        else set({ direct: v === 'direct' });
                    }}
                    options={scopeOptions}
                    disabled={targetKind === null}
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

            {/* Presets last: a fixed, unparametrized blob toggled on/off — the
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
        </Stack>
    );
};
