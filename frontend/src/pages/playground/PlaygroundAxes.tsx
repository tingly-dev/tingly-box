import React, { useMemo } from 'react';
import { Stack } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Provider } from '@/types/provider';
import type { ProbeProtocol, ProbeRouting } from '@/types/probe';
import { Axis, ExclusiveToggle, ThinkingSlider, PROTOCOL_META } from '@/components/probe/AxisPrimitives';
import { protocolAvailability, scopeAvailable, visionAvailable, type ProbeAxes } from '@/components/probe/probeConfig';
import { ruleProtocolForScenario } from '@/components/probe/ResultSections';
import type { PlaygroundTarget } from './playgroundLink';

// PlaygroundAxes: every probe axis resident, no Advanced fold — the page
// exists so that all knobs are visible and composable (.design/playground.md
// §2). Same axis logic as the probe dialog (probeConfig reducers), different
// layout.

export interface AxisAvailability {
    protocol: { value: ProbeProtocol | ''; options: ProbeProtocol[]; locked: boolean; disabled: boolean; lockHint?: string };
    scopeDisabled: boolean;
    scopeHint: string;
    visionDisabled: boolean;
    visionHint: string;
}

export function useAxisAvailability(target: PlaygroundTarget | null, provider: Provider | null, axes: ProbeAxes): AxisAvailability {
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
// path?" — so it stays one control (.design/playground.md §3).
export type ScopeValue = 'natural' | 'pinned' | 'tb' | 'direct';

export const PlaygroundAxes: React.FC<{
    axes: ProbeAxes;
    onChange: (axes: ProbeAxes) => void;
    availability: AxisAvailability;
    targetKind: 'rule' | 'provider' | null;
    routing: ProbeRouting;
    onRoutingChange: (routing: ProbeRouting) => void;
}> = ({ axes, onChange, availability, targetKind, routing, onRoutingChange }) => {
    const { t } = useTranslation();
    const set = (patch: Partial<ProbeAxes>) => onChange({ ...axes, ...patch });
    const { protocol } = availability;
    const protocolOptions = protocol.options.length ? protocol.options : protocol.value ? [protocol.value] : [];
    const isRule = targetKind === 'rule';
    const scopeValue: ScopeValue = isRule ? routing : axes.direct ? 'direct' : 'tb';
    const scopeOptions: { value: ScopeValue; label: string }[] = isRule
        ? [
              { value: 'natural', label: t('playground.scopeFull', { defaultValue: 'Full chain' }) },
              { value: 'pinned', label: t('playground.scopePinned', { defaultValue: 'Pinned rule' }) },
          ]
        : [
              { value: 'tb', label: t('probe.throughTB') },
              { value: 'direct', label: t('probe.direct') },
          ];
    const scopeHint = isRule
        ? routing === 'pinned'
            ? t('playground.scopePinnedHint', { defaultValue: 'X-Tingly-Probe-Rule forces this rule; only rule matching is skipped. Use it when the request model collides with another rule or the rule is inactive.' })
            : t('playground.scopeFullHint', { defaultValue: 'Exactly the production path: TB matches the rule from the request model. The Journey shows which rule actually matched.' })
        : availability.scopeHint;

    return (
        <Stack spacing={1.5}>
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
                    disabled={protocol.locked || protocol.disabled || protocolOptions.length === 0}
                />
            </Axis>
        </Stack>
    );
};

export default PlaygroundAxes;
