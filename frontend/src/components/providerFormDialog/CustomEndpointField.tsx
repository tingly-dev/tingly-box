import {Link, Stack, TextField, Tooltip, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

interface CustomEndpointFieldProps {
    value: string;
    /** Called with the new value on each keystroke (parent owns the state). */
    onChange: (value: string) => void;
    /** Commit the typed value to the parent form on blur. */
    onBlur: () => void;
    error: boolean;
    /** Show the "append /v1" nudge as a persistent tooltip above the field. */
    showV1Hint: boolean;
    onApplyV1: () => void;
}

/**
 * Single base-URL input for the "Custom endpoint" form. A persistent tooltip
 * nudges users to append `/v1` (most OpenAI-compatible APIs need it) without
 * blocking submit — the suffix is one click away, never forced.
 */
const CustomEndpointField = ({
                                 value,
                                 onChange,
                                 onBlur,
                                 error,
                                 showV1Hint,
                                 onApplyV1,
                             }: CustomEndpointFieldProps) => {
    const {t} = useTranslation();

    return (
        <Tooltip
            open={showV1Hint}
            title={
                <Stack direction="row" alignItems="center" spacing={0.75}>
                    <Typography variant="body2" color="text.secondary">
                        {t('providerDialog.v1Hint.message', {
                            defaultValue: 'Most OpenAI-compatible APIs need a /v1 suffix.',
                        })}
                    </Typography>
                    <Link
                        component="button"
                        type="button"
                        variant="body2"
                        onClick={onApplyV1}
                        underline="always"
                        sx={{
                            fontWeight: 600,
                            whiteSpace: 'nowrap',
                        }}
                    >
                        {t('providerDialog.v1Hint.apply', {defaultValue: 'Append /v1'})}
                    </Link>
                </Stack>
            }
            placement="top"
            arrow
            disableFocusListener
            disableHoverListener
            disableTouchListener
            slotProps={{
                tooltip: {
                    sx: {
                        bgcolor: 'background.paper',
                        color: 'text.primary',
                        border: 1,
                        borderColor: 'divider',
                        boxShadow: 2,
                        px: 1.5,
                        py: 1,
                    },
                },
                arrow: {
                    sx: {
                        fontSize: 16,
                        color: 'background.paper',
                        '&::before': {
                            border: 1,
                            borderColor: 'divider',
                        },
                    },
                },
            }}
        >
            <TextField
                size="small"
                fullWidth
                label={t('providerDialog.provider.label')}
                placeholder={t('providerDialog.provider.customPlaceholder', {defaultValue: 'https://api.example.com/v1'})}
                value={value}
                onChange={(e) => onChange(e.target.value)}
                onBlur={onBlur}
                required
                error={error}
                helperText={error ? t('providerDialog.provider.required', {defaultValue: 'Base URL is required'}) : undefined}
            />
        </Tooltip>
    );
};

export default CustomEndpointField;
