import { ToggleButton, ToggleButtonGroup, Tooltip } from '@mui/material';
import React from 'react';

interface RoutingModeSwitchProps {
    smartEnabled: boolean;
    active: boolean;
    disabled?: boolean;
    onSwitch: () => void;
}

export const RoutingModeSwitch: React.FC<RoutingModeSwitchProps> = ({
    smartEnabled,
    active,
    disabled = false,
    onSwitch,
}) => {
    const handleModeChange = (
        _event: React.MouseEvent<HTMLElement>,
        newMode: string | null,
    ) => {
        if (newMode !== null && active && !disabled) {
            onSwitch();
        }
    };

    return (
        <Tooltip
            title={smartEnabled ? "Switch to Direct Routing" : "Switch to Smart Routing"}
            arrow
        >
            <ToggleButtonGroup
                value={smartEnabled ? 'smart' : 'direct'}
                exclusive
                onChange={handleModeChange}
                size="small"
                sx={{
                    '& .MuiToggleButton-root': {
                        padding: '4px 8px',
                        fontSize: '0.75rem',
                        fontWeight: 600,
                        textTransform: 'none',
                        border: '1px solid',
                        borderColor: 'divider',
                    },
                    '& .MuiToggleButton-root.Mui-selected': {
                        color: 'white',
                    },
                    '& .MuiToggleButton-root:first-of-type.Mui-selected': {
                        backgroundColor: 'primary.main',
                        '&:hover': {
                            backgroundColor: 'primary.dark',
                        },
                    },
                    '& .MuiToggleButton-root:last-of-type.Mui-selected': {
                        backgroundColor: 'secondary.main',
                        '&:hover': {
                            backgroundColor: 'secondary.dark',
                        },
                    },
                    '& .MuiToggleButton-root:not(.Mui-selected)': {
                        color: 'text.secondary',
                        backgroundColor: 'action.hover',
                    },
                }}
            >
                <ToggleButton value="direct" disabled={!active || disabled}>
                    Direct
                </ToggleButton>
                <ToggleButton value="smart" disabled={!active || disabled}>
                    Smart
                </ToggleButton>
            </ToggleButtonGroup>
        </Tooltip>
    );
};

export default RoutingModeSwitch;
