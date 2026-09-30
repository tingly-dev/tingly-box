import { Box, Divider, Typography } from '@mui/material';
import { Warning as WarningIcon } from '@/components/icons';
import { NODE_LAYER_STYLES, StyledBotGraphNode } from './styles';
import NodeTag from './NodeTag';
import NodeTooltip from './NodeTooltip';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

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
        <StyledBotGraphNode active={active} clickable={clickable} warn={missing} onClick={handleClick}>
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
                <NodeTag label={t('remoteAgent.ccProfile.chip', { defaultValue: 'Profile' })} tone={missing ? 'warning' : 'default'}/>
            </Box>
        </StyledBotGraphNode>
    );
};

export default CCProfileNode;
