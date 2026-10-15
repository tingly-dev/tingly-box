import {Lock} from '@/components/icons';
import {MenuItem, Select, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import {composerPickerLabelSx, composerPickerSx} from '@tingly/projects';

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
                    <Lock/>
                    <Typography noWrap sx={composerPickerLabelSx}>{v || inherit}</Typography>
                </Stack>
            )}
            sx={composerPickerSx}
        >
            <MenuItem value="">{inherit}</MenuItem>
            {permissionModes.map((m) => <MenuItem key={m} value={m}>{m}</MenuItem>)}
        </Select>
    );
};

export default PermissionModeSelect;
