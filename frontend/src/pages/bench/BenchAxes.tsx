import React, { useMemo, useState } from 'react';
import { ExpandMore, HelpOutline } from '@/components/icons';
import { Box, Button, ListItemText, Menu, MenuItem, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Provider } from '@/types/provider';
import type { ProbeProtocol } from '@/types/probe';
import type { FlagSpec, RuleFlags } from '@/components/RoutingGraphTypes';
import RulePluginsCard from '@/components/rule-card/RulePluginsCard';
import { Axis, AxisGroup, ExclusiveToggle, ThinkingSlider, PROTOCOL_META } from '@/components/probe/AxisPrimitives';
import { protocolAvailability, visionAvailable, type ProbeAxes } from '@/components/probe/probeConfig';
import { templatesForProtocol, MESSAGE_ID, type ContentTemplate } from './contentOptions';
import type { BenchTarget } from './benchState';

// BenchAxes: every probe axis resident, no Advanced fold — the page exists
// so that all knobs are visible and composable (.design/bench.md §2). Not a
// reuse of the probe dialog's layout: Compose reads top to bottom as one
// sequence — Protocol (the coordinate system) → Content (who authors the
// body — Message's fragment-level knobs, or a whole-body preset for this
// protocol) → Scope (transport) → Parameters → Presets → Plugins — instead
// of Probe's frequency-ordered Shape/Scope-then-Advanced (.design/bench.md
// §1 "四种归类" and the redesign note). Plugins stays its own final group
// because it configures TB's handling, not the request body.
//
// Content replaces the old Preset/Custom mode toggle: both were always the
// same kind of object — a content preset — just at fragment granularity
// (Message + Tool/Vision) vs whole-body granularity (Templates). Splitting
// them into a mode gate you had to flip before the rest of Compose meant
// anything was the mode-picker antipattern (ux-principles #2); Content is
// one menu, scoped to the current Protocol (a Tool round-trip body isn't
// interchangeable across wire protocols), that always sets the whole body's
// one author (.design/bench.md §6 — never a partial merge).

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

// ContentMenu: the single button+menu that picks who authors the body —
// Message (the fragment-level composer below) or one of this protocol's
// whole-body templates. Replaces the old Preset/Custom ExclusiveToggle and
// RequestEditor's separate "Change starting point" menu — one mechanism,
// not two, because they always listed the same kind of thing (.design/bench.md §1).
const ContentMenu: React.FC<{
    protocol: ProbeProtocol;
    /** MESSAGE_ID, a matched template id, or null when the body no longer matches anything (hand-edited). */
    contentId: string | null;
    onSelect: (id: string) => void;
}> = ({ protocol, contentId, onSelect }) => {
    const { t } = useTranslation();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const templates = useMemo(() => templatesForProtocol(protocol), [protocol]);
    // contentId is only ever null while raw is active (BenchPage.tsx never
    // produces MESSAGE_ID's null-alias case) — a hand-edited body that no
    // longer matches any template. Labeling that "Message" would be a lie
    // (you're still editing JSON, just not any of the named starting
    // points), so it gets its own label instead of falling back to Message.
    const label =
        contentId === MESSAGE_ID
            ? t('bench.template.message', { defaultValue: 'Message' })
            : contentId === null
              ? t('bench.template.custom', { defaultValue: 'Custom' })
              : t(`bench.template.${contentId}`);
    const pick = (id: string) => {
        setAnchor(null);
        onSelect(id);
    };
    const item = (id: string, primary: string, secondary: string) => (
        <MenuItem key={id} selected={id === contentId} onClick={() => pick(id)} sx={{ maxWidth: 380, whiteSpace: 'normal' }}>
            <ListItemText
                primary={primary}
                secondary={secondary}
                slotProps={{ primary: { sx: { fontSize: '0.85rem', fontWeight: 600 } }, secondary: { sx: { fontSize: '0.72rem' } } }}
            />
        </MenuItem>
    );
    return (
        <>
            <Button
                fullWidth
                size="small"
                variant="outlined"
                onClick={(e) => setAnchor(e.currentTarget)}
                sx={{ justifyContent: 'space-between', textTransform: 'none', fontWeight: 400, py: 0.75 }}
                endIcon={<ExpandMore sx={{ fontSize: 18 }} />}
            >
                <Typography variant="body2" noWrap sx={{ flex: 1, textAlign: 'left' }}>
                    {label}
                </Typography>
            </Button>
            <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)} slotProps={{ paper: { sx: { minWidth: 260 } } }}>
                {item(MESSAGE_ID, t('bench.template.message', { defaultValue: 'Message' }), t('bench.template.messageDesc', { defaultValue: 'One message, shaped by the Tool / Vision / Thinking knobs below — the probe itself, materialized.' }))}
                {templates.map((tpl: ContentTemplate) => item(tpl.id, t(`bench.template.${tpl.id}`), t(`bench.template.${tpl.id}Desc`)))}
            </Menu>
        </>
    );
};

export const BenchAxes: React.FC<{
    axes: ProbeAxes;
    onChange: (axes: ProbeAxes) => void;
    availability: AxisAvailability;
    /** Whether the body currently has an author other than the axes below (raw request active). */
    isCustom: boolean;
    /** MESSAGE_ID, a matched template id, or null (hand-edited body) — drives the Content button's label. */
    contentId: string | null;
    onSelectContent: (id: string) => void;
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
    isCustom,
    contentId,
    onSelectContent,
    customProtocolOptions,
    protocolValue,
    onProtocolChange,
    pluginFlags,
    pluginRegistry,
    pluginsActive,
    onOpenPlugins,
}) => {
    const { t } = useTranslation();
    const set = (patch: Partial<ProbeAxes>) => onChange({ ...axes, ...patch });
    const inactiveHint = t('bench.axesInactive', { defaultValue: "Reference only — this request is your own JSON now (Content: not Message), so these values aren't sent. Set them directly in the body." });
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

            {/* Content second: who authors the body — Message (the fragment-
                level knobs below) or one of this protocol's whole-body
                templates. One menu instead of a mode gate you had to flip
                before Compose meant anything (.design/bench.md §1, §6.3). */}
            <Axis label={t('bench.content', { defaultValue: 'Content' })} hint={t('bench.contentHint', { defaultValue: "What the body is made of, for this protocol. Message keeps the knobs below live; anything else hands you a whole body to edit — either way there's exactly one author." })}>
                <ContentMenu protocol={protocolValue} contentId={contentId} onSelect={onSelectContent} />
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
                Stream applies regardless of Content (a real body field
                either way); Thinking only shapes the Message builder, so
                once Content is anything else it goes inert — shown, not
                hidden, so the field and its value ladder stay in view as a
                reference while you write the equivalent by hand (the
                original ask this redesign starts from: axes are easy to
                forget how to fill once they're not on screen). */}
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
                <Axis label={t('probe.thinking')} hint={isCustom ? inactiveHint : t('probe.thinkingHint')}>
                    <ThinkingSlider value={axes.thinking} onChange={(v) => set({ thinking: v })} disabled={isCustom} />
                </Axis>
            </AxisGroup>

            {/* Presets: fixed, unparametrized blobs toggled on/off — the
                same canned content Content's Templates offer, just
                body-fragment-sized (.design/bench.md §1). Same as Thinking:
                inert (not unmounted) once Content picks a whole body, so the
                toggle and its two states stay visible as a reminder of what
                "tool" / "vision" actually mean in the body you're writing. */}
            <AxisGroup label={t('bench.groupPresets', { defaultValue: 'Presets' })}>
                <Axis label={t('probe.tool')} hint={isCustom ? inactiveHint : t('probe.toolHint')}>
                    <ExclusiveToggle
                        value={axes.tool ? 'on' : 'off'}
                        onChange={(v) => set({ tool: v === 'on' })}
                        options={[
                            { value: 'off', label: t('probe.toolOff') },
                            { value: 'on', label: t('probe.toolOn') },
                        ]}
                        disabled={isCustom}
                    />
                </Axis>
                <Axis label={t('probe.vision')} hint={isCustom ? inactiveHint : availability.visionHint}>
                    <ExclusiveToggle
                        value={axes.vision}
                        onChange={(v) => set({ vision: v })}
                        options={[
                            { value: 'none', label: t('probe.visionNone') },
                            { value: 'user', label: t('probe.visionUser') },
                            { value: 'tool', label: t('probe.visionTool') },
                        ]}
                        disabled={isCustom || availability.visionDisabled}
                    />
                </Axis>
            </AxisGroup>

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
