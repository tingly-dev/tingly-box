import {Visibility, VisibilityOff} from '@/components/icons';
import {Box, Checkbox, FormControlLabel, IconButton, InputAdornment, TextField} from '@mui/material';
import React, {useState} from 'react';
import {useTranslation} from 'react-i18next';

interface ApiKeyFieldProps {
    mode: 'add' | 'edit';
    token: string;
    onTokenChange: (value: string) => void;
    noApiKey: boolean;
    onNoApiKeyChange: (checked: boolean) => void;
    /** When true, hides the "No API Key Required" checkbox (caller owns that toggle). */
    hideCheckbox?: boolean;
    /**
     * When true the field stays editable even when noApiKey is set.
     * Use for local providers where auth is optional but the user may still
     * want to enter a token if they configured one on their local service.
     */
    optionalEditable?: boolean;
}

const ApiKeyField: React.FC<ApiKeyFieldProps> = ({
    mode,
    token,
    onTokenChange,
    noApiKey,
    onNoApiKeyChange,
    hideCheckbox = false,
    optionalEditable = false,
}) => {
    const {t} = useTranslation();
    const [showApiKey, setShowApiKey] = useState(false);

    const isDisabled = noApiKey && !optionalEditable;
    const label = noApiKey
        ? optionalEditable ? 'API Key (Optional)' : 'API Key (Not Required)'
        : t('providerDialog.apiKey.label');
    const placeholder = optionalEditable && noApiKey
        ? 'Leave blank if your local service has no auth configured'
        : mode === 'add'
            ? t('providerDialog.apiKey.placeholderAdd')
            : t('providerDialog.apiKey.placeholderEdit');

    return (
        <Box>
            <TextField
                size="small"
                fullWidth
                label={label}
                type={showApiKey ? 'text' : 'password'}
                value={token}
                onChange={(e) => onTokenChange(e.target.value)}
                required={!noApiKey}
                placeholder={placeholder}
                helperText={mode === 'edit' && !optionalEditable && t('providerDialog.apiKey.helperEdit')}
                disabled={isDisabled}
                slotProps={{
                    input: {
                        sx: {
                            '& input': {
                                textOverflow: 'ellipsis',
                            },
                        },
                        endAdornment: (
                            <InputAdornment position="end">
                                <IconButton
                                    size="small"
                                    onClick={() => setShowApiKey(!showApiKey)}
                                    edge="end"
                                    disabled={isDisabled}
                                >
                                    {showApiKey ? (
                                        <VisibilityOff fontSize="small"/>
                                    ) : (
                                        <Visibility fontSize="small"/>
                                    )}
                                </IconButton>
                            </InputAdornment>
                        ),
                    },
                }}
            />
            {!hideCheckbox && !optionalEditable && (
                <Box sx={{display: 'flex', justifyContent: 'flex-end', mt: 0.5, pr: 2}}>
                    <FormControlLabel
                        control={
                            <Checkbox
                                size="small"
                                checked={noApiKey}
                                onChange={(e) => onNoApiKeyChange(e.target.checked)}
                            />
                        }
                        label="No API Key Required"
                        labelPlacement="start"
                    />
                </Box>
            )}
        </Box>
    );
};

export default ApiKeyField;
