import React from 'react';
import {
    Stack,
    TextField,
    Typography,
    Box,
    InputAdornment,
    IconButton,
} from '@mui/material';
import { Visibility, VisibilityOff } from '@mui/icons-material';
import { FieldSpec } from '../../types/bot';

interface BotAuthFormProps {
    platform: string;
    authType: string;
    fields: FieldSpec[];
    authData: Record<string, string>;
    onChange: (key: string, value: string) => void;
    disabled?: boolean;
}

export const BotAuthForm: React.FC<BotAuthFormProps> = ({
    platform,
    authType,
    fields,
    authData,
    onChange,
    disabled = false,
}) => {
    const [visibleFields, setVisibleFields] = React.useState<Record<string, boolean>>({});

    const toggleVisibility = (key: string) => {
        setVisibleFields(prev => ({ ...prev, [key]: !prev[key] }));
    };

    if (!fields || fields.length === 0) {
        return (
            <Box sx={{ p: 2, bgcolor: 'warning.main', borderRadius: 1 }}>
                <Typography variant="body2" color="warning.contrastText">
                    No auth fields defined for this platform.
                </Typography>
            </Box>
        );
    }

    return (
        <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
                {authType === 'oauth' ? 'OAuth Credentials' : authType === 'token' ? 'Token Authentication' : 'Authentication'}
            </Typography>
            {fields.map((field) => {
                const value = authData[field.key] || '';
                const isVisible = visibleFields[field.key] || false;

                return (
                    <TextField
                        key={field.key}
                        label={field.label}
                        placeholder={field.placeholder}
                        value={value}
                        onChange={(e) => onChange(field.key, e.target.value)}
                        fullWidth
                        size="small"
                        type={field.secret && !isVisible ? 'password' : 'text'}
                        required={field.required}
                        disabled={disabled}
                        helperText={field.helperText || (field.secret ? 'This will be stored securely' : '')}
                        slotProps={{
                            inputLabel: { shrink: true },
                            input: field.secret ? {
                                endAdornment: (
                                    <InputAdornment position="end">
                                        <IconButton
                                            onClick={() => toggleVisibility(field.key)}
                                            edge="end"
                                            size="small"
                                        >
                                            {isVisible ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
                                        </IconButton>
                                    </InputAdornment>
                                ),
                            } : undefined,
                        }}
                    />
                );
            })}
        </Stack>
    );
};

export default BotAuthForm;
