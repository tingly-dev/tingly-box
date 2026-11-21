import { Box, ButtonBase, Divider, IconButton, ListItemIcon, ListItemText, Menu, MenuItem, Typography } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Add as AddIcon,
    Delete as DeleteIcon,
    Edit as EditIcon,
    KeyboardArrowDown,
    KeyboardArrowUp,
    MoreHoriz,
    Speed as LimitsIcon,
    Warning as WarningIcon,
} from '@/components/icons';
import ConfirmDialog from '@tingly/ui/components/ConfirmDialog';
import LimitsDialog from '@/components/limits/LimitsDialog';
import type { Limits } from '@/components/limits/LimitsEditor';
import { ProviderLabel } from '@/components/ProviderLogo';
import type { ConfigProvider, ConfigRecord, SmartOp } from '@/components/RoutingGraphTypes';
import { formatTimeRange } from '@/components/rule-card/timeRange';
import type { Provider } from '@/types/provider';
import { fontMono } from '@tingly/ui/theme/fonts';
import { mobile } from '@tingly/ui/theme/mobile';
import { radius } from '@tingly/ui/theme/radius';

export interface PhoneRuleTargetsProps {
    record: ConfigRecord;
    providers: Provider[];
    active: boolean;
    saving: boolean;
    onServiceNodeClick?: (serviceUuid: string) => void;
    onEditProvider?: (providerUuid: string) => void;
    onTierChange?: (serviceUuid: string, tier: number) => void;
    onLimitsChange?: (serviceUuid: string, limits: Limits) => void;
    onDeleteService?: (serviceUuid: string) => void;
    onAddService?: (tier?: number) => void;
    onAddServiceToSmartRule?: (ruleUuid: string) => void;
    onDeleteServiceFromSmartRule?: (ruleUuid: string, serviceUuid: string) => void;
}

const opText = (op: SmartOp) => {
    const value = op.meta?.type === 'time_range' ? formatTimeRange(op.value) || op.value : op.value;
    return `${op.position} ${op.operation}${value ? ` ${value}` : ''}`;
};

/**
 * A rule's routing on a phone (`.design/mobile-ui.md`, "Agent page"): the
 * graph becomes a list. Its conditions (IF steps, read-only here) and its
 * tiers each list their target models; tapping a target opens the model
 * picker on it, its "⋯" holds what desktop shows on hover (tier, credential,
 * limits, delete), and every group can take another model.
 */
export function PhoneRuleTargets({
    record, providers, active, saving,
    onServiceNodeClick, onEditProvider, onTierChange, onLimitsChange, onDeleteService, onAddService,
    onAddServiceToSmartRule, onDeleteServiceFromSmartRule,
}: PhoneRuleTargetsProps) {
    const { t } = useTranslation();
    const smart = record.smartEnabled ? record.smartRouting ?? [] : [];

    // Tiers, in order; T0 is always there so a new rule has somewhere to add.
    const tiers = new Map<number, ConfigProvider[]>();
    for (const p of [...record.providers].sort((a, b) => (a.tier ?? 0) - (b.tier ?? 0))) {
        const tier = p.tier ?? 0;
        tiers.set(tier, [...(tiers.get(tier) ?? []), p]);
    }
    if (tiers.size === 0) tiers.set(0, []);
    const tierList = [...tiers.entries()];
    const lastTier = tierList[tierList.length - 1];

    return (
        <Box sx={{ px: 1.5, pb: 1.5, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
            {smart.map((cond, index) => (
                <Group
                    key={cond.uuid}
                    title={`IF ${index + 1}`}
                    caption={(cond.ops ?? []).length > 0 ? (cond.ops ?? []).map(opText).join(' · ') : cond.description}
                >
                    {(cond.services ?? []).map((service) => (
                        <TargetRow
                            key={service.uuid}
                            service={service}
                            providers={providers}
                            active={active && service.active !== false}
                            onOpen={() => onServiceNodeClick?.(service.uuid)}
                            onEditProvider={onEditProvider}
                            onDelete={onDeleteServiceFromSmartRule ? () => onDeleteServiceFromSmartRule(cond.uuid, service.uuid) : undefined}
                        />
                    ))}
                    <AddRow label={t('rule.nodes.addModel')} disabled={!active || saving} onClick={() => onAddServiceToSmartRule?.(cond.uuid)} />
                </Group>
            ))}
            {smart.length > 0 && (
                <Typography variant="caption" color="text.secondary" sx={{ px: 0.5 }}>
                    {t('rule.phone.conditionsOnDesktop')}
                </Typography>
            )}
            {tierList.map(([tier, services], idx) => {
                const loneBottom = idx === tierList.length - 1 && services.length === 1;
                return (
                    <Group
                        key={tier}
                        title={idx === 0 ? `${smart.length > 0 ? t('rule.ruleConditions.othersTitle') : t('rule.ruleConditions.requestTitle')} · T${tier}` : `T${tier}`}
                    >
                        {services.map((service) => (
                            <TargetRow
                                key={service.uuid}
                                service={service}
                                providers={providers}
                                active={active && service.active !== false}
                                ruleUuid={record.uuid}
                                onOpen={() => onServiceNodeClick?.(service.uuid)}
                                onEditProvider={onEditProvider}
                                onMoveUp={tier > 0 && onTierChange ? () => onTierChange(service.uuid, tier - 1) : undefined}
                                onMoveDown={onTierChange && !loneBottom ? () => onTierChange(service.uuid, tier + 1) : undefined}
                                onLimitsChange={onLimitsChange ? (limits) => onLimitsChange(service.uuid, limits) : undefined}
                                onDelete={onDeleteService ? () => onDeleteService(service.uuid) : undefined}
                            />
                        ))}
                        <AddRow label={t('rule.nodes.addModel')} disabled={!active || saving} onClick={() => onAddService?.(tier)} />
                    </Group>
                );
            })}
            {lastTier[1].length > 0 && (
                <AddRow
                    label={t('rule.nodes.fallbackTier')}
                    disabled={!active || saving}
                    onClick={() => onAddService?.(lastTier[0] + 1)}
                    outlined
                />
            )}
        </Box>
    );
}

function Group({ title, caption, children }: { title: string; caption?: string; children: React.ReactNode }) {
    return (
        <Box>
            <Box sx={{ px: 0.5, pb: 0.5 }}>
                <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>{title}</Typography>
                {caption && (
                    <Typography variant="caption" component="p" sx={{ color: 'text.secondary', fontFamily: fontMono, overflowWrap: 'anywhere' }}>
                        {caption}
                    </Typography>
                )}
            </Box>
            <Box sx={{ border: 1, borderColor: 'divider', borderRadius: radius.row, overflow: 'hidden', '& > *:not(:last-child)': { borderBottom: 1, borderColor: 'divider' } }}>
                {children}
            </Box>
        </Box>
    );
}

function AddRow({ label, onClick, disabled, outlined }: { label: string; onClick: () => void; disabled?: boolean; outlined?: boolean }) {
    return (
        <ButtonBase
            onClick={onClick}
            disabled={disabled}
            sx={{
                width: '100%', minHeight: mobile.tapMin, px: 1.5, gap: 1, justifyContent: 'flex-start',
                color: disabled ? 'text.disabled' : 'primary.main',
                ...(outlined && { border: '1px dashed', borderColor: 'divider', borderRadius: radius.row }),
            }}
        >
            <AddIcon fontSize="small" />
            <Typography variant="body2" sx={{ fontWeight: 500 }}>{label}</Typography>
        </ButtonBase>
    );
}

interface TargetRowProps {
    service: ConfigProvider;
    providers: Provider[];
    active: boolean;
    ruleUuid?: string;
    onOpen: () => void;
    onEditProvider?: (providerUuid: string) => void;
    onMoveUp?: () => void;
    onMoveDown?: () => void;
    onLimitsChange?: (limits: Limits) => void;
    onDelete?: () => void;
}

function TargetRow({ service, providers, active, ruleUuid, onOpen, onEditProvider, onMoveUp, onMoveDown, onLimitsChange, onDelete }: TargetRowProps) {
    const { t } = useTranslation();
    const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
    const [limitsOpen, setLimitsOpen] = useState(false);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const provider = providers.find((p) => p.uuid === service.provider);
    const warning = !provider
        ? t('rule.service.providerNotFound')
        : provider.enabled === false ? t('rule.service.providerDisabled') : '';
    const close = () => setMenuAnchor(null);
    const then = (fn: () => void) => () => { close(); fn(); };

    return (
        <Box sx={{ display: 'flex', alignItems: 'center', bgcolor: 'background.paper', opacity: active ? 1 : 0.6 }}>
            <ButtonBase
                onClick={onOpen}
                sx={{ flex: 1, minWidth: 0, minHeight: mobile.rowMin, px: 1.5, py: 1, justifyContent: 'flex-start', textAlign: 'left' }}
            >
                <Box sx={{ minWidth: 0 }}>
                    <Typography
                        variant="body2"
                        sx={{ fontFamily: fontMono, fontWeight: 500, overflowWrap: 'anywhere', color: service.model ? 'text.primary' : 'text.disabled' }}
                    >
                        {service.model || t('rule.service.selectModel')}
                    </Typography>
                    <ProviderLabel
                        provider={provider}
                        size={14}
                        gap={0.5}
                        leading={warning ? <WarningIcon sx={{ fontSize: 16, color: 'warning.main', flexShrink: 0 }} /> : undefined}
                    >
                        <Typography variant="caption" noWrap color={warning ? 'warning.main' : 'text.secondary'}>
                            {provider?.name ?? t('rule.service.selectProvider')}
                        </Typography>
                    </ProviderLabel>
                </Box>
            </ButtonBase>
            <IconButton aria-label={t('rule.phone.targetActions')} onClick={(e) => setMenuAnchor(e.currentTarget)} sx={{ mr: 0.5 }}>
                <MoreHoriz />
            </IconButton>
            <Menu anchorEl={menuAnchor} open={!!menuAnchor} onClose={close}>
                {onMoveUp && (
                    <MenuItem onClick={then(onMoveUp)}>
                        <ListItemIcon><KeyboardArrowUp fontSize="small" /></ListItemIcon>
                        <ListItemText>{t('common.moveUp')}</ListItemText>
                    </MenuItem>
                )}
                {onMoveDown && (
                    <MenuItem onClick={then(onMoveDown)}>
                        <ListItemIcon><KeyboardArrowDown fontSize="small" /></ListItemIcon>
                        <ListItemText>{t('common.moveDown')}</ListItemText>
                    </MenuItem>
                )}
                {provider && onEditProvider && (
                    <MenuItem onClick={then(() => onEditProvider(provider.uuid))}>
                        <ListItemIcon><EditIcon fontSize="small" /></ListItemIcon>
                        <ListItemText>{t('rule.service.editProvider')}</ListItemText>
                    </MenuItem>
                )}
                {onLimitsChange && (
                    <MenuItem onClick={then(() => setLimitsOpen(true))}>
                        <ListItemIcon><LimitsIcon fontSize="small" /></ListItemIcon>
                        <ListItemText>{t('limits.titleService')}</ListItemText>
                    </MenuItem>
                )}
                {onDelete && <Divider />}
                {onDelete && (
                    <MenuItem onClick={then(() => setConfirmDelete(true))} sx={{ color: 'error.main' }}>
                        <ListItemIcon><DeleteIcon fontSize="small" color="error" /></ListItemIcon>
                        <ListItemText>{t('rule.service.deleteService')}</ListItemText>
                    </MenuItem>
                )}
            </Menu>
            {onLimitsChange && (
                <LimitsDialog
                    open={limitsOpen}
                    onClose={() => setLimitsOpen(false)}
                    value={service.limits}
                    onSave={onLimitsChange}
                    statusKey={ruleUuid ? `svc:${ruleUuid}/${service.provider}/${service.model}` : undefined}
                />
            )}
            {onDelete && (
                <ConfirmDialog
                    open={confirmDelete}
                    title={t('rule.service.deleteService')}
                    description={service.model}
                    confirmLabel={t('common.delete')}
                    cancelLabel={t('common.cancel')}
                    confirmColor="error"
                    onClose={() => setConfirmDelete(false)}
                    onConfirm={() => { setConfirmDelete(false); onDelete(); }}
                />
            )}
        </Box>
    );
}

export default PhoneRuleTargets;
