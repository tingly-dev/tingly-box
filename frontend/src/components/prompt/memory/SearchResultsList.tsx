import { useState, useCallback, useMemo } from 'react';
import {
  Box,
  Typography,
  Card,
  Avatar,
  Chip,
  IconButton,
  Tooltip,
  Collapse,
  Divider,
  CircularProgress,
  Button,
} from '@mui/material';
import {
  ExpandMore,
  ExpandLess,
  SmartToy,
  OpenInFull,
} from '@mui/icons-material';
import type { MemorySessionItem, PromptRoundListItem } from '@/types/prompt';

interface MatchedRound {
  round: PromptRoundListItem;
  matchedIn: 'user' | 'response' | 'both';
}

interface SearchResult {
  session: MemorySessionItem;
  matchedRounds: MatchedRound[];
  expanded?: boolean;
}

interface SearchResultsListProps {
  searchQuery: string;
  sessions: MemorySessionItem[];
  roundsBySession: Map<string, PromptRoundListItem[]>;
  selectedSessionId?: string;
  onSelectSession: (session: MemorySessionItem) => void;
  isLoading?: boolean;
  loadingSessionId?: string;
}

// Scenario configuration with icons and colors
const SCENARIO_CONFIG: Record<string, { color: string; label: string }> = {
  claude_code: { color: 'primary', label: 'Claude Code' },
  opencode: { color: 'success', label: 'OpenCode' },
  anthropic: { color: 'secondary', label: 'Anthropic' },
  openai: { color: 'warning', label: 'OpenAI' },
  google: { color: 'error', label: 'Google' },
};

const formatTime = (dateString: string): string => {
  const date = new Date(dateString);
  return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
};

const formatDate = (dateString: string): string => {
  const date = new Date(dateString);
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);

  if (date.toDateString() === today.toDateString()) {
    return 'Today';
  } else if (date.toDateString() === yesterday.toDateString()) {
    return 'Yesterday';
  }
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
};

const formatTokens = (count: number): string => {
  if (count >= 1000000) return `${(count / 1000000).toFixed(1)}M`;
  if (count >= 1000) return `${(count / 1000).toFixed(1)}K`;
  return count.toString();
};

const getInitials = (name: string): string => {
  return name
    .split(' ')
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
};

// Highlight matching text
const highlightMatch = (text: string, query: string): React.ReactNode => {
  if (!query.trim()) return text;

  const regex = new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
  const parts = text.split(regex);

  return parts.map((part, i) =>
    regex.test(part) ? (
      <Box
        key={i}
        component="span"
        sx={{
          bgcolor: 'warning.light',
          color: 'warning.dark',
          px: 0.25,
          borderRadius: 0.5,
        }}
      >
        {part}
      </Box>
    ) : (
      part
    )
  );
};

// Truncate text for preview
const truncateText = (text: string, maxLength: number = 120): string => {
  if (text.length <= maxLength) return text;
  return text.slice(0, maxLength).trim() + '...';
};

const SearchResultsList: React.FC<SearchResultsListProps> = ({
  searchQuery,
  sessions,
  roundsBySession,
  selectedSessionId,
  onSelectSession,
  isLoading,
  loadingSessionId,
}) => {
  // Search and filter sessions based on query
  const searchResults = useMemo((): SearchResult[] => {
    const query = searchQuery.toLowerCase().trim();

    return sessions.map((session) => {
      const rounds = roundsBySession.get(session.session_id) || [];
      const matchedRounds: MatchedRound[] = [];

      if (query) {
        // Search through rounds for matches
        rounds.forEach((round) => {
          const userInputMatch = round.user_input.toLowerCase().includes(query);
          const responseMatch = round.round_result_preview?.toLowerCase().includes(query);

          if (userInputMatch || responseMatch) {
            matchedRounds.push({
              round,
              matchedIn: userInputMatch && responseMatch ? 'both' : userInputMatch ? 'user' : 'response',
            });
          }
        });
      }

      return {
        session,
        matchedRounds,
        expanded: false,
      };
    }).filter((result) => {
      // Filter: show session if it matches metadata or has matched rounds
      if (!query) return true;

      const { session } = result;
      const metadataMatch =
        session.account_name.toLowerCase().includes(query) ||
        session.model.toLowerCase().includes(query) ||
        session.provider_name.toLowerCase().includes(query);

      return metadataMatch || result.matchedRounds.length > 0;
    });
  }, [sessions, roundsBySession, searchQuery]);

  // Group by date
  const groupedResults = useMemo(() => {
    const groups = new Map<string, SearchResult[]>();

    searchResults.forEach((result) => {
      const dateLabel = formatDate(result.session.created_at);
      if (!groups.has(dateLabel)) {
        groups.set(dateLabel, []);
      }
      groups.get(dateLabel)!.push(result);
    });

    return groups;
  }, [searchResults]);

  const [expandedSessions, setExpandedSessions] = useState<Set<string>>(new Set());

  const toggleExpand = useCallback((sessionId: string) => {
    setExpandedSessions((prev) => {
      const next = new Set(prev);
      if (next.has(sessionId)) {
        next.delete(sessionId);
      } else {
        next.add(sessionId);
      }
      return next;
    });
  }, []);

  if (isLoading && sessions.length === 0) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
          p: 3,
        }}
      >
        <CircularProgress size={32} />
        <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
          Loading memories...
        </Typography>
      </Box>
    );
  }

  if (searchResults.length === 0) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
          p: 3,
          textAlign: 'center',
        }}
      >
        <Typography variant="body2" color="text.secondary">
          {searchQuery ? `No results for "${searchQuery}"` : 'No memories found'}
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5 }}>
      {Array.from(groupedResults.entries()).map(([dateLabel, results]) => (
        <Box key={dateLabel}>
          {/* Date Header */}
          <Box
            sx={{
              px: 1.5,
              py: 0.75,
              bgcolor: 'grey.50',
              borderBottom: 1,
              borderColor: 'divider',
              position: 'sticky',
              top: 0,
              zIndex: 1,
            }}
          >
            <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
              {dateLabel} · {results.length} session{results.length !== 1 ? 's' : ''}
            </Typography>
          </Box>

          {/* Sessions */}
          {results.map(({ session, matchedRounds }) => {
            const isSelected = selectedSessionId === session.id;
            const isLoadingThis = loadingSessionId === session.session_id;
            const isExpanded = expandedSessions.has(session.id);
            const scenarioConfig = SCENARIO_CONFIG[session.scenario] || {
              color: 'default',
              label: session.scenario,
            };

            return (
              <Card
                key={session.id}
                sx={{
                  border: '1px solid',
                  borderColor: isSelected ? 'primary.main' : 'divider',
                  borderRadius: 1.5,
                  overflow: 'hidden',
                  backgroundColor: isSelected ? 'action.selected' : 'background.paper',
                  cursor: 'pointer',
                  transition: 'all 0.2s',
                  '&:hover': {
                    borderColor: 'primary.main',
                    boxShadow: 1,
                  },
                }}
              >
                {/* Session Header */}
                <Box
                  onClick={() => onSelectSession(session)}
                  sx={{ p: 1.25 }}
                >
                  {/* Row 1: Avatar + Account + Scenario */}
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, mb: 0.5 }}>
                    <Avatar
                      sx={{
                        width: 26,
                        height: 26,
                        bgcolor: `${scenarioConfig.color}.main`,
                        fontSize: '0.65rem',
                        fontWeight: 600,
                      }}
                    >
                      {getInitials(session.account_name)}
                    </Avatar>
                    <Typography variant="body2" sx={{ fontWeight: 600, flex: 1 }}>
                      {session.account_name}
                    </Typography>
                    <Chip
                      label={scenarioConfig.label}
                      size="small"
                      sx={{
                        height: 16,
                        fontSize: '0.6rem',
                        bgcolor: `${scenarioConfig.color}.light`,
                        color: `${scenarioConfig.color}.dark`,
                      }}
                    />
                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
                      {formatTime(session.created_at)}
                    </Typography>
                  </Box>

                  {/* Row 2: Model + Stats */}
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
                    <Typography variant="caption" sx={{ fontWeight: 500, fontSize: '0.75rem' }}>
                      {session.model}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                      ·
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                      {session.total_rounds} rounds
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                      ·
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                      {formatTokens(session.total_tokens)} tokens
                    </Typography>
                  </Box>

                  {/* Matched Rounds Preview */}
                  {matchedRounds.length > 0 && (
                    <Box sx={{ mt: 0.5 }}>
                      <Box
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                        }}
                      >
                        <Chip
                          label={`${matchedRounds.length} match${matchedRounds.length > 1 ? 'es' : ''}`}
                          size="small"
                          color="warning"
                          variant="outlined"
                          sx={{ height: 18, fontSize: '0.6rem' }}
                        />
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            e.stopPropagation();
                            toggleExpand(session.id);
                          }}
                          sx={{ p: 0.25 }}
                        >
                          {isExpanded ? <ExpandLess fontSize="small" /> : <ExpandMore fontSize="small" />}
                        </IconButton>
                      </Box>
                    </Box>
                  )}

                  {/* Loading indicator */}
                  {isLoadingThis && (
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', py: 1 }}>
                      <CircularProgress size={16} />
                    </Box>
                  )}
                </Box>

                {/* Expanded Matched Rounds */}
                {matchedRounds.length > 0 && (
                  <Collapse in={isExpanded}>
                    <Divider />
                    <Box sx={{ p: 1, bgcolor: 'grey.50' }}>
                      {matchedRounds.slice(0, 5).map(({ round, matchedIn }, idx) => (
                        <Box
                          key={round.id}
                          sx={{
                            mb: idx < Math.min(matchedRounds.length, 5) - 1 ? 1 : 0,
                            p: 1,
                            bgcolor: 'background.paper',
                            borderRadius: 1,
                            border: '1px solid',
                            borderColor: 'divider',
                          }}
                        >
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
                            <Typography variant="caption" sx={{ fontWeight: 600, color: 'primary.main', fontSize: '0.65rem' }}>
                              Round {round.round_index + 1}
                            </Typography>
                            {matchedIn !== 'response' && (
                              <Chip label="User" size="small" sx={{ height: 14, fontSize: '0.55rem' }} />
                            )}
                            {matchedIn !== 'user' && (
                              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                                <SmartToy sx={{ fontSize: 10, color: 'success.main' }} />
                                <Typography variant="caption" sx={{ fontSize: '0.55rem', color: 'success.main' }}>
                                  AI
                                </Typography>
                              </Box>
                            )}
                          </Box>
                          {(matchedIn === 'user' || matchedIn === 'both') && (
                            <Typography
                              variant="body2"
                              sx={{
                                fontSize: '0.75rem',
                                color: 'text.primary',
                                lineHeight: 1.4,
                                mb: matchedIn === 'both' ? 0.5 : 0,
                              }}
                            >
                              {highlightMatch(truncateText(round.user_input, 100), searchQuery)}
                            </Typography>
                          )}
                          {(matchedIn === 'response' || matchedIn === 'both') && round.round_result_preview && (
                            <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.5 }}>
                              <SmartToy sx={{ fontSize: 12, color: 'success.main', mt: 0.25 }} />
                              <Typography
                                variant="body2"
                                sx={{
                                  fontSize: '0.75rem',
                                  color: 'text.secondary',
                                  lineHeight: 1.4,
                                }}
                              >
                                {highlightMatch(truncateText(round.round_result_preview, 100), searchQuery)}
                              </Typography>
                            </Box>
                          )}
                        </Box>
                      ))}
                      {matchedRounds.length > 5 && (
                        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', textAlign: 'center', mt: 0.5 }}>
                          +{matchedRounds.length - 5} more matches
                        </Typography>
                      )}
                      <Button
                        size="small"
                        fullWidth
                        variant="outlined"
                        onClick={() => onSelectSession(session)}
                        sx={{ mt: 1, textTransform: 'none' }}
                      >
                        View Full Session
                      </Button>
                    </Box>
                  </Collapse>
                )}
              </Card>
            );
          })}
        </Box>
      ))}
    </Box>
  );
};

export default SearchResultsList;
