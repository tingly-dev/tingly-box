import { useState, useEffect } from 'react';

/**
 * Zen agent types that support zen mode
 */
export type ZenAgentType =
  | 'claude_code'
  | 'codex'
  | 'opencode'
  | 'xcode'
  | 'vscode'
  | 'openai'
  | 'anthropic'
  | 'agent';

/**
 * Zen mode state and actions
 */
interface UseZenModeResult {
  /** Whether zen mode is enabled */
  enabled: boolean;
  /** Current agent for zen layout (empty if disabled) */
  agent: string;
  /** Loading state */
  loading: boolean;
  /** Set zen mode (empty string = disabled, agent name = enabled) */
  setZenMode: (agent: string) => Promise<boolean>;
}

// Mock API functions - these should be replaced with actual API calls
// TODO: Replace with actual API calls once swagger generation is updated
const mockApi = {
  getScenarioStringFlag: async (scenario: string, flag: string) => {
    // Mock implementation - return from localStorage for now
    const value = localStorage.getItem(`mock-flag-${scenario}-${flag}`) || '';
    return { success: true, data: { scenario, flag, value } };
  },
  setScenarioStringFlag: async (scenario: string, flag: string, value: string) => {
    // Mock implementation - save to localStorage for now
    localStorage.setItem(`mock-flag-${scenario}-${flag}`, value);
    return { success: true, data: { scenario, flag, value } };
  },
};

/**
 * Hook for managing zen mode state
 *
 * Zen mode uses the backend scenario string-flag API:
 * - Scenario: "_global" (global flags)
 * - Flag: "zen"
 * - Value: "" (disabled) | "claude_code" | "codex" | etc.
 *
 * @example
 * ```tsx
 * const { enabled, agent, setZenMode } = useZenMode();
 *
 * // Enable zen mode for Claude Code
 * await setZenMode('claude_code');
 *
 * // Disable zen mode
 * await setZenMode('');
 * ```
 */
export function useZenMode(): UseZenModeResult {
  const [agent, setAgent] = useState<string>('');
  const [loading, setLoading] = useState(true);

  // Fetch zen mode on mount
  useEffect(() => {
    const fetchZenMode = async () => {
      try {
        // TODO: Replace with actual API call: api.getScenarioStringFlag('_global', 'zen')
        const result = await mockApi.getScenarioStringFlag('_global', 'zen');
        if (result.success) {
          setAgent(result.data.value || '');
        }
      } catch (error) {
        console.error('Failed to fetch zen mode:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchZenMode();
  }, []);

  /**
   * Set zen mode to a specific agent or disable it
   * @param agent - Agent name (e.g., 'claude_code') or empty string to disable
   * @returns Promise that resolves to true if successful
   */
  const setZenMode = async (newAgent: string): Promise<boolean> => {
    try {
      // TODO: Replace with actual API call: api.setScenarioStringFlag('_global', 'zen', newAgent)
      const result = await mockApi.setScenarioStringFlag('_global', 'zen', newAgent || '');
      if (result.success) {
        setAgent(newAgent);
        return true;
      }
    } catch (error) {
      console.error('Failed to set zen mode:', error);
    }
    return false;
  };

  return {
    enabled: agent !== '',
    agent,
    loading,
    setZenMode,
  };
}

/**
 * Check if an agent supports zen mode profiles
 */
export function agentSupportsProfiles(agent: string): boolean {
  return ['claude_code', 'codex', 'opencode', 'xcode', 'vscode'].includes(agent);
}

/**
 * Get the scenario key for profile API calls
 */
export function getProfileScenario(agent: string): string {
  const scenarioMap: Record<string, string> = {
    claude_code: 'claude_code',
    codex: 'codex',
    opencode: 'opencode',
    xcode: 'xcode',
    vscode: 'vscode',
  };
  return scenarioMap[agent] || agent;
}
