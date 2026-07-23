import { Box } from '@mui/material';
import type { BotSettings } from '@/types/bot.ts';
import { ccProfileIdFromDefaultAgent } from '@/types/bot.ts';
import type { Provider } from '@/types/provider.ts';
import type { ProfileInfo } from '@/contexts/ProfileContext';
import { ArrowNode, NodeContainer } from '../nodes';
import ImBotNode from '../nodes/ImBotNode.tsx';
import BotModelNode from '../nodes/BotModelNode.tsx';
import AgentNode from '../nodes/AgentNode.tsx';
import AtNode from '../nodes/AtNode.tsx';
import CCProfileNode from '../nodes/CCProfileNode.tsx';
import { useNavigate } from 'react-router-dom';
import { useCallback } from 'react';

const graphRowStyles = (theme: any) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-start',
    gap: theme.spacing(1.5),
    flexWrap: 'nowrap',
    overflowX: 'auto',
    overflowY: 'visible',
    paddingBottom: theme.spacing(0.5),
    // Custom scrollbar styling
    scrollbarWidth: 'thin',
    scrollbarColor: (theme.palette.mode === 'dark' ? '#555' : '#ccc') + ' transparent',
    '&::-webkit-scrollbar': {
        height: 6,
    },
    '&::-webkit-scrollbar-track': {
        background: 'transparent',
    },
    '&::-webkit-scrollbar-thumb': {
        backgroundColor: theme.palette.mode === 'dark' ? '#555' : '#ccc',
        borderRadius: 3,
        '&:hover': {
            backgroundColor: theme.palette.mode === 'dark' ? '#777' : '#999',
        },
    },
});

export interface RemoteControlGraphProps {
    imbot: BotSettings;
    providers: Provider[];
    isBotEnabled: boolean;
    readOnly?: boolean;
    onModelClick?: () => void;
    onBotClick?: () => void;
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
    ccProfiles = [],
    onCCProfileClick,
}) => {
    const navigate = useNavigate();
    const providerName = getProviderName(imbot.smartguide_provider, providers);

    // Which Claude Code configuration serves @cc: '' = main claude_code
    // scenario, otherwise a profile ID from default_agent ("claude_code:<id>").
    const ccProfileId = ccProfileIdFromDefaultAgent(imbot.default_agent);
    const ccProfileName = ccProfiles.find(p => p.id === ccProfileId)?.name;

    const handleAgentClick = useCallback(() => {
        navigate('/agent/claude_code');
    }, [navigate]);

    return (
        <Box sx={graphRowStyles}>
            {/* Bot node */}
            <NodeContainer>
                <ImBotNode imbot={imbot} active={isBotEnabled} onClick={readOnly ? undefined : onBotClick} />
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
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    <NodeContainer>
                        <AtNode type="tb" />
                    </NodeContainer>

                    <ArrowNode direction="forward" />

                    <NodeContainer>
                        <AgentNode
                            agentType="smart-guide"
                            active={isBotEnabled}
                        />
                    </NodeContainer>

                    <ArrowNode direction="forward" />

                    <NodeContainer>
                        <BotModelNode
                            provider={imbot.smartguide_provider}
                            providerName={providerName}
                            model={imbot.smartguide_model}
                            active={isBotEnabled}
                            onClick={readOnly ? undefined : onModelClick}
                        />
                    </NodeContainer>
                </Box>

                {/* @cc: Claude Code agent → profile (default or a claude_code profile) */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    <NodeContainer>
                        <AtNode type="cc" />
                    </NodeContainer>

                    <ArrowNode direction="forward" />

                    <NodeContainer>
                        <AgentNode
                            agentType="claude-code"
                            active={isBotEnabled}
                            onClick={readOnly ? undefined : handleAgentClick}
                        />
                    </NodeContainer>

                    <ArrowNode direction="forward" />

                    <NodeContainer>
                        <CCProfileNode
                            profileId={ccProfileId}
                            profileName={ccProfileName}
                            active={isBotEnabled}
                            onClick={readOnly ? undefined : onCCProfileClick}
                        />
                    </NodeContainer>
                </Box>
            </Box>
        </Box>
    );
};

export default RemoteControlGraph;
