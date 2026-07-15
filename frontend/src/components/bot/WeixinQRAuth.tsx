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
    QrCode as QrCodeIcon,
    Refresh as RefreshIcon,
    CheckCircle as CheckCircleIcon,
} from '@/components/icons';
import { QRCodeSVG } from 'qrcode.react';
import { api } from '@/services/api';
import { useTranslation } from 'react-i18next';
import { useQrPollingSession, usePollingLoop } from './useQrPollingSession';

interface WeixinQRAuthProps {
    botUUID?: string; // Optional - existing bot UUID for edit mode; omit for new bot flow
    platform: string;
    botName?: string; // Optional display name for deferred bot creation
    onComplete?: (botUUID: string) => void; // Callback with real bot UUID after binding
}

type QRState = 'idle' | 'loading' | 'show_qr' | 'scanned' | 'confirmed' | 'expired' | 'error';

interface QRStartResponse {
    qrcode_id: string;
    qrcode_data: string;
    expires_in: number;
}

interface QRStatusResponse {
    status: string;
    error?: string;
}

export const WeixinQRAuth: React.FC<WeixinQRAuthProps> = ({ botUUID, platform, botName, onComplete }) => {
    const { t } = useTranslation();
    const [state, setState] = useState<QRState>('idle');
    const [qrData, setQrData] = useState<string>('');
    const [qrId, setQrId] = useState<string>('');
    const [error, setError] = useState<string>('');
    const [refreshCount, setRefreshCount] = useState(0);

    const MAX_REFRESH = 3;

    const { effectiveBotUUID, stoppedRef } = useQrPollingSession(botUUID, api.weixinQRCancel);

    const startQRLogin = useCallback(async () => {
        if (!effectiveBotUUID) {
            setError(t('remoteControl.weixinQr.uuidRequired', { defaultValue: 'Bot UUID is required' }));
            setState('error');
            return;
        }

        setState('loading');
        setError('');
        stoppedRef.current = false;

        try {
            const response = await api.weixinQRStart(effectiveBotUUID, platform, botName);

            if (response.success && response.data) {
                setQrData(response.data.qrcode_data);
                setQrId(response.data.qrcode_id);
                setState('show_qr');
                setRefreshCount(0);
            } else {
                setError(response.error || t('remoteControl.weixinQr.startFailed', { defaultValue: 'Failed to start QR login' }));
                setState('error');
            }
        } catch (err: any) {
            setError(err.message || t('remoteControl.weixinQr.startFailed', { defaultValue: 'Failed to start QR login' }));
            setState('error');
        }
    }, [effectiveBotUUID, platform, botName, t]);

    const pollQRStatus = useCallback(async (): Promise<boolean> => {
        if (!effectiveBotUUID || !qrId) return false;

        try {
            const response = await api.weixinQRStatus(effectiveBotUUID, qrId);

            if (!response.success) {
                setError(response.error || t('remoteControl.weixinQr.statusFailed', { defaultValue: 'Failed to check QR status' }));
                setState('error');
                return true;
            }

            const status = response.data?.status;

            switch (status) {
                case 'wait':
                    // Continue polling
                    break;
                case 'scaned':
                    setState('scanned');
                    break;
                case 'confirmed':
                    stoppedRef.current = true;
                    setState('confirmed');
                    onComplete?.(response.data?.bot_uuid || effectiveBotUUID);
                    return true;
                case 'expired':
                    if (refreshCount < MAX_REFRESH) {
                        // Auto-refresh QR code
                        await startQRLogin();
                    } else {
                        setState('expired');
                        return true;
                    }
                    break;
                default:
                    if (response.data?.error) {
                        setError(response.data.error);
                        setState('error');
                        return true;
                    }
            }
            return false;
        } catch (err: any) {
            setError(err.message || t('remoteControl.weixinQr.statusFailed', { defaultValue: 'Failed to check QR status' }));
            setState('error');
            return true;
        }
    }, [effectiveBotUUID, qrId, refreshCount, startQRLogin, onComplete, t]);

    // Start QR login when component mounts
    useEffect(() => {
        if (state === 'idle' && effectiveBotUUID) {
            startQRLogin();
        }
    }, [state, effectiveBotUUID, startQRLogin]);

    // Poll QR status every 2 seconds when showing QR or scanned
    usePollingLoop(state === 'show_qr' || state === 'scanned', stoppedRef, pollQRStatus);

    const handleRetry = () => {
        setRefreshCount(0);
        startQRLogin();
    };

    const renderContent = () => {
        switch (state) {
            case 'idle':
            case 'loading':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CircularProgress size={40} />
                        <Typography sx={{ mt: 2 }} color="text.secondary">
                            {t('remoteControl.weixinQr.initializing', { defaultValue: 'Initializing Weixin QR binding...' })}
                        </Typography>
                    </Box>
                );

            case 'show_qr':
                return (
                    <Stack spacing={2} alignItems="center">
                        <Typography variant="h6">{t('remoteControl.weixinQr.scanTitle', { defaultValue: 'Scan QR Code to Bind' })}</Typography>
                        <Paper sx={{ p: 2, bgcolor: 'background.paper' }}>
                            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
                                <QRCodeSVG
                                    value={qrData}
                                    size={200}
                                    level="M"
                                    bgColor="#ffffff"
                                    fgColor="#000000"
                                />
                            </Box>
                        </Paper>
                        <Typography variant="body2" color="text.secondary" align="center">
                            {t('remoteControl.weixinQr.step1', { defaultValue: '1. Open Weixin on your phone and scan the QR code' })}
                            <br />
                            {t('remoteControl.weixinQr.step2', { defaultValue: '2. Confirm to complete binding' })}
                        </Typography>
                        <Button
                            startIcon={<RefreshIcon />}
                            onClick={handleRetry}
                            variant="outlined"
                            size="small"
                        >
                            {t('remoteControl.weixinQr.refreshQr', { defaultValue: 'Refresh QR Code' })}
                        </Button>
                    </Stack>
                );

            case 'scanned':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CircularProgress size={40} />
                        <Typography sx={{ mt: 2 }} color="text.secondary">
                            {t('remoteControl.weixinQr.scannedWaiting', { defaultValue: 'QR code scanned! Please confirm on your Weixin...' })}
                        </Typography>
                    </Box>
                );

            case 'confirmed':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CheckCircleIcon sx={{ fontSize: 60, color: 'success.main', mb: 2 }} />
                        <Typography variant="h6" color="success.main">
                            {t('remoteControl.weixinQr.successTitle', { defaultValue: 'Weixin Binding Successful!' })}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            {t('remoteControl.weixinQr.successBody', { defaultValue: 'Your bot is now connected to Weixin.' })}
                        </Typography>
                    </Box>
                );

            case 'expired':
                return (
                    <Stack spacing={2} alignItems="center">
                        <Alert severity="warning">
                            {t('remoteControl.weixinQr.expiredWarning', { defaultValue: 'QR code expired. Please refresh to get a new one.' })}
                        </Alert>
                        <Button
                            startIcon={<RefreshIcon />}
                            onClick={handleRetry}
                            variant="contained"
                        >
                            {t('remoteControl.weixinQr.getNewQr', { defaultValue: 'Get New QR Code' })}
                        </Button>
                    </Stack>
                );

            case 'error':
                return (
                    <Alert
                        severity="error"
                        action={
                            <Button color="inherit" size="small" onClick={startQRLogin}>
                                {t('remoteControl.weixinQr.retry', { defaultValue: 'Retry' })}
                            </Button>
                        }
                    >
                        {error || t('remoteControl.weixinQr.errorFallback', { defaultValue: 'An error occurred during Weixin binding' })}
                    </Alert>
                );

            default:
                return null;
        }
    };

    return (
        <Box sx={{ p: 2 }}>
            <Typography variant="subtitle2" gutterBottom>
                {t('remoteControl.weixinQr.headerLabel', { defaultValue: 'Weixin QR Code Binding' })}
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

export default WeixinQRAuth;
