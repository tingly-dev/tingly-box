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
                position: 'relative',
                width: 24,
                height: PROVIDER_NODE_STYLES.height,
                flexShrink: 0,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                opacity: active ? 1 : 0.5,
            }}
        >
            {/* Full-height line */}
            <Box sx={(theme) => ({
                position: 'absolute',
                top: 0,
                bottom: 0,
                left: '50%',
                width: '1px',
                backgroundColor: alpha(
                    getRouteGraphActiveColor(theme),
                    theme.palette.mode === 'dark' ? 0.28 : 0.20,
                ),
                borderRadius: '1px',
            })} />

            {/* Help icon sitting on the line */}
            <Tooltip title={t('rule.priority.dividerHelp')} placement="top" arrow>
                <Box sx={{ lineHeight: 0, cursor: 'help', zIndex: 1, bgcolor: 'background.paper', borderRadius: '50%' }}>
                    <HelpOutline
                        sx={(theme) => ({
                            fontSize: '0.85rem',
                            color: alpha(
                                getRouteGraphActiveColor(theme),
                                theme.palette.mode === 'dark' ? 0.55 : 0.45,
                            ),
                            display: 'block',
                            transition: 'color 0.15s',
                            '&:hover': { color: getRouteGraphActiveColor(theme) },
                        })}
                    />
                </Box>
            </Tooltip>
        </Box>
    );
};

export default DividerNode;
