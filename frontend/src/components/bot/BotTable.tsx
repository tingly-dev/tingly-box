import {ContentCopy as CopyIcon, Delete as DeleteIcon, Edit as EditIcon, RestartAlt as RestartIcon} from '@/components/icons';
import ConfirmPopover from '@/components/ConfirmPopover';
import {capabilityEnabled} from '@/types/bot';
import type {BotSettings} from '@/types/bot';
import {notify} from '@/utils/notify';
import {
    Box,
    Chip,
    IconButton,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Tooltip,
    Typography,
} from '@mui/material';
import {useCallback, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Link as RouterLink} from 'react-router-dom';
import { fontMono } from '@/theme/fonts';
import { writeToClipboard } from '@/utils/clipboard';

// BotTable is the RESOURCE table for connected bots — the table counterpart
// of ApiKeyTable, replacing the former BotCard. Every bot, across every
// platform, is one row. Crucially it surfaces the bot's UUID as a copyable
// value, because the bot interaction API (POST /api/v1/bots/:bot/{notify,interact})
// takes that UUID in the path — without it visible, the API is unusable from
// the UI. See .design/bot-interaction-api.md.
//
// Capability chips open configuration for the selected bot. The shell (title/padding/border) is provided by
// the page's UnifiedCard — this component renders a borderless table so the
// card is the single container (no double border).
interface BotTableProps {
    bots: BotSettings[];
    onEdit: (uuid: string, platformId?: string) => void;
    onDelete: (uuid: string) => void;
    onBotToggle: (uuid: string, enabled: boolean) => void;
    onRestart: (uuid: string) => void;
    isToggling?: (uuid: string) => boolean;
    isRestarting?: (uuid: string) => boolean;
    onCapabilityClick?: (bot: BotSettings, capability: 'remote_control' | 'notify') => void;
}

// Status chip sizing — matches ApiKeyTable's Status cell so On/Off reads the
// same across tables. A disabled bot is conveyed by the chip alone (default /
// outlined), NOT by muting the whole row — ApiKeyTable doesn't hatch off rows,
// and doing so here made the On and Off switches render inconsistently.
const statusChipSx = {height: 22, minWidth: 40} as const;
// Fixed table layout with explicit per-column widths (mirrors ApiKeyTable) so
// column proportions stay stable regardless of content — auto layout let the
// Pairing column balloon and starved the others. Widths sum to tableMinWidth
// so the table fills the card at any width above it.
const headCellSx = {fontWeight: 600, py: 1.25, whiteSpace: 'nowrap', overflow: 'hidden'} as const;
const tableMinWidth = 820;
const col = (width: number, extra = {}) => ({...headCellSx, width, ...extra});

const BotTable: React.FC<BotTableProps> = ({
    bots,
    onEdit,
    onDelete,
    onBotToggle,
    onRestart,
    isToggling,
    isRestarting,
    onCapabilityClick,
}) => {
    const {t} = useTranslation();

    const [deleteModal, setDeleteModal] = useState<{anchorEl: HTMLElement | null; bot: BotSettings | null}>({
        anchorEl: null,
        bot: null,
    });

    const handleDeleteClick = useCallback((bot: BotSettings, anchorEl: HTMLElement) => {
        setDeleteModal({anchorEl, bot});
    }, []);

    const handleConfirmDelete = useCallback(() => {
        const uuid = deleteModal.bot?.uuid;
        setDeleteModal({anchorEl: null, bot: null});
        if (uuid) onDelete(uuid);
    }, [deleteModal.bot, onDelete]);

    const handleCopyUuid = useCallback(async (uuid: string) => {
        if (!uuid) return;
        try {
            await writeToClipboard(uuid);
            notify.success(t('bots.table.uuidCopied', {defaultValue: 'Bot UUID copied'}));
        } catch {
            notify.error(t('bots.table.uuidCopyFailed', {defaultValue: 'Copy failed — check clipboard permissions'}));
        }
    }, [t]);

    return (
        <>
            <TableContainer
                // No Paper, no border — the page's UnifiedCard is the shell.
                sx={{display: {xs: 'none', md: 'block'}, overflowX: 'auto'}}
            >
                <Table sx={{tableLayout: 'fixed', minWidth: tableMinWidth}}>
                    <TableHead>
                        <TableRow sx={{bgcolor: 'action.hover'}}>
                            <TableCell sx={col(120)}>{t('bots.table.status', {defaultValue: 'Status'})}</TableCell>
                            <TableCell sx={col(150)}>{t('bots.table.name', {defaultValue: 'Name'})}</TableCell>
                            <TableCell sx={col(180)}>{t('bots.table.botId', {defaultValue: 'Bot UUID'})}</TableCell>
                            <TableCell sx={col(100)}>{t('bots.table.platform', {defaultValue: 'Platform'})}</TableCell>
                            <TableCell sx={col(170)}>{t('bots.table.capabilities', {defaultValue: 'Capabilities'})}</TableCell>
                            <TableCell sx={col(200, {textAlign: 'right'})}>{t('bots.table.actions', {defaultValue: 'Actions'})}</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {bots.map((bot) => {
                            const isActive = bot.enabled ?? true;
                            const isMounted = isActive && capabilityEnabled(bot,'remote_control');
                            const isNotified = isActive && capabilityEnabled(bot,'notify');
                            const toggling = isToggling?.(bot.uuid!) ?? false;
                            const restarting = isRestarting?.(bot.uuid!) ?? false;

                            return (
                                <TableRow
                                    key={bot.uuid}
                                    hover
                                    sx={{
                                        '& > .MuiTableCell-root': {py: 1.25},
                                    }}
                                >
                                    {/* Status */}
                                    <TableCell>
                                        <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                                            <Tooltip title={isActive
                                                ? t('remoteControl.card.disableBot', {defaultValue: 'Disable Bot'})
                                                : t('remoteControl.card.enableBot', {defaultValue: 'Enable Bot'})}>
                                                <Switch
                                                    checked={isActive}
                                                    onChange={() => onBotToggle(bot.uuid!, !isActive)}
                                                    size="small"
                                                    color="success"
                                                    disabled={toggling}
                                                />
                                            </Tooltip>
                                            <Chip
                                                label={isActive
                                                    ? t('common.on', {defaultValue: 'On'})
                                                    : t('common.off', {defaultValue: 'Off'})}
                                                size="small"
                                                color={isActive ? 'success' : 'default'}
                                                variant={isActive ? 'filled' : 'outlined'}
                                                sx={statusChipSx}
                                            />
                                        </Stack>
                                    </TableCell>
                                    {/* Name */}
                                    <TableCell>
                                        <Tooltip title={bot.name || bot.platform}>
                                            <Typography variant="body2" noWrap sx={{fontWeight: 600, maxWidth: 160}}>
                                                {bot.name || bot.platform}
                                            </Typography>
                                        </Tooltip>
                                    </TableCell>
                                    {/* Bot UUID — the identifier notify/interact takes in the path
                                        (:bot), copyable. Icon + text on one row (mirrors ApiKeyTable's
                                        API Key cell); UUID truncates with the full value in the tooltip.
                                        Labeled "Bot UUID" (not "Bot ID") to split the name collision with
                                        the Chat ID surfaced by BotChatsButton — ux-principles #3. */}
                                    <TableCell>
                                        <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                                            <Tooltip title={t('bots.table.copyUuid', {defaultValue: 'Copy Bot UUID'})}>
                                                <span>
                                                    <IconButton
                                                        size="small"
                                                        onClick={() => handleCopyUuid(bot.uuid!)}
                                                        disabled={!bot.uuid}
                                                        sx={{p: 0.25, flexShrink: 0}}
                                                    >
                                                        <CopyIcon fontSize="inherit"/>
                                                    </IconButton>
                                                </span>
                                            </Tooltip>
                                            <Tooltip title={bot.uuid}>
                                                <Typography
                                                    variant="caption"
                                                    component="span"
                                                    sx={{
                                                        fontFamily: fontMono,
                                                        color: 'text.secondary',
                                                        overflow: 'hidden',
                                                        textOverflow: 'ellipsis',
                                                        whiteSpace: 'nowrap',
                                                        minWidth: 0,
                                                    }}
                                                >
                                                    {bot.uuid}
                                                </Typography>
                                            </Tooltip>
                                        </Stack>
                                    </TableCell>
                                    {/* Platform */}
                                    <TableCell>
                                        <Chip label={bot.platform} size="small"/>
                                    </TableCell>
                                    {/* Capability chips open the selected bot’s settings. */}
                                    <TableCell>
                                        <Box sx={{display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap'}}>
                                            <Chip {...(onCapabilityClick ? {onClick: () => onCapabilityClick(bot, 'remote_control')} : {component: RouterLink, to: '/remote-agent', clickable: true})} label={t('bots.card.remoteAgentChip', {defaultValue: 'Remote'})} size="small" variant={isMounted ? 'filled' : 'outlined'} color={isMounted ? 'primary' : 'default'}/>
                                            <Chip {...(onCapabilityClick ? {onClick: () => onCapabilityClick(bot, 'notify')} : {component: RouterLink, to: '/notify', clickable: true})} label={t('bots.card.notifyChip', {defaultValue: 'Notify'})} size="small" variant={isNotified ? 'filled' : 'outlined'} color={isNotified ? 'primary' : 'default'}/>
                                        </Box>
                                    </TableCell>
                                    {/* Actions */}
                                    <TableCell align="right">
                                        <Stack direction="row" spacing={0.5} sx={{alignItems: 'center', justifyContent: 'flex-end'}}>
                                            <Tooltip title={isActive
                                                ? t('remoteControl.card.restartBot', {defaultValue: 'Restart Bot'})
                                                : t('remoteControl.card.enableToRestart', {defaultValue: 'Enable bot to restart'})}>
                                                <span>
                                                    <IconButton
                                                        size="small"
                                                        color="primary"
                                                        aria-label={t('remoteControl.card.restartBot', {defaultValue: 'Restart Bot'})}
                                                        onClick={() => onRestart(bot.uuid!)}
                                                        disabled={!isActive || toggling || restarting}
                                                    >
                                                        <RestartIcon fontSize="small"/>
                                                    </IconButton>
                                                </span>
                                            </Tooltip>
                                            <Tooltip title={t('remoteControl.card.edit', {defaultValue: 'Edit'})}>
                                                <IconButton
                                                    size="small"
                                                    color="primary"
                                                    onClick={() => onEdit(bot.uuid!, bot.platform!)}
                                                    disabled={toggling || restarting}
                                                >
                                                    <EditIcon fontSize="small"/>
                                                </IconButton>
                                            </Tooltip>
                                            <Tooltip title={t('remoteControl.card.delete', {defaultValue: 'Delete'})}>
                                                <IconButton
                                                    size="small"
                                                    color="error"
                                                    onClick={(e) => handleDeleteClick(bot, e.currentTarget)}
                                                    disabled={toggling || restarting}
                                                >
                                                    <DeleteIcon fontSize="small"/>
                                                </IconButton>
                                            </Tooltip>
                                        </Stack>
                                    </TableCell>
                                </TableRow>
                            );
                        })}
                    </TableBody>
                </Table>
            </TableContainer>

            {/* On narrow screens the table's six columns hide the actions the
                user came for. Cards preserve the same facts but organize each
                Bot around the questions: is it running, what can it do, and
                what can I do next? */}
            <Stack spacing={1.25} sx={{display: {xs: 'flex', md: 'none'}}}>
                {bots.map((bot) => {
                    const isActive = bot.enabled ?? true;
                    const isMounted = isActive && capabilityEnabled(bot, 'remote_control');
                    const isNotified = isActive && capabilityEnabled(bot, 'notify');
                    const toggling = isToggling?.(bot.uuid!) ?? false;
                    const restarting = isRestarting?.(bot.uuid!) ?? false;
                    return (
                        <Box key={bot.uuid} sx={{p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1.5}}>
                            <Stack direction="row" sx={{alignItems: 'flex-start', justifyContent: 'space-between', gap: 1}}>
                                <Box sx={{minWidth: 0}}>
                                    <Typography variant="body2" noWrap sx={{fontWeight: 600}}>{bot.name || bot.platform}</Typography>
                                    <Chip label={bot.platform} size="small" sx={{mt: 0.75}}/>
                                </Box>
                                <Stack direction="row" spacing={0.75} sx={{alignItems: 'center'}}>
                                    <Switch checked={isActive} onChange={() => onBotToggle(bot.uuid!, !isActive)} size="small" color="success" disabled={toggling}/>
                                    <Chip label={isActive ? t('common.on', {defaultValue: 'On'}) : t('common.off', {defaultValue: 'Off'})} size="small" color={isActive ? 'success' : 'default'} variant={isActive ? 'filled' : 'outlined'} sx={statusChipSx}/>
                                </Stack>
                            </Stack>
                            <Stack direction="row" spacing={0.5} sx={{alignItems: 'center', mt: 1.25, minWidth: 0}}>
                                <Typography variant="caption" noWrap sx={{fontFamily: fontMono, color: 'text.secondary', flex: 1}}>{bot.uuid}</Typography>
                                <IconButton size="small" onClick={() => handleCopyUuid(bot.uuid!)} disabled={!bot.uuid}><CopyIcon fontSize="inherit"/></IconButton>
                            </Stack>
                            <Stack direction="row" spacing={0.75} sx={{mt: 1, flexWrap: 'wrap'}}>
                                <Chip {...(onCapabilityClick ? {onClick: () => onCapabilityClick(bot, 'remote_control')} : {component: RouterLink, to: '/remote-agent', clickable: true})} label={t('bots.card.remoteAgentChip', {defaultValue: 'Remote'})} size="small" variant={isMounted ? 'filled' : 'outlined'} color={isMounted ? 'primary' : 'default'}/>
                                <Chip {...(onCapabilityClick ? {onClick: () => onCapabilityClick(bot, 'notify')} : {component: RouterLink, to: '/notify', clickable: true})} label={t('bots.card.notifyChip', {defaultValue: 'Notify'})} size="small" variant={isNotified ? 'filled' : 'outlined'} color={isNotified ? 'primary' : 'default'}/>
                            </Stack>
                            <Stack direction="row" spacing={0.5} sx={{alignItems: 'center', justifyContent: 'flex-end', mt: 1.25, pt: 1, borderTop: 1, borderColor: 'divider'}}>
                                <IconButton size="small" color="primary" aria-label={t('remoteControl.card.restartBot', {defaultValue: 'Restart Bot'})} onClick={() => onRestart(bot.uuid!)} disabled={!isActive || toggling || restarting}><RestartIcon fontSize="small"/></IconButton>
                                <IconButton size="small" color="primary" onClick={() => onEdit(bot.uuid!, bot.platform!)} disabled={toggling || restarting}><EditIcon fontSize="small"/></IconButton>
                                <IconButton size="small" color="error" onClick={(e) => handleDeleteClick(bot, e.currentTarget)} disabled={toggling || restarting}><DeleteIcon fontSize="small"/></IconButton>
                            </Stack>
                        </Box>
                    );
                })}
            </Stack>

            <ConfirmPopover
                anchorEl={deleteModal.anchorEl}
                title={t('remoteControl.card.deleteTitle', {defaultValue: 'Delete Bot Configuration'})}
                description={t('remoteControl.card.deleteConfirm', {
                    defaultValue: 'Are you sure you want to delete "{{name}}"? This action cannot be undone.',
                    name: deleteModal.bot?.name || deleteModal.bot?.platform,
                })}
                confirmLabel={t('remoteControl.card.delete', {defaultValue: 'Delete'})}
                confirmColor="error"
                onClose={() => setDeleteModal({anchorEl: null, bot: null})}
                onConfirm={handleConfirmDelete}
            />
        </>
    );
};

export default BotTable;
