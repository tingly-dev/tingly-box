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
import { alpha } from '@mui/material/styles';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { SmartRouting, SmartOp } from '../RoutingGraphTypes.ts';
import {
    ActionButtonsBox,
    getRouteGraphActiveColor,
    StyledSmartNodePrimary,
    StyledSmartNodeWrapper,
} from './styles.tsx';

export interface SmartNodeProps {
    smartRouting: SmartRouting;
    index?: number;
    active: boolean;
    onEdit: () => void;
    onDelete: () => void;
    onMoveUp?: () => void;
    onMoveDown?: () => void;
}

// Full tooltip text for a single op.
const opTooltip = (op: SmartOp): string => {
    const val = op.value ? `: ${op.value}` : '';
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
            <StyledSmartNodePrimary active={active} onClick={onEdit}>

                {/* ── Header ── */}
                <Stack direction="row" alignItems="center" gap={0.5} sx={{ width: '100%', flexShrink: 0 }}>
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
                                fontSize: '0.6rem',
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
                            fontSize: '0.65rem',
                            fontWeight: 700,
                            color: 'text.secondary',
                            flexGrow: 1,
                            lineHeight: 1,
                            letterSpacing: '0.04em',
                        }}
                    >
                        IF
                    </Typography>

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
                    <Box
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
                                fontSize: '0.65rem',
                                color: 'text.disabled',
                                fontStyle: 'italic',
                                textAlign: 'center',
                                lineHeight: 1.3,
                            }}
                        >
                            {t('rule.smart.unconditional', { defaultValue: 'Unconditional, ignore' })}
                        </Typography>
                    </Box>
                ) : (
                    <Stack gap={0.4} sx={{ width: '100%' }}>
                        {ops.map((op) => (
                            <NodeTooltip key={op.uuid} title={opTooltip(op)} placement="right">
                                <Box
                                    sx={(theme) => ({
                                        width: '100%',
                                        px: 0.75,
                                        py: 0.35,
                                        borderRadius: 0.75,
                                        border: '1px solid',
                                        borderColor: alpha(
                                            getRouteGraphActiveColor(theme),
                                            theme.palette.mode === 'dark' ? 0.28 : 0.18,
                                        ),
                                        backgroundColor: alpha(
                                            getRouteGraphActiveColor(theme),
                                            theme.palette.mode === 'dark' ? 0.07 : 0.03,
                                        ),
                                        display: 'flex',
                                        alignItems: 'center',
                                        gap: 0.5,
                                        overflow: 'hidden',
                                        minHeight: 22,
                                    })}
                                >
                                    {/* position — accent label */}
                                    <Typography
                                        component="span"
                                        sx={(theme) => ({
                                            fontSize: '0.6rem',
                                            fontWeight: 700,
                                            color: getRouteGraphActiveColor(theme),
                                            flexShrink: 0,
                                            lineHeight: 1,
                                        })}
                                    >
                                        {op.position}
                                    </Typography>

                                    {/* operation — muted */}
                                    <Typography
                                        component="span"
                                        sx={{
                                            fontSize: '0.6rem',
                                            color: 'text.disabled',
                                            flexShrink: 0,
                                            lineHeight: 1,
                                        }}
                                    >
                                        · {op.operation}
                                    </Typography>

                                    {/* value — primary, truncated */}
                                    {op.value && (
                                        <Typography
                                            component="span"
                                            sx={{
                                                fontSize: '0.6rem',
                                                fontWeight: 500,
                                                color: 'text.primary',
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                whiteSpace: 'nowrap',
                                                flexGrow: 1,
                                                lineHeight: 1,
                                            }}
                                        >
                                            : {op.value}
                                        </Typography>
                                    )}
                                </Box>
                            </NodeTooltip>
                        ))}
                    </Stack>
                )}
            </StyledSmartNodePrimary>

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
