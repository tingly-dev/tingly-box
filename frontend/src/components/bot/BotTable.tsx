import StatusDot from '@tingly/ui/components/StatusDot';
import { quietDangerIconSx } from '@tingly/ui/theme/selection';
import {Delete as DeleteIcon, Edit as EditIcon, RestartAlt as RestartIcon} from '@/components/icons';
import ConfirmPopover from '@/components/ConfirmPopover';
import {capabilityEnabled} from '@/types/bot';
import type {BotSettings} from '@/types/bot';
import {EntityLabel} from '@tingly/ui';
import {useEntityLabels} from '@/components/entity/useEntityLabels';
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
// A capability chip: a status dot carries on/off, the label stays neutral;
// an off capability is dashed (it can still be opened to turn it on).
const capabilityChipProps = (on: boolean) => ({
    size: 'small' as const,
    variant: 'outlined' as const,
    icon: <StatusDot tone={on ? 'success' : 'muted'} sx={{ml: '8px !important', mr: '-2px !important'}}/>,
    sx: {color: on ? 'text.primary' : 'text.secondary', borderStyle: on ? 'solid' : 'dashed'},
});
// Fixed table layout with explicit per-column widths (mirrors ApiKeyTable) so
// column proportions stay stable regardless of content — auto layout let the
// Pairing column balloon and starved the others. Widths sum to tableMinWidth
// so the table fills the card at any width above it.
const headCellSx = {overflow: 'hidden'} as const;
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
    const entities = useEntityLabels();

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

    return (
        <>
            <TableContainer
                // No Paper, no border — the page's UnifiedCard is the shell.
                sx={{display: {xs: 'none', md: 'block'}, overflowX: 'auto'}}
            >
                <Table sx={{tableLayout: 'fixed', minWidth: tableMinWidth}}>
                    <TableHead>
                        <TableRow>
                            <TableCell sx={col(120)}>{t('bots.table.status', {defaultValue: 'Status'})}</TableCell>
                            <TableCell sx={col(430)}>{t('bots.table.bot', {defaultValue: 'Bot'})}</TableCell>
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
                                        </Stack>
                                    </TableCell>
                                    {/* Bot: 平台 · alias · id in one cell; the full uuid is the
                                        copy value (the notify/interact API takes it as :bot). */}
                                    <TableCell>
                                        <EntityLabel {...entities.bot(bot)} copy/>
                                    </TableCell>
                                    {/* Capability chips open the selected bot’s settings. */}
                                    <TableCell>
                                        <Box sx={{display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap'}}>
                                            <Chip {...(onCapabilityClick ? {onClick: () => onCapabilityClick(bot, 'remote_control')} : {component: RouterLink, to: '/remote-agent', clickable: true})} label={t('bots.card.remoteAgentChip', {defaultValue: 'Remote'})} {...capabilityChipProps(isMounted)}/>
                                            <Chip {...(onCapabilityClick ? {onClick: () => onCapabilityClick(bot, 'notify')} : {component: RouterLink, to: '/notify', clickable: true})} label={t('bots.card.notifyChip', {defaultValue: 'Notify'})} {...capabilityChipProps(isNotified)}/>
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
                                                    onClick={() => onEdit(bot.uuid!, bot.platform!)}
                                                    disabled={toggling || restarting}
                                                >
                                                    <EditIcon fontSize="small"/>
                                                </IconButton>
                                            </Tooltip>
                                            <Tooltip title={t('remoteControl.card.delete', {defaultValue: 'Delete'})}>
                                                <IconButton
                                                    size="small"
                                                    sx={quietDangerIconSx}
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
                                    <EntityLabel {...entities.bot(bot)} copy/>
                                </Box>
                                <Stack direction="row" spacing={0.75} sx={{alignItems: 'center'}}>
                                    <Switch checked={isActive} onChange={() => onBotToggle(bot.uuid!, !isActive)} size="small" color="success" disabled={toggling}/>
                                </Stack>
                            </Stack>
                            <Stack direction="row" spacing={0.75} sx={{mt: 1, flexWrap: 'wrap'}}>
                                <Chip {...(onCapabilityClick ? {onClick: () => onCapabilityClick(bot, 'remote_control')} : {component: RouterLink, to: '/remote-agent', clickable: true})} label={t('bots.card.remoteAgentChip', {defaultValue: 'Remote'})} {...capabilityChipProps(isMounted)}/>
                                <Chip {...(onCapabilityClick ? {onClick: () => onCapabilityClick(bot, 'notify')} : {component: RouterLink, to: '/notify', clickable: true})} label={t('bots.card.notifyChip', {defaultValue: 'Notify'})} {...capabilityChipProps(isNotified)}/>
                            </Stack>
                            <Stack direction="row" spacing={0.5} sx={{alignItems: 'center', justifyContent: 'flex-end', mt: 1.25, pt: 1, borderTop: 1, borderColor: 'divider'}}>
                                <IconButton size="small" aria-label={t('remoteControl.card.restartBot', {defaultValue: 'Restart Bot'})} onClick={() => onRestart(bot.uuid!)} disabled={!isActive || toggling || restarting}><RestartIcon fontSize="small"/></IconButton>
                                <IconButton size="small" onClick={() => onEdit(bot.uuid!, bot.platform!)} disabled={toggling || restarting}><EditIcon fontSize="small"/></IconButton>
                                <IconButton size="small" sx={quietDangerIconSx} onClick={(e) => handleDeleteClick(bot, e.currentTarget)} disabled={toggling || restarting}><DeleteIcon fontSize="small"/></IconButton>
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
                    name: deleteModal.bot ? entities.botText(deleteModal.bot) : '',
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
