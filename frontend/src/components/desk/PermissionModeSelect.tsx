import {Lock} from '@/components/icons';
import {MenuItem, Select, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

interface PermissionModeSelectProps {
    value: string;
    disabled?: boolean;
    permissionModes: string[];
    onChange: (mode: string) => void;
}

// A compact, chip-sized picker for the composer's context row. Empty means
// "inherit": the Claude Code settings' own default mode decides.
const PermissionModeSelect = ({value, permissionModes, onChange, disabled}: PermissionModeSelectProps) => {
    const {t} = useTranslation();
    const inherit = t('desk.permissionInherit', {defaultValue: 'Default permissions'});
    return (
        <Select
            disabled={disabled}
            size="small"
            variant="standard"
            disableUnderline
            displayEmpty
            inputProps={{'aria-label': t('desk.permissions', {defaultValue: 'Permissions'})}}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            renderValue={(v) => (
                <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                    <Lock sx={{fontSize: 14}}/>
                    <Typography variant="caption" noWrap sx={{color: 'inherit', minWidth: 0}}>{v || inherit}</Typography>
                </Stack>
            )}
            sx={{
                px: 1,
                borderRadius: 1.5,
                border: 1,
                borderColor: 'divider',
                maxWidth: '100%',
                minWidth: 0,
                color: 'text.secondary',
                '& .MuiSelect-select': {py: {xs: 0.75, md: 0.25}, minHeight: {xs: '24px !important', md: 'initial'}, display: 'flex', alignItems: 'center'},
            }}
        >
            <MenuItem value="">{inherit}</MenuItem>
            {permissionModes.map((m) => <MenuItem key={m} value={m}>{m}</MenuItem>)}
        </Select>
    );
};

export default PermissionModeSelect;
