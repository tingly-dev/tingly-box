import {Box, Divider, Typography} from '@mui/material';
import {NODE_LAYER_STYLES, StyledBotGraphNode} from './styles';
import NodeTag from './NodeTag';
import NodeTooltip from './NodeTooltip';
import { fontMono, fontSizes } from '@/theme/fonts';

// ApiEntryNode is the source of the notify routing graph: the authenticated
// HTTP surface (POST /api/v1/bots/:bot/notify|interact) that drives a bot's
// channel. Same dimensions as PlatformNode/ChatNode so the graph rows align.

export interface ApiEntryNodeProps {
    /** Path shown on the node, e.g. "/api/v1/bots/:bot/notify". */
    path: string;
    active?: boolean;
    /** Optional click-through (e.g. open the API guide / curl example). */
    onClick?: () => void;
}

const ApiEntryNode: React.FC<ApiEntryNodeProps> = ({path, active = true, onClick}) => {
    // The node shows the call ("POST /notify"); the full path, with the bot
    // UUID the card header already identifies, is in the tooltip. Showing
    // the whole path only ever rendered a truncated "/api/v1/bots/…".
    const action = `/${path.split('/').filter(Boolean).pop() ?? ''}`;
    return (
        <StyledBotGraphNode active={active} clickable={!!onClick} onClick={onClick} sx={{width: 140}}>
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <NodeTooltip title={<>POST {path}<br/>Authenticated with the operator user token.</>} placement="top">
                    <Typography
                        variant="body2"
                        noWrap
                        sx={{
                            ...NODE_LAYER_STYLES.typography,
                            fontFamily: fontMono,
                            fontSize: fontSizes.md,
                            maxWidth: 190,
                            color: active ? 'text.primary' : 'text.disabled',
                        }}
                    >
                        POST {action}
                    </Typography>
                </NodeTooltip>
            </Box>
            <Divider sx={NODE_LAYER_STYLES.divider}/>
            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <NodeTag label="API" active={active}/>
            </Box>
        </StyledBotGraphNode>
    );
};

export default ApiEntryNode;
