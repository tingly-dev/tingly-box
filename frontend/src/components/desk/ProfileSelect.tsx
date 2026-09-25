import {Route} from '@/components/icons';
import {useProfileContext} from '@/contexts/ProfileContext';
import {MenuItem, Select, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

interface ProfileSelectProps {
    value: string;
    onChange: (profile: string) => void;
}

// ProfileSelect picks the Claude Code profile a session runs with: which
// upstream models its requests route to. Empty is the main Claude Code
// routing. Hidden when no profile exists, since there is nothing to choose.
const ProfileSelect = ({value, onChange}: ProfileSelectProps) => {
    const {t} = useTranslation();
    const profiles = useProfileContext().getProfiles('claude_code');
    const defaultLabel = t('desk.profileDefault', {defaultValue: 'Default routing'});
    if (profiles.length === 0 && !value) return null;

    const label = (id: string) => {
        const p = profiles.find((x) => x.id === id);
        return p ? p.name : id;
    };

    return (
        <Select
            size="small"
            variant="standard"
            disableUnderline
            displayEmpty
            value={value}
            onChange={(e) => onChange(e.target.value)}
            renderValue={(v) => (
                <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                    <Route sx={{fontSize: 14}}/>
                    <Typography variant="caption" sx={{color: 'inherit'}}>{v ? label(v) : defaultLabel}</Typography>
                </Stack>
            )}
            sx={{
                px: 1,
                borderRadius: 1.5,
                border: 1,
                borderColor: 'divider',
                color: 'text.secondary',
                '& .MuiSelect-select': {py: 0.25, display: 'flex', alignItems: 'center'},
            }}
        >
            <MenuItem value="">{defaultLabel}</MenuItem>
            {profiles.map((p) => (
                <MenuItem key={p.id} value={p.id}>
                    {p.name}
                    <Typography component="span" variant="caption" sx={{ml: 1, color: 'text.secondary', fontFamily: 'monospace'}}>{p.id}</Typography>
                </MenuItem>
            ))}
        </Select>
    );
};

export default ProfileSelect;
