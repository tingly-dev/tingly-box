import { ContentCopy, Download, FileDownload, MoreVert } from '@mui/icons-material';
import { IconButton, Menu, MenuItem, Tooltip } from '@mui/material';
import React, { useCallback, useState } from 'react';
import type { ExportFormat } from '@/components/rule-card/utils';
import type { Provider } from '../types/provider';

export interface ProviderExportMenuProps {
    provider: Provider;
    onExport: (provider: Provider, format: ExportFormat) => void;
    onCopyJsonl?: (provider: Provider) => void;
    onCopyBase64?: (provider: Provider) => void;
}

export const ProviderExportMenu: React.FC<ProviderExportMenuProps> = ({
    provider,
    onExport,
    onCopyJsonl,
    onCopyBase64,
}) => {
    const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
    const open = Boolean(anchorEl);

    const handleOpen = useCallback((event: React.MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setAnchorEl(event.currentTarget);
    }, []);

    const handleClose = useCallback(() => {
        setAnchorEl(null);
    }, []);

    const handleExportAsJsonl = useCallback(() => {
        handleClose();
        onExport(provider, 'jsonl');
    }, [provider, onExport, handleClose]);

    const handleExportAsBase64File = useCallback(() => {
        handleClose();
        onExport(provider, 'base64');
    }, [provider, onExport, handleClose]);

    const handleCopyJsonl = useCallback(() => {
        handleClose();
        onCopyJsonl?.(provider);
    }, [provider, onCopyJsonl, handleClose]);

    const handleCopyBase64 = useCallback(() => {
        handleClose();
        onCopyBase64?.(provider);
    }, [provider, onCopyBase64, handleClose]);

    return (
        <>
            <Tooltip title="Export provider">
                <IconButton
                    size="small"
                    onClick={handleOpen}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': {
                            backgroundColor: 'action.hover',
                        },
                    }}
                >
                    <FileDownload fontSize="small" />
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
                <MenuItem onClick={handleExportAsJsonl}>
                    <Download fontSize="small" sx={{ mr: 1 }} />
                    Download as JSONL
                </MenuItem>
                <MenuItem onClick={handleExportAsBase64File}>
                    <Download fontSize="small" sx={{ mr: 1 }} />
                    Download as Base64
                </MenuItem>
                {onCopyJsonl && (
                    <MenuItem onClick={handleCopyJsonl}>
                        <ContentCopy fontSize="small" sx={{ mr: 1 }} />
                        Copy JSONL to Clipboard
                    </MenuItem>
                )}
                {onCopyBase64 && (
                    <MenuItem onClick={handleCopyBase64}>
                        <ContentCopy fontSize="small" sx={{ mr: 1 }} />
                        Copy Base64 to Clipboard
                    </MenuItem>
                )}
            </Menu>
        </>
    );
};

export default ProviderExportMenu;
