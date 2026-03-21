import { useState, useEffect } from 'react';
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
    Sync as SyncIcon,
    ContentCopy as ContentCopyIcon,
    CheckCircle as CheckCircleIcon,
    Warning as WarningIcon,
    Visibility as VisibilityIcon,
    VisibilityOff as VisibilityOffIcon,
    Security as SecurityIcon,
} from '@mui/icons-material';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { useAuth } from '@/contexts/AuthContext';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';

interface TokenInfo {
    token: string;
    is_default: boolean;
}

const AccessControl = () => {
    const { t } = useTranslation();
    const { login } = useAuth();
    const [userTokenInfo, setUserTokenInfo] = useState<TokenInfo | null>(null);
    const [modelToken, setModelToken] = useState<string>('');
    const [loading, setLoading] = useState(true);
    const [resettingUser, setResettingUser] = useState(false);
    const [resettingModel, setResettingModel] = useState(false);
    const [resetUserDialogOpen, setResetUserDialogOpen] = useState(false);
    const [resetModelDialogOpen, setResetModelDialogOpen] = useState(false);
    const [userSuccessToken, setUserSuccessToken] = useState<string | null>(null);
    const [modelSuccessToken, setModelSuccessToken] = useState<string | null>(null);
    const [copied, setCopied] = useState(false);

    // Visibility states for showing full tokens
    const [showUserToken, setShowUserToken] = useState(false);
    const [showModelToken, setShowModelToken] = useState(false);

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
        if (result && result.token) {
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

    const handleUserResetClick = () => {
        setResetUserDialogOpen(true);
    };

    const handleModelResetClick = () => {
        setResetModelDialogOpen(true);
    };

    const handleUserResetConfirm = async () => {
        setResettingUser(true);
        setResetUserDialogOpen(false);

        const result = await api.resetUserToken();
        if (result.success && result.data?.token) {
            setUserSuccessToken(result.data.token);
            // Update auth context with new token
            await login(result.data.token);
            // Reload token info
            await loadTokenInfo();
        }

        setResettingUser(false);
    };

    const handleModelResetConfirm = async () => {
        setResettingModel(true);
        setResetModelDialogOpen(false);

        const result = await api.resetModelToken();
        if (result.success && result.data?.token) {
            setModelSuccessToken(result.data.token);
            // Reload model token
            await loadModelToken();
        }

        setResettingModel(false);
    };

    const handleUserSuccessAcknowledge = () => {
        setUserSuccessToken(null);
    };

    const handleModelSuccessAcknowledge = () => {
        setModelSuccessToken(null);
    };

    const maskToken = (token: string): string => {
        if (!token) return '';
        if (token.length <= 16) {
            return `${token.slice(0, 4)}...${token.slice(-4)}`;
        }
        return `${token.slice(0, 12)}...${token.slice(-4)}`;
    };

    const isUsingDefaultToken = userTokenInfo?.is_default ?? false;

    // Get the actual token to display (considering reset success)
    const displayUserToken = userSuccessToken || userTokenInfo?.token || '';
    const displayModelToken = modelSuccessToken || modelToken;

    return (
        <PageLayout loading={loading} notification={{ open: false }}>
            <Stack spacing={3}>
                {/* Page Header Card */}
                <UnifiedCard size="full">
                    <Stack spacing={2}>
                        <Stack direction="row" alignItems="center" spacing={2}>
                            <Box sx={{ flex: 1 }}>
                                <Typography variant="h5" fontWeight={600}>
                                    {t('accessControl.pageTitle')}
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    {t('accessControl.pageDescription')}
                                </Typography>
                            </Box>
                        </Stack>
                        <Box sx={{ pt: 1 }}>
                            <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                                {t('accessControl.securityInfo.title')}
                            </Typography>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                {t('accessControl.securityInfo.description')}
                            </Typography>
                            <Stack spacing={0.5}>
                                <Typography variant="body2" sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                                    <Box component="span" sx={{ color: 'success.main', minWidth: 20 }}>✓</Box>
                                    {t('accessControl.securityInfo.point1')}
                                </Typography>
                                <Typography variant="body2" sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                                    <Box component="span" sx={{ color: 'success.main', minWidth: 20 }}>✓</Box>
                                    {t('accessControl.securityInfo.point2')}
                                </Typography>
                                <Typography variant="body2" sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                                    <Box component="span" sx={{ color: 'warning.main', minWidth: 20 }}>!</Box>
                                    {t('accessControl.securityInfo.point3')}
                                </Typography>
                            </Stack>
                        </Box>
                    </Stack>
                </UnifiedCard>

                {/* Security Warning for Default Token */}
                {isUsingDefaultToken && (
                    <Alert severity="warning" icon={<WarningIcon />}>
                        <Typography variant="body2" fontWeight={500}>
                            {t('accessControl.warning.default')}
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 1 }}>
                            {t('accessControl.warning.description')}
                        </Typography>
                        <Button
                            size="small"
                            variant="outlined"
                            onClick={handleUserResetClick}
                            sx={{ mt: 1 }}
                        >
                            {t('accessControl.warning.resetNow')}
                        </Button>
                    </Alert>
                )}

                {/* Success Banner after User Token Reset */}
                {userSuccessToken && (
                    <Alert severity="success" icon={<CheckCircleIcon />}>
                        <Typography variant="body2" fontWeight={500}>
                            {t('accessControl.userToken.resetSuccess')}
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 0.5 }}>
                            {t('accessControl.userToken.resetSuccessMessage')}
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
                                {userSuccessToken}
                            </Typography>
                            <Button
                                size="small"
                                startIcon={<ContentCopyIcon />}
                                onClick={() => handleCopyToken(userSuccessToken)}
                            >
                                {copied ? t('accessControl.copied') : t('accessControl.copy')}
                            </Button>
                        </Box>
                        <Button
                            size="small"
                            variant="contained"
                            onClick={handleUserSuccessAcknowledge}
                            sx={{ mt: 1.5 }}
                        >
                            {t('accessControl.userToken.saved')}
                        </Button>
                    </Alert>
                )}

                {/* User Token Card */}
                <UnifiedCard
                    title={t('accessControl.userToken.title')}
                    size="full"
                    rightAction={
                        !isUsingDefaultToken && !userSuccessToken && (
                            <Button
                                size="small"
                                variant="text"
                                color="warning"
                                onClick={handleUserResetClick}
                                disabled={resettingUser}
                                startIcon={resettingUser ? <CircularProgress size={16} /> : <SyncIcon />}
                            >
                                {resettingUser ? t('accessControl.resetting') : t('accessControl.userToken.resetToken')}
                            </Button>
                        )
                    }
                >
                    <Stack spacing={2}>
                        <Typography variant="body2" color="text.secondary">
                            {t('accessControl.userToken.description')}
                        </Typography>

                        <Box
                            sx={{
                                p: 2,
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
                                {showUserToken ? displayUserToken : maskToken(displayUserToken)}
                            </Typography>
                            <Box sx={{ display: 'flex', gap: 0.5 }}>
                                <Tooltip title={showUserToken ? 'Hide token' : 'Show token'}>
                                    <IconButton
                                        size="small"
                                        onClick={() => setShowUserToken(!showUserToken)}
                                        disabled={!displayUserToken}
                                    >
                                        {showUserToken ? <VisibilityOffIcon fontSize="small" /> : <VisibilityIcon fontSize="small" />}
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title={copied ? 'Copied!' : 'Copy token'}>
                                    <IconButton
                                        size="small"
                                        onClick={() => handleCopyToken(displayUserToken)}
                                        disabled={!displayUserToken}
                                    >
                                        <ContentCopyIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Box>
                        </Box>

                        {!isUsingDefaultToken && !userSuccessToken && (
                            <Typography variant="body2" color="success.main" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                                <CheckCircleIcon fontSize="inherit" />
                                {t('accessControl.secure')}
                            </Typography>
                        )}
                    </Stack>
                </UnifiedCard>

                {/* Success Banner after Model Token Reset */}
                {modelSuccessToken && (
                    <Alert severity="success" icon={<CheckCircleIcon />}>
                        <Typography variant="body2" fontWeight={500}>
                            {t('accessControl.modelToken.resetSuccess')}
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 0.5 }}>
                            {t('accessControl.modelToken.resetSuccessMessage')}
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
                                {modelSuccessToken}
                            </Typography>
                            <Button
                                size="small"
                                startIcon={<ContentCopyIcon />}
                                onClick={() => handleCopyToken(modelSuccessToken)}
                            >
                                {copied ? t('accessControl.copied') : t('accessControl.copy')}
                            </Button>
                        </Box>
                        <Button
                            size="small"
                            variant="contained"
                            onClick={handleModelSuccessAcknowledge}
                            sx={{ mt: 1.5 }}
                        >
                            {t('accessControl.modelToken.saved')}
                        </Button>
                    </Alert>
                )}

                {/* Model Token Card */}
                <UnifiedCard
                    title={t('accessControl.modelToken.title')}
                    size="full"
                    rightAction={
                        <Button
                            size="small"
                            variant="text"
                            color="warning"
                            onClick={handleModelResetClick}
                            disabled={resettingModel}
                            startIcon={resettingModel ? <CircularProgress size={16} /> : <SyncIcon />}
                        >
                            {resettingModel ? t('accessControl.resetting') : t('accessControl.modelToken.resetToken')}
                        </Button>
                    }
                >
                    <Stack spacing={2}>
                        <Typography variant="body2" color="text.secondary">
                            {t('accessControl.modelToken.description')}
                        </Typography>

                        <Box
                            sx={{
                                p: 2,
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
                                {showModelToken ? displayModelToken : maskToken(displayModelToken)}
                            </Typography>
                            <Box sx={{ display: 'flex', gap: 0.5 }}>
                                <Tooltip title={showModelToken ? 'Hide token' : 'Show token'}>
                                    <IconButton
                                        size="small"
                                        onClick={() => setShowModelToken(!showModelToken)}
                                        disabled={!displayModelToken}
                                    >
                                        {showModelToken ? <VisibilityOffIcon fontSize="small" /> : <VisibilityIcon fontSize="small" />}
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title={copied ? 'Copied!' : 'Copy token'}>
                                    <IconButton
                                        size="small"
                                        onClick={() => handleCopyToken(displayModelToken)}
                                        disabled={!displayModelToken}
                                    >
                                        <ContentCopyIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Box>
                        </Box>

                        <Alert severity="info" sx={{ mt: 1 }}>
                            <Typography variant="body2">
                                {t('accessControl.modelToken.sharing')}
                            </Typography>
                        </Alert>
                    </Stack>
                </UnifiedCard>
            </Stack>

            {/* User Reset Confirmation Dialog */}
            <Dialog open={resetUserDialogOpen} onClose={() => setResetUserDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <WarningIcon color="warning" />
                        <Typography variant="h6">{t('accessControl.userToken.resetTitle')}</Typography>
                    </Box>
                </DialogTitle>
                <DialogContent>
                    <Typography variant="body1" gutterBottom>
                        {t('accessControl.userToken.resetConfirm')}
                    </Typography>
                    <Stack sx={{ mt: 2 }} spacing={1}>
                        <Typography variant="body2" color="text.secondary">
                            • {t('accessControl.userToken.resetPoints.new')}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            • {t('accessControl.userToken.resetPoints.session')}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            • {t('accessControl.userToken.resetPoints.other')}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            • {t('accessControl.userToken.resetPoints.stop')}
                        </Typography>
                    </Stack>
                    <Alert severity="warning" sx={{ mt: 2 }}>
                        {t('accessControl.userToken.resetWarning')}
                    </Alert>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setResetUserDialogOpen(false)} disabled={resettingUser}>
                        {t('accessControl.userToken.resetCancel')}
                    </Button>
                    <Button
                        onClick={handleUserResetConfirm}
                        variant="contained"
                        color="warning"
                        disabled={resettingUser}
                        startIcon={resettingUser ? <CircularProgress size={16} /> : undefined}
                    >
                        {resettingUser ? t('accessControl.resetting') : t('accessControl.userToken.resetConfirmButton')}
                    </Button>
                </DialogActions>
            </Dialog>

            {/* Model Reset Confirmation Dialog */}
            <Dialog open={resetModelDialogOpen} onClose={() => setResetModelDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <WarningIcon color="warning" />
                        <Typography variant="h6">{t('accessControl.modelToken.resetTitle')}</Typography>
                    </Box>
                </DialogTitle>
                <DialogContent>
                    <Typography variant="body1" gutterBottom>
                        {t('accessControl.modelToken.resetConfirm')}
                    </Typography>
                    <Stack sx={{ mt: 2 }} spacing={1}>
                        <Typography variant="body2" color="text.secondary">
                            • {t('accessControl.modelToken.resetPoints.new')}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            • {t('accessControl.modelToken.resetPoints.stop')}
                        </Typography>
                    </Stack>
                    <Alert severity="warning" sx={{ mt: 2 }}>
                        {t('accessControl.modelToken.resetWarning')}
                    </Alert>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setResetModelDialogOpen(false)} disabled={resettingModel}>
                        {t('accessControl.modelToken.resetCancel')}
                    </Button>
                    <Button
                        onClick={handleModelResetConfirm}
                        variant="contained"
                        color="warning"
                        disabled={resettingModel}
                        startIcon={resettingModel ? <CircularProgress size={16} /> : undefined}
                    >
                        {resettingModel ? t('accessControl.resetting') : t('accessControl.modelToken.resetConfirmButton')}
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default AccessControl;
