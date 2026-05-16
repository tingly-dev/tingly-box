import {Autocomplete, Box, TextField, Typography} from '@mui/material';
import React from 'react';
import type {UniqueProvider} from '../../services/serviceProviders';
import ProviderIcon from '../ProviderIcon';

interface ProviderAutocompleteProps {
    options: UniqueProvider[];
    value: UniqueProvider | null;
    inputValue: string;
    onChange: (newValue: string | UniqueProvider | null) => void;
    onInputChange: (event: React.SyntheticEvent, newValue: string) => void;
    onBlur: () => void;
}

const ProviderAutocomplete: React.FC<ProviderAutocompleteProps> = ({
    options,
    value,
    inputValue,
    onChange,
    onInputChange,
    onBlur,
}) => {
    return (
        <Autocomplete
            freeSolo
            autoHighlight
            openOnFocus
            selectOnFocus
            handleHomeEndKeys
            size="small"
            options={options}
            filterOptions={(opts, state) => {
                const needle = state.inputValue.trim().toLowerCase();
                if (!needle) return opts;
                return opts.filter(option => {
                    const displayName = (option.alias || option.name).toLowerCase();
                    return (
                        displayName.includes(needle) ||
                        (option.baseUrlOpenAI || '').toLowerCase().includes(needle) ||
                        (option.baseUrlAnthropic || '').toLowerCase().includes(needle)
                    );
                });
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
                    label="Provider"
                    placeholder="Select a provider or enter custom base URL"
                />
            )}
            renderOption={(props, option) => {
                const {key, ...optionProps} = props;
                return (
                    <Box
                        component="li"
                        key={key}
                        {...optionProps}
                        sx={{display: 'flex', alignItems: 'center', gap: 1}}
                    >
                        {option.icon ? <ProviderIcon identifier={option.icon} size={18}/> : null}
                        <Box>
                            <Typography variant="body2">{option.alias || option.name}</Typography>
                            <Typography variant="caption" color="text.secondary">
                                {option.baseUrlOpenAI || option.baseUrlAnthropic}
                            </Typography>
                        </Box>
                    </Box>
                );
            }}
        />
    );
};

export default ProviderAutocomplete;
