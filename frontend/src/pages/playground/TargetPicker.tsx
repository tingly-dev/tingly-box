import React, { useMemo } from 'react';
import { Autocomplete, Box, Chip, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Rule } from '@/components/RoutingGraphTypes';
import type { Provider } from '@/types/provider';
import type { PlaygroundTarget } from './playgroundLink';
import { targetKey } from './playgroundState';
import type { TargetCatalog } from './useTargetCatalog';

// TargetPicker: one searchable list, grouped Rules / <provider name>, where
// picking an entry IS the target. No mode picker in front of it
// (ux-principles #2).

export interface TargetOption {
    key: string;
    target: PlaygroundTarget;
    group: string;
    primary: string;
    secondary: string;
    search: string;
}

export const ruleLabel = (rule: Rule): string => rule.description || rule.request_model || rule.uuid;

export function buildTargetOptions(catalog: TargetCatalog, t: (k: string, o?: any) => string): TargetOption[] {
    const rules: TargetOption[] = catalog.rules.map((r) => ({
        key: `rule:${r.uuid}`,
        target: { kind: 'rule', ruleUuid: r.uuid, scenario: r.scenario },
        group: t('playground.rules', { defaultValue: 'Rules' }),
        primary: ruleLabel(r),
        secondary: `${r.scenario}${r.request_model && r.request_model !== ruleLabel(r) ? ` · ${r.request_model}` : ''}`,
        search: `${ruleLabel(r)} ${r.scenario} ${r.request_model} ${r.uuid}`.toLowerCase(),
    }));
    const providers: TargetOption[] = [];
    [...catalog.providers]
        .sort((a, b) => a.name.localeCompare(b.name))
        .forEach((p: Provider) => {
            (catalog.modelsByProvider[p.uuid] ?? []).forEach((model) => {
                providers.push({
                    key: `provider:${p.uuid}:${model}`,
                    target: { kind: 'provider', providerUuid: p.uuid, model },
                    group: p.name,
                    primary: model,
                    secondary: `${p.name} · ${p.api_style}`,
                    search: `${p.name} ${model} ${p.api_style} ${p.uuid}`.toLowerCase(),
                });
            });
        });
    return [...rules, ...providers];
}

export const TargetPicker: React.FC<{
    catalog: TargetCatalog;
    value: PlaygroundTarget | null;
    onChange: (target: PlaygroundTarget | null) => void;
}> = ({ catalog, value, onChange }) => {
    const { t } = useTranslation();
    const options = useMemo(() => buildTargetOptions(catalog, t), [catalog, t]);
    const selected = useMemo(() => options.find((o) => o.key === targetKey(value)) ?? null, [options, value]);
    const missing = !!value && !catalog.loading && !selected;

    return (
        <Autocomplete
            size="small"
            options={options}
            value={selected}
            loading={catalog.loading}
            loadingText={t('playground.targetLoading', { defaultValue: 'Loading rules and providers…' })}
            groupBy={(o) => o.group}
            getOptionLabel={(o) => (o.target.kind === 'rule' ? `${o.primary} · ${o.secondary}` : `${o.group} ▸ ${o.primary}`)}
            isOptionEqualToValue={(a, b) => a.key === b.key}
            filterOptions={(opts, state) => {
                const q = state.inputValue.trim().toLowerCase();
                return q ? opts.filter((o) => o.search.includes(q)) : opts;
            }}
            onChange={(_, o) => onChange(o?.target ?? null)}
            renderInput={(params) => (
                <TextField
                    {...params}
                    placeholder={t('playground.targetPlaceholder', { defaultValue: 'Search rules & providers…' })}
                    error={missing}
                    helperText={
                        missing
                            ? t('playground.targetMissing', { defaultValue: 'The saved target no longer exists — pick another.' })
                            : !value
                              ? t('playground.targetEmpty', { defaultValue: 'Pick a rule or a provider model to start.' })
                              : undefined
                    }
                />
            )}
            renderOption={(props, o) => {
                const { key, ...rest } = props as React.HTMLAttributes<HTMLLIElement> & { key: string };
                return (
                    <li key={key} {...rest} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                        <Chip
                            size="small"
                            label={o.target.kind}
                            sx={{ height: 18, fontSize: '0.6rem', textTransform: 'uppercase', letterSpacing: '0.04em' }}
                            color={o.target.kind === 'rule' ? 'primary' : 'default'}
                            variant="outlined"
                        />
                        <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2" sx={{ color: 'text.primary', fontWeight: 500 }} noWrap>
                                {o.primary}
                            </Typography>
                            <Typography variant="caption" sx={{ fontFamily: 'monospace', display: 'block' }} noWrap>
                                {o.secondary}
                            </Typography>
                        </Box>
                    </li>
                );
            }}
        />
    );
};

export default TargetPicker;
