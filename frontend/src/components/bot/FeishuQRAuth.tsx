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

interface FeishuQRAuthProps {
    botUUID?: string; // Existing bot UUID for edit mode; omit for new bot flow
    platform: string; // "feishu" or "lark"
    botName?: string; // Optional display name for deferred bot creation
    onComplete?: (botUUID: string) => void; // Callback with real bot UUID after registration
}

type QRState = 'idle' | 'loading' | 'show_qr' | 'confirmed' | 'expired' | 'denied' | 'error';

export const FeishuQRAuth: React.FC<FeishuQRAuthProps> = ({ botUUID, platform, botName, onComplete }) => {
    const [state, setState] = useState<QRState>('idle');
    const [qrUrl, setQrUrl] = useState<string>('');
    const [error, setError] = useState<string>('');
    const stoppedRef = React.useRef(false);

    const label = platform === 'lark' ? 'Lark' : 'Feishu';

    // Generate a temporary UUID for the QR flow if botUUID is not provided
    const [tempUUID] = useState(() => {
        if (botUUID) return botUUID;
        return `temp-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`;
    });
    const effectiveBotUUID = botUUID || tempUUID;

    const startRegistration = useCallback(async () => {
        if (!effectiveBotUUID) {
            setError('Bot UUID is required');
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
                setError(response.error || 'Failed to start one-click registration');
                setState('error');
            }
        } catch (err: any) {
            setError(err.message || 'Failed to start one-click registration');
            setState('error');
        }
    }, [effectiveBotUUID, platform, botName]);

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
                    setError(response.error || 'Registration failed');
                    setState('error');
                    return true;
            }
        } catch (err: any) {
            setError(err.message || 'Failed to check registration status');
            setState('error');
            return true;
        }
    }, [effectiveBotUUID, onComplete]);

    // Start registration when the component mounts
    useEffect(() => {
        if (state === 'idle' && effectiveBotUUID) {
            startRegistration();
        }
    }, [state, effectiveBotUUID, startRegistration]);

    // Cancel the pending session if the user navigates away before completing
    useEffect(() => {
        return () => {
            if (!stoppedRef.current && effectiveBotUUID) {
                api.feishuRegCancel(effectiveBotUUID).catch(() => {});
            }
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Poll status every 2 seconds while the QR code is displayed
    useEffect(() => {
        if (stoppedRef.current) return;
        if (state !== 'show_qr') return;

        const interval = setInterval(async () => {
            const shouldStop = await pollStatus();
            if (shouldStop) {
                clearInterval(interval);
            }
        }, 2000);

        return () => clearInterval(interval);
    }, [state, pollStatus]);

    const renderContent = () => {
        switch (state) {
            case 'idle':
            case 'loading':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CircularProgress size={40} />
                        <Typography sx={{ mt: 2 }} color="text.secondary">
                            Preparing one-click {label} registration...
                        </Typography>
                    </Box>
                );

            case 'show_qr':
                return (
                    <Stack spacing={2} alignItems="center">
                        <Typography variant="h6">Scan to create your {label} app</Typography>
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
                        <Typography variant="body2" color="text.secondary" align="center">
                            1. Open {label} on your phone and scan the QR code
                            <br />
                            2. Confirm authorization — the app, permissions and events are created for you
                        </Typography>
                        <Button
                            startIcon={<RefreshIcon />}
                            onClick={startRegistration}
                            variant="outlined"
                            size="small"
                        >
                            Refresh QR Code
                        </Button>
                    </Stack>
                );

            case 'confirmed':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CheckCircleIcon sx={{ fontSize: 60, color: 'success.main', mb: 2 }} />
                        <Typography variant="h6" color="success.main">
                            {label} app created!
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            Credentials were saved automatically. Your bot is ready.
                        </Typography>
                    </Box>
                );

            case 'expired':
                return (
                    <Stack spacing={2} alignItems="center">
                        <Alert severity="warning">
                            The QR code expired. Please get a new one.
                        </Alert>
                        <Button startIcon={<RefreshIcon />} onClick={startRegistration} variant="contained">
                            Get New QR Code
                        </Button>
                    </Stack>
                );

            case 'denied':
                return (
                    <Stack spacing={2} alignItems="center">
                        <Alert severity="warning">
                            Authorization was declined in {label}.
                        </Alert>
                        <Button startIcon={<RefreshIcon />} onClick={startRegistration} variant="contained">
                            Try Again
                        </Button>
                    </Stack>
                );

            case 'error':
                return (
                    <Alert
                        severity="error"
                        action={
                            <Button color="inherit" size="small" onClick={startRegistration}>
                                Retry
                            </Button>
                        }
                    >
                        {error || `An error occurred during ${label} registration`}
                    </Alert>
                );

            default:
                return null;
        }
    };

    return (
        <Box sx={{ p: 2 }}>
            <Typography variant="subtitle2" gutterBottom>
                {label} One-Click App Creation
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
