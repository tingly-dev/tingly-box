import { MenuItem, Select, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap } from '../../client';
import { ExpandMore, Lock } from '../icons';
import { composerPickerLabelSx, composerPickerSx } from '../composerPicker';

interface Props {
    value: string;
    modes: string[];
    onChange: (mode: string) => void;
    disabled?: boolean;
}

/**
 * The permission mode of a session, as a quiet picker in the message box's
 * strip. Empty means the default (edits run, anything else asks).
 */
export function PermissionModeSelect({ value, modes, onChange, disabled }: Props) {
    const { t } = useTranslation('projects');
    const inherit = t('session.permissionDefault');
    return (
        <Select
            disabled={disabled}
            size="small"
            variant="standard"
            disableUnderline
            IconComponent={ExpandMore}
            displayEmpty
            inputProps={{ 'aria-label': t('session.permissions') }}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            renderValue={(v) => (
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                    <Lock />
                    <Typography noWrap sx={composerPickerLabelSx}>{v ? t(`session.mode.${v}`, { defaultValue: v }) : inherit}</Typography>
                </Stack>
            )}
            sx={composerPickerSx}
        >
            <MenuItem value="">{inherit}</MenuItem>
            {modes.map((m) => <MenuItem key={m} value={m}>{t(`session.mode.${m}`, { defaultValue: m })}</MenuItem>)}
        </Select>
    );
}

/** The selectable modes, from the server. */
export async function loadPermissionModes(): Promise<string[]> {
    return (await unwrap(api.GET('/api/v1/projects/permission-modes'))).modes;
}
