import { Box, Tooltip } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { HelpOutline } from '@/components/icons';
import { getRouteGraphActiveColor, PROVIDER_NODE_STYLES } from './styles.tsx';

export interface DividerNodeProps {
    active?: boolean;
}

/**
 * A vertical divider placed between priority-tier groups in the service list.
 * Shows a help icon with a tooltip explaining the priority/fallback logic.
 */
export const DividerNode: React.FC<DividerNodeProps> = ({ active = true }) => {
    const { t } = useTranslation();

    return (
        <Box
            sx={{
                width: 24,
                height: PROVIDER_NODE_STYLES.height,
                flexShrink: 0,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 0.5,
                opacity: active ? 1 : 0.5,
            }}
        >
            {/* Top segment of the line */}
            <Box sx={(theme) => ({
                width: '1px',
                flex: 1,
                backgroundColor: alpha(
                    getRouteGraphActiveColor(theme),
                    theme.palette.mode === 'dark' ? 0.28 : 0.20,
                ),
                borderRadius: '1px',
            })} />

            {/* Help icon centered on the line */}
            <Tooltip
                title={t('rule.priority.dividerHelp')}
                placement="top"
                arrow
            >
                <Box sx={{ lineHeight: 0, cursor: 'help' }}>
                    <HelpOutline
                        sx={(theme) => ({
                            fontSize: '0.85rem',
                            color: alpha(
                                getRouteGraphActiveColor(theme),
                                theme.palette.mode === 'dark' ? 0.55 : 0.45,
                            ),
                            '&:hover': {
                                color: getRouteGraphActiveColor(theme),
                            },
                            transition: 'color 0.15s',
                        })}
                    />
                </Box>
            </Tooltip>

            {/* Bottom segment of the line */}
            <Box sx={(theme) => ({
                width: '1px',
                flex: 1,
                backgroundColor: alpha(
                    getRouteGraphActiveColor(theme),
                    theme.palette.mode === 'dark' ? 0.28 : 0.20,
                ),
                borderRadius: '1px',
            })} />
        </Box>
    );
};

export default DividerNode;
