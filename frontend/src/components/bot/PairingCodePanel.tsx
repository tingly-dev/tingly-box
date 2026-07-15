import {
    ContentCopy as CopyIcon,
    Refresh as RotateIcon,
    Visibility as RevealIcon,
    VisibilityOff as HideIcon,
} from '@/components/icons';
import {
    Box,
    CircularProgress,
    IconButton,
    Tooltip,
    Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { notify } from '@/utils/notify';
import type { BotSettings } from '@/types/bot';

// Token-DM platforms default to TOFU pairing on. Mirrors
// bot.PlatformDefaultsRequirePairing on the backend.
const PLATFORM_DEFAULT_REQUIRE_PAIRING: Record<string, boolean> = {
    telegram: true,
    discord: true,
    slack: true,
};

const isPairingRequired = (bot: BotSettings): boolean => {
    if (typeof bot.require_pairing === 'boolean') {
        return bot.require_pairing;
    }
    return Boolean(PLATFORM_DEFAULT_REQUIRE_PAIRING[bot.platform || '']);
};

// Returns null once expired so the caller can render a localized label.
const formatRemaining = (expiresAt: string): string | null => {
    const ms = new Date(expiresAt).getTime() - Date.now();
    if (Number.isNaN(ms) || ms <= 0) return null;
    const total = Math.floor(ms / 1000);
    const m = Math.floor(total / 60);
    const s = total % 60;
    return m > 0 ? `${m}m ${s}s` : `${s}s`;
};

interface Props {
    bot: BotSettings;
}

const PairingCodePanel: React.FC<Props> = ({ bot }) => {
    const { t } = useTranslation();
    const required = useMemo(() => isPairingRequired(bot), [bot]);

    const [loading, setLoading] = useState(false);
    const [active, setActive] = useState(false);
    const [code, setCode] = useState('');
    const [expiresAt, setExpiresAt] = useState('');
    const [message, setMessage] = useState('');
    const [revealed, setRevealed] = useState(false);

    // Re-render countdown once per second while a code is active and revealed
    const [, setTick] = useState(0);
    useEffect(() => {
        if (!active || !revealed) return;
        const id = window.setInterval(() => setTick((t) => t + 1), 1000);
        return () => window.clearInterval(id);
    }, [active, revealed]);

    const fetchCode = useCallback(async () => {
        if (!bot.uuid) return;
        setLoading(true);
        try {
            const res = await api.getImBotPairingCode(bot.uuid);
            if (res.success) {
                setActive(Boolean(res.active));
                setCode(res.code || '');
                setExpiresAt(res.expires_at || '');
                setMessage(res.message || '');
            } else {
                setActive(false);
                setCode('');
                setExpiresAt('');
                setMessage(res.error || t('remoteControl.pairing.fetchFailed', { defaultValue: 'Failed to fetch pairing code' }));
            }
        } finally {
            setLoading(false);
        }
    }, [bot.uuid, t]);

    useEffect(() => {
        if (!required) return;
        fetchCode();
    }, [required, fetchCode, bot.enabled]);

    const handleReveal = useCallback(() => setRevealed((v) => !v), []);

    const handleCopy = useCallback(async () => {
        if (!code) return;
        try {
            await navigator.clipboard.writeText(`/bind ${code}`);
            notify.success(t('remoteControl.pairing.copied', { defaultValue: 'Pairing command copied' }));
        } catch {
            notify.error(t('remoteControl.pairing.copyFailed', { defaultValue: 'Copy failed — check clipboard permissions' }));
        }
    }, [code, t]);

    const handleRotate = useCallback(async () => {
        if (!bot.uuid) return;
        setLoading(true);
        try {
            const res = await api.rotateImBotPairingCode(bot.uuid);
            if (res.success && res.code) {
                setActive(true);
                setCode(res.code);
                setExpiresAt(res.expires_at || '');
                setMessage('');
                setRevealed(true);
                notify.success(t('remoteControl.pairing.rotated', { defaultValue: 'Pairing code rotated' }));
            } else {
                notify.error(res.error || res.message || t('remoteControl.pairing.rotateFailed', { defaultValue: 'Rotate failed' }));
            }
        } finally {
            setLoading(false);
        }
    }, [bot.uuid, t]);

    if (!required) return null;

    // null once the code has lapsed (see formatRemaining); drives the
    // "expires in {time}" vs "expired" label below.
    const remaining = expiresAt ? formatRemaining(expiresAt) : null;
    const expiryLabel = remaining
        ? t('remoteControl.pairing.expiresIn', { defaultValue: 'expires in {{time}}', time: remaining })
        : t('remoteControl.pairing.expired', { defaultValue: 'expired' });

    return (
        <Box
            sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                flexWrap: 'wrap',
                pt: 0.5,
            }}
        >
            <Typography variant="caption" sx={{ color: 'text.secondary', fontWeight: 600 }}>
                {t('remoteControl.pairing.label', { defaultValue: 'Pairing code:' })}
            </Typography>

            {loading && !code ? (
                <CircularProgress size={14} />
            ) : active ? (
                <>
                    <Typography
                        component="span"
                        variant="caption"
                        aria-label={revealed ? 'pairing code' : 'hidden pairing code'}
                        sx={{
                            fontFamily: 'monospace',
                            fontSize: '0.85rem',
                            letterSpacing: revealed ? 0.5 : 2,
                            backgroundColor: 'action.hover',
                            px: 1,
                            py: 0.25,
                            borderRadius: 1,
                        }}
                    >
                        {revealed ? `/bind ${code}` : '••••••••'}
                    </Typography>
                    {expiresAt && (
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                            {expiryLabel}
                        </Typography>
                    )}
                    <Tooltip title={revealed ? t('remoteControl.pairing.hide', { defaultValue: 'Hide' }) : t('remoteControl.pairing.reveal', { defaultValue: 'Reveal' })}>
                        <IconButton size="small" onClick={handleReveal}>
                            {revealed ? <HideIcon fontSize="inherit" /> : <RevealIcon fontSize="inherit" />}
                        </IconButton>
                    </Tooltip>
                    <Tooltip title={t('remoteControl.pairing.copy', { defaultValue: 'Copy' })}>
                        <IconButton size="small" onClick={handleCopy} disabled={!code}>
                            <CopyIcon fontSize="inherit" />
                        </IconButton>
                    </Tooltip>
                </>
            ) : (
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {message || t('remoteControl.pairing.noActiveCode', { defaultValue: 'No active code — bot may be stopped, or the code was already consumed. Click Rotate to mint a new one.' })}
                </Typography>
            )}

            <Tooltip title={t('remoteControl.pairing.rotateTooltip', { defaultValue: 'Rotate (invalidates current code)' })}>
                <span>
                    <IconButton size="small" onClick={handleRotate} disabled={loading}>
                        <RotateIcon fontSize="inherit" />
                    </IconButton>
                </span>
            </Tooltip>
        </Box>
    );
};

export default PairingCodePanel;
