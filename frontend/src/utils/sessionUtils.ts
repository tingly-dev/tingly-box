import type { PromptRoundListItem, SessionGroup, AccountInfo, SessionGroupMap } from '@/types/prompt';

/**
 * Extract account information from metadata
 * For claude_code scenario, extracts user_id from metadata
 */
export const extractAccountInfo = (item: PromptRoundListItem): AccountInfo => {
  // The backend stores account info in metadata, but for list items
  // we need to derive it from available data
  // For now, use provider_name as a fallback, and scenario-specific logic

  const baseAccount: AccountInfo = {
    id: item.provider_name,
    name: item.provider_name,
  };

  // For claude_code, the account ID is typically extracted from the metadata
  // Since we're working with PromptRoundListItem (lightweight), we'll use
  // a combination of provider_name and scenario as the account ID
  if (item.scenario === 'claude_code') {
    return {
      id: `${item.scenario}:${item.provider_name}`,
      name: item.provider_name,
    };
  }

  return baseAccount;
};

/**
 * Extract session ID from a prompt round
 * For list items, we don't have session_id directly, so we need to
 * infer it from the available data or use a placeholder
 */
export const extractSessionId = (item: PromptRoundListItem): string => {
  // For lightweight items, we don't have session_id
  // In the full implementation, this would be provided by the backend
  // For now, use a generated key based on date and provider
  const date = new Date(item.created_at);
  const dateKey = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;

  // Group by date + provider + scenario as a fallback
  // This ensures conversations on the same day from the same provider are grouped
  return `${item.scenario}:${item.provider_name}:${dateKey}`;
};

/**
 * Create a unique group key for account + session
 */
export const createGroupKey = (accountId: string, sessionId: string): string => {
  return `${accountId}::${sessionId}`;
};

/**
 * Parse a group key into account and session IDs
 */
export const parseGroupKey = (groupKey: string): { accountId: string; sessionId: string } => {
  const parts = groupKey.split('::');
  if (parts.length !== 2) {
    return { accountId: '', sessionId: '' };
  }
  return { accountId: parts[0], sessionId: parts[1] };
};

/**
 * Group prompt rounds by account and session
 */
export const groupBySession = (rounds: PromptRoundListItem[]): SessionGroup[] => {
  const groupMap = new Map<string, PromptRoundListItem[]>();

  // First pass: group rounds by account + session
  for (const round of rounds) {
    const account = extractAccountInfo(round);
    const sessionId = extractSessionId(round);
    const groupKey = createGroupKey(account.id, sessionId);

    if (!groupMap.has(groupKey)) {
      groupMap.set(groupKey, []);
    }
    groupMap.get(groupKey)!.push(round);
  }

  // Second pass: convert to SessionGroup objects with stats
  const sessionGroups: SessionGroup[] = [];

  for (const [groupKey, groupRounds] of groupMap.entries()) {
    // Sort rounds by created_at (oldest first for chronological conversation flow)
    groupRounds.sort((a, b) =>
      new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
    );

    const { accountId, sessionId } = parseGroupKey(groupKey);
    const account = extractAccountInfo(groupRounds[0]);

    // Calculate statistics
    // Note: PromptRoundListItem doesn't include token counts, so totalTokens will be 0
    // This could be improved by fetching full details or including tokens in list response
    const stats = {
      totalRounds: groupRounds.length,
      totalTokens: 0, // Not available in lightweight list items
      firstMessageTime: groupRounds[0].created_at,
      lastMessageTime: groupRounds[groupRounds.length - 1].created_at,
      models: Array.from(new Set(groupRounds.map((r) => r.model))),
      scenario: groupRounds[0].scenario,
    };

    sessionGroups.push({
      groupKey,
      account,
      sessionId,
      projectId: (groupRounds[0] as any).project_id,
      rounds: groupRounds,
      stats,
    });
  }

  // Sort session groups by last message time (most recent first)
  sessionGroups.sort((a, b) =>
    new Date(b.stats.lastMessageTime).getTime() - new Date(a.stats.lastMessageTime).getTime()
  );

  return sessionGroups;
};

/**
 * Flatten session groups back to individual rounds (for search/filter)
 */
export const flattenSessionGroups = (groups: SessionGroup[]): PromptRoundListItem[] => {
  const rounds: PromptRoundListItem[] = [];
  for (const group of groups) {
    rounds.push(...group.rounds);
  }
  return rounds;
};

/**
 * Filter session groups by search query
 * Searches through all rounds in the group
 */
export const filterSessionGroups = (
  groups: SessionGroup[],
  searchQuery: string
): SessionGroup[] => {
  if (!searchQuery.trim()) {
    return groups;
  }

  const query = searchQuery.toLowerCase();

  return groups
    .map((group) => ({
      ...group,
      rounds: group.rounds.filter((round) =>
        round.user_input.toLowerCase().includes(query)
      ),
    }))
    .filter((group) => group.rounds.length > 0);
};
