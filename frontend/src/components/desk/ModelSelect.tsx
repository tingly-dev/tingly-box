import {Psychology} from '@/components/icons';
import {profileApi} from '@/services/profileApi';
import type {ClaudeCodeModels as ModelChoice, ClaudeCodeModelTier as ModelTier} from '@/services/profileApi';
import {Box, MenuItem, Select, Stack, Tooltip, Typography} from '@mui/material';
import {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';

interface ModelSelectProps {
    // The profile whose tiers are offered ('' is the main routing).
    profile: string;
    // The chosen tier alias ('' is the profile's default model).
    value: string;
    onChange: (model: string) => void;
}

const chipSx = {
    px: 1,
    borderRadius: 1.5,
    border: 1,
    borderColor: 'divider',
    color: 'text.secondary',
};

// The concrete model a tier ends up on (ux-principles #5): the routed
// provider model when a rule matches, else the gateway model id.
const concrete = (tier: ModelTier) => tier.provider_model || tier.model;

// ModelSelect sits beside the profile picker and always names the model a
// session runs on. A profile that routes tiers separately lets the session
// pick one (passed to Claude Code as --model); a unified profile has one
// model for every tier, so the chip shows it without a menu — changing it
// means editing the profile's rules, which would affect every client.
const ModelSelect = ({profile, value, onChange}: ModelSelectProps) => {
    const {t} = useTranslation();
    const [choice, setChoice] = useState<{profile: string; data: ModelChoice} | null>(null);

    useEffect(() => {
        let live = true;
        profileApi.getClaudeCodeModels(profile)
            .then((data) => live && setChoice(data ? {profile, data} : null))
            .catch(() => live && setChoice(null));
        return () => {
            live = false;
        };
    }, [profile]);

    // A response for the previous profile is not shown under the new one.
    const data = choice?.profile === profile ? choice.data : undefined;
    if (!data || data.tiers.length === 0 || !data.tiers[0].model) return null;

    const tierLabel = (alias: string) => (alias
        ? alias.charAt(0).toUpperCase() + alias.slice(1)
        : t('desk.modelDefault', {defaultValue: 'Default'}));
    const route = (tier: ModelTier) => (tier.provider_name ? `${concrete(tier)} @ ${tier.provider_name}` : tier.model);

    if (data.unified) {
        const only = data.tiers[0];
        return (
            <Tooltip title={t('desk.modelUnified', {
                defaultValue: '{{route}} — this profile uses one model for every tier; change it in the profile\'s rules',
                route: route(only),
            })}
            >
                <Stack direction="row" spacing={0.5} sx={{...chipSx, alignItems: 'center', py: 0.25}} aria-label={t('desk.model', {defaultValue: 'Model'})}>
                    <Psychology sx={{fontSize: 14}}/>
                    <Typography variant="caption" sx={{color: 'inherit'}}>{concrete(only)}</Typography>
                </Stack>
            </Tooltip>
        );
    }

    const selected = data.tiers.find((x) => x.alias === value) ?? data.tiers[0];
    return (
        <Select
            size="small"
            variant="standard"
            disableUnderline
            displayEmpty
            value={selected.alias}
            onChange={(e) => onChange(e.target.value)}
            inputProps={{'aria-label': t('desk.model', {defaultValue: 'Model'})}}
            renderValue={() => (
                <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                    <Psychology sx={{fontSize: 14}}/>
                    <Typography variant="caption" sx={{color: 'inherit'}}>
                        {tierLabel(selected.alias)} · {concrete(selected)}
                    </Typography>
                </Stack>
            )}
            sx={{...chipSx, '& .MuiSelect-select': {py: 0.25, display: 'flex', alignItems: 'center'}}}
        >
            {data.tiers.map((tier) => (
                <MenuItem key={tier.alias} value={tier.alias}>
                    <Box>
                        <Typography variant="body2" sx={{color: 'text.primary'}}>{tierLabel(tier.alias)}</Typography>
                        <Typography variant="caption" sx={{color: 'text.secondary', fontFamily: 'monospace'}}>{route(tier)}</Typography>
                    </Box>
                </MenuItem>
            ))}
        </Select>
    );
};

export default ModelSelect;
