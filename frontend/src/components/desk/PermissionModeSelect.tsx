import {MenuItem, TextField} from '@mui/material';
import {useTranslation} from 'react-i18next';

interface PermissionModeSelectProps {
    value: string;
    permissionModes: string[];
    onChange: (mode: string) => void;
}

// The permission-mode picker used both when starting a session
// (SessionListPanel) and when changing an existing one's mode for its next
// turn (TranscriptPanel) — same field, same empty-string-means-inherit
// semantics either way.
const PermissionModeSelect = ({value, permissionModes, onChange}: PermissionModeSelectProps) => {
    const {t} = useTranslation();
    return (
        <TextField
            select
            size="small"
            label={t('desk.permissionMode', {defaultValue: 'Permission'})}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            sx={{minWidth: 160}}
        >
            <MenuItem value="">{t('desk.permissionInherit', {defaultValue: 'Inherit (default)'})}</MenuItem>
            {permissionModes.map((m) => <MenuItem key={m} value={m}>{m}</MenuItem>)}
        </TextField>
    );
};

export default PermissionModeSelect;
