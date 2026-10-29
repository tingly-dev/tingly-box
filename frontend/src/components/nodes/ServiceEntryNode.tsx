import { Box, ButtonBase, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import {
    getRouteGraphActiveColor,
    NODE_LAYER_STYLES,
    SMART_NODE_STYLES,
    StyledSmartNodeWrapper,
} from './styles.tsx';

export interface ServiceEntryNodeProps {
    providersCount: number;
    active: boolean;
    /** Rule has conditions above this node, so it only catches the rest. */
    hasConditions?: boolean;
    /** Shown in the title bar while there are no conditions: Default is then
     * the whole rule, so adding the first condition lives on the node itself. */
    onAddCondition?: () => void;
    addConditionDisabled?: boolean;
}

export const ServiceEntryNode: React.FC<ServiceEntryNodeProps> = ({
    active,
    hasConditions,
    onAddCondition,
    addConditionDisabled,
}) => {
    const { t } = useTranslation();
    return (
        <StyledSmartNodeWrapper>
            <Box
                sx={(theme) => ({
                    width: SMART_NODE_STYLES.width,
                    boxSizing: 'border-box',
                    px: 1.25,
                    py: 0.75,
                    borderRadius: `${theme.shape.borderRadius}px`,
                    border: '1px solid',
                    borderColor: alpha(
                        getRouteGraphActiveColor(theme),
                        theme.palette.mode === 'dark' ? 0.45 : 0.38,
                    ),
                    opacity: active ? 1 : 0.6,
                    transition: 'border-color 0.16s ease, opacity 0.16s ease',
                })}
            >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography sx={{ ...NODE_LAYER_STYLES.typography, color: 'text.primary', fontWeight: 600 }}>
                        {t('rule.ruleConditions.defaultTitle')}
                    </Typography>
                    {!hasConditions && onAddCondition && (
                        <ButtonBase
                            onClick={onAddCondition}
                            disabled={!active || addConditionDisabled}
                            sx={(theme) => ({
                                ml: 'auto',
                                px: 0.75,
                                borderRadius: 0.75,
                                fontSize: 12,
                                color: getRouteGraphActiveColor(theme),
                                '&:hover': { backgroundColor: alpha(getRouteGraphActiveColor(theme), 0.1) },
                                '&.Mui-disabled': { color: theme.palette.text.disabled },
                            })}
                        >
                            {t('rule.ruleConditions.addConditionShort')}
                        </ButtonBase>
                    )}
                </Box>
                <Typography sx={{ fontSize: 11.5, color: 'text.secondary', lineHeight: 1.4 }}>
                    {hasConditions ? t('rule.ruleConditions.defaultOther') : t('rule.ruleConditions.defaultAll')}
                </Typography>
            </Box>
        </StyledSmartNodeWrapper>
    );
};

export default ServiceEntryNode;
