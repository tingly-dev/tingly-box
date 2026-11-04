import {Box, Divider, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import {NODE_LAYER_STYLES, StyledBotGraphNode} from './styles';
import NodeTag from './NodeTag';
import NodeTooltip from './NodeTooltip';
import {fontMono} from '@/theme/fonts';
import {shortId} from '@tingly/ui';

// ChatNode is a leaf of the notify routing graph: one chat a bot's channel can
// deliver to. Shares StyledBotGraphNode with the rest of the remote/notify
// graph family.
//
// State mapping: active=false (bot off) or blocked (chat disabled) dims the
// node like every other graph node; a blocked chat additionally strikes its
// id through and carries a "disabled" tag, so the reason is spelled out
// rather than drawn as a hatch.

export interface ChatNodeProps {
    /** The id drawn on the node (platform prefix already stripped). */
    chatID: string;
    /** The stored external id, shown in full in the tooltip; defaults to chatID. */
    fullChatID?: string;
    kind?: 'direct_chat' | 'group';
    /** The chat's alias: primary text when present, the id then drops to a muted second line. */
    alias?: string;
    /** `平台 · alias · 群 · id` as plain text, shown first in the tooltip. */
    labelText?: string;
    targetID?: string;
    isPaired?: boolean;
    projectPath?: string;
    updatedAt?: string;
    /** false when the bot is off — the whole branch is unreachable. */
    active?: boolean;
    /** true when the chat itself is blocklisted (disabled flag). */
    blocked?: boolean;
}

const ChatNode: React.FC<ChatNodeProps> = ({chatID, fullChatID, kind = 'direct_chat', alias, labelText, targetID, isPaired, projectPath, updatedAt, active = true, blocked = false}) => {
    const {t} = useTranslation();
    // Long platform ids are shortened here; the tooltip carries the full one.
    const shownId = shortId(kind === 'group' ? 'group' : 'direct', chatID).text;
    return (
        <StyledBotGraphNode active={active && !blocked}>
            {/* Top layer identifies the real platform conversation; the tooltip
                pairs it with the internal UUID used by notify/interact. */}
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <NodeTooltip
                    title={
                        <>
                            {labelText && (<>{labelText}<br/></>)}
                            {kind === 'group' ? 'Group ID' : 'Chat ID'}: {fullChatID ?? chatID}
                            {targetID && (<><br/>{t('notify.chat.internalUuid', {defaultValue: 'Internal target UUID'})}: {targetID}</>)}
                            {projectPath && (<><br/>Project: {projectPath}</>)}
                            {updatedAt && (<><br/>Updated: {new Date(updatedAt).toLocaleString()}</>)}
                        </>
                    }
                    placement="top"
                >
                    <Box sx={{display: 'flex', flexDirection: 'column', alignItems: 'center', maxWidth: 190}}>
                        <Typography
                            variant="body2"
                            noWrap
                            sx={{
                                ...NODE_LAYER_STYLES.typography,
                                fontFamily: alias ? undefined : fontMono,
                                fontWeight: alias ? 600 : undefined,
                                maxWidth: 190,
                                color: blocked ? 'text.disabled' : 'text.primary',
                                textDecoration: blocked ? 'line-through' : 'none',
                            }}
                        >
                            {alias || shownId}
                        </Typography>
                        {alias && (
                            <Typography variant="caption" noWrap sx={{fontFamily: fontMono, maxWidth: 190, color: 'text.secondary'}}>
                                {shownId}
                            </Typography>
                        )}
                    </Box>
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
