import React from 'react';
import {
    Stack,
    TextField,
    Typography,
    Box,
    InputAdornment,
    IconButton,
    Link,
} from '@mui/material';
import { Visibility, VisibilityOff, OpenInNew } from '@mui/icons-material';
import { type FieldSpec } from '@/types/bot.ts';

interface BotAuthFormProps {
    platform: string;
    authType: string;
    fields: FieldSpec[];
    authData: Record<string, string>;
    onChange: (key: string, value: string) => void;
    disabled?: boolean;
}

// OAuth platform help links
const oauthHelpLinks: Record<string, { url: string; label: string }> = {
    dingtalk: {
        url: 'https://open.dingtalk.com/document/orgapp/obtain-the-appkey-and-appsecret-of-an-internal-app',
        label: 'DingTalk Developer Docs',
    },
    feishu: {
        url: 'https://open.feishu.cn/document/home/introduction-to-feishu-platform/',
        label: 'Feishu Developer Docs',
    },
};

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

    const helpLink = oauthHelpLinks[platform];

    return (
        <Stack spacing={2}>
            <Box>
                {authType === 'oauth' && (
                    <Typography variant="body2" color="text.secondary">
                        Enter your App credentials from the developer console.
                        {helpLink && (
                            <Link
                                href={helpLink.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                sx={{ ml: 1, display: 'inline-flex', alignItems: 'center', gap: 0.5 }}
                            >
                                {helpLink.label}
                                <OpenInNew fontSize="inherit" />
                            </Link>
                        )}
                    </Typography>
                )}
            </Box>
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
