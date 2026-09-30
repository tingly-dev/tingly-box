import {Chip} from '@mui/material';
import {alpha} from '@mui/material/styles';
import {getRouteGraphActiveColor} from './styles';

interface NodeTagProps {
    label: string;
    /** 'warning' only when the node itself has a problem (e.g. a missing profile). */
    tone?: 'default' | 'warning';
    /** Counts / secondary facts: outlined, not filled. */
    outlined?: boolean;
}

// NodeTag is the bottom-row type tag of the remote/notify graph nodes
// (Agent, Model, Profile, platform). It used to be a raw MUI Chip in a
// different semantic color per node (success green, info teal, warning
// orange, even for a correctly configured model), so a healthy route
// looked like a traffic light. The tags now share the route graph's own
// accent as a soft fill, the same language as the Agent page's graph, and
// color means something again: warning appears only when a node needs
// attention.
const NodeTag = ({label, tone = 'default', outlined = false}: NodeTagProps) => (
    <Chip
        label={label}
        size="small"
        variant={outlined ? 'outlined' : 'filled'}
        sx={(theme) => {
            const accent = tone === 'warning' ? theme.palette.warning.main : getRouteGraphActiveColor(theme);
            return {
                height: 22,
                fontSize: '0.7rem',
                fontWeight: 500,
                color: tone === 'warning' ? theme.palette.warning.main : accent,
                bgcolor: outlined ? 'transparent' : alpha(accent, theme.palette.mode === 'dark' ? 0.16 : 0.1),
                borderColor: outlined ? alpha(accent, 0.35) : 'transparent',
            };
        }}
    />
);

export default NodeTag;
