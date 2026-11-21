// The Agent rail item and the app's landing both open the agent page the
// user was last on, instead of a directory of agents that repeated the
// sidebar. Kept per browser: it is a convenience, not state that must
// follow the user around.
import { SCENARIOS, getHiddenScenarios } from './scenarioRegistry';

const STORAGE_KEY = 'agent.lastPath';
const PROFILE_PATH = /^\/agent\/claude_code\/profile\/[^/]+$/;

// Team pages live under /agent/team but belong to the Team rail item; Image
// API is under /image. Only the Agent sidebar's own pages count.
const agentScenarioOf = (path: string) =>
    PROFILE_PATH.test(path)
        ? SCENARIOS.find((s) => s.id === 'claude_code')
        : SCENARIOS.find((s) => s.path === path && s.path.startsWith('/agent/') && s.id !== 'team');

export function rememberAgentPath(path: string): void {
    if (!agentScenarioOf(path)) return;
    try {
        localStorage.setItem(STORAGE_KEY, path);
    } catch {
        // Private mode or blocked storage: the landing falls back below.
    }
}

/** Last agent page if still shown in the sidebar, else the first shown one. */
export function lastAgentPath(): string {
    const hidden = getHiddenScenarios();
    let saved: string | null = null;
    try {
        saved = localStorage.getItem(STORAGE_KEY);
    } catch {
        saved = null;
    }
    const savedScenario = saved ? agentScenarioOf(saved) : undefined;
    if (saved && savedScenario && !hidden.has(savedScenario.id)) return saved;
    const first = SCENARIOS.find((s) => s.path.startsWith('/agent/') && s.id !== 'team' && !hidden.has(s.id));
    return first?.path ?? '/agent/claude_code';
}
