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
                    '& .MuiToggleButton-root': {
                        padding: '6px 10px',
                        fontSize: '0.7rem',
                        fontWeight: 600,
                        textTransform: 'none',
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 1,
                        minWidth: 60,
                    },
                    '& .MuiToggleButton-root:first-of-type': {
                        borderBottomLeftRadius: 0,
                        borderBottomRightRadius: 0,
                        marginBottom: '-1px',
                    },
                    '& .MuiToggleButton-root:last-of-type': {
                        borderTopLeftRadius: 0,
                        borderTopRightRadius: 0,
                    },
                    '& .MuiToggleButton-root.Mui-selected': {
                        backgroundColor: 'secondary.main',
                        color: 'white',
                        borderColor: 'secondary.main',
                        '&:hover': {
                            backgroundColor: 'secondary.dark',
                        },
                    },
                    '& .MuiToggleButton-root:not(.Mui-selected)': {
                        color: 'text.secondary',
                        backgroundColor: 'background.paper',
                        borderColor: 'divider',
                        '&:hover': {
                            backgroundColor: 'action.hover',
                        },
                    },
                    '& .MuiToggleButton-root:disabled': {
                        borderColor: 'action.disabled',
                        color: 'text.disabled',
                        backgroundColor: 'action.disabledBackground',
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
