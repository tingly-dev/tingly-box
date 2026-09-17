import { useCallback, useState } from 'react';
import {
    Chip,
    Dialog,
    DialogContent,
    DialogTitle,
    Divider,
    FormControlLabel,
    List,
    ListItemButton,
    ListItemIcon,
    ListItemText,
    Radio,
    Switch,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { BotSettings } from '@/types/bot';
import { ccProfileIdFromDefaultAgent } from '@/types/bot';
import type { ProfileInfo } from '@/contexts/ProfileContext';

interface CCProfileDialogProps {
    open: boolean;
    bot: BotSettings | null;
    profiles: ProfileInfo[];
    /** Persists the selection; profileId '' = default (main claude_code scenario). */
    onSelect: (uuid: string, profileId: string) => Promise<void>;
    /**
     * Persists the persistent-session toggle. Independent of profile
     * selection — which profile runs and whether its process stays warm
     * between messages are orthogonal settings.
     */
    onTogglePersistentSession: (uuid: string, enabled: boolean) => Promise<void>;
    onClose: () => void;
}

// CCProfileDialog picks which Claude Code configuration serves @cc for a bot:
// the default claude_code scenario or one of the configured profiles. The
// selection routes remote @cc executions through the profiled scenario with
// the profile's models and env overrides — same as `tingly-box cc --profile`.
const CCProfileDialog: React.FC<CCProfileDialogProps> = ({
    open,
    bot,
    profiles,
    onSelect,
    onTogglePersistentSession,
    onClose,
}) => {
    const { t } = useTranslation();
    const [saving, setSaving] = useState(false);
    const [togglingPersistentSession, setTogglingPersistentSession] = useState(false);

    const currentId = ccProfileIdFromDefaultAgent(bot?.default_agent);

    const handlePick = useCallback(async (profileId: string) => {
        if (!bot?.uuid || saving) return;
        setSaving(true);
        try {
            await onSelect(bot.uuid, profileId);
            onClose();
        } finally {
            setSaving(false);
        }
    }, [bot, saving, onSelect, onClose]);

    const handleTogglePersistentSession = useCallback(async (enabled: boolean) => {
        if (!bot?.uuid || togglingPersistentSession) return;
        setTogglingPersistentSession(true);
        try {
            await onTogglePersistentSession(bot.uuid, enabled);
        } finally {
            setTogglingPersistentSession(false);
        }
    }, [bot, togglingPersistentSession, onTogglePersistentSession]);

    if (!open) return null;

    return (
        <Dialog open={open} onClose={saving ? undefined : onClose} maxWidth="xs" fullWidth>
            <DialogTitle sx={{ textAlign: 'center' }}>
                <Typography variant="h6">
                    {t('remoteAgent.ccProfile.dialogTitle', { defaultValue: 'Claude Code Profile for @cc' })}
                </Typography>
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {t('remoteAgent.ccProfile.dialogSubtitle', {
                        defaultValue: 'Remote @cc sessions route through the selected profile — its rules, model mapping, and settings overrides.',
                    })}
                </Typography>
            </DialogTitle>
            <DialogContent>
                <List dense disablePadding>
                    <ListItemButton
                        selected={currentId === ''}
                        disabled={saving}
                        onClick={() => handlePick('')}
                    >
                        <ListItemIcon sx={{ minWidth: 36 }}>
                            <Radio edge="start" checked={currentId === ''} tabIndex={-1} disableRipple size="small" />
                        </ListItemIcon>
                        <ListItemText
                            primary={t('remoteAgent.ccProfile.default', { defaultValue: 'Default' })}
                            secondary={t('remoteAgent.ccProfile.defaultSecondary', { defaultValue: 'Main claude_code scenario' })}
                        />
                    </ListItemButton>
                    {profiles.map((p) => (
                        <ListItemButton
                            key={p.id}
                            selected={currentId === p.id}
                            disabled={saving}
                            onClick={() => handlePick(p.id)}
                        >
                            <ListItemIcon sx={{ minWidth: 36 }}>
                                <Radio edge="start" checked={currentId === p.id} tabIndex={-1} disableRipple size="small" />
                            </ListItemIcon>
                            <ListItemText
                                primary={p.name}
                                secondary={`claude_code:${p.id}`}
                            />
                            <Chip
                                label={p.unified
                                    ? t('remoteAgent.ccProfile.unified', { defaultValue: 'unified' })
                                    : t('remoteAgent.ccProfile.separate', { defaultValue: 'separate' })}
                                size="small"
                                variant="outlined"
                                sx={{ height: 20, fontSize: '0.65rem' }}
                            />
                        </ListItemButton>
                    ))}
                </List>
                {profiles.length === 0 && (
                    <Typography variant="body2" sx={{ color: 'text.secondary', mt: 1, textAlign: 'center' }}>
                        {t('remoteAgent.ccProfile.empty', {
                            defaultValue: 'No Claude Code profiles yet. Create one on the Claude Code scenario page first.',
                        })}
                    </Typography>
                )}
                <Divider sx={{ my: 1.5 }} />
                <FormControlLabel
                    sx={{ mx: 0, alignItems: 'flex-start' }}
                    control={
                        <Switch
                            checked={Boolean(bot?.persistent_session)}
                            disabled={togglingPersistentSession}
                            onChange={(e) => handleTogglePersistentSession(e.target.checked)}
                            sx={{ mt: -0.5 }}
                        />
                    }
                    label={
                        <>
                            <Typography variant="body2">
                                {t('remoteAgent.ccProfile.persistentSession', { defaultValue: 'Keep the process warm between messages' })}
                            </Typography>
                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                {t('remoteAgent.ccProfile.persistentSessionCaption', {
                                    defaultValue: 'Experimental: skips process startup cost on every @cc message by reusing one Claude Code process across a chat’s turns.',
                                })}
                            </Typography>
                        </>
                    }
                />
            </DialogContent>
        </Dialog>
    );
};

export default CCProfileDialog;
