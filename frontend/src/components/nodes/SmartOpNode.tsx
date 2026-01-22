import {
    Delete as DeleteIcon,
    Edit as EditIcon,
    SmartToy as SmartToyIcon,
} from '@mui/icons-material';
import {
    Box,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { SmartRouting } from '../RoutingGraphTypes.ts';
import {
    ActionButtonsBox,
    SMART_NODE_STYLES,
    StyledSmartNodePrimary,
    StyledSmartNodeWrapper,
} from './styles.tsx';

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
    const { t } = useTranslation();
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

    // Format op display: e.g., "model: contains" or "user: regex"
    const getOpDisplay = () => {
        if (!firstOp) return 'No Op';
        const opLabel = firstOp.operation || 'unknown';
        return `${firstOp.position}: ${opLabel} : ${firstOp.value || ''}`;
    };

    // Full display for tooltip
    const getOpDisplayFull = () => {
        if (!firstOp) return 'No Op';
        const opLabel = firstOp.operation || 'unknown';
        return `${firstOp.position}: ${opLabel} : ${firstOp.value || ''}`;
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
                <Box sx={{ mt: 1, width: '100%' }}>
                    {/* Description */}
                    <Typography
                        variant="body2"
                        sx={{
                            fontWeight: 600,
                            color: 'text.primary',
                            fontSize: '0.85rem',
                            mb: 1,
                        }}
                    >
                        {smartRouting.description || 'Untitled Smart Rule'}
                    </Typography>

                    {/* Summary Info */}
                    <Box
                        sx={{
                            width: '100%',
                        }}
                    >
                        <Tooltip title={getOpDisplayFull()} arrow>
                            <Box
                                sx={{
                                    width: '100%',
                                    p: 1,
                                    border: '1px solid',
                                    borderColor: active ? 'primary.main' : 'divider',
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
                                        color: active ? 'primary.main' : 'text.secondary',
                                        fontWeight: 500,
                                        overflow: 'hidden',
                                        textOverflow: 'ellipsis',
                                        whiteSpace: 'nowrap',
                                        width: '100%',
                                    }}
                                >
                                    {getOpDisplay()}
                                </Typography>
                            </Box>
                        </Tooltip>
                    </Box>
                </Box>

                {/* Action Buttons - visible on hover */}
                <ActionButtonsBox className="action-buttons">
                    <Tooltip title="Delete smart rule">
                        <IconButton
                            size="small"
                            onClick={handleMenuClick}
                            sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                        >
                            <DeleteIcon sx={{ fontSize: '1rem', color: 'error.main' }} />
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
                transformOrigin={{ horizontal: 'right', vertical: 'top' }}
                anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
            >
                <MenuItem onClick={handleDelete} disabled={!active}>
                    <ListItemIcon>
                        <DeleteIcon color="error" />
                    </ListItemIcon>
                    <ListItemText sx={{ color: 'error.main' }}>
                        {t('rule.menu.deleteSmartRule')}
                    </ListItemText>
                </MenuItem>
                <MenuItem onClick={handleMenuClose} sx={{ color: 'text.secondary' }}>
                    <ListItemText>Cancel</ListItemText>
                </MenuItem>
            </Menu>
        </StyledSmartNodeWrapper>
    );
};

export default SmartOpNode;
