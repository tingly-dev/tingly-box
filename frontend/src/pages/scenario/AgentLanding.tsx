import { Navigate } from 'react-router-dom';
import { lastAgentPath } from '../../features/scenario/lastAgent';

// /agent has no page of its own: it opens the agent the user was last on.
// Hiding agents moved into the Agent sidebar's edit mode; Power-ups into
// the rail.
const AgentLanding = () => <Navigate to={lastAgentPath()} replace />;

export default AgentLanding;
