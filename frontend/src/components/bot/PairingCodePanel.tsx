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

const formatRemaining = (expiresAt: string): string => {
    const ms = new Date(expiresAt).getTime() - Date.now();
    if (Number.isNaN(ms) || ms <= 0) return 'expired';
    const total = Math.floor(ms / 1000);
    const m = Math.floor(total / 60);
    const s = total % 60;
    return m > 0 ? `${m}m ${s}s` : `${s}s`;
};

interface Props {
    bot: BotSettings;
}

const PairingCodePanel: React.FC<Props> = ({ bot }) => {
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
                setMessage(res.error || 'Failed to fetch pairing code');
            }
        } finally {
            setLoading(false);
        }
    }, [bot.uuid]);

    useEffect(() => {
        if (!required) return;
        fetchCode();
    }, [required, fetchCode, bot.enabled]);

    const handleReveal = useCallback(() => setRevealed((v) => !v), []);

    const handleCopy = useCallback(async () => {
        if (!code) return;
        try {
            await navigator.clipboard.writeText(`/bind ${code}`);
            notify.success('Pairing command copied');
        } catch {
            notify.error('Copy failed — check clipboard permissions');
        }
    }, [code]);

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
                notify.success('Pairing code rotated');
            } else {
                notify.error(res.error || res.message || 'Rotate failed');
            }
        } finally {
            setLoading(false);
        }
    }, [bot.uuid]);

    if (!required) return null;

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
                Pairing code:
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
                            expires in {formatRemaining(expiresAt)}
                        </Typography>
                    )}
                    <Tooltip title={revealed ? 'Hide' : 'Reveal'}>
                        <IconButton size="small" onClick={handleReveal}>
                            {revealed ? <HideIcon fontSize="inherit" /> : <RevealIcon fontSize="inherit" />}
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy">
                        <IconButton size="small" onClick={handleCopy} disabled={!code}>
                            <CopyIcon fontSize="inherit" />
                        </IconButton>
                    </Tooltip>
                </>
            ) : (
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {message || 'No active code — bot may be stopped, or the code was already consumed. Click Rotate to mint a new one.'}
                </Typography>
            )}

            <Tooltip title="Rotate (invalidates current code)">
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
