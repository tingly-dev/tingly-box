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
} from '@mui/icons-material';
import { QRCodeSVG } from 'qrcode.react';
import { api } from '@/services/api';

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
    const [state, setState] = useState<QRState>('idle');
    const [qrData, setQrData] = useState<string>('');
    const [qrId, setQrId] = useState<string>('');
    const [error, setError] = useState<string>('');
    const [refreshCount, setRefreshCount] = useState(0);
    const stoppedRef = React.useRef(false);

    const MAX_REFRESH = 3;

    // Generate a temporary UUID for QR flow if botUUID is not provided
    const [tempUUID] = useState(() => {
        if (botUUID) return botUUID;
        // Generate a simple temp UUID (format: temp-{timestamp}-{random})
        return `temp-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`;
    });

    const effectiveBotUUID = botUUID || tempUUID;

    const startQRLogin = useCallback(async () => {
        if (!effectiveBotUUID) {
            setError('Bot UUID is required');
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
                setError(response.error || 'Failed to start QR login');
                setState('error');
            }
        } catch (err: any) {
            setError(err.message || 'Failed to start QR login');
            setState('error');
        }
    }, [effectiveBotUUID, platform, botName]);

    const pollQRStatus = useCallback(async () => {
        if (!effectiveBotUUID || !qrId) return;

        try {
            const response = await api.weixinQRStatus(effectiveBotUUID, qrId);

            if (!response.success) {
                setError(response.error || 'Failed to check QR status');
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
            setError(err.message || 'Failed to check QR status');
            setState('error');
            return true;
        }
    }, [effectiveBotUUID, qrId, refreshCount, startQRLogin, onComplete]);

    // Start QR login when component mounts
    useEffect(() => {
        if (state === 'idle' && effectiveBotUUID) {
            startQRLogin();
        }
    }, [state, effectiveBotUUID, startQRLogin]);

    // Cancel QR session on unmount (user navigates away or closes dialog)
    useEffect(() => {
        return () => {
            if (!stoppedRef.current && effectiveBotUUID) {
                api.weixinQRCancel(effectiveBotUUID).catch(() => {});
            }
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Poll QR status every 2 seconds when showing QR or scanned
    useEffect(() => {
        if (stoppedRef.current) return;
        if (state !== 'show_qr' && state !== 'scanned') return;

        const interval = setInterval(async () => {
            const shouldStop = await pollQRStatus();
            if (shouldStop) {
                clearInterval(interval);
            }
        }, 2000);

        return () => clearInterval(interval);
    }, [state, pollQRStatus]);

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
                            Initializing Weixin QR binding...
                        </Typography>
                    </Box>
                );

            case 'show_qr':
                return (
                    <Stack spacing={2} alignItems="center">
                        <Typography variant="h6">Scan QR Code to Bind</Typography>
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
                            1. Open Weixin on your phone and scan the QR code
                            <br />
                            2. Confirm to complete binding
                        </Typography>
                        <Button
                            startIcon={<RefreshIcon />}
                            onClick={handleRetry}
                            variant="outlined"
                            size="small"
                        >
                            Refresh QR Code
                        </Button>
                    </Stack>
                );

            case 'scanned':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CircularProgress size={40} />
                        <Typography sx={{ mt: 2 }} color="text.secondary">
                            QR code scanned! Please confirm on your Weixin...
                        </Typography>
                    </Box>
                );

            case 'confirmed':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CheckCircleIcon sx={{ fontSize: 60, color: 'success.main', mb: 2 }} />
                        <Typography variant="h6" color="success.main">
                            Weixin Binding Successful!
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            Your bot is now connected to Weixin.
                        </Typography>
                    </Box>
                );

            case 'expired':
                return (
                    <Stack spacing={2} alignItems="center">
                        <Alert severity="warning">
                            QR code expired. Please refresh to get a new one.
                        </Alert>
                        <Button
                            startIcon={<RefreshIcon />}
                            onClick={handleRetry}
                            variant="contained"
                        >
                            Get New QR Code
                        </Button>
                    </Stack>
                );

            case 'error':
                return (
                    <Alert
                        severity="error"
                        action={
                            <Button color="inherit" size="small" onClick={startQRLogin}>
                                Retry
                            </Button>
                        }
                    >
                        {error || 'An error occurred during Weixin binding'}
                    </Alert>
                );

            default:
                return null;
        }
    };

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
                {renderContent()}
            </Box>
        </Box>
    );
};

export default WeixinQRAuth;
