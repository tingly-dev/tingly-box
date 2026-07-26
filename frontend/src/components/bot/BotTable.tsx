import {ContentCopy as CopyIcon, Delete as DeleteIcon, Edit as EditIcon, RestartAlt as RestartIcon} from '@/components/icons';
import ConfirmDialog from '@/components/ConfirmDialog';
import PairingCodePanel from './PairingCodePanel';
import BotChatsButton from './BotChatsButton';
import {isRemoteAgentMounted, isNotifyMounted, isPairingRequired} from '@/types/bot';
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
import {useNavigate} from 'react-router-dom';
import {useTranslation} from 'react-i18next';

// BotTable is the RESOURCE table for connected bots — the table counterpart
// of ApiKeyTable, replacing the former BotCard. Every bot, across every
// platform, is one row. Crucially it surfaces the bot's UUID as a copyable
// value, because the bot interaction API (POST /api/v1/bots/:bot/{notify,interact})
// takes that UUID in the path — without it visible, the API is unusable from
// the UI. See .design/bot-interaction-api.md.
//
// What each bot is used FOR (Remote Agent, notifications) stays on the
// purpose pages; this table only shows mount status as a click-through chip,
// mirroring the old card. The shell (title/padding/border) is provided by
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
const tableMinWidth = 600;
const col = (width: number, extra = {}) => ({...headCellSx, width, ...extra});

const BotTable: React.FC<BotTableProps> = ({
    bots,
    onEdit,
    onDelete,
    onBotToggle,
    onRestart,
    isToggling,
    isRestarting,
}) => {
    const {t} = useTranslation();
    const navigate = useNavigate();

    const [deleteModal, setDeleteModal] = useState<{open: boolean; bot: BotSettings | null}>({
        open: false,
        bot: null,
    });

    const handleDeleteClick = useCallback((bot: BotSettings) => {
        setDeleteModal({open: true, bot});
    }, []);

    const handleConfirmDelete = useCallback(() => {
        const uuid = deleteModal.bot?.uuid;
        setDeleteModal({open: false, bot: null});
        if (uuid) onDelete(uuid);
    }, [deleteModal.bot, onDelete]);

    const handleCopyUuid = useCallback(async (uuid: string) => {
        if (!uuid) return;
        try {
            await navigator.clipboard.writeText(uuid);
            notify.success(t('bots.table.uuidCopied', {defaultValue: 'Bot UUID copied'}));
        } catch {
            notify.error(t('bots.table.uuidCopyFailed', {defaultValue: 'Copy failed — check clipboard permissions'}));
        }
    }, [t]);

    return (
        <>
            <TableContainer
                // No Paper, no border — the page's UnifiedCard is the shell.
                sx={{overflowX: 'auto'}}
            >
                <Table sx={{tableLayout: 'fixed', minWidth: tableMinWidth}}>
                    <TableHead>
                        <TableRow sx={{bgcolor: 'action.hover'}}>
                            <TableCell sx={col(80)}>{t('bots.table.status', {defaultValue: 'Status'})}</TableCell>
                            <TableCell sx={col(100)}>{t('bots.table.name', {defaultValue: 'Name'})}</TableCell>
                            <TableCell sx={col(100)}>{t('bots.table.botId', {defaultValue: 'Bot UUID'})}</TableCell>
                            <TableCell sx={col(80)}>{t('bots.table.platform', {defaultValue: 'Platform'})}</TableCell>
                            <TableCell sx={col(120)}>{t('bots.table.purpose', {defaultValue: 'Purpose'})}</TableCell>
                            <TableCell sx={col(200, {textAlign: 'left'})}>{t('bots.table.pairing', {defaultValue: 'Pairing'})}</TableCell>
                            <TableCell sx={col(100, {textAlign: 'left'})}>{t('bots.table.actions', {defaultValue: 'Actions'})}</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {bots.map((bot) => {
                            const isActive = bot.enabled ?? true;
                            const isMounted = isRemoteAgentMounted(bot.scenarios);
                            const isNotified = isNotifyMounted(bot.scenarios);
                            const toggling = isToggling?.(bot.uuid!) ?? false;
                            const restarting = isRestarting?.(bot.uuid!) ?? false;
                            const pairingNeeded = isPairingRequired(bot);

                            return (
                                <TableRow
                                    key={bot.uuid}
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
                                                        fontFamily: 'monospace',
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
                                    {/* Purpose — mount status, click-through to configure. */}
                                    <TableCell>
                                        <Box sx={{display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap'}}>
                                            <Tooltip title={t('bots.card.remoteAgentChipHint', {defaultValue: 'Configure on the Remote Control page'})}>
                                                <Chip
                                                    label={t('bots.card.remoteAgentChip', {defaultValue: 'Remote Control'})}
                                                    size="small"
                                                    variant={isMounted ? 'filled' : 'outlined'}
                                                    color={isMounted ? 'primary' : 'default'}
                                                    onClick={() => navigate(`/remote-agent/${bot.platform}`)}
                                                />
                                            </Tooltip>
                                            <Tooltip title={t('bots.card.notifyChipHint', {defaultValue: 'Configure on the IM Notify page'})}>
                                                <Chip
                                                    label={t('bots.card.notifyChip', {defaultValue: 'IM Notify'})}
                                                    size="small"
                                                    variant={isNotified ? 'filled' : 'outlined'}
                                                    color={isNotified ? 'primary' : 'default'}
                                                    onClick={() => navigate('/notify')}
                                                />
                                            </Tooltip>
                                        </Box>
                                    </TableCell>
                                    {/* Pairing — the full PairingCodePanel inline (reveal/copy/rotate
                                        + countdown), rendered directly so there's no popover layer.
                                        Returns null when TOFU isn't required for the platform. */}
                                    <TableCell align="left">
                                        {pairingNeeded ? (
                                            <PairingCodePanel bot={bot}/>
                                        ) : (
                                            <Typography variant="body2" sx={{color: 'text.disabled'}}>—</Typography>
                                        )}
                                    </TableCell>
                                    {/* Actions */}
                                    <TableCell align="right">
                                        <Stack direction="row" spacing={0.5} sx={{alignItems: 'center', justifyContent: 'flex-end'}}>
                                            {/* Reachable chats — surfaces the chat_id the notify/interact API
                                                needs in its body, copyable. Kept in Actions (rather than its
                                                own column) because it is reference data the operator copies
                                                on demand, not a per-row status; the fixed column budget is
                                                tuned for the always-visible columns. The button owns its own
                                                tooltip so it can dismiss it when the popover opens (a
                                                wrapping Tooltip here leaves its hover text lingering). */}
                                            <BotChatsButton
                                                botUUID={bot.uuid!}
                                                platform={bot.platform}
                                                pairingRequired={pairingNeeded}
                                                disabled={toggling || restarting}
                                            />
                                            <Tooltip title={isActive
                                                ? t('remoteControl.card.restartBot', {defaultValue: 'Restart Bot'})
                                                : t('remoteControl.card.enableToRestart', {defaultValue: 'Enable bot to restart'})}>
                                                <span>
                                                    <IconButton
                                                        size="small"
                                                        color="primary"
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
                                                    onClick={() => handleDeleteClick(bot)}
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

            <ConfirmDialog
                open={deleteModal.open}
                title={t('remoteControl.card.deleteTitle', {defaultValue: 'Delete Bot Configuration'})}
                description={t('remoteControl.card.deleteConfirm', {
                    defaultValue: 'Are you sure you want to delete "{{name}}"? This action cannot be undone.',
                    name: deleteModal.bot?.name || deleteModal.bot?.platform,
                })}
                confirmLabel={t('remoteControl.card.delete', {defaultValue: 'Delete'})}
                confirmColor="error"
                onClose={() => setDeleteModal({open: false, bot: null})}
                onConfirm={handleConfirmDelete}
            />
        </>
    );
};

export default BotTable;
