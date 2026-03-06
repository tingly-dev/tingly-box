import {Delete as DeleteIcon,} from '@mui/icons-material';
import {Box, IconButton, ListItemIcon, ListItemText, Menu, MenuItem, Tooltip, Typography,} from '@mui/material';
import React, {useState} from 'react';
import {useTranslation} from 'react-i18next';
import type {SmartRouting, SmartOp} from '../RoutingGraphTypes.ts';
import {ActionButtonsBox, StyledSmartNodePrimary, StyledSmartNodeWrapper,} from './styles.tsx';

// Smart node internal dimensions
const SMART_NODE_INTERNAL_STYLES = {
    badgeHeight: 20,
    fieldPadding: 4,
} as const;

export interface SmartNodeProps {
    smartRouting: SmartRouting;
    index?: number; // Frontend-generated index for numbering
    active: boolean;
    onEdit: () => void;
    onDelete: () => void;
}

export const SmartOpNode: React.FC<SmartNodeProps> = ({
                                                          smartRouting,
                                                          index,
                                                          active,
                                                          onEdit,
                                                          onDelete,
                                                      }) => {
    const {t} = useTranslation();
    const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
    const menuOpen = Boolean(anchorEl);

    const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setAnchorEl(event.currentTarget);
    };

    const handleMenuClose = () => {
        setAnchorEl(null);
    };

    const handleDelete = () => {
        handleMenuClose();
        onDelete();
    };

    const handleNodeClick = () => {
        onEdit();
    };

    const firstOp = smartRouting.ops?.[0];

    // Format single op display: e.g., "model: contains" or "user: regex"
    const getOpDisplay = (op: SmartOp | undefined): string => {
        if (!op) return t('rule.smart.noOperation');
        const opLabel = op.operation || 'unknown';
        return `[${op.position}] [${opLabel}]`;
    };

    // Get value for second line - truncated if too long
    const getOpValue = (op: SmartOp | undefined): string => {
        if (!op?.value) return '';
        return op.value;
    };

    // Truncate value for display (max 20 chars)
    const getTruncatedValue = (op: SmartOp | undefined): string => {
        const value = getOpValue(op);
        if (value.length > 20) {
            return value.slice(0, 20) + '...';
        }
        return value;
    };

    // Full display for single op (includes operation and value)
    const getOpDisplayFull = (op: SmartOp | undefined): string => {
        if (!op) return t('rule.smart.noOperation');
        const opLabel = op.operation || 'unknown';
        const valueStr = op.value ? `: ${op.value}` : '';
        return `[${op.position}] [${opLabel}]${valueStr}`;
    };

    // Summary for multi-op display - shows count and first op
    const getMultiOpSummary = (): string => {
        const ops = smartRouting.ops || [];
        if (ops.length === 0) return t('rule.smart.noOperation');

        if (ops.length === 1) {
            return getOpDisplay(ops[0]);
        }

        // For multiple ops, show count and first op
        return `${ops.length} conditions (AND)`;
    };

    // Full tooltip for multi-op - shows all ops with AND logic
    const getMultiOpDisplayFull = (): string => {
        const ops = smartRouting.ops || [];
        if (ops.length === 0) return t('rule.smart.noOperation');

        const opStrings = ops.map(op => getOpDisplayFull(op));
        if (ops.length === 1) {
            return opStrings[0];
        }

        // Join with AND
        return `If ${opStrings.join(' AND ')}`;
    };

    return (
        <StyledSmartNodeWrapper>
            <StyledSmartNodePrimary active={active} onClick={handleNodeClick}>
                {/* Index Badge */}
                {index !== undefined && (
                    <Box
                        sx={{
                            position: 'absolute',
                            top: -8,
                            left: -8,
                            backgroundColor: 'primary.main',
                            color: 'primary.contrastText',
                            borderRadius: '50%',
                            width: 24,
                            height: 24,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            fontSize: '0.75rem',
                            fontWeight: 'bold',
                            boxShadow: 1,
                            zIndex: 1,
                        }}
                    >
                        {index + 1}
                    </Box>
                )}
                {/* Content */}
                <Box sx={{mt: 1, width: '100%'}}>
                    {/* Value display - show truncated with operation details on hover */}
                    <Tooltip title={getMultiOpDisplayFull()} arrow>
                        <Typography
                            variant="body2"
                            sx={{
                                fontWeight: 600,
                                color: 'text.primary',
                                fontSize: '0.85rem',
                                mb: 1,
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                whiteSpace: 'nowrap',
                            }}
                        >
                            {getTruncatedValue(firstOp) || t('rule.smart.noValue')}
                        </Typography>
                    </Tooltip>

                    {/* Summary Info */}
                    <Box
                        sx={{
                            width: '100%',
                        }}
                    >
                        <Tooltip title={getMultiOpDisplayFull()} arrow>
                            <Box
                                sx={{
                                    width: '100%',
                                    p: 1,
                                    border: '1px solid',
                                    borderColor: 'divider',
                                    borderRadius: 1,
                                    backgroundColor: 'background.paper',
                                    transition: 'all 0.2s',
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                }}
                            >
                                <Typography
                                    variant="body2"
                                    sx={{
                                        fontSize: '0.8rem',
                                        color: 'text.secondary',
                                        fontWeight: 500,
                                        overflow: 'hidden',
                                        textOverflow: 'ellipsis',
                                        whiteSpace: 'nowrap',
                                        width: '100%',
                                    }}
                                >
                                    {getMultiOpSummary()}
                                </Typography>
                            </Box>
                        </Tooltip>
                    </Box>
                </Box>

                {/* Action Buttons - visible on hover */}
                <ActionButtonsBox className="action-buttons">
                    <Tooltip title={t('rule.smart.deleteTooltip')}>
                        <IconButton
                            size="small"
                            onClick={handleMenuClick}
                            sx={{p: 0.5, backgroundColor: 'background.paper'}}
                        >
                            <DeleteIcon sx={{fontSize: '1rem', color: 'error.main'}}/>
                        </IconButton>
                    </Tooltip>
                </ActionButtonsBox>
            </StyledSmartNodePrimary>

            {/* Delete Confirmation Menu */}
            <Menu
                anchorEl={anchorEl}
                open={menuOpen}
                onClose={handleMenuClose}
                onClick={(e) => e.stopPropagation()}
                transformOrigin={{horizontal: 'right', vertical: 'top'}}
                anchorOrigin={{horizontal: 'right', vertical: 'bottom'}}
            >
                <MenuItem onClick={handleDelete} disabled={!active}>
                    <ListItemIcon>
                        <DeleteIcon />
                    </ListItemIcon>
                    <ListItemText>
                        {t('rule.menu.deleteSmartRule')}
                    </ListItemText>
                </MenuItem>
                <MenuItem onClick={handleMenuClose} sx={{color: 'text.secondary'}}>
                    <ListItemText>{t('common.cancel')}</ListItemText>
                </MenuItem>
            </Menu>
        </StyledSmartNodeWrapper>
    );
};

export default SmartOpNode;
