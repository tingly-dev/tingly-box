import {
    Add as AddIcon,
    Delete as DeleteIcon,
    Edit as EditIcon,
    SmartToy as SmartToyIcon,
} from '@mui/icons-material';
import {
    Box,
    Chip,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Tooltip,
    Typography,
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { SmartRouting } from '../RoutingGraphTypes.ts';

// Node dimensions - smaller for better layout
const NODE_STYLES = {
    width: 220,
    height: 90,
    padding: 8,
} as const;

// Smart node internal dimensions
const SMART_NODE_STYLES = {
    badgeHeight: 20,
    fieldPadding: 4,
} as const;

const { node } = { node: NODE_STYLES };

// SmartOpNode Container - styled similar to ModelNode but with primary color
const StyledSmartNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: node.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: active ? 'primary.main' : 'divider',
    backgroundColor: active ? 'primary.50' : 'background.paper',
    textAlign: 'center',
    width: node.width,
    height: node.height,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    cursor: 'pointer',
    opacity: active ? 1 : 0.6,
    '&:hover': {
        borderColor: 'primary.main',
        backgroundColor: 'primary.100',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    }
}));

// Action button container
const ActionButtonsBox = styled(Box)(({ theme }) => ({
    position: 'absolute',
    top: 4,
    right: 4,
    display: 'flex',
    gap: 2,
    opacity: 0,
    transition: 'opacity 0.2s',
    '&:hover': {
        opacity: 1,
    },
}));

const StyledSmartNodeWrapper = styled(Box)(({ theme }) => ({
    position: 'relative',
    '&:hover .action-buttons': {
        opacity: 1,
    }
}));

export interface SmartNodeProps {
    smartRouting: SmartRouting;
    active: boolean;
    onEdit: () => void;
    onDelete: () => void;
    onAddService: () => void;
}

export const SmartOpNode: React.FC<SmartNodeProps> = ({
    smartRouting,
    active,
    onEdit,
    onDelete,
    onAddService,
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

    const handleEdit = () => {
        handleMenuClose();
        onEdit();
    };

    const handleAddService = () => {
        handleMenuClose();
        onAddService();
    };

    const handleDelete = () => {
        handleMenuClose();
        onDelete();
    };

    const servicesCount = smartRouting.services?.length || 0;
    const firstOp = smartRouting.ops?.[0];

    // Format op display: e.g., "model: contains" or "user: regex"
    const getOpDisplay = () => {
        if (!firstOp) return 'No Op';
        const opLabel = firstOp.operation || 'unknown';
        const valuePreview = firstOp.value?.length > 15
            ? `${firstOp.value.slice(0, 15)}...`
            : firstOp.value;
        return `${firstOp.position}: ${opLabel}`;
    };

    return (
        <StyledSmartNodeWrapper>
            <StyledSmartNode active={active}>
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
                            display: '-webkit-box',
                            WebkitLineClamp: 2,
                            WebkitBoxOrient: 'vertical',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            minHeight: 40,
                        }}
                    >
                        {smartRouting.description || 'Untitled Smart Rule'}
                    </Typography>

                    {/* Summary Info */}
                    <Box
                        sx={{
                            display: 'flex',
                            gap: 1,
                            justifyContent: 'center',
                            alignItems: 'center',
                        }}
                    >
                        <Chip
                            label={getOpDisplay()}
                            size="small"
                            variant="outlined"
                            sx={{
                                fontSize: '0.7rem',
                                height: 20,
                                borderColor: active ? 'primary.main' : 'divider',
                            }}
                        />
                        <Chip
                            label={`${servicesCount} ${servicesCount === 1 ? 'Service' : 'Services'}`}
                            size="small"
                            variant="outlined"
                            sx={{
                                fontSize: '0.7rem',
                                height: 20,
                                borderColor: active ? 'primary.main' : 'divider',
                            }}
                        />
                    </Box>
                </Box>

                {/* Action Buttons - visible on hover */}
                <ActionButtonsBox className="action-buttons">
                    <Tooltip title="Edit operations">
                        <IconButton
                            size="small"
                            onClick={(e) => {
                                e.stopPropagation();
                                handleEdit();
                            }}
                            sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                        >
                            <EditIcon sx={{ fontSize: '1rem' }} />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Add service">
                        <IconButton
                            size="small"
                            onClick={(e) => {
                                e.stopPropagation();
                                handleAddService();
                            }}
                            sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                        >
                            <AddIcon sx={{ fontSize: '1rem' }} />
                        </IconButton>
                    </Tooltip>
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
            </StyledSmartNode>

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
