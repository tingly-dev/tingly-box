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
                orientation="vertical"
                sx={{
                    height: 90, // Match ModelNode height
                    '& .MuiToggleButton-root': {
                        height: 45, // Half of total height
                        padding: '4px 10px',
                        fontSize: '0.7rem',
                        fontWeight: 600,
                        textTransform: 'none',
                        border: '1px solid',
                        borderColor: 'text.primary',
                        minWidth: 60,
                    },
                    '& .MuiToggleButton-root.Mui-selected': {
                        backgroundColor: 'secondary.main',
                        color: 'white',
                        borderColor: 'text.primary',
                        '&:hover': {
                            backgroundColor: 'secondary.dark',
                        },
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
