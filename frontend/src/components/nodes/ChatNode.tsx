import {Box, Chip, Divider, Typography, styled} from '@mui/material';
import {useTranslation} from 'react-i18next';
import {NODE_LAYER_STYLES, StyledBotGraphNode, getInactiveHatchSx} from './styles';
import NodeTooltip from './NodeTooltip';
import {fontMono} from '@/theme/fonts';

// ChatNode is a leaf of the notify routing graph: one chat a bot's channel can
// deliver to. Shares StyledBotGraphNode with the rest of the remote/notify
// graph family.
//
// State mapping follows the product's conventions: active=false (bot off)
// dims the node like every other graph node; blocked (chat disabled) draws
// the same diagonal-hatch overlay the bot cards use for an off purpose —
// "hatched = deliberately not running", distinct from "dimmed = upstream is
// off".
const StyledChatNode = styled(StyledBotGraphNode, {
    shouldForwardProp: (prop) => prop !== 'blocked',
})<{ blocked?: boolean }>(({blocked = false, theme}) => ({
    ...(blocked && getInactiveHatchSx(theme)),
}));

export interface ChatNodeProps {
    chatID: string;
    kind?: 'direct_chat' | 'group';
    name?: string;
    targetID?: string;
    isPaired?: boolean;
    projectPath?: string;
    updatedAt?: string;
    /** false when the bot is off — the whole branch is unreachable. */
    active?: boolean;
    /** true when the chat itself is blocklisted (disabled flag). */
    blocked?: boolean;
}

const ChatNode: React.FC<ChatNodeProps> = ({chatID, kind = 'direct_chat', name, targetID, isPaired, projectPath, updatedAt, active = true, blocked = false}) => {
    const {t} = useTranslation();
    return (
        <StyledChatNode active={active} blocked={blocked}>
            {/* Top layer identifies the real platform conversation; the tooltip
                pairs it with the internal UUID used by notify/interact. */}
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <NodeTooltip
                    title={
                        <>
                            {kind === 'group' ? 'Group ID' : 'Chat ID'}: {chatID}
                            {targetID && (<><br/>Target UUID: {targetID}</>)}
                            {projectPath && (<><br/>Project: {projectPath}</>)}
                            {updatedAt && (<><br/>Updated: {new Date(updatedAt).toLocaleString()}</>)}
                        </>
                    }
                    placement="top"
                >
                    <Typography
                        variant="body2"
                        noWrap
                        sx={{
                            ...NODE_LAYER_STYLES.typography,
                            fontFamily: fontMono,
                            maxWidth: 190,
                            color: blocked ? 'text.disabled' : 'text.primary',
                            textDecoration: blocked ? 'line-through' : 'none',
                        }}
                    >
                        {name || chatID}
                    </Typography>
                </NodeTooltip>
            </Box>
            <Divider sx={NODE_LAYER_STYLES.divider}/>
            {/* Bottom layer — status chips. */}
            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Chip
                    label={kind === 'group'
                        ? t('notify.target.group', {defaultValue: 'Group'})
                        : t('notify.target.direct', {defaultValue: 'Direct'})}
                    size="small"
                    color={active && !blocked ? 'info' : 'default'}
                    sx={{height: 24, fontSize: '0.7rem', fontWeight: 500}}
                />
                {blocked ? (
                    <Chip label={t('notify.group.disabledChat', {defaultValue: 'disabled'})} size="small" variant="outlined" sx={{height: 24, fontSize: '0.7rem'}}/>
                ) : isPaired ? (
                    <Chip label={t('notify.group.paired', {defaultValue: 'paired'})} size="small" color="success" variant="outlined" sx={{height: 24, fontSize: '0.7rem'}}/>
                ) : null}
            </Box>
        </StyledChatNode>
    );
};

export default ChatNode;
