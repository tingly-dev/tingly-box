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
import { api } from '@/services/api';

interface WeChatQRAuthProps {
    botUUID: string;
    platform: string;
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

export const WeChatQRAuth: React.FC<WeChatQRAuthProps> = ({ botUUID, platform }) => {
    const [state, setState] = useState<QRState>('idle');
    const [qrData, setQrData] = useState<string>('');
    const [qrId, setQrId] = useState<string>('');
    const [error, setError] = useState<string>('');
    const [refreshCount, setRefreshCount] = useState(0);

    const MAX_REFRESH = 3;

    const startQRLogin = useCallback(async () => {
        if (!botUUID) {
            setError('Bot UUID is required');
            setState('error');
            return;
        }

        setState('loading');
        setError('');

        try {
            const response = await api.wechatQRStart(botUUID);

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
    }, [botUUID]);

    const pollQRStatus = useCallback(async () => {
        if (!botUUID || !qrId) return;

        try {
            const response = await api.wechatQRStatus(botUUID, qrId);

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
                    setState('confirmed');
                    // Stop polling on success
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
    }, [botUUID, qrId, refreshCount, startQRLogin]);

    // Start QR login when component mounts
    useEffect(() => {
        if (state === 'idle' && botUUID) {
            startQRLogin();
        }
    }, [state, botUUID, startQRLogin]);

    // Poll QR status every 2 seconds when showing QR or scanned
    useEffect(() => {
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
                            Initializing WeChat QR binding...
                        </Typography>
                    </Box>
                );

            case 'show_qr':
                return (
                    <Stack spacing={2} alignItems="center">
                        <Typography variant="h6">Scan QR Code to Bind</Typography>
                        <Paper sx={{ p: 2, bgcolor: 'background.paper' }}>
                            {/* QR code rendering - use a simple data URL for now */}
                            <Box
                                component="img"
                                src={`https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(qrData)}`}
                                alt="WeChat QR Code"
                                sx={{
                                    width: 200,
                                    height: 200,
                                    border: '1px solid',
                                    borderColor: 'divider',
                                    borderRadius: 1,
                                }}
                            />
                        </Paper>
                        <Typography variant="body2" color="text.secondary" align="center">
                            1. Open WeChat on your phone and scan the QR code
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
                            QR code scanned! Please confirm on your WeChat...
                        </Typography>
                    </Box>
                );

            case 'confirmed':
                return (
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', py: 4 }}>
                        <CheckCircleIcon sx={{ fontSize: 60, color: 'success.main', mb: 2 }} />
                        <Typography variant="h6" color="success.main">
                            WeChat Binding Successful!
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            Your bot is now connected to WeChat.
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
                        {error || 'An error occurred during WeChat binding'}
                    </Alert>
                );

            default:
                return null;
        }
    };

    return (
        <Box sx={{ p: 2 }}>
            <Typography variant="subtitle2" gutterBottom>
                WeChat QR Code Binding
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

export default WeChatQRAuth;
