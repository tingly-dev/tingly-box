import {Autocomplete, Box, TextField, Typography, Stack} from '@mui/material';
import React from 'react';
import {useTranslation} from 'react-i18next';
import type {UniqueProvider} from '../../services/serviceProviders';
import {searchProviders} from '../../services/serviceProviders';
import ProviderIcon from '../ProviderIcon';
import RegionBadge from '../RegionBadge';

interface ProviderAutocompleteProps {
    options: UniqueProvider[];
    value: UniqueProvider | null;
    inputValue: string;
    onChange: (newValue: string | UniqueProvider | null) => void;
    onInputChange: (event: React.SyntheticEvent, newValue: string) => void;
    onBlur: () => void;
    required?: boolean;
    error?: boolean;
    helperText?: string;
}

const ProviderAutocomplete: React.FC<ProviderAutocompleteProps> = ({
    options,
    value,
    inputValue,
    onChange,
    onInputChange,
    onBlur,
    required,
    error,
    helperText,
}) => {
    const {t} = useTranslation();

    // Group providers by region (CN vs Global)
    const {cnProviders, globalProviders} = React.useMemo(() => {
        const cn: UniqueProvider[] = [];
        const global: UniqueProvider[] = [];
        options.forEach(provider => {
            // Use provider's region field directly (already classified by backend)
            if (provider.region === 'cn') {
                cn.push(provider);
            } else {
                global.push(provider);
            }
        });
        return {cnProviders: cn, globalProviders: global};
    }, [options]);

    // Combine with CN providers first, then global
    const groupedOptions = React.useMemo(() => {
        return [...cnProviders, ...globalProviders];
    }, [cnProviders, globalProviders]);

    return (
        <Autocomplete
            freeSolo
            autoHighlight
            openOnFocus
            selectOnFocus
            handleHomeEndKeys
            size="small"
            options={groupedOptions}
            filterOptions={(opts, state) => {
                return searchProviders(opts, state.inputValue);
            }}
            getOptionLabel={(option) => {
                if (typeof option === 'string') return option;
                return option.alias || option.name;
            }}
            isOptionEqualToValue={(option, val) =>
                typeof option !== 'string' &&
                typeof val !== 'string' &&
                option.id === val.id
            }
            value={value}
            onChange={(_event, newValue) => onChange(newValue)}
            inputValue={inputValue}
            onInputChange={onInputChange}
            onBlur={onBlur}
            renderInput={(params) => (
                <TextField
                    {...params}
                    label={t('providerDialog.provider.label')}
                    placeholder={t('providerDialog.provider.placeholder')}
                    required={required}
                    error={error}
                    helperText={helperText}
                />
            )}
            renderOption={(props, option) => {
                const {key, ...optionProps} = props;
                // Find the index of this option in groupedOptions
                const optionIndex = groupedOptions.findIndex(opt => opt.id === option.id);
                const isFirstGlobal = optionIndex === cnProviders.length;
                const isFirstOption = optionIndex === 0;
                const region = option.region || 'global';
                return (
                    <Box
                        key={key}
                    >
                        {isFirstOption && cnProviders.length > 0 && (
                            <Box
                                sx={{
                                    px: 1.5,
                                    py: 0.5,
                                    bgcolor: 'action.hover',
                                    borderBottom: 1,
                                    borderColor: 'divider',
                                }}
                            >
                                <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                                    {t('provider.region.cn', {defaultValue: 'China (Mainland)'})}
                                </Typography>
                            </Box>
                        )}
                        {isFirstGlobal && cnProviders.length > 0 && (
                            <Box
                                sx={{
                                    px: 1.5,
                                    py: 0.5,
                                    bgcolor: 'action.hover',
                                    borderBottom: 1,
                                    borderColor: 'divider',
                                }}
                            >
                                <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                                    {t('provider.region.global', {defaultValue: 'Global'})}
                                </Typography>
                            </Box>
                        )}
                        <Box
                            component="li"
                            {...optionProps}
                            sx={{display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.75}}
                        >
                            <ProviderIcon identifier={option.icon || option.id} size={18}/>
                            <Box sx={{flex: 1, minWidth: 0}}>
                                <Stack direction="row" alignItems="center" spacing={0.5} sx={{mb: 0.25}}>
                                    <Typography variant="body2" sx={{fontWeight: 500}}>
                                        {option.alias || option.name}
                                    </Typography>
                                    <RegionBadge region={region} size="small" />
                                </Stack>
                                <Typography variant="caption" color="text.secondary" sx={{display: 'block'}}>
                                    {option.baseUrlOpenAI || option.baseUrlAnthropic}
                                </Typography>
                            </Box>
                        </Box>
                    </Box>
                );
            }}
        />
    );
};

export default ProviderAutocomplete;
