import {Delete as DeleteIcon, Edit as EditIcon, RestartAlt as RestartIcon} from '@/components/icons';
import {
    Box,
    CardContent,
    Chip,
    Collapse,
    IconButton,
    Stack,
    Switch,
    Tooltip,
    Typography
} from '@mui/material';
import ConfirmDialog from '@/components/ConfirmDialog';
import type {BotSettings} from '@/types/bot';
import {isRemoteAgentMounted, isNotifyMounted} from '@/types/bot';
import {botCardSx, statusChipSx} from './botCardStyles';
import PairingCodePanel from './PairingCodePanel';
import {useCallback, useState} from 'react';
import {useNavigate} from 'react-router-dom';
import {useTranslation} from 'react-i18next';

interface BotCardProps {
    bot: BotSettings;
    onEdit: () => void;
    onDelete: () => void;
    onBotToggle: () => void;
    onRestart: () => void;
    isToggling?: boolean;
    isRestarting?: boolean;
}

// BotCard is the RESOURCE card: it manages the bot connection itself
// (enable/restart/edit/delete, pairing, proxy). What the bot is used FOR
// (Remote Agent, notifications) is configured on the purpose pages — the card
// only surfaces mount status as a link to the next action.
const BotCard: React.FC<BotCardProps> = ({
    bot,
    onEdit,
    onDelete,
    onBotToggle,
    onRestart,
    isToggling = false,
    isRestarting = false,
}) => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const isActive = bot.enabled ?? true;
    const isMounted = isRemoteAgentMounted(bot.scenarios);
    const isNotified = isNotifyMounted(bot.scenarios);
    const [deleteModalOpen, setDeleteModalOpen] = useState(false);

    const handleDeleteClick = useCallback(() => setDeleteModalOpen(true), []);
    const handleConfirmDelete = useCallback(() => {
        setDeleteModalOpen(false);
        onDelete();
    }, [onDelete]);

    return (
        <Box sx={botCardSx(isActive)}>
            {/* Header */}
            <Box sx={{display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 1, minHeight: 56, px: 2, py: 1.5}}>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1, minWidth: 0}}>
                    <Tooltip title={bot.name || bot.platform}>
                        <Typography noWrap variant="body2" sx={{fontWeight: 600, minWidth: 0}}>
                            {bot.name || bot.platform}
                        </Typography>
                    </Tooltip>
                    {/* Chips never shrink or truncate — the name above gives
                        up its width first, so this group stays predictable
                        (same size, same relative position) across cards.
                        The platform chip is unconditional: when it was only
                        shown alongside a custom alias, an unnamed bot (whose
                        main text falls back to the platform id) skipped it
                        entirely, shifting every chip after it out of line
                        with named bots' rows. */}
                    <Box sx={{display: 'flex', alignItems: 'center', gap: 1, flexShrink: 0, flexWrap: 'wrap'}}>
                        <Chip label={bot.platform} size="small"/>
                        {/* Purpose status: where this bot is used. Click-through to configure. */}
                        <Tooltip title={t('bots.card.remoteAgentChipHint', { defaultValue: 'Configure on the Remote Control page' })}>
                            <Chip
                                label={t('bots.card.remoteAgentChip', { defaultValue: 'Remote Control' })}
                                size="small"
                                variant={isMounted ? 'filled' : 'outlined'}
                                color={isMounted ? 'primary' : 'default'}
                                onClick={() => navigate(`/remote-agent/${bot.platform}`)}
                            />
                        </Tooltip>
                        <Tooltip title={t('bots.card.notifyChipHint', { defaultValue: 'Configure on the Notify page' })}>
                            <Chip
                                label={t('bots.card.notifyChip', { defaultValue: 'IM Notify' })}
                                size="small"
                                variant={isNotified ? 'filled' : 'outlined'}
                                color={isNotified ? 'primary' : 'default'}
                                onClick={() => navigate('/notify')}
                            />
                        </Tooltip>
                    </Box>
                </Box>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.5}}>
                    <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                        <Tooltip title={isActive ? t('remoteControl.card.disableBot', { defaultValue: 'Disable Bot' }) : t('remoteControl.card.enableBot', { defaultValue: 'Enable Bot' })}>
                            <Switch checked={isActive} onChange={() => onBotToggle()} size="small" color="success" disabled={isToggling}/>
                        </Tooltip>
                        <Chip
                            label={isActive ? t('common.on', { defaultValue: 'On' }) : t('common.off', { defaultValue: 'Off' })}
                            size="small"
                            color={isActive ? 'success' : 'default'}
                            variant={isActive ? 'filled' : 'outlined'}
                            sx={statusChipSx}
                        />
                    </Stack>
                    <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                        <Tooltip title={isActive ? t('remoteControl.card.restartBot', { defaultValue: 'Restart Bot' }) : t('remoteControl.card.enableToRestart', { defaultValue: 'Enable bot to restart' })}>
                            <span>
                                <IconButton size="small" color="primary" onClick={onRestart} disabled={!isActive || isToggling || isRestarting}>
                                    <RestartIcon fontSize="small"/>
                                </IconButton>
                            </span>
                        </Tooltip>
                        <Tooltip title={t('remoteControl.card.edit', { defaultValue: 'Edit' })}>
                            <IconButton size="small" color="primary" onClick={onEdit} disabled={isToggling || isRestarting}>
                                <EditIcon fontSize="small"/>
                            </IconButton>
                        </Tooltip>
                        <Tooltip title={t('remoteControl.card.delete', { defaultValue: 'Delete' })}>
                            <IconButton size="small" color="error" onClick={handleDeleteClick} disabled={isToggling || isRestarting}>
                                <DeleteIcon fontSize="small"/>
                            </IconButton>
                        </Tooltip>
                    </Stack>
                </Box>
            </Box>

            <Collapse in timeout="auto" unmountOnExit>
                <CardContent sx={{pt: 0, pb: 1}}>
                    <Box sx={{display: 'flex', flexDirection: 'column', gap: 1}}>
                        <PairingCodePanel bot={bot}/>
                        {bot.proxy_url && (
                            <Tooltip title={bot.proxy_url}>
                                <Typography variant="caption" sx={{color: 'text.secondary', fontFamily: 'monospace'}}>
                                    {t('remoteControl.card.proxyLabel', { defaultValue: 'Proxy:' })} {bot.proxy_url}
                                </Typography>
                            </Tooltip>
                        )}
                    </Box>
                </CardContent>
            </Collapse>

            <ConfirmDialog
                open={deleteModalOpen}
                title={t('remoteControl.card.deleteTitle', { defaultValue: 'Delete Bot Configuration' })}
                description={t('remoteControl.card.deleteConfirm', { defaultValue: 'Are you sure you want to delete "{{name}}"? This action cannot be undone.', name: bot.name || bot.platform })}
                confirmLabel={t('remoteControl.card.delete', { defaultValue: 'Delete' })}
                confirmColor="error"
                onClose={() => setDeleteModalOpen(false)}
                onConfirm={handleConfirmDelete}
            />
        </Box>
    );
};

export default BotCard;
