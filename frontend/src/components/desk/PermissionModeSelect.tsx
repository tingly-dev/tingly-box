import {Lock} from '@/components/icons';
import {MenuItem, Select, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

interface PermissionModeSelectProps {
    value: string;
    permissionModes: string[];
    onChange: (mode: string) => void;
}

// A compact, chip-sized picker for the composer's context row. Empty means
// "inherit": the Claude Code settings' own default mode decides.
const PermissionModeSelect = ({value, permissionModes, onChange}: PermissionModeSelectProps) => {
    const {t} = useTranslation();
    const inherit = t('desk.permissionInherit', {defaultValue: 'Default permissions'});
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
                    <Lock sx={{fontSize: 14}}/>
                    <Typography variant="caption" sx={{color: 'inherit'}}>{v || inherit}</Typography>
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
            <MenuItem value="">{inherit}</MenuItem>
            {permissionModes.map((m) => <MenuItem key={m} value={m}>{m}</MenuItem>)}
        </Select>
    );
};

export default PermissionModeSelect;
