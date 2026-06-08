import { useState, useEffect } from 'react';
import { api } from '@/services/api';
import { PROFILE_SCENARIOS, type ProfileScenario } from '@/constants/profileScenarios';

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
        const result = await api.getScenarioStringFlag('_global', 'zen');
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
      const result = await api.setScenarioStringFlag('_global', 'zen', newAgent || '');
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
  return PROFILE_SCENARIOS.includes(agent as ProfileScenario);
}

/**
 * Get the scenario key for profile API calls.
 * For built-in profile scenarios the agent name equals the scenario name.
 */
export function getProfileScenario(agent: string): string {
  return agent;
}
