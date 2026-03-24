import React from 'react';
import {
    Stack,
    TextField,
    Typography,
    Box,
    InputAdornment,
    IconButton,
    Link,
    Button,
    Chip,
} from '@mui/material';
import { Visibility, VisibilityOff, OpenInNew, CheckCircle as CheckCircleIcon, QrCode as QrCodeIcon } from '@mui/icons-material';
import { type FieldSpec } from '@/types/bot.ts';
import { WeixinQRAuth } from './WeixinQRAuth.tsx';

interface BotAuthFormProps {
    platform: string;
    authType: string;
    fields: FieldSpec[];
    authData: Record<string, string>;
    onChange: (key: string, value: string) => void;
    disabled?: boolean;
    botUUID?: string;   // Existing bot UUID (edit mode); omit for new bot flow
    botName?: string;   // Bot display name (for deferred creation in add mode)
    onBindingComplete?: (botUUID: string) => void; // Callback with real bot UUID after QR binding
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
    lark: {
        url: 'https://open.larksuite.com/document/home/introduction-to-lark-platform/',
        label: 'Lark Developer Docs',
    },
};

export const BotAuthForm: React.FC<BotAuthFormProps> = ({
    platform,
    authType,
    fields,
    authData,
    onChange,
    disabled = false,
    botUUID,
    botName,
    onBindingComplete,
}) => {
    const [visibleFields, setVisibleFields] = React.useState<Record<string, boolean>>({});
    const [showQR, setShowQR] = React.useState(false);

    const toggleVisibility = (key: string) => {
        setVisibleFields(prev => ({ ...prev, [key]: !prev[key] }));
    };

    // QR code auth for Weixin
    if (authType === 'qr') {
        const isBound = !!authData?.token;

        // Show credentials if already bound and not in re-bind mode
        if (isBound && !showQR) {
            return (
                <Box sx={{ p: 2 }}>
                    <Typography variant="subtitle2" gutterBottom>
                        Weixin QR Code Binding
                    </Typography>
                    <Box
                        sx={{
                            border: 1,
                            borderColor: 'divider',
                            borderRadius: 1,
                            p: 2,
                            bgcolor: 'background.default',
                        }}
                    >
                        <Stack spacing={1.5}>
                            <Stack direction="row" alignItems="center" spacing={1}>
                                <CheckCircleIcon sx={{ color: 'success.main', fontSize: 20 }} />
                                <Typography variant="body2" color="success.main" fontWeight={500}>
                                    Weixin account bound
                                </Typography>
                            </Stack>
                            {authData.bot_id && (
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <Typography variant="caption" color="text.secondary" sx={{ minWidth: 60 }}>Bot ID:</Typography>
                                    <Chip label={authData.bot_id} size="small" variant="outlined" />
                                </Stack>
                            )}
                            {authData.user_id && (
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <Typography variant="caption" color="text.secondary" sx={{ minWidth: 60 }}>User ID:</Typography>
                                    <Chip label={authData.user_id} size="small" variant="outlined" />
                                </Stack>
                            )}
                            <Button
                                startIcon={<QrCodeIcon />}
                                size="small"
                                variant="outlined"
                                onClick={() => setShowQR(true)}
                                disabled={disabled}
                                sx={{ alignSelf: 'flex-start', mt: 0.5 }}
                            >
                                Re-bind Account
                            </Button>
                        </Stack>
                    </Box>
                </Box>
            );
        }

        // Show QR scan (new bind or re-bind)
        return (
            <WeixinQRAuth
                botUUID={botUUID}
                platform={platform}
                botName={botName}
                onComplete={(realUUID) => {
                    setShowQR(false);
                    onBindingComplete?.(realUUID);
                }}
            />
        );
    }

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
