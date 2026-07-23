import {Delete as DeleteIcon, Edit as EditIcon, RestartAlt as RestartIcon} from '@/components/icons';
import {
    Box,
    Button,
    Card,
    CardContent,
    Chip,
    Collapse,
    IconButton,
    Modal,
    Switch,
    Tooltip,
    Typography
} from '@mui/material';
import {styled} from '@mui/material/styles';
import type {BotSettings} from '@/types/bot';
import {isRemoteAgentMounted} from '@/types/bot';
import PairingCodePanel from './PairingCodePanel';
import {useCallback, useState} from 'react';
import {useNavigate} from 'react-router-dom';
import {useTranslation} from 'react-i18next';

const RULE_GRAPH_STYLES = {
    header: { paddingX: 16, paddingY: 6 },
} as const;

const {header} = RULE_GRAPH_STYLES;

const StyledCard = styled(Card, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({active, theme}) => ({
    transition: 'all 0.2s ease-in-out',
    opacity: active ? 1 : 0.6,
    filter: active ? 'none' : 'grayscale(0.3)',
    border: active ? 'none' : '2px dashed',
    borderColor: active ? 'transparent' : theme.palette.text.disabled,
    margin: '3px',
    position: 'relative',
    ...(active ? {} : {
        '&::before': {
            content: '""',
            position: 'absolute',
            top: 0, left: 0, right: 0, bottom: 0,
            backgroundImage: 'repeating-linear-gradient(45deg, transparent, transparent 10px, rgba(0,0,0,0.03) 10px, rgba(0,0,0,0.03) 20px)',
            pointerEvents: 'none',
            borderRadius: theme.shape.borderRadius,
        },
    }),
    '&:hover': {
        boxShadow: active ? theme.shadows[4] : theme.shadows[1],
    },
}));

const SummarySection = styled(Box)({
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: `${header.paddingY}px ${header.paddingX}px`,
});

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
    const [deleteModalOpen, setDeleteModalOpen] = useState(false);

    const handleDeleteClick = useCallback(() => setDeleteModalOpen(true), []);
    const handleConfirmDelete = useCallback(() => {
        setDeleteModalOpen(false);
        onDelete();
    }, [onDelete]);

    return (
        <StyledCard active={isActive}>
            {/* Header */}
            <SummarySection>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1, minWidth: 0}}>
                    <Tooltip title={bot.name || bot.platform}>
                        <Typography sx={{
                            fontFamily: 'monospace', fontSize: '0.875rem', fontWeight: 600,
                            color: isActive ? 'text.primary' : 'text.disabled',
                            opacity: isActive ? 1 : 0.5, cursor: 'default',
                        }}>
                            {bot.name || bot.platform}
                        </Typography>
                    </Tooltip>
                    {bot.name && <Chip label={bot.platform} size="small" sx={{opacity: isActive ? 1 : 0.5}}/>}
                    {/* Purpose status: where this bot is used. Click-through to configure. */}
                    <Tooltip title={t('bots.card.remoteAgentChipHint', { defaultValue: 'Configure on the Remote Agent page' })}>
                        <Chip
                            label={t('bots.card.remoteAgentChip', { defaultValue: 'Remote Agent' })}
                            size="small"
                            variant={isMounted ? 'filled' : 'outlined'}
                            color={isMounted ? 'primary' : 'default'}
                            onClick={() => navigate(`/remote-agent/${bot.platform}`)}
                            sx={{opacity: isActive ? 1 : 0.5}}
                        />
                    </Tooltip>
                </Box>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 0.5}}>
                    <Tooltip title={isActive ? t('remoteControl.card.disableBot', { defaultValue: 'Disable Bot' }) : t('remoteControl.card.enableBot', { defaultValue: 'Enable Bot' })}>
                        <Switch checked={isActive} onChange={() => onBotToggle()} size="small" color="success" disabled={isToggling}/>
                    </Tooltip>
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
                </Box>
            </SummarySection>

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

            {/* Delete Confirmation */}
            <Modal open={deleteModalOpen} onClose={() => setDeleteModalOpen(false)}>
                <Box sx={{
                    position: 'absolute', top: '50%', left: '50%',
                    transform: 'translate(-50%, -50%)',
                    width: 400, maxWidth: '80vw',
                    bgcolor: 'background.paper', boxShadow: 24, p: 4, borderRadius: 2,
                }}>
                    <Typography variant="h6" sx={{mb: 2}}>{t('remoteControl.card.deleteTitle', { defaultValue: 'Delete Bot Configuration' })}</Typography>
                    <Typography variant="body2" sx={{mb: 3}}>
                        {t('remoteControl.card.deleteConfirm', { defaultValue: 'Are you sure you want to delete "{{name}}"? This action cannot be undone.', name: bot.name || bot.platform })}
                    </Typography>
                    <Box sx={{display: 'flex', gap: 2, justifyContent: 'flex-end'}}>
                        <Button onClick={() => setDeleteModalOpen(false)} color="inherit">{t('remoteControl.dialog.cancel', { defaultValue: 'Cancel' })}</Button>
                        <Button onClick={handleConfirmDelete} color="error" variant="contained">{t('remoteControl.card.delete', { defaultValue: 'Delete' })}</Button>
                    </Box>
                </Box>
            </Modal>
        </StyledCard>
    );
};

export default BotCard;
