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
    Chip,
    Collapse,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import {styled} from '@mui/material/styles';
import ConfirmDialog from '@/components/ConfirmDialog';
import type {BotSettings} from '@/types/bot';
import {isRemoteAgentMounted} from '@/types/bot';
import type {Provider} from '@/types/provider';
import type {ProfileInfo} from '@/contexts/ProfileContext';
import {BOT_CARD_SX, statusChipSx} from './botCardStyles';
import PairingCodePanel from './PairingCodePanel';
import RemoteControlGraph from './RemoteControlGraph';
import {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';

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
    /** Configured Claude Code profiles (resolves the @cc profile node label). */
    ccProfiles?: ProfileInfo[];
    /** Opens the Claude Code profile picker for this bot. */
    onCCProfileClick?: () => void;
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
    ccProfiles,
    onCCProfileClick,
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

    return (
        <Box sx={BOT_CARD_SX}>
            {/* Header: bot identity (read-only here) + mount switch */}
            <Box sx={{
                display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 1,
                px: 2, py: 1.5,
            }}>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 0}}>
                    <Typography noWrap variant="body2" sx={{fontWeight: 600, minWidth: 0}}>
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
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.5}}>
                    <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                        <Tooltip title={isMounted
                            ? t('remoteControl.card.remoteAgentUnmount', { defaultValue: 'Unmount Remote Control (bot stays configured but stops)' })
                            : t('remoteControl.card.remoteAgentMountOn', { defaultValue: 'Mount Remote Control (also enables the bot)' })}>
                            <Switch checked={isMounted} onChange={() => onMountToggle(!isMounted)} size="small" color="primary" disabled={isToggling}/>
                        </Tooltip>
                        <Chip
                            label={isMounted ? t('common.on', { defaultValue: 'On' }) : t('common.off', { defaultValue: 'Off' })}
                            size="small"
                            color={isMounted ? 'success' : 'default'}
                            variant={isMounted ? 'filled' : 'outlined'}
                            sx={statusChipSx}
                        />
                    </Stack>
                    <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
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
                    </Stack>
                </Box>
            </Box>

            {/* Agent behavior — always fully shown. */}
            <Collapse in timeout="auto" unmountOnExit>
                <GraphContainer>
                    <RemoteControlGraph
                        imbot={bot}
                        providers={providers}
                        isBotEnabled={isEnabled}
                        readOnly={isToggling}
                        onModelClick={onModelClick}
                        ccProfiles={ccProfiles}
                        onCCProfileClick={onCCProfileClick}
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

            <ConfirmDialog
                open={deleteModalOpen}
                title={t('remoteControl.card.deleteTitle', { defaultValue: 'Delete Bot Configuration' })}
                description={t('remoteControl.card.deleteConfirm', { defaultValue: 'Are you sure you want to delete "{{name}}"? This action cannot be undone.', name: bot.name || bot.platform })}
                confirmLabel={t('remoteControl.card.delete', { defaultValue: 'Delete' })}
                confirmColor="error"
                onClose={() => setDeleteModalOpen(false)}
                onConfirm={() => { setDeleteModalOpen(false); onDelete(); }}
            />
        </Box>
    );
};

export default RemoteAgentBotCard;
