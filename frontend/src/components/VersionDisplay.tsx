import { Box, Tooltip, Typography, useTheme } from '@mui/material';
import React, { type ReactNode } from 'react';
import { useVersion } from '@/contexts/VersionContext';
import { FiberManualRecord, Check, Refresh, UpgradeOutlined } from '@/components/icons';

interface VersionDisplayProps {
    /**
     * Optional click handler for opening update panel
     */
    onClick?: () => void;
    /**
     * Whether to show the update indicator badge
     */
    showIndicator?: boolean;
    /**
     * Additional class name for styling
     */
    className?: string;
    /**
     * Custom content to display instead of version text
     */
    children?: ReactNode;
}

/**
 * VersionDisplay Component
 *
 * Displays the current version with an optional update indicator.
 * Interactive by default - clicking opens the update panel dialog.
 *
 * States:
 * - Update available: Orange badge with upgrade icon
 * - Up to date: Green badge with checkmark (when showIndicator=true)
 * - Checking: Blue spinner with loader icon
 * - Error: Red badge
 */
export const VersionDisplay: React.FC<VersionDisplayProps> = ({
    onClick,
    showIndicator = true,
    className,
    children,
}) => {
    const theme = useTheme();
    const { currentVersion, latestVersion, checking, hasUpdate } = useVersion();

    const displayVersion = (currentVersion || 'Unknown').split('+')[0];
    const displayLatestVersion = (latestVersion || 'Unknown').split('+')[0];
    const isInteractive = Boolean(onClick);

    // Use backend's has_update for accurate version comparison
    const hasVersionUpdate = hasUpdate && latestVersion && currentVersion;

    // Determine badge state and styling
    const getBadgeState = () => {
        if (checking) {
            return {
                show: true,
                color: 'info' as const,
                icon: <Refresh sx={{ fontSize: 10, animation: 'spin 1s linear infinite' }} />,
                tooltip: 'Checking for updates...',
            };
        }

        if (hasVersionUpdate) {
            return {
                show: true,
                color: 'warning' as const,
                icon: <UpgradeOutlined sx={{ fontSize: 10 }} />,
                tooltip: 'New version available - click to update',
            };
        }

        if (showIndicator) {
            return {
                show: true,
                color: 'success' as const,
                icon: <Check sx={{ fontSize: 10 }} />,
                tooltip: 'Up to date',
            };
        }

        return { show: false, color: 'default' as const, icon: null, tooltip: '' };
    };

    const badgeState = getBadgeState();

    // Default click handler if none provided
    const handleClick = () => {
        if (isInteractive && onClick) {
            onClick();
        }
    };

    const content = children || (
        <Typography
            variant="caption"
            sx={{
                color: 'text.secondary',
                textAlign: 'center',
                display: 'block',
                fontStyle: 'italic',
                maxWidth: '100%',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                cursor: isInteractive ? 'pointer' : 'default',
                transition: 'color 0.2s ease',
                ...(isInteractive && {
                    '&:hover': {
                        color: 'primary.main',
                    },
                }),
            }}
        >
            version {displayVersion}
        </Typography>
    );

    // If not showing badge, return content wrapped in Box
    if (!badgeState.show || !badgeState.icon) {
        return (
            <Box
                className={className}
                onClick={handleClick}
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    gap: 0.5,
                    cursor: isInteractive ? 'pointer' : 'default',
                    ...(isInteractive && {
                        '&:hover': {
                            opacity: 0.8,
                        },
                    }),
                }}
            >
                {content}
            </Box>
        );
    }

    // With badge indicator - positioned to the right of text
    return (
        <Tooltip title={badgeState.tooltip} placement="top" arrow>
            <Box
                className={className}
                onClick={handleClick}
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    gap: 0.75,
                    cursor: isInteractive ? 'pointer' : 'default',
                    position: 'relative',
                    ...(isInteractive && {
                        '&:hover': {
                            opacity: 0.8,
                            '& .indicator-badge': {
                                transform: 'scale(1.1)',
                            },
                        },
                    }),
                }}
            >
                {content}
                <Box
                    className="indicator-badge"
                    sx={{
                        height: 14,
                        width: 14,
                        borderRadius: 7,
                        backgroundColor: checking
                            ? theme.palette.info.main
                            : hasVersionUpdate
                            ? theme.palette.warning.main
                            : theme.palette.success.main,
                        color: 'white',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        boxShadow: '0 2px 4px rgba(0,0,0,0.2)',
                        flexShrink: 0,
                        transition: 'transform 0.2s ease',
                    }}
                >
                    {badgeState.icon}
                </Box>
            </Box>
        </Tooltip>
    );
};

export default VersionDisplay;
