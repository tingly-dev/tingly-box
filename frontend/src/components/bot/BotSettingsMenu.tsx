import { Block as InactiveIcon, CheckCircle as ActiveIcon, Delete as DeleteIcon, Edit as EditIcon, MoreHoriz as MoreHorizIcon } from '@mui/icons-material';
import { IconButton, ListItemIcon, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';
import { MouseEvent, useState, useCallback } from 'react';

interface BotSettingsMenuProps {
    onEdit: () => void;
    onDelete: () => void;
    onToggleEnabled: () => void;
    enabled: boolean;
    disabled?: boolean;
    isToggling?: boolean;
}

const BotSettingsMenu: React.FC<BotSettingsMenuProps> = ({
    onEdit,
    onDelete,
    onToggleEnabled,
    enabled,
    disabled = false,
    isToggling = false,
}) => {
    const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
    const open = Boolean(anchorEl);

    const handleOpen = useCallback((event: MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setAnchorEl(event.currentTarget);
    }, []);

    const handleClose = useCallback((event?: MouseEvent<HTMLElement>) => {
        event?.stopPropagation();
        setAnchorEl(null);
    }, []);

    const handleEdit = useCallback((event: MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        handleClose();
        onEdit();
    }, [handleClose, onEdit]);

    const handleDelete = useCallback((event: MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        handleClose();
        onDelete();
    }, [handleClose, onDelete]);

    const handleToggle = useCallback((event: MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        handleClose();
        onToggleEnabled();
    }, [handleClose, onToggleEnabled]);

    return (
        <>
            <Tooltip title="Bot actions">
                <IconButton
                    size="small"
                    onClick={handleOpen}
                    disabled={disabled || isToggling}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': {
                            backgroundColor: 'action.hover',
                        },
                    }}
                >
                    <MoreHorizIcon fontSize="small" />
                </IconButton>
            </Tooltip>
            <Menu
                anchorEl={anchorEl}
                open={open}
                onClose={handleClose}
                onClick={(e) => e.stopPropagation()}
                anchorOrigin={{
                    vertical: 'bottom',
                    horizontal: 'right',
                }}
                transformOrigin={{
                    vertical: 'top',
                    horizontal: 'right',
                }}
            >
                <MenuItem onClick={handleEdit} disabled={isToggling}>
                    <ListItemIcon>
                        <EditIcon fontSize="small" />
                    </ListItemIcon>
                    <ListItemText>Edit Configuration</ListItemText>
                </MenuItem>

                <MenuItem
                    onClick={handleToggle}
                    disabled={isToggling}
                    sx={{
                        color: enabled ? 'warning.main' : 'success.main',
                    }}
                >
                    <ListItemIcon>
                        {enabled ? <InactiveIcon fontSize="small" /> : <ActiveIcon fontSize="small" />}
                    </ListItemIcon>
                    <ListItemText>{enabled ? 'Disable Bot' : 'Enable Bot'}</ListItemText>
                </MenuItem>

                <MenuItem onClick={handleDelete} disabled={isToggling} sx={{ color: 'error.main' }}>
                    <ListItemIcon>
                        <DeleteIcon fontSize="small" color="error" />
                    </ListItemIcon>
                    <ListItemText>Delete Bot</ListItemText>
                </MenuItem>
            </Menu>
        </>
    );
};

export default BotSettingsMenu;
