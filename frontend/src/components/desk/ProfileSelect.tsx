import {Route, ExpandMore} from '@/components/icons';
import {useProfileContext} from '@/contexts/ProfileContext';
import {MenuItem, Select, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import { fontMono } from '@/theme/fonts';
import {composerPickerLabelSx, composerPickerSx} from '@tingly/projects';

interface ProfileSelectProps {
    value: string;
    disabled?: boolean;
    onChange: (profile: string) => void;
}

// ProfileSelect picks the Claude Code profile a session runs with: which
// upstream models its requests route to. Empty is the main Claude Code
// routing. Hidden when no profile exists, since there is nothing to choose.
const ProfileSelect = ({value, onChange, disabled}: ProfileSelectProps) => {
    const {t} = useTranslation();
    const profiles = useProfileContext().getProfiles('claude_code');
    const defaultLabel = t('desk.profileDefault', {defaultValue: 'Default configuration profile'});
    if (profiles.length === 0 && !value) return null;

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
            inputProps={{'aria-label': t('desk.profile', {defaultValue: 'Configuration profile'})}}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            renderValue={(v) => (
                <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                    <Route/>
                    <Typography noWrap sx={composerPickerLabelSx}>{v ? label(v) : defaultLabel}</Typography>
                </Stack>
            )}
            sx={composerPickerSx}
        >
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
