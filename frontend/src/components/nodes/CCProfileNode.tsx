import { Box, Chip, Divider, Typography, styled } from '@mui/material';
import { Warning as WarningIcon } from '@/components/icons';
import { NODE_LAYER_STYLES } from './styles';
import NodeTooltip from './NodeTooltip';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

const StyledCCProfileNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active' && prop !== 'clickable' && prop !== 'missing',
})<{ active: boolean; clickable: boolean; missing: boolean }>(({ active, clickable, missing, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 12,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: missing ? 'warning.main' : (active ? 'info.main' : 'divider'),
    backgroundColor: missing ? 'warning.50' : (active ? 'info.50' : 'background.paper'),
    textAlign: 'center',
    width: 220,
    height: 90,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    opacity: active ? 1 : 0.6,
    cursor: clickable ? 'pointer' : 'default',
    '&:hover': clickable ? {
        borderColor: 'info.main',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    } : {},
}));

interface CCProfileNodeProps {
    /** Selected Claude Code profile ID ("" = default / main scenario). */
    profileId?: string;
    /** Resolved display name of the selected profile (if it still exists). */
    profileName?: string;
    active?: boolean;
    onClick?: () => void;
}

// CCProfileNode shows which Claude Code profile serves @cc for this bot:
// "Default" (main claude_code scenario) or a named profile
// ("claude_code:<id>" scenario). Click to switch.
const CCProfileNode: React.FC<CCProfileNodeProps> = ({
    profileId,
    profileName,
    active = true,
    onClick,
}) => {
    const { t } = useTranslation();
    const clickable = !!onClick;
    const hasProfile = !!profileId;
    // Selected profile no longer exists — surface it instead of hiding it;
    // execution falls back to the default scenario until re-picked.
    const missing = hasProfile && !profileName;

    const handleClick = useCallback((event: React.MouseEvent) => {
        event.stopPropagation();
        if (onClick) onClick();
    }, [onClick]);

    const label = hasProfile
        ? (profileName || profileId)
        : t('remoteAgent.ccProfile.default', { defaultValue: 'Default' });

    const tooltip = missing
        ? t('remoteAgent.ccProfile.missingTooltip', {
            defaultValue: 'Profile "{{id}}" no longer exists — @cc falls back to the default claude_code scenario. Click to pick another.',
            id: profileId,
        })
        : hasProfile
            ? <>{t('remoteAgent.ccProfile.profileTooltip', { defaultValue: 'Claude Code profile' })}: {profileName} ({profileId})<br/>{t('remoteAgent.ccProfile.scenario', { defaultValue: 'Scenario' })}: claude_code:{profileId}</>
            : t('remoteAgent.ccProfile.defaultTooltip', { defaultValue: 'Uses the main claude_code scenario. Click to route @cc through a Claude Code profile.' });

    return (
        <StyledCCProfileNode active={active} clickable={clickable} missing={missing} onClick={handleClick}>
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <NodeTooltip title={tooltip} placement="top">
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5 }}>
                        {active && missing && (
                            <WarningIcon sx={{ fontSize: '1rem', color: 'warning.main' }} />
                        )}
                        <Typography
                            variant="body2"
                            noWrap
                            sx={{
                                color: 'text.primary',
                                ...NODE_LAYER_STYLES.typography,
                                fontStyle: hasProfile ? 'normal' : 'italic',
                                maxWidth: '180px',
                                textAlign: 'center',
                            }}>
                            {label}
                        </Typography>
                    </Box>
                </NodeTooltip>
            </Box>
            <Divider sx={NODE_LAYER_STYLES.divider} />
            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Chip
                    label={t('remoteAgent.ccProfile.chip', { defaultValue: 'Profile' })}
                    size="small"
                    color={missing ? 'warning' : (hasProfile ? 'info' : 'default')}
                    sx={{ height: 24, fontSize: '0.7rem', fontWeight: 500 }}
                />
            </Box>
        </StyledCCProfileNode>
    );
};

export default CCProfileNode;
