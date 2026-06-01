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

            {/* Help icon sitting on the line, centered */}
            <Tooltip title={t('rule.tier.dividerHelp')} placement="top" arrow>
                <Box sx={{
                    position: 'absolute',
                    top: '50%',
                    left: '50%',
                    transform: 'translate(-50%, -50%)',
                    lineHeight: 0,
                    cursor: 'help',
                    zIndex: 1,
                    bgcolor: 'background.paper',
                    borderRadius: '50%',
                    p: '1px',
                }}>
                    <HelpOutline
                        sx={(theme) => ({
                            fontSize: '1rem',
                            color: alpha(
                                getRouteGraphActiveColor(theme),
                                theme.palette.mode === 'dark' ? 0.60 : 0.50,
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
