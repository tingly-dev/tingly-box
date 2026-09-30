import { Box } from '@mui/material';
import type { BotSettings } from '@/types/bot.ts';
import { ccProfileIdFromDefaultAgent } from '@/types/bot.ts';
import type { Provider } from '@/types/provider.ts';
import type { ProfileInfo } from '@/contexts/ProfileContext';
import { AccessNode, ArrowNode, ImBotNode, NodeContainer, graphRowStyles } from '../nodes';
import BotModelNode from '../nodes/BotModelNode.tsx';
import AgentNode from '../nodes/AgentNode.tsx';
import AtNode from '../nodes/AtNode.tsx';
import CCProfileNode from '../nodes/CCProfileNode.tsx';

export interface RemoteControlGraphProps {
    imbot: BotSettings;
    providers: Provider[];
    isBotEnabled: boolean;
    readOnly?: boolean;
    onModelClick?: () => void;
    onBotClick?: () => void;
    onAccessClick?: () => void;
    directChatCount?: number;
    groupCount?: number;
    accessLoading?: boolean;
    accessError?: string;
    /** Configured Claude Code profiles (to resolve the selected profile name). */
    ccProfiles?: ProfileInfo[];
    /** Opens the Claude Code profile picker for this bot's @cc branch. */
    onCCProfileClick?: () => void;
}

const getProviderName = (providerUuid: string | undefined, providersData: Provider[]): string => {
    if (!providerUuid) return '';
    const provider = providersData.find(p => p.uuid === providerUuid);
    return provider?.name || '';
};

const RemoteControlGraph: React.FC<RemoteControlGraphProps> = ({
    imbot,
    providers,
    isBotEnabled,
    readOnly = false,
    onModelClick,
    onBotClick,
    ccProfiles,
    onCCProfileClick,
    onAccessClick,
    directChatCount = 0,
    groupCount = 0,
    accessLoading = false,
    accessError,
}) => {
    const providerName = getProviderName(imbot.smartguide_provider, providers);

    // Which Claude Code configuration serves @cc: '' = main claude_code
    // scenario, otherwise a profile ID from default_agent ("claude_code:<id>").
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
    const tbNode = <AtNode type="tb"/>;
    const ccNode = <AtNode type="cc"/>;
    const smartGuideNode = <AgentNode agentType="smart-guide" active={isBotEnabled}/>;
    const claudeCodeNode = <AgentNode agentType="claude-code" active={isBotEnabled}/>;
    const modelNode = <BotModelNode provider={imbot.smartguide_provider} providerName={providerName} model={imbot.smartguide_model} active={isBotEnabled} onClick={readOnly ? undefined : onModelClick}/>;
    const profileNode = <CCProfileNode profileId={ccProfileId} profileName={ccProfileName} active={isBotEnabled} onClick={readOnly ? undefined : onCCProfileClick}/>;
    const smartGuideBranch = (<>
        <NodeContainer>{tbNode}</NodeContainer>
        <ArrowNode direction="forward" />
        <NodeContainer>{smartGuideNode}</NodeContainer>
        <ArrowNode direction="forward" />
        <NodeContainer>{modelNode}</NodeContainer>
    </>);
    const claudeCodeBranch = (<>
        <NodeContainer>{ccNode}</NodeContainer>
        <ArrowNode direction="forward" />
        <NodeContainer>{claudeCodeNode}</NodeContainer>
        <ArrowNode direction="forward" />
        <NodeContainer>{profileNode}</NodeContainer>
    </>);

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

            {/* Fork: @tb and @cc branches */}
            <Box
                sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 2,
                    borderLeft: '2px solid',
                    borderColor: 'divider',
                    pl: 2,
                }}
            >
                {/* @tb: SmartGuide agent → model */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>{smartGuideBranch}</Box>

                {/* @cc: Claude Code agent → profile (default or a claude_code profile) */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>{claudeCodeBranch}</Box>
            </Box>
        </Box>
    );
};

export default RemoteControlGraph;
