import {
    Delete as DeleteIcon,
    Edit as EditIcon,
    MoreVert as MoreVertIcon,
    RestartAlt as RestartIcon,
    Warning as WarningIcon,
} from '@/components/icons';
import {
    Box,
    Button,
    Card,
    Chip,
    Collapse,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Modal,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import {styled} from '@mui/material/styles';
import type {BotSettings} from '@/types/bot';
import {isRemoteAgentMounted} from '@/types/bot';
import type {Provider} from '@/types/provider';
import PairingCodePanel from './PairingCodePanel';
import RemoteControlGraph from './RemoteControlGraph';
import {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';

// Same inactive treatment as the original bot card (BotCard.tsx): grayscale +
// dashed border + the 45° hatch overlay. Deliberately identical — "inactive"
// must read the same across the twin sections.
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

const GraphContainer = styled(Box)(({theme}) => ({
    padding: '10px 16px',
    borderRadius: theme.shape.borderRadius,
    margin: '8px 16px 0',
    overflowX: 'auto',
}));

interface RemoteAgentBotCardProps {
    bot: BotSettings;
    providers: Provider[];
    onMountToggle: (mounted: boolean) => void;
    onModelClick: () => void;
    onAgentSettingsSave: (settings: { chat_id: string; bash_allowlist: string[] }) => Promise<void>;
    /** Opens the shared BotConfigDialog in edit mode (bot resource fields). */
    onEdit: () => void;
    onRestart: () => void;
    onDelete: () => void;
    isToggling?: boolean;
    isRestarting?: boolean;
}

// RemoteAgentBotCard is the PURPOSE card: one row per bot on the Remote page.
// The mount switch decides whether this bot drives Claude Code / SmartGuide
// from chat; below it live the agent behavior (model, chat lock, allowlist).
//
// While the Bots nav section is hidden (bot has a single purpose today), this
// card also hosts the bot RESOURCE operations so the Remote page is fully
// self-sufficient: edit (shared BotConfigDialog), restart/delete (overflow
// menu), and the pairing code — the artifact the user needs next to actually
// start chatting. When a second purpose ships and Bots is surfaced again,
// these graduate back to the Bots section.
const RemoteAgentBotCard: React.FC<RemoteAgentBotCardProps> = ({
    bot,
    providers,
    onMountToggle,
    onModelClick,
    onAgentSettingsSave,
    onEdit,
    onRestart,
    onDelete,
    isToggling = false,
    isRestarting = false,
}) => {
    const {t} = useTranslation();
    const isMounted = isRemoteAgentMounted(bot.scenarios);
    const isEnabled = bot.enabled ?? true;
    const hasModel = Boolean(bot.smartguide_provider && bot.smartguide_model);

    const [chatIdDraft, setChatIdDraft] = useState(bot.chat_id || '');
    const [allowlistDraft, setAllowlistDraft] = useState((bot.bash_allowlist || []).join('\n'));
    const [saving, setSaving] = useState(false);
    const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null);
    const [deleteModalOpen, setDeleteModalOpen] = useState(false);

    // Re-sync drafts when the bot record refreshes (e.g. after reload).
    useEffect(() => {
        setChatIdDraft(bot.chat_id || '');
        setAllowlistDraft((bot.bash_allowlist || []).join('\n'));
    }, [bot.chat_id, bot.bash_allowlist]);

    const dirty = chatIdDraft.trim() !== (bot.chat_id || '') ||
        allowlistDraft.split(/[\n,]+/).map((s: string) => s.trim()).filter(Boolean).join(',') !==
        (bot.bash_allowlist || []).join(',');

    const handleSave = async () => {
        setSaving(true);
        try {
            await onAgentSettingsSave({
                chat_id: chatIdDraft.trim(),
                bash_allowlist: allowlistDraft.split(/[\n,]+/).map((s: string) => s.trim()).filter(Boolean),
            });
        } finally {
            setSaving(false);
        }
    };

    // Active = this purpose is actually live: mounted AND the bot may run.
    // Either switch being off puts the whole card behind the hatch overlay,
    // but the full configuration stays visible and editable.
    const isActive = isMounted && isEnabled;

    return (
        <StyledCard active={isActive}>
            {/* Header: bot identity (read-only here) + mount switch */}
            <Box sx={{
                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                px: 2, py: 1,
            }}>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 0}}>
                    <Typography sx={{fontFamily: 'monospace', fontSize: '0.875rem', fontWeight: 600}}>
                        {bot.name || bot.platform}
                    </Typography>
                    <Chip label={bot.platform} size="small"/>
                    {!isEnabled && (
                        <Tooltip title={t('remoteAgent.card.botDisabledHint', { defaultValue: 'The bot itself is disabled — mounting will re-enable it' })}>
                            <Chip label={t('remoteAgent.card.botDisabled', { defaultValue: 'bot off' })} size="small" color="warning" variant="outlined"/>
                        </Tooltip>
                    )}
                    {isMounted && !hasModel && (
                        <Tooltip title={t('remoteControl.card.noModelConfigured', { defaultValue: 'No model configured - click to select a model' })}>
                            <WarningIcon sx={{fontSize: '1.1rem', color: 'warning.main'}}/>
                        </Tooltip>
                    )}
                </Box>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 0.5}}>
                    <Tooltip title={isMounted
                        ? t('remoteControl.card.remoteAgentUnmount', { defaultValue: 'Unmount Remote Agent (bot stays configured but stops)' })
                        : t('remoteControl.card.remoteAgentMountOn', { defaultValue: 'Mount Remote Agent (also enables the bot)' })}>
                        <Switch checked={isMounted} onChange={() => onMountToggle(!isMounted)} size="small" color="primary" disabled={isToggling}/>
                    </Tooltip>
                    <Tooltip title={t('remoteControl.card.edit', { defaultValue: 'Edit' })}>
                        <IconButton size="small" color="primary" onClick={onEdit} disabled={isToggling || isRestarting}>
                            <EditIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                    <IconButton size="small" onClick={(e) => setMenuAnchor(e.currentTarget)} disabled={isToggling || isRestarting}>
                        <MoreVertIcon fontSize="small"/>
                    </IconButton>
                    <Menu
                        anchorEl={menuAnchor}
                        open={Boolean(menuAnchor)}
                        onClose={() => setMenuAnchor(null)}
                    >
                        <MenuItem
                            onClick={() => { setMenuAnchor(null); onRestart(); }}
                            disabled={!isEnabled || isRestarting}
                        >
                            <ListItemIcon><RestartIcon fontSize="small"/></ListItemIcon>
                            <ListItemText>{t('remoteControl.card.restartBot', { defaultValue: 'Restart Bot' })}</ListItemText>
                        </MenuItem>
                        <MenuItem onClick={() => { setMenuAnchor(null); setDeleteModalOpen(true); }}>
                            <ListItemIcon><DeleteIcon fontSize="small" color="error"/></ListItemIcon>
                            <ListItemText sx={{color: 'error.main'}}>{t('remoteControl.card.delete', { defaultValue: 'Delete' })}</ListItemText>
                        </MenuItem>
                    </Menu>
                </Box>
            </Box>

            {/* Agent behavior — always fully shown; the hatch overlay (not
                collapsing) communicates the unmounted/disabled state. */}
            <Collapse in timeout="auto" unmountOnExit>
                <GraphContainer>
                    <RemoteControlGraph
                        imbot={bot}
                        providers={providers}
                        isBotEnabled={isEnabled}
                        readOnly={isToggling}
                        onModelClick={onModelClick}
                    />
                </GraphContainer>
                <Stack spacing={1.5} sx={{px: 2, py: 1.5}}>
                    <TextField
                        label={t('remoteControl.dialog.chatIdLock', { defaultValue: 'Chat ID Lock' })}
                        placeholder="e.g. 123456789"
                        value={chatIdDraft}
                        onChange={(e) => setChatIdDraft(e.target.value)}
                        fullWidth
                        size="small"
                        helperText={t('remoteControl.dialog.chatIdLockHelper', { defaultValue: 'Optional: when set, only this chat ID can use the bot.' })}
                        disabled={saving}
                    />
                    <TextField
                        label={t('remoteControl.dialog.bashAllowlist', { defaultValue: 'Bash Allowlist' })}
                        placeholder={'cd\nls\npwd'}
                        value={allowlistDraft}
                        onChange={(e) => setAllowlistDraft(e.target.value)}
                        fullWidth
                        multiline
                        minRows={2}
                        size="small"
                        helperText={t('remoteControl.dialog.bashAllowlistHelper', { defaultValue: 'Allowlisted /bash subcommands. Default: cd, ls, pwd.' })}
                        disabled={saving}
                    />
                    {dirty && (
                        <Box sx={{display: 'flex', justifyContent: 'flex-end'}}>
                            <Button size="small" variant="contained" onClick={handleSave} disabled={saving}>
                                {saving
                                    ? t('remoteControl.dialog.saving', { defaultValue: 'Saving...' })
                                    : t('remoteAgent.card.saveAgentSettings', { defaultValue: 'Save agent settings' })}
                            </Button>
                        </Box>
                    )}
                    {/* Pairing code — the artifact needed for the next step
                        (send /bind in the chat), so it lives right here. */}
                    <PairingCodePanel bot={bot}/>
                </Stack>
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
                        <Button onClick={() => { setDeleteModalOpen(false); onDelete(); }} color="error" variant="contained">{t('remoteControl.card.delete', { defaultValue: 'Delete' })}</Button>
                    </Box>
                </Box>
            </Modal>
        </StyledCard>
    );
};

export default RemoteAgentBotCard;
