import {Route, ExpandMore} from '@/components/icons';
import {useProfileContext} from '@/contexts/ProfileContext';
import {MenuItem, Select, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import { fontMono } from '@/theme/fonts';
import {composerPickerLabelSx, composerPickerSx} from '@tingly/projects';

interface ProfileSelectProps {
    value: string;
    disabled?: boolean;
    allowInherit?: boolean;
    inherited?: boolean;
    onInherit?: () => void;
    onChange: (profile: string) => void;
}

// ProfileSelect picks the Claude Code profile a session runs with: which
// upstream models its requests route to. Empty is the main Claude Code
// routing. Hidden when no profile exists, since there is nothing to choose.
const ProfileSelect = ({value, onChange, disabled, allowInherit, inherited, onInherit}: ProfileSelectProps) => {
    const {t} = useTranslation();
    const profiles = useProfileContext().getProfiles('claude_code');
    const defaultLabel = t('claudeSession.profileDefault', {defaultValue: 'Default configuration profile'});
    const defaultDisplay = t('claudeSession.profileDefaultShort', {defaultValue: 'Default profile'});
    if (profiles.length === 0 && !value && !allowInherit) return null;

    const label = (id: string) => {
        const p = profiles.find((x) => x.id === id);
        return p ? p.name : id;
    };

    return (
        <Select
            disabled={disabled}
            size="small"
            variant="standard"
            disableUnderline
            IconComponent={ExpandMore}
            displayEmpty
            inputProps={{'aria-label': t('claudeSession.profile', {defaultValue: 'Configuration profile'})}}
            value={inherited ? "__project_inherit__" : value}
            onChange={(e) => e.target.value === "__project_inherit__" ? onInherit?.() : onChange(e.target.value)}
            renderValue={(v) => (
                <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                    <Route/>
                    <Typography noWrap sx={composerPickerLabelSx}>{v === "__project_inherit__" ? `${t("claudeSession.projectProfile", {defaultValue: "Project profile"})} · ${value ? label(value) : defaultDisplay}` : v ? label(v) : defaultDisplay}</Typography>
                </Stack>
            )}
            sx={composerPickerSx}
        >
            {allowInherit && <MenuItem value="__project_inherit__">{t("claudeSession.projectProfile", {defaultValue: "Project profile"})}</MenuItem>}
            <MenuItem value="">{defaultLabel}</MenuItem>
            {profiles.map((p) => (
                <MenuItem key={p.id} value={p.id}>
                    {p.name}
                    <Typography component="span" variant="caption" sx={{ml: 1, color: 'text.secondary', fontFamily: fontMono}}>{p.id}</Typography>
                </MenuItem>
            ))}
        </Select>
    );
};

export default ProfileSelect;
