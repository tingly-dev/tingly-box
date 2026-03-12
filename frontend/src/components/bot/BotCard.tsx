import {Delete as DeleteIcon, Edit as EditIcon} from '@mui/icons-material';
import {
    Box,
    Button,
    Card,
    CardContent,
    Chip,
    Collapse,
    IconButton,
    Modal,
    Switch,
    Tooltip,
    Typography
} from '@mui/material';
import {styled} from '@mui/material/styles';
import type {BotSettings} from '@/types/bot';
import type {Provider} from '@/types/provider';
import {CrossNode, NodeContainer} from '../nodes';
import ImBotNode from '../nodes/ImBotNode';
import SmartGuideNode from '../nodes/SmartGuideNode';
import {useCallback, useState} from 'react';

// Use same style constants as RuleGraph for consistency
const RULE_GRAPH_STYLES = {
    header: {
        paddingX: 16,
        paddingY: 6,
    },
    graphContainer: {
        paddingX: 16,
        paddingY: 10,
        marginX: 16,
        marginY: 8,
    },
    graph: {
        rowGap: 16,
    },
} as const;

const {header, graphContainer, graph} = RULE_GRAPH_STYLES;

// Styled Card matching RuleCard style exactly
const StyledCard = styled(Card, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({active, theme}) => ({
    transition: 'all 0.2s ease-in-out',
    opacity: active ? 1 : 0.6,
    filter: active ? 'none' : 'grayscale(0.3)',
    border: active ? 'none' : '2px dashed',
    borderColor: active ? 'transparent' : theme.palette.text.disabled,
    margin: '3px',
    position: 'relative',
    ...(active ? {} : {
        '&::before': {
            content: '""',
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundImage: 'repeating-linear-gradient(45deg, transparent, transparent 10px, rgba(0,0,0,0.03) 10px, rgba(0,0,0,0.03) 20px)',
            pointerEvents: 'none',
            borderRadius: theme.shape.borderRadius,
        },
    }),
    '&:hover': {
        boxShadow: active ? theme.shadows[4] : theme.shadows[1],
    },
}));

const SummarySection = styled(Box)({
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: `${header.paddingY}px ${header.paddingX}px`,
});

const GraphContainer = styled(Box)(({theme}) => ({
    padding: `${graphContainer.paddingY}px ${graphContainer.paddingX}px`,
    backgroundColor: 'grey.50',
    borderRadius: theme.shape.borderRadius,
    margin: `${graphContainer.marginY}px ${graphContainer.marginX}px 0`,
}));

const GraphRow = styled(Box)({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: graph.rowGap,
    marginBottom: 1,
});

interface BotCardProps {
    bot: BotSettings;
    providers: Provider[];
    onEdit: () => void;
    onDelete: () => void;
    onBotToggle: (enabled: boolean) => void;
    onSmartGuideClick: () => void;
    onCWDChange: (cwd: string) => void;
    isToggling?: boolean;
}

const BotCard: React.FC<BotCardProps> = ({
                                             bot,
                                             providers,
                                             onEdit,
                                             onDelete,
                                             onBotToggle,
                                             onSmartGuideClick,
                                             onCWDChange,
                                             isToggling = false,
                                         }) => {
    const isActive = bot.enabled ?? true;
    const isExpanded = true;
    const [deleteModalOpen, setDeleteModalOpen] = useState(false);

    // Get provider name for SmartGuide node
    const getProviderName = (providerUuid: string | undefined): string => {
        if (!providerUuid) return '';
        const provider = providers.find((p) => p.uuid === providerUuid);
        return provider?.name || '';
    };

    const providerName = getProviderName(bot.smartguide_provider);

    const handleDeleteClick = useCallback(() => {
        setDeleteModalOpen(true);
    }, []);

    const handleConfirmDelete = useCallback(() => {
        setDeleteModalOpen(false);
        onDelete();
    }, [onDelete]);

    return (
        <StyledCard active={isActive}>
            {/* Header Section */}
            <SummarySection>
                {/* Left side */}
                <Box sx={{display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1, minWidth: 0}}>
                    <Tooltip title={bot.name || bot.platform}>
                        <Typography
                            sx={{
                                fontFamily: 'monospace',
                                fontSize: '0.875rem',
                                fontWeight: 600,
                                color: isActive ? 'text.primary' : 'text.disabled',
                                opacity: isActive ? 1 : 0.5,
                                cursor: 'default',
                            }}
                        >
                            {bot.name || bot.platform}
                        </Typography>
                    </Tooltip>
                    {bot.name && (
                        <Chip
                            label={bot.platform}
                            size="small"
                            sx={{
                                opacity: isActive ? 1 : 0.5,
                            }}
                        />
                    )}
                    {bot.chat_id && (
                        <Tooltip title={`Chat ID: ${bot.chat_id}`}>
                            <Typography
                                variant="caption"
                                sx={{
                                    color: 'text.secondary',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                    maxWidth: '120px',
                                }}
                            >
                                {bot.chat_id}
                            </Typography>
                        </Tooltip>
                    )}
                    {bot.proxy_url && (
                        <Tooltip title={bot.proxy_url}>
                            <Chip
                                label={`Proxy: ${bot.proxy_url}`}
                                size="small"
                                sx={{
                                    fontFamily: 'monospace',
                                    fontSize: '0.7rem',
                                    maxWidth: '300px',
                                }}
                            />
                        </Tooltip>
                    )}
                </Box>
                {/* Right side - All buttons expanded */}
                <Box sx={{display: 'flex', alignItems: 'center', gap: 0.5}}>
                    <Tooltip title={isActive ? 'Disable Bot' : 'Enable Bot'}>
                        <Switch
                            checked={isActive}
                            onChange={() => onBotToggle(!isActive)}
                            size="small"
                            color="success"
                            disabled={isToggling}
                        />
                    </Tooltip>
                    <Tooltip title="Edit">
                        <IconButton
                            size="small"
                            color="primary"
                            onClick={onEdit}
                            disabled={isToggling}
                        >
                            <EditIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Delete">
                        <IconButton
                            size="small"
                            color="error"
                            onClick={handleDeleteClick}
                            disabled={isToggling}
                        >
                            <DeleteIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                </Box>
            </SummarySection>

            {/* Expanded Content - Graph View */}
            <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                <CardContent sx={{pt: 0, pb: 1}}>
                    {/* Graph Visualization */}
                    <Box sx={{overflowX: 'auto'}}>
                        <GraphContainer>
                            <GraphRow>
                                <NodeContainer>
                                    <ImBotNode imbot={bot} active={isActive}/>
                                </NodeContainer>

                                <CrossNode/>

                                <NodeContainer>
                                    <SmartGuideNode
                                        provider={bot.smartguide_provider}
                                        providerName={providerName}
                                        model={bot.smartguide_model}
                                        active={isActive}
                                        onClick={isActive ? onSmartGuideClick : undefined}
                                    />
                                </NodeContainer>

                            </GraphRow>
                        </GraphContainer>
                    </Box>

                    {/* Metadata row below graph */}
                    {bot.bash_allowlist && bot.bash_allowlist.length > 0 && (
                        <Box sx={{mt: 2, display: 'flex', gap: 2, flexWrap: 'wrap'}}>
                            <Tooltip title={bot.bash_allowlist.join(', ')}>
                                <Typography variant="caption" sx={{color: 'text.secondary'}}>
                                    Allowlist: <span
                                    style={{fontFamily: 'monospace'}}>{bot.bash_allowlist.join(', ')}</span>
                                </Typography>
                            </Tooltip>
                        </Box>
                    )}
                </CardContent>
            </Collapse>

            {/* Delete Confirmation Modal */}
            <Modal open={deleteModalOpen} onClose={() => setDeleteModalOpen(false)}>
                <Box
                    sx={{
                        position: 'absolute',
                        top: '50%',
                        left: '50%',
                        transform: 'translate(-50%, -50%)',
                        width: 400,
                        maxWidth: '80vw',
                        bgcolor: 'background.paper',
                        boxShadow: 24,
                        p: 4,
                        borderRadius: 2,
                    }}
                >
                    <Typography variant="h6" sx={{mb: 2}}>Delete Bot Configuration</Typography>
                    <Typography variant="body2" sx={{mb: 3}}>
                        Are you sure you want to delete the bot configuration "{bot.name || bot.platform}"? This action
                        cannot be undone.
                    </Typography>
                    <Box sx={{display: 'flex', gap: 2, justifyContent: 'flex-end'}}>
                        <Button onClick={() => setDeleteModalOpen(false)} color="inherit">
                            Cancel
                        </Button>
                        <Button onClick={handleConfirmDelete} color="error" variant="contained">
                            Delete
                        </Button>
                    </Box>
                </Box>
            </Modal>
        </StyledCard>
    );
};

export default BotCard;
