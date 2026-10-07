import { Navigate } from 'react-router-dom';

// OnboardingGate decides where a freshly-authenticated user lands: always
// /agent, which opens the agent page they were last on. Brand-new installs
// land there too — the agent pages carry the guided Quick Start (provider →
// model → install → apply), so no separate "first-run" destination is needed.
// (Previously, installs with no provider were sent to /help; that flow was
// superseded by the agent-page Quick Start.) Stale activity state is cleared
// so the default agent page, not a leftover activity, wins.
const OnboardingGate: React.FC = () => {
    localStorage.removeItem('layout.activeActivity');
    sessionStorage.removeItem('layout.activeActivity');
    return <Navigate to="/agent" replace />;
};

export default OnboardingGate;
