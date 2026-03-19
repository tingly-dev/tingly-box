import React from 'react';
import {
    Menu,
    MenuItem,
    ListItemIcon,
    ListItemText,
} from '@mui/material';
import {
    Delete as DeleteIcon,
} from '@mui/icons-material';

interface ProviderNodeContentProps {
    menuAnchorEl: HTMLElement | null;
    menuOpen: boolean;
    onMenuClose: () => void;
    onDelete: () => void;
}

const ProviderNodeContent: React.FC<ProviderNodeContentProps> = ({
    menuAnchorEl,
    menuOpen,
    onMenuClose,
    onDelete,
}) => {
    return (
        <Menu
            anchorEl={menuAnchorEl}
            open={menuOpen}
            onClose={onMenuClose}
            onClick={(e) => e.stopPropagation()}
            transformOrigin={{ horizontal: 'right', vertical: 'top' }}
            anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
        >
            <MenuItem onClick={onDelete}>
                <ListItemIcon>
                    <DeleteIcon />
                </ListItemIcon>
                <ListItemText>Delete Provider</ListItemText>
            </MenuItem>
            <MenuItem onClick={onMenuClose} sx={{ color: 'text.secondary' }}>
                <ListItemText>Cancel</ListItemText>
            </MenuItem>
        </Menu>
    );
};

export default ProviderNodeContent;
