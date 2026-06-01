import React from 'react';
import {
    Menu,
    MenuItem,
    ListItemIcon,
    ListItemText,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import {
    Delete as DeleteIcon,
} from '@/components/icons';

interface ServiceNodeContentProps {
    menuAnchorEl: HTMLElement | null;
    menuOpen: boolean;
    onMenuClose: () => void;
    onDelete: () => void;
}

const ServiceNodeContent: React.FC<ServiceNodeContentProps> = ({
    menuAnchorEl,
    menuOpen,
    onMenuClose,
    onDelete,
}) => {
    const { t } = useTranslation();

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
                <ListItemText>{t('rule.service.deleteService')}</ListItemText>
            </MenuItem>
            <MenuItem onClick={onMenuClose} sx={{ color: 'text.secondary' }}>
                <ListItemText>{t('common.cancel')}</ListItemText>
            </MenuItem>
        </Menu>
    );
};

export default ServiceNodeContent;
