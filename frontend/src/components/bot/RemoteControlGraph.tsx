import { Box } from '@mui/material';
import type { BotSettings } from '@/types/bot.ts';
import { ccProfileIdFromDefaultAgent } from '@/types/bot.ts';
import type { ProfileInfo } from '@/contexts/ProfileContext';
import { AccessNode, ArrowNode, ImBotNode, NodeContainer, graphRowStyles } from '../nodes';
import AgentNode from '../nodes/AgentNode.tsx';
import CCProfileNode from '../nodes/CCProfileNode.tsx';

export interface RemoteControlGraphProps {
    imbot: BotSettings;
    isBotEnabled: boolean;
    readOnly?: boolean;
    onBotClick?: () => void;
    onAccessClick?: () => void;
    directChatCount?: number;
    groupCount?: number;
    accessLoading?: boolean;
    accessError?: string;
    /** Configured Claude Code profiles (to resolve the selected profile name). */
    ccProfiles?: ProfileInfo[];
    /** Opens the Claude Code profile picker for this bot. */
    onCCProfileClick?: () => void;
}

const RemoteControlGraph: React.FC<RemoteControlGraphProps> = ({
    imbot,
    isBotEnabled,
    readOnly = false,
    onBotClick,
    ccProfiles,
    onCCProfileClick,
    onAccessClick,
    directChatCount = 0,
    groupCount = 0,
    accessLoading = false,
    accessError,
}) => {
    // Which Claude Code configuration the bot's chats run under: '' = main
    // claude_code scenario, otherwise a profile ID from default_agent
    // ("claude_code:<id>").
    const ccProfileId = ccProfileIdFromDefaultAgent(imbot.default_agent);
    const ccProfileName = ccProfiles?.find(p => p.id === ccProfileId)?.name;

    const accessNode = (
        <AccessNode
            directChats={directChatCount}
            groups={groupCount}
            active={isBotEnabled}
            loading={accessLoading}
            error={accessError}
            onClick={readOnly ? undefined : onAccessClick}
        />
    );
    // The card header already names the bot, so this node only says which
    // platform the message arrives through.
    const botNode = <ImBotNode imbot={imbot} variant="platform" active={isBotEnabled} onClick={readOnly ? undefined : onBotClick}/>;
    const claudeCodeNode = <AgentNode agentType="claude-code" active={isBotEnabled}/>;
    const profileNode = <CCProfileNode profileId={ccProfileId} profileName={ccProfileName} active={isBotEnabled} onClick={readOnly ? undefined : onCCProfileClick}/>;

    // Always the full route, left to right. When the card is narrower than
    // the route, THIS graph scrolls sideways on its own; it never folds into
    // a vertical stack, because the left-to-right flow is the thing being
    // shown. The padding keeps node borders and hover rings clear of the
    // scroll box's edge. 8px between items (graphRowStyles' default is 12px)
    // keeps the full row inside the card at common desktop widths.
    return (
        <Box sx={(t) => ({...graphRowStyles(t), gap: t.spacing(1), py: 1, px: 0.5})}>
            {/* Access is both the summary and the entry point for concrete
                authorized resources, so authorization has one work surface. */}
            <NodeContainer>
                {accessNode}
            </NodeContainer>

            <ArrowNode direction="forward" />

            <NodeContainer>
                {botNode}
            </NodeContainer>

            <ArrowNode direction="forward" />

            {/* Every chat talks to Claude Code, in its folder, under the
                bot's profile. */}
            <NodeContainer>
                {claudeCodeNode}
            </NodeContainer>

            <ArrowNode direction="forward" />

            <NodeContainer>
                {profileNode}
            </NodeContainer>
        </Box>
    );
};

export default RemoteControlGraph;
