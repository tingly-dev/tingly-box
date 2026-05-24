import { Laptop as LaptopIcon } from '@/components/icons';
import { Box, Tooltip, Typography } from '@mui/material';
import IconButton from '@mui/material/IconButton';
import React from 'react';
import DockerOriginal from 'devicons-react/icons/DockerOriginal';
import { ActiveBadge } from './ActiveBadge';

// ============================================================================
// Types
// ============================================================================

export type EnvironmentMode = 'local' | 'docker' | 'cli' | 'npx' | 'wsl';

export interface EnvironmentModeOption {
    value: EnvironmentMode;
    label: string;
    tooltip: string;
    icon: React.ReactElement;
}

// ============================================================================
// Default Mode Options
// ============================================================================

const DEFAULT_MODES: EnvironmentModeOption[] = [
    {
        value: 'local',
        label: 'Local',
        tooltip: 'Local mode - use localhost or 127.0.0.1',
        icon: <LaptopIcon fontSize="small" />,
    },
    {
        value: 'docker',
        label: 'Docker',
        tooltip: 'Docker mode - use host.docker.internal for container access',
        icon: <DockerOriginal size={20} color="blue" />,
    },
];

// ============================================================================
// Props
// ============================================================================

interface EnvironmentModeSwitcherProps {
    /** Current active mode */
    value: EnvironmentMode;
    /** Callback when mode changes */
    onChange: (mode: EnvironmentMode) => void;
    /** Available mode options (defaults to local + docker) */
    modes?: EnvironmentModeOption[];
}

// ============================================================================
// Component
// ============================================================================

/**
 * Environment mode switcher for URL transformation.
 *
 * Displays all available mode icons side by side:
 * - [💻] [🐳]  <- Click to switch
 *
 * Active mode is highlighted with a green checkmark badge.
 */
export const EnvironmentModeSwitcher: React.FC<EnvironmentModeSwitcherProps> = ({
    value,
    onChange,
    modes = DEFAULT_MODES,
}) => {
    return (
        <Box sx={{ display: 'flex', gap: 0.25 }}>
            {modes.map((mode) => {
                const isActive = value === mode.value;

                return (
                    <Tooltip key={mode.value} title={mode.tooltip} arrow>
                        <IconButton
                            onClick={() => onChange(mode.value)}
                            size="small"
                            sx={{
                                position: 'relative',
                                opacity: isActive ? 1 : 0.5,
                                transition: 'opacity 0.2s',
                                '&:hover': {
                                    opacity: 1,
                                    backgroundColor: 'action.hover',
                                },
                            }}
                        >
                            {mode.icon}
                            {isActive && <ActiveBadge />}
                        </IconButton>
                    </Tooltip>
                );
            })}
        </Box>
    );
};

export default EnvironmentModeSwitcher;
