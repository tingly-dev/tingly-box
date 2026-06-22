import {Box, Checkbox, FormControlLabel, TextField, Typography} from '@mui/material';
import React from 'react';
import {useTranslation} from 'react-i18next';

interface ProxyUrlFieldProps {
    mode: 'add' | 'edit';
    proxyUrl: string;
    onProxyUrlChange: (value: string) => void;
    globalProxyUrl: string;
    useGlobalProxy: boolean;
    onUseGlobalProxyChange: (checked: boolean) => void;
}

const ProxyUrlField: React.FC<ProxyUrlFieldProps> = ({
    mode,
    proxyUrl,
    onProxyUrlChange,
    globalProxyUrl,
    useGlobalProxy,
    onUseGlobalProxyChange,
}) => {
    const {t} = useTranslation();

    return (
        <Box>
            <TextField
                size="small"
                fullWidth
                label={t('providerDialog.advanced.proxyUrl.label')}
                placeholder={t('providerDialog.advanced.proxyUrl.placeholder')}
                value={proxyUrl}
                onChange={(e) => onProxyUrlChange(e.target.value)}
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
        </Box>
    );
};

export default ProxyUrlField;
