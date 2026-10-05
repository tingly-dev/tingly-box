import {
    Delete as DeleteIcon,
    Edit as EditIcon,
    MoreVert as MoreVertIcon,
    RestartAlt as RestartIcon,
} from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    Collapse,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import ConfirmDialog from '@/components/ConfirmDialog';
import type {BotSettings} from '@/types/bot';
import {capabilityEnabled, ccProfileIdFromDefaultAgent, isPairingRequired} from '@/types/bot';
import {PLATFORM_BRAND_ICONS} from '@/constants/platformGuides';
import type {Provider} from '@/types/provider';
import type {ProfileInfo} from '@/contexts/ProfileContext';
import RemoteControlGraph from './RemoteControlGraph';
import BotAccessDialog from './BotAccessDialog';
import PairingCodePanel from './PairingCodePanel';
import {useRemoteAccess} from './useRemoteAccess';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import { fontMono } from '@/theme/fonts';

interface RemoteAgentBotCardProps {
    bot: BotSettings;
    providers: Provider[];
    onMountToggle: (mounted: boolean) => void;
    onModelClick: () => void;
    /** Configured Claude Code profiles (resolves the @cc profile node label). */
    ccProfiles?: ProfileInfo[];
    /** Opens the Claude Code profile picker for this bot. */
    onCCProfileClick?: () => void;
    /** Opens the shared BotConfigDialog in edit mode (bot resource fields). */
    onEdit: () => void;
    onRestart: () => void;
    onDelete: () => void;
    isToggling?: boolean;
    isRestarting?: boolean;
}

// RemoteAgentBotCard is the PURPOSE card: one row per bot on the Remote page.
// The switch decides whether this bot drives Claude Code / SmartGuide from
// chat. Below it, the bot's route (RemoteControlGraph): who can send commands
// in → this bot → @tb / @cc forks; each node opens its editor. Access and
// pairing live in the Access work surface opened from the entry node, which
// reads this card's own access data, so authorization has one source of
// truth. Edit / restart / delete stay here so the page is self-sufficient.
const RemoteAgentBotCard: React.FC<RemoteAgentBotCardProps> = ({
    bot,
    providers,
    onMountToggle,
    onModelClick,
    ccProfiles,
    onCCProfileClick,
    onEdit,
    onRestart,
    onDelete,
    isToggling = false,
    isRestarting = false,
}) => {
    const {t} = useTranslation();
    const isMounted = capabilityEnabled(bot,'remote_control');
    const isEnabled = bot.enabled ?? true;
    const ccProfileId = ccProfileIdFromDefaultAgent(bot.default_agent);

    const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null);
    const [deleteModalOpen, setDeleteModalOpen] = useState(false);
    const [accessDialogOpen, setAccessDialogOpen] = useState(false);

    // Off when this purpose isn't live: unmounted, or the bot itself is
    // disabled. An off card is quiet rather than struck through: no paper
    // background, grey identity, and the route graph folds into one summary
    // line. Its settings stay one click away (done ≠ locked), and they carry
    // no warning colors — an off bot isn't broken, it just isn't running.
    const isActive = isMounted && isEnabled;
    const [showSettings, setShowSettings] = useState(false);
    const expanded = isActive || showSettings;
    // Nothing on a folded off card shows access, so it doesn't fetch it.
    const access = useRemoteAccess(bot.uuid, expanded);
    const controllerCount = access.controllers.length + access.controllingGroups.length;
    // Nothing is judged until the first fetch has come back — before that,
    // empty lists mean "not known yet", not "nobody".
    const checked = isActive && access.loaded && !access.loading;
    const failed = checked && Boolean(access.error);
    // Live, checked, and nobody can drive it: the state the notice is for.
    const nobody = checked && !failed && controllerCount === 0;
    const noModel = checked && !failed && controllerCount > 0 && !(bot.smartguide_provider && bot.smartguide_model);
    const ccProfileName = ccProfiles?.find((p) => p.id === ccProfileId)?.name;
    const BrandIcon = PLATFORM_BRAND_ICONS[bot.platform || ''];

    // One sentence answering "is this usable right now?" — replaces the
    // separate On/Off chip, which only repeated the switch next to it.
    const status = !isActive
        ? t('remoteAgent.card.statusOff', {defaultValue: 'Remote Control off'})
        : !checked
            ? t('remoteAgent.card.statusChecking', {defaultValue: 'Checking access…'})
            : failed
                ? t('remoteAgent.card.statusError', {defaultValue: 'Couldn\'t check who can control'})
                : nobody
                    ? t('remoteAgent.card.statusNobody', {defaultValue: 'Nobody can control yet'})
                    : noModel
                        ? t('remoteAgent.card.statusNoModel', {defaultValue: '@tb has no model yet'})
                        : t('remoteAgent.card.statusControllers', {defaultValue: '{{count}} can control', count: controllerCount});
    const statusColor = !checked ? 'text.secondary' : (failed || nobody || noModel) ? 'warning.main' : 'success.main';

    // Readiness: while a live bot is unusable, the concrete next step sits in
    // the same row (see ux-principles #11).
    const pairing = isPairingRequired(bot);
    const noticeText = access.pendingChats.length > 0
        ? t('remoteAgent.card.pendingChats', {
            defaultValue: '{{count}} direct chats reached this bot but none can control it. Grant Remote Control to the right one.',
            count: access.pendingChats.length,
        })
        : pairing
            ? t('remoteAgent.card.pairHint', {defaultValue: 'Nobody can control this bot yet. Send this to the bot in a direct message:'})
            : t('remoteAgent.card.messageHint', {defaultValue: 'Nobody can control this bot yet. Message the bot directly, then grant Remote Control to that chat.'});

    return (
        <Box sx={{
            bgcolor: isActive ? 'background.paper' : 'transparent',
            border: '1px solid',
            borderColor: 'divider',
            borderRadius: 2,
            transition: 'background-color 0.18s ease-out',
        }}>
            {/* Header: platform + bot name, one status line, one switch */}
            <Box sx={{
                display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1,
                px: 2, py: 1.5,
            }}>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 0}}>
                    {BrandIcon && <BrandIcon size={24} grayscale={!isActive}/>}
                    <Box sx={{minWidth: 0}}>
                        <Typography noWrap variant="subtitle2" sx={{fontWeight: 600, color: isActive ? 'text.primary' : 'text.secondary'}}>
                            {bot.name || bot.platform}
                        </Typography>
                        <Typography noWrap variant="caption" sx={{display: 'block', color: statusColor}}>
                            {status}
                        </Typography>
                    </Box>
                </Box>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0}}>
                    <Tooltip title={isActive
                        ? t('remoteControl.card.remoteAgentOff', { defaultValue: 'Turn off Remote Control. The bot remains available to other capabilities.' })
                        : t('remoteControl.card.remoteAgentOn', { defaultValue: 'Turn on Remote Control. The bot starts automatically if needed.' })}>
                        <Switch checked={isActive} onChange={() => onMountToggle(!isActive)} size="small" color="primary" disabled={isToggling}/>
                    </Tooltip>
                    <Tooltip title={t('remoteControl.card.edit', { defaultValue: 'Edit' })}>
                        <IconButton size="small" onClick={onEdit} disabled={isToggling || isRestarting}>
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

            {nobody && (
                <Box sx={{px: 2, pb: 1}}>
                    <Alert
                        severity="warning"
                        action={<Button color="inherit" size="small" onClick={() => setAccessDialogOpen(true)}>
                            {t('remoteAgent.card.manageAccess', {defaultValue: 'Manage access'})}
                        </Button>}
                    >
                        {noticeText}
                        {pairing && access.pendingChats.length === 0 && <PairingCodePanel bot={bot} revealByDefault/>}
                    </Alert>
                </Box>
            )}

            {!isActive && (
                // Off: one line saying what the bot WOULD do when turned on.
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1, px: 2, pb: expanded ? 0 : 1.25, mt: -0.5}}>
                    {!expanded && (
                        <Typography variant="caption" noWrap sx={{minWidth: 0, color: 'text.secondary'}}>
                            <Box component="span" sx={{fontFamily: fontMono}}>@tb</Box>{' '}
                            {bot.smartguide_model || '—'}
                            {'  ·  '}
                            <Box component="span" sx={{fontFamily: fontMono}}>@cc</Box>{' '}
                            {ccProfileId
                                ? (ccProfileName || ccProfileId)
                                : t('remoteAgent.ccProfile.default', {defaultValue: 'Default'})}
                        </Typography>
                    )}
                    <Button size="small" color="inherit" sx={{ml: 'auto', color: 'text.secondary', flexShrink: 0}} onClick={() => setShowSettings((v) => !v)}>
                        {showSettings
                            ? t('remoteAgent.card.hideSettings', {defaultValue: 'Hide settings'})
                            : t('remoteAgent.card.showSettings', {defaultValue: 'Settings'})}
                    </Button>
                </Box>
            )}

            <Collapse in={expanded} timeout="auto" unmountOnExit>
                <Box sx={{px: 2, pb: 0.5}}>
                    <RemoteControlGraph
                        imbot={bot}
                        providers={providers}
                        isBotEnabled={isActive}
                        readOnly={isToggling}
                        onModelClick={onModelClick}
                        onBotClick={isRestarting ? undefined : onEdit}
                        ccProfiles={ccProfiles}
                        onCCProfileClick={onCCProfileClick}
                        directChatCount={access.controllers.length}
                        groupCount={access.controllingGroups.length}
                        accessLoading={access.loading}
                        accessError={access.error}
                        onAccessClick={() => setAccessDialogOpen(true)}
                    />
                </Box>
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
            <BotAccessDialog
                scope="remote_control"
                remoteAccess={access}
                open={accessDialogOpen}
                bot={bot}
                onClose={() => setAccessDialogOpen(false)}
            />
        </Box>
    );
};

export default RemoteAgentBotCard;
