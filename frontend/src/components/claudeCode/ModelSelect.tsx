import {AutoAwesome, ExpandMore} from '@/components/icons';
import {profileApi} from '@/services/profileApi';
import type {ClaudeCodeModels as ModelChoice, ClaudeCodeModelTier as ModelTier} from '@/services/profileApi';
import {Box, MenuItem, Select, Stack, Typography} from '@mui/material';
import {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {fontMono, fontSizes} from '@/theme/fonts';
import {composerPickerLabelSx, composerPickerSx} from '@tingly/projects';

interface ModelSelectProps {
    // The profile whose tiers are offered ('' is the main routing).
    profile: string;
    // The chosen tier alias ('' is the profile's default model).
    value: string;
    disabled?: boolean;
    onChange: (model: string) => void;
}


// The concrete model a tier ends up on (ux-principles #5): the routed
// provider model when a rule matches, else the gateway model id.
const concrete = (tier: ModelTier) => tier.provider_model || tier.model;

// ModelSelect sits beside the profile picker and always names the model a
// session runs on. A profile that routes tiers separately lets the session
// pick one (passed to Claude Code as --model); a unified profile has one
// model for every tier, so its menu shows that one model and says that
// changing it means editing the profile's rules (it affects every client).
const ModelSelect = ({profile, value, onChange, disabled}: ModelSelectProps) => {
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
        : t('claudeSession.modelDefault', {defaultValue: 'Default'}));
    const route = (tier: ModelTier) => (tier.provider_name ? `${concrete(tier)} @ ${tier.provider_name}` : tier.model);

    // A unified profile has one model for every tier: the menu still opens,
    // shows that model and says where to change it.
    const tiers = data.unified ? [data.tiers[0]] : data.tiers;
    const selected = tiers.find((x) => x.alias === value) ?? tiers[0];
    return (
        <Select
            disabled={disabled}
            size="small"
            variant="standard"
            disableUnderline
            IconComponent={ExpandMore}
            displayEmpty
            value={selected.alias}
            onChange={(e) => onChange(e.target.value)}
            inputProps={{'aria-label': t('claudeSession.model', {defaultValue: 'Model'})}}
            renderValue={() => (
                <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                    <AutoAwesome/>
                    <Typography noWrap sx={composerPickerLabelSx}>
                        {data.unified ? concrete(selected) : `${tierLabel(selected.alias)} · ${concrete(selected)}`}
                    </Typography>
                </Stack>
            )}
            sx={composerPickerSx}
        >
            {tiers.map((tier) => (
                <MenuItem key={tier.alias} value={tier.alias}>
                    <Box>
                        <Typography variant="body2" sx={{color: 'text.primary'}}>{data.unified ? concrete(tier) : tierLabel(tier.alias)}</Typography>
                        <Typography sx={{color: 'text.secondary', fontFamily: fontMono, fontSize: fontSizes.xs}}>{route(tier)}</Typography>
                    </Box>
                </MenuItem>
            ))}
            {data.unified && (
                <Typography component="li" sx={{px: 2, py: 1, maxWidth: 320, fontSize: fontSizes.sm, color: 'text.secondary'}}>
                    {t('claudeSession.modelUnifiedHint', {defaultValue: 'This profile uses one model for every tier. Change it in the profile\'s rules.'})}
                </Typography>
            )}
        </Select>
    );
};

export default ModelSelect;
