import {Box} from '@mui/material';
import type {BotSettings} from '@/types/bot.ts';
import type {Provider} from '@/types/provider.ts';
import {ArrowNode, NodeContainer} from '../nodes';
import ImBotNode from '../nodes/ImBotNode.tsx';
import BotModelNode from '../nodes/BotModelNode.tsx';
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
    currentCWD: string;
    isBotEnabled: boolean;
    readOnly?: boolean;
    onCWDChange: (cwd: string) => void;
    onModelClick?: () => void;
    onBotClick?: () => void;
    showAgentNode?: boolean; // Optional prop to show Agent node for future use
}

// Helper function to get provider name from providersData
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
    showAgentNode = false, // Default to false for simplified 3-node layout
}) => {
    const providerName = getProviderName(imbot.smartguide_provider, providers);

    return (
        <Box sx={graphRowStyles}>
            <NodeContainer>
                <ImBotNode imbot={imbot} active={isBotEnabled} onClick={readOnly ? undefined : onBotClick}/>
            </NodeContainer>

            <ArrowNode direction="forward"/>

            <NodeContainer>
                <BotModelNode
                    provider={imbot.smartguide_provider}
                    providerName={providerName}
                    model={imbot.smartguide_model}
                    active={isBotEnabled}
                    onClick={readOnly ? undefined : onModelClick}
                />
            </NodeContainer>

            <ArrowNode direction="forward"/>

            <NodeContainer>
                <CWDNode currentPath={currentCWD} onPathChange={onCWDChange} disabled={readOnly || !isBotEnabled}/>
            </NodeContainer>
        </Box>
    );
};

export default RemoteControlGraph;
