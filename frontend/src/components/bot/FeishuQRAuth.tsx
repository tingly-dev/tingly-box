import React, { useEffect, useState, useCallback } from 'react';
import {
    Box,
    Button,
    Typography,
    CircularProgress,
    Alert,
    Stack,
    Paper,
} from '@mui/material';
import {
    Refresh as RefreshIcon,
    CheckCircle as CheckCircleIcon,
} from '@/components/icons';
import { QRCodeSVG } from 'qrcode.react';
import { api } from '@/services/api';
import { useTranslation } from 'react-i18next';
import { useQrPollingSession, usePollingLoop } from './useQrPollingSession';

interface FeishuQRAuthProps {
    botUUID?: string; // Existing bot UUID for edit mode; omit for new bot flow
    platform: string; // "feishu" or "lark"
    botName?: string; // Optional display name for deferred bot creation
    onComplete?: (botUUID: string) => void; // Callback with real bot UUID after registration
}

type QRState = 'idle' | 'loading' | 'show_qr' | 'confirmed' | 'expired' | 'denied' | 'error';

export const FeishuQRAuth: React.FC<FeishuQRAuthProps> = ({ botUUID, platform, botName, onComplete }) => {
    const { t } = useTranslation();
    const [state, setState] = useState<QRState>('idle');
    const [qrUrl, setQrUrl] = useState<string>('');
    const [error, setError] = useState<string>('');

    const label = platform === 'lark' ? 'Lark' : 'Feishu';

    const { effectiveBotUUID, stoppedRef } = useQrPollingSession(botUUID, api.feishuRegCancel);

    const startRegistration = useCallback(async () => {
        if (!effectiveBotUUID) {
            setError(t('remoteControl.feishuQr.uuidRequired', { defaultValue: 'Bot UUID is required' }));
            setState('error');
            return;
        }

        setState('loading');
        setError('');
        stoppedRef.current = false;

        try {
            const response = await api.feishuRegStart(effectiveBotUUID, platform, botName);
            if (response.success && response.data?.qr_url) {
                setQrUrl(response.data.qr_url);
                setState('show_qr');
            } else {
                setError(response.error || t('remoteControl.feishuQr.startFailed', { defaultValue: 'Failed to start one-click registration' }));
                setState('error');
            }
        } catch (err: any) {
            setError(err.message || t('remoteControl.feishuQr.startFailed', { defaultValue: 'Failed to start one-click registration' }));
            setState('error');
        }
    }, [effectiveBotUUID, platform, botName, t]);

    const pollStatus = useCallback(async (): Promise<boolean> => {
        if (!effectiveBotUUID) return true;

        try {
            const response = await api.feishuRegStatus(effectiveBotUUID);
            const status = response.data?.status;

            switch (status) {
                case 'pending':
                    return false;
                case 'confirmed':
                    stoppedRef.current = true;
                    setState('confirmed');
                    onComplete?.(response.data?.bot_uuid || effectiveBotUUID);
                    return true;
                case 'expired':
                    stoppedRef.current = true;
                    setState('expired');
                    return true;
                case 'denied':
                    stoppedRef.current = true;
                    setState('denied');
                    return true;
                default:
                    stoppedRef.current = true;
                    setError(response.error || t('remoteControl.feishuQr.registrationFailed', { defaultValue: 'Registration failed' }));
                    setState('error');
                    return true;
            }
        } catch (err: any) {
            setError(err.message || t('remoteControl.feishuQr.statusFailed', { defaultValue: 'Failed to check registration status' }));
            setState('error');
            return true;
        }
    }, [effectiveBotUUID, onComplete, t]);

    // Start registration when the component mounts
    useEffect(() => {
        if (state === 'idle' && effectiveBotUUID) {
            startRegistration();
        }
    }, [state, effectiveBotUUID, startRegistration]);

    // Poll status every 2 seconds while the QR code is displayed
    usePollingLoop(state === 'show_qr', stoppedRef, pollStatus);

    const renderContent = () => {
        switch (state) {
            case 'idle':
            case 'loading':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CircularProgress size={40} />
                        <Typography
                            sx={{
                                color: "text.secondary",
                                mt: 2
                            }}>
                            {t('remoteControl.feishuQr.preparing', { defaultValue: 'Preparing one-click {{label}} registration...', label })}
                        </Typography>
                    </Box>
                );

            case 'show_qr':
                return (
                    <Stack spacing={2} sx={{
                        alignItems: "center"
                    }}>
                        <Typography variant="h6">{t('remoteControl.feishuQr.scanTitle', { defaultValue: 'Scan to create your {{label}} app', label })}</Typography>
                        <Paper sx={{ p: 2, bgcolor: 'background.paper' }}>
                            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
                                <QRCodeSVG
                                    value={qrUrl}
                                    size={200}
                                    level="M"
                                    bgColor="#ffffff"
                                    fgColor="#000000"
                                />
                            </Box>
                        </Paper>
                        <Typography variant="body2" align="center" sx={{
                            color: "text.secondary"
                        }}>
                            {t('remoteControl.feishuQr.step1', { defaultValue: '1. Open {{label}} on your phone and scan the QR code', label })}
                            <br />
                            {t('remoteControl.feishuQr.step2', { defaultValue: '2. Confirm authorization — the app, permissions and events are created for you' })}
                        </Typography>
                        <Button
                            startIcon={<RefreshIcon />}
                            onClick={startRegistration}
                            variant="outlined"
                            size="small"
                        >
                            {t('remoteControl.feishuQr.refreshQr', { defaultValue: 'Refresh QR Code' })}
                        </Button>
                    </Stack>
                );

            case 'confirmed':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CheckCircleIcon sx={{ fontSize: 60, color: 'success.main', mb: 2 }} />
                        <Typography variant="h6" sx={{
                            color: "success.main"
                        }}>
                            {t('remoteControl.feishuQr.createdTitle', { defaultValue: '{{label}} app created!', label })}
                        </Typography>
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>
                            {t('remoteControl.feishuQr.createdBody', { defaultValue: 'Credentials were saved automatically. Your bot is ready.' })}
                        </Typography>
                    </Box>
                );

            case 'expired':
                return (
                    <Stack spacing={2} sx={{
                        alignItems: "center"
                    }}>
                        <Alert severity="warning">
                            {t('remoteControl.feishuQr.expiredWarning', { defaultValue: 'The QR code expired. Please get a new one.' })}
                        </Alert>
                        <Button startIcon={<RefreshIcon />} onClick={startRegistration} variant="contained">
                            {t('remoteControl.feishuQr.getNewQr', { defaultValue: 'Get New QR Code' })}
                        </Button>
                    </Stack>
                );

            case 'denied':
                return (
                    <Stack spacing={2} sx={{
                        alignItems: "center"
                    }}>
                        <Alert severity="warning">
                            {t('remoteControl.feishuQr.deniedWarning', { defaultValue: 'Authorization was declined in {{label}}.', label })}
                        </Alert>
                        <Button startIcon={<RefreshIcon />} onClick={startRegistration} variant="contained">
                            {t('remoteControl.feishuQr.tryAgain', { defaultValue: 'Try Again' })}
                        </Button>
                    </Stack>
                );

            case 'error':
                return (
                    <Alert
                        severity="error"
                        action={
                            <Button color="inherit" size="small" onClick={startRegistration}>
                                {t('remoteControl.feishuQr.retry', { defaultValue: 'Retry' })}
                            </Button>
                        }
                    >
                        {error || t('remoteControl.feishuQr.errorFallback', { defaultValue: 'An error occurred during {{label}} registration', label })}
                    </Alert>
                );

            default:
                return null;
        }
    };

    return (
        <Box sx={{ p: 2 }}>
            <Typography variant="subtitle2" gutterBottom>
                {t('remoteControl.feishuQr.headerLabel', { defaultValue: '{{label}} One-Click App Creation', label })}
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
                {renderContent()}
            </Box>
        </Box>
    );
};

export default FeishuQRAuth;
