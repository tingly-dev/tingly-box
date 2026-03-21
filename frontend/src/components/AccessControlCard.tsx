import React, { useState, useEffect } from 'react';
import {
    Box,
    Stack,
    Typography,
    Button,
    IconButton,
    Alert,
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    CircularProgress,
    Tooltip,
} from '@mui/material';
import {
    Refresh as RefreshIcon,
    ContentCopy as ContentCopyIcon,
    CheckCircle as CheckCircleIcon,
    Warning as WarningIcon,
    Visibility as VisibilityIcon,
} from '@mui/icons-material';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { useAuth } from '@/contexts/AuthContext';

interface TokenInfo {
    token: string;
    is_default: boolean;
}

interface AccessControlCardProps {
    title?: string;
}

const AccessControlCard: React.FC<AccessControlCardProps> = ({ title }) => {
    const { t } = useTranslation();
    const { login } = useAuth();
    const [userTokenInfo, setUserTokenInfo] = useState<TokenInfo | null>(null);
    const [modelToken, setModelToken] = useState<string>('');
    const [loading, setLoading] = useState(true);
    const [resetting, setResetting] = useState(false);
    const [resetDialogOpen, setResetDialogOpen] = useState(false);
    const [successToken, setSuccessToken] = useState<string | null>(null);
    const [copied, setCopied] = useState(false);
    const [fullTokenDialogOpen, setFullTokenDialogOpen] = useState(false);

    const loadTokenInfo = async () => {
        setLoading(true);
        const result = await api.getUserAuthTokenInfo();
        if (result.success && result.data) {
            setUserTokenInfo(result.data);
        }
        setLoading(false);
    };

    const loadModelToken = async () => {
        const result = await api.getToken();
        if (result.success && result.token) {
            setModelToken(result.token);
        }
    };

    useEffect(() => {
        loadTokenInfo();
        loadModelToken();
    }, []);

    const handleCopyToken = async (token: string) => {
        try {
            await navigator.clipboard.writeText(token);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy token:', err);
        }
    };

    const handleResetClick = () => {
        setResetDialogOpen(true);
    };

    const handleResetConfirm = async () => {
        setResetting(true);
        setResetDialogOpen(false);

        const result = await api.resetUserToken();
        if (result.success && result.data?.token) {
            setSuccessToken(result.data.token);
            // Update auth context with new token
            await login(result.data.token);
            // Reload token info
            await loadTokenInfo();
        }

        setResetting(false);
    };

    const handleSuccessAcknowledge = () => {
        setSuccessToken(null);
    };

    const maskToken = (token: string): string => {
        if (!token || token.includes('...')) return token;
        if (token.length <= 16) {
            return `${token.slice(0, 4)}...${token.slice(-4)}`;
        }
        return `${token.slice(0, 12)}...${token.slice(-4)}`;
    };

    if (loading) {
        return (
            <Box sx={{ p: 3, display: 'flex', justifyContent: 'center' }}>
                <CircularProgress size={24} />
            </Box>
        );
    }

    const isUsingDefaultToken = userTokenInfo?.is_default ?? false;

    return (
        <>
            <Stack spacing={2}>
                {/* Security Warning for Default Token */}
                {isUsingDefaultToken && (
                    <Alert severity="warning" icon={<WarningIcon />}>
                        <Typography variant="body2" fontWeight={500}>
                            {t('system.accessControl.warning.default')}
                        </Typography>
                        <Button
                            size="small"
                            variant="outlined"
                            onClick={handleResetClick}
                            sx={{ mt: 1 }}
                        >
                            {t('system.accessControl.warning.resetNow')}
                        </Button>
                    </Alert>
                )}

                {/* Success Banner after Reset */}
                {successToken && (
                    <Alert severity="success" icon={<CheckCircleIcon />}>
                        <Typography variant="body2" fontWeight={500}>
                            {t('system.accessControl.success.title')}
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 0.5 }}>
                            {t('system.accessControl.success.message')}
                        </Typography>
                        <Box
                            sx={{
                                mt: 1.5,
                                p: 1,
                                bgcolor: 'action.hover',
                                borderRadius: 1,
                                fontFamily: 'monospace',
                                fontSize: '0.875rem',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'space-between',
                                gap: 1,
                            }}
                        >
                            <Typography
                                variant="body2"
                                sx={{
                                    fontFamily: 'monospace',
                                    wordBreak: 'break-all',
                                    flex: 1,
                                }}
                            >
                                {successToken}
                            </Typography>
                            <Button
                                size="small"
                                startIcon={<ContentCopyIcon />}
                                onClick={() => handleCopyToken(successToken)}
                            >
                                {copied ? t('system.accessControl.copied') : t('system.accessControl.copy')}
                            </Button>
                        </Box>
                        <Button
                            size="small"
                            variant="contained"
                            onClick={handleSuccessAcknowledge}
                            sx={{ mt: 1.5 }}
                        >
                            {t('system.accessControl.success.saved')}
                        </Button>
                    </Alert>
                )}

                {/* User Token Section */}
                <Box>
                    <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                        {t('system.accessControl.userToken')}
                    </Typography>
                    <Box
                        sx={{
                            p: 1.5,
                            bgcolor: 'action.hover',
                            borderRadius: 1,
                            fontFamily: 'monospace',
                            fontSize: '0.875rem',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                            gap: 1,
                        }}
                    >
                        <Typography
                            variant="body2"
                            sx={{
                                fontFamily: 'monospace',
                                wordBreak: 'break-all',
                                flex: 1,
                            }}
                        >
                            {successToken || userTokenInfo?.token || 'N/A'}
                        </Typography>
                        <Tooltip title={t('system.accessControl.copy') || 'Copy'}>
                            <IconButton
                                size="small"
                                onClick={() => handleCopyToken(successToken || userTokenInfo?.token || '')}
                                disabled={successToken ? false : !userTokenInfo?.token}
                            >
                                <ContentCopyIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    </Box>
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                        {t('system.accessControl.userTokenDesc')}
                    </Typography>
                    {!isUsingDefaultToken && !successToken && (
                        <Box sx={{ mt: 1.5, display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                            <Typography variant="body2" color="success.main" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                                <CheckCircleIcon fontSize="inherit" />
                                {t('system.accessControl.secure')}
                            </Typography>
                            <Button size="small" variant="outlined" onClick={handleResetClick} disabled={resetting}>
                                {t('system.accessControl.resetToken')}
                            </Button>
                            <Button
                                size="small"
                                variant="text"
                                startIcon={<VisibilityIcon />}
                                onClick={() => setFullTokenDialogOpen(true)}
                            >
                                {t('system.accessControl.viewFullToken')}
                            </Button>
                        </Box>
                    )}
                </Box>

                {/* Divider */}
                <Box sx={{ borderTop: 1, borderColor: 'divider', my: 1 }} />

                {/* Model Token Section */}
                <Box>
                    <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                        {t('system.accessControl.modelToken')}
                    </Typography>
                    <Box
                        sx={{
                            p: 1.5,
                            bgcolor: 'action.hover',
                            borderRadius: 1,
                            fontFamily: 'monospace',
                            fontSize: '0.875rem',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                            gap: 1,
                        }}
                    >
                        <Typography
                            variant="body2"
                            sx={{
                                fontFamily: 'monospace',
                                wordBreak: 'break-all',
                                flex: 1,
                            }}
                        >
                            {maskToken(modelToken)}
                        </Typography>
                        <Tooltip title={t('system.accessControl.copy') || 'Copy'}>
                            <IconButton size="small" onClick={() => handleCopyToken(modelToken)}>
                                <ContentCopyIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    </Box>
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                        {t('system.accessControl.modelTokenDesc')}
                    </Typography>
                </Box>
            </Stack>

            {/* Reset Confirmation Dialog */}
            <Dialog open={resetDialogOpen} onClose={() => setResetDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <WarningIcon color="warning" />
                        <Typography variant="h6">{t('system.accessControl.reset.title')}</Typography>
                    </Box>
                </DialogTitle>
                <DialogContent>
                    <Typography variant="body1" gutterBottom>
                        {t('system.accessControl.reset.confirm')}
                    </Typography>
                    <Stack sx={{ mt: 2 }} spacing={1}>
                        <Typography variant="body2" color="text.secondary">
                            • {t('system.accessControl.reset.points.new')}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            • {t('system.accessControl.reset.points.session')}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            • {t('system.accessControl.reset.points.other')}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            • {t('system.accessControl.reset.points.stop')}
                        </Typography>
                    </Stack>
                    <Alert severity="warning" sx={{ mt: 2 }}>
                        {t('system.accessControl.reset.warning')}
                    </Alert>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setResetDialogOpen(false)} disabled={resetting}>
                        {t('system.accessControl.reset.cancel')}
                    </Button>
                    <Button
                        onClick={handleResetConfirm}
                        variant="contained"
                        color="warning"
                        disabled={resetting}
                        startIcon={resetting ? <CircularProgress size={16} /> : undefined}
                    >
                        {resetting ? t('system.accessControl.resetting') : t('system.accessControl.reset.confirm')}
                    </Button>
                </DialogActions>
            </Dialog>

            {/* View Full Token Dialog */}
            <Dialog open={fullTokenDialogOpen} onClose={() => setFullTokenDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{t('system.accessControl.viewFullToken')}</DialogTitle>
                <DialogContent>
                    <Alert severity="warning" sx={{ mb: 2 }}>
                        <Typography variant="body2">
                            {t('system.accessControl.fullTokenWarning')}
                        </Typography>
                    </Alert>
                    <Box
                        sx={{
                            p: 2,
                            bgcolor: 'action.hover',
                            borderRadius: 1,
                            fontFamily: 'monospace',
                            fontSize: '0.875rem',
                            wordBreak: 'break-all',
                            userSelect: 'all',
                        }}
                    >
                        {userTokenInfo?.token || 'N/A'}
                    </Box>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setFullTokenDialogOpen(false)}>
                        {t('close')}
                    </Button>
                    <Button
                        variant="contained"
                        startIcon={<ContentCopyIcon />}
                        onClick={() => handleCopyToken(userTokenInfo?.token || '')}
                    >
                        {copied ? t('system.accessControl.copied') : t('system.accessControl.copy')}
                    </Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

export default AccessControlCard;
