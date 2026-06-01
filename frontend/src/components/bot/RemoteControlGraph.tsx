import { Box } from '@mui/material';
import type { BotSettings } from '@/types/bot.ts';
import type { Provider } from '@/types/provider.ts';
import { ArrowNode, NodeContainer } from '../nodes';
import ImBotNode from '../nodes/ImBotNode.tsx';
import BotModelNode from '../nodes/BotModelNode.tsx';
import CWDNode from '../nodes/ConfigNode.tsx';
import AgentNode from '../nodes/AgentNode.tsx';
import AtNode from '../nodes/AtNode.tsx';
import { useNavigate } from 'react-router-dom';
import { useCallback } from 'react';

const graphRowStyles = (theme: any) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: theme.spacing(1.5),
    mb: 2,
    flexWrap: 'wrap',
});

interface RemoteGraphRowProps {
    imbot: BotSettings;
    providers: Provider[];
    currentCWD: string;
    isBotEnabled: boolean;
    readOnly?: boolean;
    onCWDChange: (cwd: string) => void;
    onModelClick?: () => void;
    onBotClick?: () => void;
}

const getProviderName = (providerUuid: string | undefined, providersData: Provider[]): string => {
    if (!providerUuid) return '';
    const provider = providersData.find(p => p.uuid === providerUuid);
    return provider?.name || '';
};

const RemoteControlGraph: React.FC<RemoteGraphRowProps> = ({
    imbot,
    providers,
    currentCWD,
    isBotEnabled,
    readOnly = false,
    onCWDChange,
    onModelClick,
    onBotClick,
}) => {
    const navigate = useNavigate();
    const providerName = getProviderName(imbot.smartguide_provider, providers);

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

            {/* Two branches: @tb and @cc */}
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
                {/* @tb branch: tingly-box service routing */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    <NodeContainer>
                        <AtNode type="tb" />
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

                    <ArrowNode direction="forward" />

                    <NodeContainer>
                        <CWDNode currentPath={currentCWD} onPathChange={onCWDChange} disabled={readOnly || !isBotEnabled} />
                    </NodeContainer>
                </Box>

                {/* @cc branch: Claude Code agent */}
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
                </Box>
            </Box>
        </Box>
    );
};

export default RemoteControlGraph;
