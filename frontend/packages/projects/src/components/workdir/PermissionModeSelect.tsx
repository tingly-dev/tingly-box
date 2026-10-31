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
    allowInherit?: boolean;
    defaultLabel?: string;
    defaultDisplayLabel?: string;
    label?: string;
}

/**
 * The permission mode of a session, as a quiet picker in the message box's
 * strip. Empty means the default (edits run, anything else asks).
 */
export function PermissionModeSelect({ value, modes, onChange, disabled, allowInherit = true, defaultLabel, defaultDisplayLabel, label }: Props) {
    const { t } = useTranslation('projects');
    const inherit = defaultLabel || t('session.permissionDefault');
    return (
        <Select
            disabled={disabled}
            size="small"
            variant="standard"
            disableUnderline
            IconComponent={ExpandMore}
            displayEmpty
            inputProps={{ 'aria-label': label || t('session.permissions') }}
            value={value}
            onChange={(e) => { if (!disabled) onChange(e.target.value); }}
            renderValue={(v) => (
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                    <Lock />
                    <Typography noWrap sx={composerPickerLabelSx}>{v ? t(`session.mode.${v}`, { defaultValue: v }) : (defaultDisplayLabel || inherit)}</Typography>
                </Stack>
            )}
            sx={composerPickerSx}
        >
            {allowInherit && <MenuItem value="">{inherit}</MenuItem>}
            {modes.map((m) => <MenuItem key={m} value={m}>{t(`session.mode.${m}`, { defaultValue: m })}</MenuItem>)}
        </Select>
    );
}

/** The selectable modes, from the server. */
export async function loadPermissionModes(): Promise<string[]> {
    return (await unwrap(api.GET('/api/v1/threads/permission-modes'))).modes;
}
