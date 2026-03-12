import {Box} from '@mui/material';
import type {BotSettings} from '@/types/bot.ts';
import type {Provider} from '@/types/provider.ts';
import {ArrowNode, NodeContainer} from '../nodes';
import ImBotNode from '../nodes/ImBotNode.tsx';
import SmartGuideNode from '../nodes/SmartGuideNode.tsx';
import AgentNode from '../nodes/AgentNode.tsx';
import CWDNode from '../nodes/ConfigNode.tsx';

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
    currentAgentUuid: string | null;
    currentCWD: string;
    isBotEnabled: boolean;
    readOnly?: boolean;
    onBotToggle?: (enabled: boolean) => void;
    onCWDChange: (cwd: string) => void;
    onSmartGuideClick?: () => void;
    isToggling?: boolean;
}

// Determine agent type from UUID pattern or default to 'claude-code'
const getAgentTypeFromUuid = (uuid: string | null): 'claude-code' | 'custom' | 'mock' => {
    if (!uuid) return 'claude-code';
    if (uuid.startsWith('custom-')) return 'custom';
    if (uuid.startsWith('mock-')) return 'mock';
    return 'claude-code'; // Default for Claude Code agents
};

// Helper function to get provider name from providersData
const getProviderName = (providerUuid: string | undefined, providersData: Provider[]): string => {
    if (!providerUuid) return '';
    const provider = providersData.find(p => p.uuid === providerUuid);
    return provider?.name || '';
};

const RemoteControlGraph: React.FC<RemoteGraphRowProps> = ({
                                                               imbot,
                                                               providers,
                                                               currentAgentUuid,
                                                               currentCWD,
                                                               isBotEnabled,
                                                               readOnly = false,
                                                               onBotToggle,
                                                               onCWDChange,
                                                               onSmartGuideClick,
                                                               isToggling = false,
                                                           }) => {
    const currentAgentType = getAgentTypeFromUuid(currentAgentUuid);
    const agentLabel = 'Claude Code';
    const providerName = getProviderName(imbot.smartguide_provider, providers);

    return (
        <Box sx={graphRowStyles}>
            <NodeContainer>
                <ImBotNode imbot={imbot} active={isBotEnabled} onToggle={onBotToggle} isToggling={isToggling}/>
            </NodeContainer>

            <ArrowNode direction="forward"/>

            <NodeContainer>
                <SmartGuideNode
                    provider={imbot.smartguide_provider}
                    providerName={providerName}
                    model={imbot.smartguide_model}
                    active={isBotEnabled}
                    onClick={readOnly ? undefined : onSmartGuideClick}
                />
            </NodeContainer>

            <ArrowNode direction="forward"/>

            <NodeContainer>
                <AgentNode
                    agentType={currentAgentType}
                    active={isBotEnabled}
                    label={agentLabel}
                />
            </NodeContainer>

            <CWDNode currentPath={currentCWD} onPathChange={onCWDChange} disabled={readOnly || !isBotEnabled}/>
        </Box>
    );
};

export default RemoteControlGraph;
