import {Box, Divider, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import {NODE_LAYER_STYLES, StyledBotGraphNode} from './styles';
import NodeTag from './NodeTag';
import NodeTooltip from './NodeTooltip';
import {fontMono} from '@/theme/fonts';

// ChatNode is a leaf of the notify routing graph: one chat a bot's channel can
// deliver to. Shares StyledBotGraphNode with the rest of the remote/notify
// graph family.
//
// State mapping: active=false (bot off) or blocked (chat disabled) dims the
// node like every other graph node; a blocked chat additionally strikes its
// id through and carries a "disabled" tag, so the reason is spelled out
// rather than drawn as a hatch.

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
        <StyledBotGraphNode active={active && !blocked}>
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
                <NodeTag
                    label={kind === 'group'
                        ? t('notify.target.group', {defaultValue: 'Group'})
                        : t('notify.target.direct', {defaultValue: 'Direct'})}
                    active={active && !blocked}
                />
                {blocked ? (
                    <NodeTag outlined label={t('notify.group.disabledChat', {defaultValue: 'disabled'})} active={false}/>
                ) : isPaired ? (
                    <NodeTag outlined label={t('notify.group.paired', {defaultValue: 'paired'})} active={active}/>
                ) : null}
            </Box>
        </StyledBotGraphNode>
    );
};

export default ChatNode;
