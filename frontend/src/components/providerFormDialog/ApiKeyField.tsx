import {Visibility, VisibilityOff} from '@mui/icons-material';
import {Box, Checkbox, FormControlLabel, IconButton, InputAdornment, TextField} from '@mui/material';
import React, {useState} from 'react';
import {useTranslation} from 'react-i18next';

interface ApiKeyFieldProps {
    mode: 'add' | 'edit';
    token: string;
    onTokenChange: (value: string) => void;
    noApiKey: boolean;
    onNoApiKeyChange: (checked: boolean) => void;
}

const ApiKeyField: React.FC<ApiKeyFieldProps> = ({
    mode,
    token,
    onTokenChange,
    noApiKey,
    onNoApiKeyChange,
}) => {
    const {t} = useTranslation();
    const [showApiKey, setShowApiKey] = useState(false);

    return (
        <Box>
            <TextField
                size="small"
                fullWidth
                label={noApiKey ? 'API Key (Not Required)' : t('providerDialog.apiKey.label')}
                type={showApiKey ? 'text' : 'password'}
                value={token}
                onChange={(e) => onTokenChange(e.target.value)}
                required={!noApiKey}
                placeholder={
                    mode === 'add'
                        ? t('providerDialog.apiKey.placeholderAdd')
                        : t('providerDialog.apiKey.placeholderEdit')
                }
                helperText={mode === 'edit' && t('providerDialog.apiKey.helperEdit')}
                disabled={noApiKey}
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
                                    disabled={noApiKey}
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
        </Box>
    );
};

export default ApiKeyField;
