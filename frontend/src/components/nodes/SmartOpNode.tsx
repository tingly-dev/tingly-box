import { Delete as DeleteIcon, KeyboardArrowUp, KeyboardArrowDown } from '@/components/icons';
import {
    Box,
    Divider,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Stack,
    Typography,
} from '@mui/material';
import NodeTooltip from './NodeTooltip.tsx';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { SmartRouting, SmartOp } from '../RoutingGraphTypes.ts';
import { formatTimeRange } from '../rule-card/timeRange.ts';
import {
    ActionButtonsBox,
    NODE_LAYER_STYLES,
    StyledSmartNodePrimary,
    StyledSmartNodeWrapper,
} from './styles.tsx';
import { fontSizes } from '@/theme/fonts';
import { ONE } from './draft';
import { draftLevel, draftOpText, draftText, DRAFT_HIT_RULE } from './draft';

export interface SmartNodeProps {
    smartRouting: SmartRouting;
    index?: number;
    active: boolean;
    onEdit: (opUuid?: string) => void;
    onDelete: () => void;
    onMoveUp?: () => void;
    onMoveDown?: () => void;
}

const opValue = (op: SmartOp): string =>
    op.meta?.type === 'time_range' ? formatTimeRange(op.value) || op.value : op.value;

// Full tooltip text for a single op.
const opTooltip = (op: SmartOp): string => {
    const value = opValue(op);
    const val = value ? `: ${value}` : '';
    return `${op.position} · ${op.operation}${val}`;
};

export const SmartOpNode: React.FC<SmartNodeProps> = ({
    smartRouting,
    index,
    active,
    onEdit,
    onDelete,
    onMoveUp,
    onMoveDown,
}) => {
    const { t } = useTranslation();
    const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
    const menuOpen = Boolean(anchorEl);
    const ops: SmartOp[] = smartRouting.ops ?? [];
    const draft = draftLevel();
    const hit = [2,3].includes(draft) && smartRouting.uuid === DRAFT_HIT_RULE;

    const handleDeleteClick = (e: React.MouseEvent<HTMLElement>) => {
        e.stopPropagation();
        setAnchorEl(e.currentTarget);
    };

    const handleMenuClose = () => setAnchorEl(null);

    const handleConfirmDelete = () => {
        handleMenuClose();
        onDelete();
    };

    return (
        <StyledSmartNodeWrapper>
            {ONE ? (
                <NodeTooltip title={ops.map(opTooltip).join('\n') || t('rule.smart.unconditional', { defaultValue: 'Unconditional, ignore' })} placement="top">
                    <StyledSmartNodePrimary active={active} onClick={() => onEdit()} sx={{ flexDirection: 'row', alignItems: 'center', gap: 0.75, px: 1, cursor: 'pointer' }}>
                        <Box sx={{ width: 18, height: 18, borderRadius: '50%', bgcolor: 'primary.main', color: 'primary.contrastText', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: fontSizes.micro, fontWeight: 700, flexShrink: 0 }}>
                            {(index ?? 0) + 1}
                        </Box>
                        <Typography variant="body2" noWrap sx={{ ...NODE_LAYER_STYLES.typography, fontWeight: 500, minWidth: 0, color: ops.length ? 'text.primary' : 'text.disabled', fontStyle: ops.length ? 'normal' : 'italic' }}>
                            {ops.length === 0
                                ? t('rule.smart.unconditional', { defaultValue: 'Unconditional, ignore' })
                                : ops.map((op) => draftOpText(op.position, op.operation, opValue(op)) ?? `${op.position} ${op.operation} ${opValue(op)}`).join(' · ')}
                        </Typography>
                    </StyledSmartNodePrimary>
                </NodeTooltip>
            ) : (
            <StyledSmartNodePrimary active={active} onClick={() => onEdit()} sx={hit ? { borderColor: 'primary.main', boxShadow: (th) => `0 0 0 1px ${th.palette.primary.main}` } : undefined}>

                {/* ── Header ── */}
                <Stack
                    direction="row"
                    sx={{
                        alignItems: "center",
                        gap: 0.5,
                        width: '100%',
                        flexShrink: 0
                    }}>
                    {/* Inline index badge */}
                    {index !== undefined && (
                        <Box
                            sx={{
                                width: 18,
                                height: 18,
                                borderRadius: '50%',
                                backgroundColor: 'primary.main',
                                color: 'primary.contrastText',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                fontSize: fontSizes.micro,
                                fontWeight: 700,
                                flexShrink: 0,
                                lineHeight: 1,
                            }}
                        >
                            {index + 1}
                        </Box>
                    )}

                    <Typography
                        sx={{
                            fontSize: fontSizes.xs,
                            fontWeight: 700,
                            color: 'text.secondary',
                            flexGrow: 1,
                            lineHeight: 1,
                            letterSpacing: '0.04em',
                        }}
                    >
                        IF
                    </Typography>
                    {hit && (
                        <Typography sx={{ fontSize: fontSizes.micro, fontWeight: 600, color: 'primary.main', lineHeight: 1, whiteSpace: 'nowrap' }}>
                            {draftText('hit')}
                        </Typography>
                    )}

                    {/* Action buttons — revealed on hover via .action-buttons class */}
                    <Stack
                        direction="row"
                        className="action-buttons"
                        sx={{ opacity: 0, transition: 'opacity 0.2s', gap: 0.25 }}
                        onClick={(e) => e.stopPropagation()}
                    >
                        {onMoveUp && (
                            <NodeTooltip title={t('common.moveUp', { defaultValue: 'Move up' })} placement="top">
                                <IconButton
                                    size="small"
                                    onClick={(e) => { e.stopPropagation(); onMoveUp(); }}
                                    sx={{ p: 0.25, backgroundColor: 'background.paper' }}
                                    aria-label="Move smart rule up"
                                >
                                    <KeyboardArrowUp sx={{ fontSize: '0.875rem' }} />
                                </IconButton>
                            </NodeTooltip>
                        )}
                        {onMoveDown && (
                            <NodeTooltip title={t('common.moveDown', { defaultValue: 'Move down' })} placement="top">
                                <IconButton
                                    size="small"
                                    onClick={(e) => { e.stopPropagation(); onMoveDown(); }}
                                    sx={{ p: 0.25, backgroundColor: 'background.paper' }}
                                    aria-label="Move smart rule down"
                                >
                                    <KeyboardArrowDown sx={{ fontSize: '0.875rem' }} />
                                </IconButton>
                            </NodeTooltip>
                        )}
                        <NodeTooltip title={t('rule.smart.deleteTooltip')} placement="top">
                            <IconButton
                                size="small"
                                onClick={handleDeleteClick}
                                sx={{ p: 0.25, backgroundColor: 'background.paper' }}
                                aria-label={t('rule.smart.deleteTooltip')}
                            >
                                <DeleteIcon sx={{ fontSize: '0.875rem' }} />
                            </IconButton>
                        </NodeTooltip>
                    </Stack>
                </Stack>

                <Divider sx={{ width: '100%', my: 0.5 }} />

                {/* ── Body ── */}
                {ops.length === 0 ? (
                    /* Unconditional — no conditions means this branch always fires */
                    (<Box
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            flexGrow: 1,
                            py: 0.5,
                        }}
                    >
                        <Typography
                            sx={{
                                fontSize: fontSizes.xs,
                                color: 'text.disabled',
                                fontStyle: 'italic',
                                textAlign: 'center',
                                lineHeight: 1.3,
                            }}
                        >
                            {t('rule.smart.unconditional', { defaultValue: 'Unconditional, ignore' })}
                        </Typography>
                    </Box>)
                ) : (
                    <Stack
                        sx={{
                            gap: 0.4,
                            width: '100%'
                        }}>
                        {ops.map((op) => (
                            <NodeTooltip key={op.uuid} title={opTooltip(op)} placement="right">
                                <Box
                                    onClick={(e) => { e.stopPropagation(); onEdit(op.uuid); }}
                                    sx={{
                                        width: '100%',
                                        cursor: 'pointer',
                                        px: 0.75,
                                        py: 0.35,
                                        borderRadius: 0.75,
                                        border: '1px solid',
                                        borderColor: 'transparent',
                                        backgroundColor: 'action.hover',
                                        display: 'flex',
                                        flexWrap: 'wrap',
                                        alignItems: 'center',
                                        columnGap: 0.5,
                                        rowGap: 0.25,
                                        overflow: 'hidden',
                                        minHeight: 34,
                                    }}
                                >
                                    {draft >= 1 && draftOpText(op.position, op.operation, opValue(op)) ? (
                                        <Typography variant="body2" component="span" sx={{ ...NODE_LAYER_STYLES.typography, fontSize: fontSizes.sm, fontWeight: 500, color: 'text.primary', width: '100%' }}>
                                            {draftOpText(op.position, op.operation, opValue(op))}
                                        </Typography>
                                    ) : (<>
                                    {/* position — primary label */}
                                    <Typography
                                        variant="body2"
                                        component="span"
                                        sx={{
                                            ...NODE_LAYER_STYLES.typography,
                                            fontSize: fontSizes.sm,
                                            fontWeight: 500,
                                            color: 'text.secondary',
                                            width: '100%',
                                        }}
                                    >
                                        {op.position}
                                    </Typography>

                                    {/* operation */}
                                    <Typography
                                        variant="body2"
                                        component="span"
                                        sx={{
                                            ...NODE_LAYER_STYLES.typography,
                                            fontSize: fontSizes.sm,
                                            fontWeight: 400,
                                            color: 'text.secondary',
                                            flexShrink: 0,
                                        }}
                                    >
                                        · {op.operation}
                                    </Typography>

                                    {op.value && (
                                        <Typography
                                            variant="body2"
                                            component="span"
                                            sx={{
                                                ...NODE_LAYER_STYLES.typography,
                                                fontSize: fontSizes.sm,
                                                fontWeight: 400,
                                                color: 'text.secondary',
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                whiteSpace: 'nowrap',
                                                flexGrow: 1,
                                            }}
                                        >
                                            : {opValue(op)}
                                        </Typography>
                                    )}
                                    </>)}
                                </Box>
                            </NodeTooltip>
                        ))}
                    </Stack>
                )}
            </StyledSmartNodePrimary>
            )}
            {/* Delete confirmation menu */}
            <Menu
                anchorEl={anchorEl}
                open={menuOpen}
                onClose={handleMenuClose}
                onClick={(e) => e.stopPropagation()}
                transformOrigin={{ horizontal: 'right', vertical: 'top' }}
                anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
            >
                <MenuItem onClick={handleConfirmDelete} disabled={!active}>
                    <ListItemIcon>
                        <DeleteIcon />
                    </ListItemIcon>
                    <ListItemText>{t('rule.menu.deleteSmartRule')}</ListItemText>
                </MenuItem>
                <MenuItem onClick={handleMenuClose} sx={{ color: 'text.secondary' }}>
                    <ListItemText>{t('common.cancel')}</ListItemText>
                </MenuItem>
            </Menu>
        </StyledSmartNodeWrapper>
    );
};

export default SmartOpNode;
