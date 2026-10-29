import { Box, Divider, Stack, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@/theme/fonts';
import {
    NODE_LAYER_STYLES,
    StyledSmartNodePrimary,
    StyledSmartNodeWrapper,
} from './styles.tsx';

export interface ServiceEntryNodeProps {
    providersCount: number;
    active: boolean;
    /** Rule has conditions above this node, so it only catches the rest. */
    hasConditions?: boolean;
    /** While there are no conditions Default is the whole rule, so the node's
     * body is where the first one is added. */
    onAddCondition?: () => void;
    addConditionDisabled?: boolean;
}

// Same box as a condition node (SmartOpNode): a header line, a divider, a body.
export const ServiceEntryNode: React.FC<ServiceEntryNodeProps> = ({
    active,
    hasConditions,
    onAddCondition,
    addConditionDisabled,
}) => {
    const { t } = useTranslation();
    const canAdd = !hasConditions && !!onAddCondition && active && !addConditionDisabled;
    return (
        <StyledSmartNodeWrapper>
            <StyledSmartNodePrimary active={active} onClick={canAdd ? onAddCondition : undefined}>
                <Stack direction="row" sx={{ alignItems: 'center', width: '100%', flexShrink: 0 }}>
                    <Typography
                        sx={{
                            fontSize: fontSizes.xs,
                            fontWeight: 700,
                            color: 'text.secondary',
                            lineHeight: 1,
                            letterSpacing: '0.04em',
                            textTransform: 'uppercase',
                        }}
                    >
                        {t('rule.ruleConditions.defaultTitle')}
                    </Typography>
                </Stack>

                <Divider sx={{ width: '100%', my: 0.5 }} />

                {hasConditions ? (
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flexGrow: 1, py: 0.5 }}>
                        <Typography
                            sx={{
                                fontSize: fontSizes.xs,
                                color: 'text.disabled',
                                fontStyle: 'italic',
                                textAlign: 'center',
                                lineHeight: 1.3,
                            }}
                        >
                            {t('rule.ruleConditions.defaultOther')}
                        </Typography>
                    </Box>
                ) : (
                    <Box
                        sx={{
                            width: '100%',
                            px: 0.75,
                            py: 0.35,
                            borderRadius: 0.75,
                            backgroundColor: 'action.hover',
                            minHeight: 34,
                            display: 'flex',
                            alignItems: 'center',
                            cursor: canAdd ? 'pointer' : 'default',
                        }}
                    >
                        <Typography
                            sx={{
                                ...NODE_LAYER_STYLES.typography,
                                fontSize: fontSizes.sm,
                                color: 'text.secondary',
                            }}
                        >
                            {t('rule.ruleConditions.addConditionShort')}
                        </Typography>
                    </Box>
                )}
            </StyledSmartNodePrimary>
        </StyledSmartNodeWrapper>
    );
};

export default ServiceEntryNode;
