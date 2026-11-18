import {Box, Button, Checkbox, FormControlLabel, Switch, TextField, Typography} from '@mui/material';
import React from 'react';
import {useTranslation} from 'react-i18next';

interface ProxyUrlFieldProps {
    mode: 'add' | 'edit';
    proxyUrl: string;
    onProxyUrlChange: (value: string) => void;
    globalProxyUrl: string;
    useGlobalProxy: boolean;
    onUseGlobalProxyChange: (checked: boolean) => void;
    /** Edit mode: whether the stored URL is in use. Omit to hide the switch. */
    proxyEnabled?: boolean;
    onProxyEnabledChange?: (enabled: boolean) => void;
}

const ProxyUrlField: React.FC<ProxyUrlFieldProps> = ({
    mode,
    proxyUrl,
    onProxyUrlChange,
    globalProxyUrl,
    useGlobalProxy,
    onUseGlobalProxyChange,
    proxyEnabled,
    onProxyEnabledChange,
}) => {
    const {t} = useTranslation();
    // Entering or applying a URL means the user wants it used.
    const changeUrl = (value: string) => {
        onProxyUrlChange(value);
        if (mode === 'edit' && value) onProxyEnabledChange?.(true);
    };

    return (
        <Box>
            <TextField
                size="small"
                fullWidth
                label={t('providerDialog.advanced.proxyUrl.label')}
                placeholder={t('providerDialog.advanced.proxyUrl.placeholder')}
                value={proxyUrl}
                onChange={(e) => changeUrl(e.target.value)}
            />
            {mode === 'edit' && proxyUrl && onProxyEnabledChange && (
                <FormControlLabel
                    sx={{mt: 0.5, ml: 0}}
                    control={
                        <Switch
                            size="small"
                            checked={proxyEnabled !== false}
                            onChange={(e) => onProxyEnabledChange(e.target.checked)}
                        />
                    }
                    label={<Typography variant="body2" color="text.secondary">{t('providerDialog.advanced.proxyUrl.useThis')}</Typography>}
                />
            )}
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
                <Box sx={{display: 'flex', justifyContent: 'flex-end', mt: 0.5, pr: 2}}>
                    <Button
                        size="small"
                        variant="text"
                        disabled={proxyUrl === globalProxyUrl}
                        onClick={() => changeUrl(globalProxyUrl)}
                        sx={{textTransform: 'none', minWidth: 0, p: 0}}
                    >
                        {proxyUrl === globalProxyUrl
                            ? t('providerDialog.advanced.proxyUrl.applyGlobalApplied')
                            : t('providerDialog.advanced.proxyUrl.applyGlobal', {url: globalProxyUrl})}
                    </Button>
                </Box>
            )}
        </Box>
    );
};

export default ProxyUrlField;
