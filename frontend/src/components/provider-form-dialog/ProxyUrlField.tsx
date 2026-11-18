import {Box, Button, Checkbox, FormControlLabel, InputAdornment, Switch, TextField, Typography} from '@mui/material';
import React, {useState} from 'react';
import {useTranslation} from 'react-i18next';
import {isProxyUrl} from './validation';

interface ProxyUrlFieldProps {
    mode: 'add' | 'edit';
    proxyUrl: string;
    onProxyUrlChange: (value: string) => void;
    globalProxyUrl: string;
    useGlobalProxy: boolean;
    onUseGlobalProxyChange: (checked: boolean) => void;
    /** Edit mode: whether the stored URL is in use. Provide both props to show the switch. */
    proxyEnabled?: boolean;
    onProxyEnabledChange?: (enabled: boolean) => void;
    /** Show an invalid URL as an error even before the field was left (set once a save was attempted). */
    showErrors?: boolean;
}

/**
 * One control for a credential's proxy: the URL and its on/off switch live in
 * the same field. The switch needs a valid URL, typing or applying a URL turns
 * it on, and the helper line says what the credential will actually do.
 */
const ProxyUrlField: React.FC<ProxyUrlFieldProps> = ({
    mode,
    proxyUrl,
    onProxyUrlChange,
    globalProxyUrl,
    useGlobalProxy,
    onUseGlobalProxyChange,
    proxyEnabled,
    onProxyEnabledChange,
    showErrors = false,
}) => {
    const {t} = useTranslation();
    const [touched, setTouched] = useState(false);
    const trimmed = proxyUrl.trim();
    const invalid = !isProxyUrl(proxyUrl);
    const showError = invalid && (touched || showErrors);
    const hasSwitch = mode === 'edit' && !!onProxyEnabledChange;
    const on = proxyEnabled !== false;

    const changeUrl = (value: string) => {
        onProxyUrlChange(value);
        if (mode === 'edit' && value.trim() && isProxyUrl(value)) onProxyEnabledChange?.(true);
    };

    let helper: string | undefined;
    if (showError) helper = t('providerDialog.advanced.proxyUrl.invalid');
    else if (hasSwitch) {
        helper = t(!trimmed ? 'providerDialog.advanced.proxyUrl.stateNone'
            : on ? 'providerDialog.advanced.proxyUrl.stateOn' : 'providerDialog.advanced.proxyUrl.stateOff');
    }

    return (
        <Box>
            <TextField
                size="small"
                fullWidth
                label={t('providerDialog.advanced.proxyUrl.label')}
                placeholder={t('providerDialog.advanced.proxyUrl.placeholder')}
                value={proxyUrl}
                error={showError}
                helperText={helper}
                onChange={(e) => changeUrl(e.target.value)}
                onBlur={() => setTouched(true)}
                slotProps={hasSwitch ? {
                    input: {
                        endAdornment: (
                            <InputAdornment position="end">
                                <Switch
                                    size="small"
                                    edge="end"
                                    checked={!!trimmed && !invalid && on}
                                    disabled={!trimmed || invalid}
                                    onChange={(e) => onProxyEnabledChange?.(e.target.checked)}
                                    slotProps={{input: {'aria-label': t('providerDialog.advanced.proxyUrl.useThis')}}}
                                />
                            </InputAdornment>
                        ),
                    },
                } : undefined}
            />
            {mode === 'add' && (
                <Box sx={{display: 'flex', justifyContent: 'flex-end', mt: 0.5, pr: 2}}>
                    <FormControlLabel
                        control={
                            <Checkbox
                                size="small"
                                checked={useGlobalProxy}
                                disabled={!globalProxyUrl}
                                onChange={(e) => onUseGlobalProxyChange(e.target.checked)}
                            />
                        }
                        label={
                            <Typography variant="body2" color={globalProxyUrl ? 'text.secondary' : 'text.disabled'}>
                                {globalProxyUrl
                                    ? t('providerDialog.advanced.proxyUrl.useGlobal', {url: globalProxyUrl})
                                    : t('providerDialog.advanced.proxyUrl.useGlobalNotSet')}
                            </Typography>
                        }
                        labelPlacement="start"
                    />
                </Box>
            )}
            {mode === 'edit' && globalProxyUrl && (
                proxyUrl === globalProxyUrl ? (
                    <Typography variant="caption" color="text.secondary" sx={{display: 'block', mt: 0.5, px: 1.75}}>
                        {t('providerDialog.advanced.proxyUrl.matchesGlobal')}
                    </Typography>
                ) : (
                    <Box sx={{display: 'flex', justifyContent: 'flex-end', mt: 0.5, pr: 2}}>
                        <Button
                            size="small"
                            variant="text"
                            onClick={() => changeUrl(globalProxyUrl)}
                            sx={{textTransform: 'none', minWidth: 0, p: 0}}
                        >
                            {t('providerDialog.advanced.proxyUrl.applyGlobal', {url: globalProxyUrl})}
                        </Button>
                    </Box>
                )
            )}
        </Box>
    );
};

export default ProxyUrlField;
