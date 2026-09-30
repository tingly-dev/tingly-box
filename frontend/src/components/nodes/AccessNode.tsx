import {Security} from '@/components/icons';
import {Box} from '@mui/material';
import {StyledBotGraphNode, NODE_LAYER_STYLES} from './styles';
import NodeTag from './NodeTag';
import NodeTooltip from './NodeTooltip';

interface AccessNodeProps {
    directChats: number;
    groups: number;
    active?: boolean;
    loading?: boolean;
    error?: string;
    onClick?: () => void;
}

const AccessNode: React.FC<AccessNodeProps> = ({directChats, groups, active = true, loading = false, error, onClick}) => {
    const total = directChats + groups;
    const title = error
        ? `Access could not be loaded: ${error}. Click to open access management.`
        : loading
            ? 'Loading authorized Direct Chats and Groups…'
            : total > 0
                ? `${directChats} authorized Direct Chat(s) · ${groups} authorized Group(s). Click to manage access.`
                : 'No Direct Chats or Groups currently allow Remote Control. Click to manage access.';
    return (
        <NodeTooltip
            title={title}
            placement="top"
        >
            <StyledBotGraphNode active={active} clickable={Boolean(onClick)} warn={active && (Boolean(error) || (!loading && total === 0))} onClick={onClick}>
                <Box sx={{...NODE_LAYER_STYLES.topLayer, gap: 0.75}}>
                    <Security sx={{fontSize: 18, color: !error && (loading || total > 0) ? 'primary.main' : 'warning.main'}}/>
                    <Box component="span" sx={{...NODE_LAYER_STYLES.typography, fontWeight: 600}}>
                        Authorized access
                    </Box>
                </Box>
                <Box sx={{width: '85%', borderTop: '1px solid', borderColor: 'divider', my: 0.25}}/>
                <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                    <NodeTag outlined active={active} label={loading ? '… direct' : `${directChats} direct`}/>
                    <NodeTag outlined active={active} label={loading ? '… groups' : `${groups} ${groups === 1 ? 'group' : 'groups'}`}/>
                </Box>
            </StyledBotGraphNode>
        </NodeTooltip>
    );
};

export default AccessNode;
