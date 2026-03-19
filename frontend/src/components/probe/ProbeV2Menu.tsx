import React, { useState } from 'react';
import {
    Menu,
    MenuItem,
    ListItemIcon,
    ListItemText,
    Divider,
    Typography,
} from '@mui/material';
import {
    PlayArrow as DirectIcon,
    Stream as StreamingIcon,
    Build as ToolIcon,
} from '@mui/icons-material';
import { useTranslation } from 'react-i18next';
import { ProbeV2Dialog } from './ProbeV2Dialog';
import type { ProbeV2TestMode, ProbeV2TargetType } from '@/types/probe-v2.ts';

interface ProbeV2MenuProps {
    anchorEl: HTMLElement | null;
    open: boolean;
    onClose: () => void;
    targetType: ProbeV2TargetType;
    targetId: string;
    targetName: string;
    scenario?: string;
    model?: string;
}

interface ProbeOption {
    mode: ProbeV2TestMode;
    label: string;
    icon: React.ReactNode;
    description: string;
}

const PROBE_OPTIONS: ProbeOption[] = [
    {
        mode: 'simple',
        label: 'Direct Test',
        icon: <DirectIcon fontSize="small" />,
        description: 'Send a simple non-streaming request',
    },
    {
        mode: 'streaming',
        label: 'Streaming Test',
        icon: <StreamingIcon fontSize="small" />,
        description: 'Stream the response in real-time',
    },
    {
        mode: 'tool',
        label: 'Tool Calling',
        icon: <ToolIcon fontSize="small" />,
        description: 'Test with tool calling enabled',
    },
];

export const ProbeV2Menu: React.FC<ProbeV2MenuProps> = ({
    anchorEl,
    open,
    onClose,
    targetType,
    targetId,
    targetName,
    scenario,
    model,
}) => {
    const { t } = useTranslation();
    const [dialogOpen, setDialogOpen] = useState(false);
    const [selectedMode, setSelectedMode] = useState<ProbeV2TestMode>('simple');

    const handleProbeClick = (mode: ProbeV2TestMode) => {
        setSelectedMode(mode);
        setDialogOpen(true);
        onClose();
    };

    const handleDialogClose = () => {
        setDialogOpen(false);
    };

    const getTargetTypeLabel = () => {
        switch (targetType) {
            case 'provider':
                return 'Provider';
            case 'rule':
                return 'Rule';
            default:
                return 'Target';
        }
    };

    return (
        <>
            <Menu
                anchorEl={anchorEl}
                open={open}
                onClose={onClose}
                transformOrigin={{ horizontal: 'right', vertical: 'top' }}
                anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
                PaperProps={{
                    sx: { minWidth: 250 }
                }}
            >
                <MenuItem disabled sx={{ opacity: 1 }}>
                    <Typography variant="subtitle2" color="text.secondary">
                        Test {getTargetTypeLabel()}
                    </Typography>
                </MenuItem>
                <Divider />
                {PROBE_OPTIONS.map((option) => (
                    <MenuItem
                        key={option.mode}
                        onClick={() => handleProbeClick(option.mode)}
                    >
                        <ListItemIcon>
                            {option.icon}
                        </ListItemIcon>
                        <ListItemText
                            primary={option.label}
                            secondary={option.description}
                            secondaryTypographyProps={{
                                variant: 'caption',
                                sx: { fontSize: '0.75rem' }
                            }}
                        />
                    </MenuItem>
                ))}
            </Menu>

            <ProbeV2Dialog
                open={dialogOpen}
                onClose={handleDialogClose}
                targetType={targetType}
                targetId={targetId}
                targetName={targetName}
                scenario={scenario}
                model={model}
                testMode={selectedMode}
            />
        </>
    );
};

export default ProbeV2Menu;
