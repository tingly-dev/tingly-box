import React, { useMemo } from 'react';
import { Box, Button, Chip, IconButton, MenuItem, Select, Stack, Switch, TextField, Tooltip, Typography } from '@mui/material';
import { RestartAlt as ResetIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import type { FlagSpec } from '@/components/RoutingGraphTypes';

// PluginsPanel: the rule-flag overlay, registry-driven and three-state
// (.design/playground.md §5.4). Every row shows the resolved baseline for the
// target — a concrete value with its source, never the word "default" — and
// a highlighted row is one the request will override. Adding a flag to the
// backend registry needs zero changes here.

export type BaselineSource = 'rule' | 'scenario';

export interface FlagBaseline {
    values: Record<string, unknown>;
    sources: Record<string, BaselineSource>;
}

const CATEGORY_ORDER = ['app', 'request', 'request_openai', 'request_anthropic', 'response', 'reasoning', 'vision', 'routing'];
const EDITABLE_TYPES = new Set(['bool', 'string', 'int', 'enum']);

export function isFlagSet(spec: FlagSpec, value: unknown): boolean {
    switch (spec.type) {
        case 'bool': return !!value;
        case 'string': return typeof value === 'string' && value !== '';
        case 'int': return typeof value === 'number' && value > 0;
        case 'enum': return !!value && value !== (spec.options?.[0]?.value ?? '');
        case 'headers': return !!value && Object.keys(value as object).length > 0;
        case 'service_ref': { const r = value as { provider?: string; model?: string } | undefined; return !!(r?.provider && r?.model); }
        default: return false;
    }
}

export function formatFlagValue(spec: FlagSpec, value: unknown): string {
    switch (spec.type) {
        case 'bool': return value ? 'on' : 'off';
        case 'enum': {
            const v = (value as string) || '';
            const opt = spec.options?.find((o) => o.value === v) ?? spec.options?.[0];
            return opt?.label ?? (v || '—');
        }
        case 'int': return typeof value === 'number' && value > 0 ? String(value) : '0';
        case 'string': return (value as string) || '—';
        case 'headers': { const n = value ? Object.keys(value as object).length : 0; return n ? `${n} header${n > 1 ? 's' : ''}` : '—'; }
        case 'service_ref': { const r = value as { provider?: string; model?: string } | undefined; return r?.provider ? `${r.provider} / ${r.model}` : '—'; }
        default: return value === undefined ? '—' : String(value);
    }
}

export function groupRegistry(registry: FlagSpec[]): { category: string; specs: FlagSpec[] }[] {
    const groups = new Map<string, FlagSpec[]>();
    registry.forEach((spec) => {
        if (!groups.has(spec.category)) groups.set(spec.category, []);
        groups.get(spec.category)!.push(spec);
    });
    const ordered = CATEGORY_ORDER.filter((c) => groups.has(c));
    groups.forEach((_, c) => { if (!ordered.includes(c)) ordered.push(c); });
    return ordered.map((category) => ({ category, specs: groups.get(category) || [] }));
}

const FlagControl: React.FC<{ spec: FlagSpec; value: unknown; overridden: boolean; onSet: (v: unknown) => void }> = ({ spec, value, overridden, onSet }) => {
    const { t } = useTranslation();
    const muted = overridden ? {} : { color: 'text.secondary' };
    switch (spec.type) {
        case 'bool':
            return <Switch size="small" checked={!!value} onChange={(e) => onSet(e.target.checked)} slotProps={{ input: { 'aria-label': spec.key } }} />;
        case 'enum':
            return (
                <Select
                    size="small"
                    value={(value as string) || spec.options?.[0]?.value || ''}
                    onChange={(e) => onSet(e.target.value)}
                    sx={{ fontSize: '0.75rem', minWidth: 110, '& .MuiSelect-select': { py: 0.4 }, ...muted }}
                    inputProps={{ 'aria-label': spec.key }}
                >
                    {(spec.options ?? []).map((o) => (
                        <MenuItem key={o.value} value={o.value} sx={{ fontSize: '0.8rem' }}>{o.label}</MenuItem>
                    ))}
                </Select>
            );
        case 'int':
            return (
                <TextField
                    key={`${spec.key}-${overridden ? 'o' : 'i'}-${String(value)}`}
                    size="small"
                    type="number"
                    defaultValue={typeof value === 'number' ? value : 0}
                    onBlur={(e) => onSet(Math.max(0, Number(e.target.value) || 0))}
                    sx={{ width: 90 }}
                    slotProps={{ htmlInput: { min: 0, 'aria-label': spec.key, sx: { fontSize: '0.75rem', py: 0.5 } } }}
                />
            );
        case 'string':
            return (
                <TextField
                    key={`${spec.key}-${overridden ? 'o' : 'i'}-${String(value ?? '')}`}
                    size="small"
                    defaultValue={(value as string) ?? ''}
                    placeholder={spec.placeholder || '—'}
                    onBlur={(e) => { if (e.target.value !== ((value as string) ?? '')) onSet(e.target.value); }}
                    sx={{ width: 130 }}
                    slotProps={{ htmlInput: { 'aria-label': spec.key, sx: { fontSize: '0.75rem', py: 0.5, fontFamily: 'monospace' } } }}
                />
            );
        default:
            return (
                <Tooltip title={t('playground.notEditable', { defaultValue: 'This flag type is edited on the rule itself.' })}>
                    <Chip size="small" label={formatFlagValue(spec, value)} variant="outlined" sx={{ fontSize: '0.65rem', height: 20 }} />
                </Tooltip>
            );
    }
};

export const PluginsPanel: React.FC<{
    registry: FlagSpec[];
    loading?: boolean;
    baseline: FlagBaseline;
    overlay: Record<string, unknown>;
    onChange: (overlay: Record<string, unknown>) => void;
    disabled: boolean;
}> = ({ registry, loading, baseline, overlay, onChange, disabled }) => {
    const { t } = useTranslation();
    const groups = useMemo(() => groupRegistry(registry), [registry]);
    const count = Object.keys(overlay).length;

    const setFlag = (key: string, value: unknown) => onChange({ ...overlay, [key]: value });
    const resetFlag = (key: string) => {
        const next = { ...overlay };
        delete next[key];
        onChange(next);
    };

    return (
        <Stack spacing={1}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography variant="caption" sx={{ color: count ? 'text.primary' : 'text.secondary' }}>
                    {count
                        ? t('playground.overridden', { count, defaultValue: '{{count}} overridden' })
                        : t('playground.nothingOverridden', { defaultValue: 'nothing overridden — all inherited' })}
                </Typography>
                <Box sx={{ flex: 1 }} />
                <Button size="small" onClick={() => onChange({})} disabled={count === 0 || disabled} sx={{ minWidth: 0, fontSize: '0.72rem' }}>
                    {t('playground.resetAll', { defaultValue: 'Reset all' })}
                </Button>
            </Box>

            {disabled && (
                <Typography variant="caption" sx={{ color: 'warning.main', display: 'block', lineHeight: 1.4 }}>
                    {t('playground.pluginsDirect', { defaultValue: 'Direct bypasses TB. Flags are TB middleware, so they cannot apply here — switch Scope to “Through TB” to test flags.' })}
                </Typography>
            )}

            {loading && registry.length === 0 && (
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {t('playground.pluginsLoading', { defaultValue: 'Loading the flag registry…' })}
                </Typography>
            )}

            <Box sx={{ opacity: disabled ? 0.45 : 1, pointerEvents: disabled ? 'none' : 'auto', filter: disabled ? 'grayscale(1)' : 'none' }}>
                {groups.map(({ category, specs }) => (
                    <Box key={category} sx={{ mb: 1 }}>
                        <Typography
                            variant="overline"
                            sx={{ display: 'flex', alignItems: 'center', gap: 1, fontSize: '0.6rem', '&::after': { content: '""', flex: 1, height: '1px', bgcolor: 'divider' } }}
                        >
                            {category}
                        </Typography>
                        {specs.map((spec) => {
                            const overridden = Object.prototype.hasOwnProperty.call(overlay, spec.key);
                            const base = baseline.values[spec.key];
                            const value = overridden ? overlay[spec.key] : base;
                            const source = baseline.sources[spec.key];
                            const baseText = formatFlagValue(spec, base);
                            const caption = overridden
                                ? t('playground.overriddenRow', { defaultValue: 'overridden for this request' })
                                : source
                                  ? t('playground.inheritFrom', { value: baseText, source: source === 'rule' ? t('playground.sourceRule', { defaultValue: 'rule' }) : t('playground.sourceScenario', { defaultValue: 'scenario' }), defaultValue: 'inherit: {{value}} ({{source}})' })
                                  : t('playground.inherit', { value: baseText, defaultValue: 'inherit: {{value}}' });
                            const editable = EDITABLE_TYPES.has(spec.type);
                            return (
                                <Tooltip key={spec.key} title={spec.description} placement="left" enterDelay={600}>
                                    <Box
                                        sx={{
                                            display: 'grid',
                                            gridTemplateColumns: '10px minmax(0, 1fr) auto 26px',
                                            alignItems: 'center',
                                            gap: 1,
                                            px: 0.75,
                                            py: 0.4,
                                            borderRadius: 1,
                                            border: '1px solid',
                                            borderColor: overridden ? 'primary.main' : 'transparent',
                                            bgcolor: overridden ? 'action.selected' : 'transparent',
                                        }}
                                    >
                                        <Box
                                            sx={{
                                                width: 8,
                                                height: 8,
                                                borderRadius: '50%',
                                                border: '1.5px solid',
                                                borderColor: overridden ? 'primary.main' : 'text.disabled',
                                                bgcolor: overridden ? 'primary.main' : 'transparent',
                                            }}
                                        />
                                        <Box sx={{ minWidth: 0 }}>
                                            <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem', color: overridden ? 'text.primary' : 'text.secondary', overflowWrap: 'anywhere', lineHeight: 1.3 }}>
                                                {spec.key}
                                            </Typography>
                                            <Typography variant="caption" sx={{ display: 'block', color: 'text.disabled', lineHeight: 1.25, fontSize: '0.62rem' }}>
                                                {caption}
                                            </Typography>
                                        </Box>
                                        <Box sx={{ opacity: overridden || !editable ? 1 : 0.7 }}>
                                            <FlagControl spec={spec} value={value} overridden={overridden} onSet={(v) => setFlag(spec.key, v)} />
                                        </Box>
                                        <Box>
                                            {overridden && (
                                                <Tooltip title={t('playground.reset', { defaultValue: 'Reset to inherited' })}>
                                                    <IconButton size="small" onClick={() => resetFlag(spec.key)} sx={{ p: 0.25 }} aria-label={`reset ${spec.key}`}>
                                                        <ResetIcon sx={{ fontSize: 15 }} />
                                                    </IconButton>
                                                </Tooltip>
                                            )}
                                        </Box>
                                    </Box>
                                </Tooltip>
                            );
                        })}
                    </Box>
                ))}
            </Box>

            <Typography variant="caption" sx={{ color: 'text.secondary', lineHeight: 1.4 }}>
                {t('playground.pluginsNote', { defaultValue: "Muted rows show the resolved baseline for this target (rule flags + scenario inheritance). Highlighted rows ride in the request's flags overlay. The response's Flags row is the authority." })}
            </Typography>
        </Stack>
    );
};

export default PluginsPanel;
