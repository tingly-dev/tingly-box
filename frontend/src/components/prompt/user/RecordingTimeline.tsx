import { useState, useCallback } from 'react';
import {
  Box,
  Typography,
  Card,
  CardContent,
  Collapse,
  Chip,
  Grid,
  Avatar,
  Tooltip,
} from '@mui/material';
import {
  ExpandMore,
  ExpandLess,
  ChevronRight,
  Message,
  Token,
  Psychology,
  SmartToy,
  Code,
  Terminal,
  Chat,
} from '@mui/icons-material';
import type { SessionGroup, PromptRoundListItem } from '@/types/prompt';

interface RecordingTimelineProps {
  sessionGroups: SessionGroup[];
  onPlay: (round: PromptRoundListItem) => void;
  onViewDetails: (round: PromptRoundListItem) => void;
  onDelete: (round: PromptRoundListItem) => void;
  onSelectRound: (round: PromptRoundListItem | null) => void;
  selectedRound: PromptRoundListItem | null;
}

// Scenario configuration with icons and colors
const SCENARIO_CONFIG: Record<
  string,
  { icon: React.ReactElement; color: string; label: string }
> = {
  claude_code: {
    icon: <Code fontSize="small" />,
    color: 'primary',
    label: 'Claude Code',
  },
  opencode: {
    icon: <Terminal fontSize="small" />,
    color: 'success',
    label: 'OpenCode',
  },
  anthropic: {
    icon: <Psychology fontSize="small" />,
    color: 'secondary',
    label: 'Anthropic',
  },
  openai: {
    icon: <SmartToy fontSize="small" />,
    color: 'warning',
    label: 'OpenAI',
  },
  google: {
    icon: <Chat fontSize="small" />,
    color: 'error',
    label: 'Google',
  },
};

// Utility functions
const formatTime = (dateString: string): string => {
  const date = new Date(dateString);
  return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
};

const formatDateTime = (dateString: string): string => {
  const date = new Date(dateString);
  return date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
};

const formatDuration = (startTime: string, endTime: string): string => {
  const start = new Date(startTime);
  const end = new Date(endTime);
  const diff = end.getTime() - start.getTime();

  const hours = Math.floor(diff / (1000 * 60 * 60));
  const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

  if (hours > 24) {
    const days = Math.floor(hours / 24);
    return `${days}d ${hours % 24}h`;
  }
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
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

const RecordingTimeline: React.FC<RecordingTimelineProps> = ({
  sessionGroups,
  onPlay,
  onViewDetails,
  onDelete,
  onSelectRound,
  selectedRound,
}) => {
  // Track which session groups are expanded
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());

  const toggleGroupExpansion = useCallback((groupKey: string) => {
    setExpandedGroups((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(groupKey)) {
        newSet.delete(groupKey);
      } else {
        newSet.add(groupKey);
      }
      return newSet;
    });
  }, []);

  const expandAll = useCallback(() => {
    setExpandedGroups(new Set(sessionGroups.map((g) => g.groupKey)));
  }, [sessionGroups]);

  const collapseAll = useCallback(() => {
    setExpandedGroups(new Set());
  }, []);

  const handleRoundClick = useCallback(
    (round: PromptRoundListItem) => {
      if (selectedRound?.id === round.id) {
        onSelectRound(null);
      } else {
        onSelectRound(round);
        onViewDetails(round);
      }
    },
    [selectedRound, onSelectRound, onViewDetails]
  );

  // Keyboard navigation
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent, groupKey: string) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        toggleGroupExpansion(groupKey);
      }
    },
    [toggleGroupExpansion]
  );

  if (sessionGroups.length === 0) {
    return null;
  }

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        gap: 1.5,
      }}
    >
      {/* Expand/Collapse All Buttons */}
      {sessionGroups.length > 1 && (
        <Box sx={{ display: 'flex', gap: 1, justifyContent: 'flex-end' }}>
          <Chip
            label="Expand All"
            size="small"
            variant="outlined"
            onClick={expandAll}
            sx={{ cursor: 'pointer' }}
          />
          <Chip
            label="Collapse All"
            size="small"
            variant="outlined"
            onClick={collapseAll}
            sx={{ cursor: 'pointer' }}
          />
        </Box>
      )}

      {sessionGroups.map((group) => {
        const isExpanded = expandedGroups.has(group.groupKey);
        const scenarioConfig = SCENARIO_CONFIG[group.stats.scenario] || {
          icon: <Message fontSize="small" />,
          color: 'default',
          label: group.stats.scenario,
        };

        return (
          <Card
            key={group.groupKey}
            sx={{
              border: '1px solid',
              borderColor: 'divider',
              borderRadius: 2,
              overflow: 'hidden',
              backgroundColor: 'background.paper',
              transition: 'box-shadow 0.2s, border-color 0.2s',
              '&:hover': {
                borderColor: `${scenarioConfig.color}.main`,
                boxShadow: 1,
              },
            }}
          >
            {/* Session Header - Always Visible */}
            <Box
              role="button"
              tabIndex={0}
              onClick={() => toggleGroupExpansion(group.groupKey)}
              onKeyDown={(e) => handleKeyDown(e, group.groupKey)}
              sx={{
                display: 'flex',
                alignItems: 'center',
                p: 1.5,
                cursor: 'pointer',
                bgcolor: 'action.hover',
                '&:hover': { bgcolor: 'action.selected' },
                '&:focus-visible': {
                  outline: 2,
                  outlineColor: `${scenarioConfig.color}.main`,
                  outlineOffset: -2,
                },
              }}
            >
              {/* Expand Icon */}
              <Box sx={{ mr: 1 }}>
                {isExpanded ? (
                  <ExpandLess sx={{ fontSize: 20, color: 'text.secondary' }} />
                ) : (
                  <ExpandMore sx={{ fontSize: 20, color: 'text.secondary' }} />
                )}
              </Box>

              {/* Avatar */}
              <Avatar
                sx={{
                  width: 32,
                  height: 32,
                  mr: 1.5,
                  bgcolor: `${scenarioConfig.color}.main`,
                  fontSize: '0.75rem',
                  fontWeight: 600,
                }}
              >
                {getInitials(group.account.name || group.account.id)}
              </Avatar>

              {/* Session Info */}
              <Box sx={{ flex: 1, minWidth: 0 }}>
                {/* Account and Session */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.75 }}>
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    {group.account.name || group.account.id}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    ·
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ fontFamily: 'monospace' }}
                  >
                    {group.sessionId.slice(-8)}
                  </Typography>
                </Box>

                {/* Stats Grid */}
                <Grid container spacing={0.5} sx={{ mb: 1 }}>
                  {/* Messages */}
                  <Grid item xs={6}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                      <Message sx={{ fontSize: 12, color: 'primary.main' }} />
                      <Typography variant="caption" sx={{ fontSize: '0.65rem' }}>
                        {group.stats.totalRounds} msg
                      </Typography>
                    </Box>
                  </Grid>
                  {/* Tokens */}
                  <Grid item xs={6}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                      <Token sx={{ fontSize: 12, color: 'secondary.main' }} />
                      <Typography variant="caption" sx={{ fontSize: '0.65rem' }}>
                        {formatTokens(group.stats.totalTokens)}
                      </Typography>
                    </Box>
                  </Grid>
                  {/* Model */}
                  <Grid item xs={6}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                      <Box sx={{ fontSize: 12, color: `${scenarioConfig.color}.main` }}>
                        {scenarioConfig.icon}
                      </Box>
                      <Typography variant="caption" sx={{ fontSize: '0.65rem' }}>
                        {group.stats.models[0] || 'N/A'}
                      </Typography>
                    </Box>
                  </Grid>
                  {/* Duration */}
                  <Grid item xs={6}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                      <Typography variant="caption" sx={{ fontSize: '0.65rem' }}>
                        {formatDuration(group.stats.firstMessageTime, group.stats.lastMessageTime)}
                      </Typography>
                    </Box>
                  </Grid>
                </Grid>

                {/* Mini Timeline Visualization */}
                <Box sx={{ display: 'flex', gap: 0.25, alignItems: 'center' }}>
                  {group.rounds.slice(0, 30).map((round, i) => (
                    <Tooltip
                      key={i}
                      title={`${formatTime(round.created_at)}${
                        round.has_tool_use ? ' · Tools' : ''
                      }${round.is_streaming ? ' · Streaming' : ''}`}
                    >
                      <Box
                        sx={{
                          flex: 1,
                          height: 3,
                          borderRadius: 0.5,
                          bgcolor: round.has_tool_use
                            ? `${scenarioConfig.color}.main`
                            : round.is_streaming
                            ? `${scenarioConfig.color}.light`
                            : `${scenarioConfig.color}.dark`,
                          opacity: round.has_tool_use ? 1 : 0.6,
                          transition: 'all 0.15s',
                          '&:hover': {
                            height: 4,
                            opacity: 1,
                          },
                        }}
                      />
                    </Tooltip>
                  ))}
                  {group.rounds.length > 30 && (
                    <Typography
                      variant="caption"
                      sx={{ fontSize: '0.55rem', color: 'text.secondary', ml: 0.5 }}
                    >
                      +{group.rounds.length - 30}
                    </Typography>
                  )}
                </Box>
              </Box>

              {/* Time and Badge */}
              <Box sx={{ textAlign: 'right', ml: 1, minWidth: 60 }}>
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'flex-end',
                    gap: 0.5,
                    mb: 0.5,
                  }}
                >
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 0.25,
                      px: 0.5,
                      py: 0.25,
                      borderRadius: 1,
                      bgcolor: `${scenarioConfig.color}.light`,
                      color: `${scenarioConfig.color}.dark`,
                    }}
                  >
                    <Box sx={{ fontSize: 11 }}>{scenarioConfig.icon}</Box>
                  </Box>
                </Box>
                <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                  {formatDateTime(group.stats.lastMessageTime)}
                </Typography>
              </Box>
            </Box>

            {/* Rounds List - Expandable */}
            <Collapse in={isExpanded}>
              <Box sx={{ p: 1 }}>
                {group.rounds.map((round, index) => {
                  const isSelected = selectedRound?.id === round.id;

                  return (
                    <Card
                      key={round.id}
                      onClick={(e) => {
                        e.stopPropagation();
                        handleRoundClick(round);
                      }}
                      sx={{
                        mb: index < group.rounds.length - 1 ? 0.75 : 0,
                        border: '1px solid',
                        borderColor: isSelected ? 'primary.main' : 'divider',
                        borderRadius: 1.5,
                        cursor: 'pointer',
                        transition: 'all 0.2s',
                        backgroundColor: isSelected ? 'action.selected' : 'background.paper',
                        '&:hover': {
                          borderColor: 'primary.main',
                          boxShadow: 1,
                          transform: 'translateX(2)',
                        },
                      }}
                    >
                      <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
                        <Box sx={{ display: 'flex', gap: 1 }}>
                          {/* Time */}
                          <Box sx={{ minWidth: 45, flexShrink: 0 }}>
                            <Typography
                              variant="body2"
                              sx={{
                                fontWeight: 500,
                                color: 'text.secondary',
                                fontSize: '0.7rem',
                              }}
                            >
                              {formatTime(round.created_at)}
                            </Typography>
                          </Box>

                          {/* Content - Two row conversation style */}
                          <Box sx={{ flex: 1, minWidth: 0 }}>
                            {/* User Input Row */}
                            <Box sx={{ display: 'flex', gap: 0.5, mb: 0.5 }}>
                              <Typography
                                variant="caption"
                                sx={{
                                  fontWeight: 600,
                                  color: 'primary.main',
                                  fontSize: '0.65rem',
                                  flexShrink: 0,
                                }}
                              >
                                You:
                              </Typography>
                              <Typography
                                variant="body2"
                                sx={{
                                  fontWeight: 400,
                                  whiteSpace: 'pre-wrap',
                                  overflow: 'hidden',
                                  textOverflow: 'ellipsis',
                                  display: '-webkit-box',
                                  WebkitLineClamp: 2,
                                  WebkitBoxOrient: 'vertical',
                                  fontSize: '0.8rem',
                                  lineHeight: 1.3,
                                }}
                              >
                                {round.user_input}
                              </Typography>
                            </Box>

                            {/* AI Output Row */}
                            {round.round_result_preview && (
                              <Box sx={{ display: 'flex', gap: 0.5 }}>
                                <Typography
                                  variant="caption"
                                  sx={{
                                    fontWeight: 600,
                                    color: 'success.main',
                                    fontSize: '0.65rem',
                                    flexShrink: 0,
                                  }}
                                >
                                  AI:
                                </Typography>
                                <Typography
                                  variant="body2"
                                  sx={{
                                    color: 'text.secondary',
                                    whiteSpace: 'pre-wrap',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    display: '-webkit-box',
                                    WebkitLineClamp: 2,
                                    WebkitBoxOrient: 'vertical',
                                    fontSize: '0.75rem',
                                    lineHeight: 1.3,
                                  }}
                                >
                                  {round.round_result_preview}
                                </Typography>
                              </Box>
                            )}

                            {/* Meta Info */}
                            <Box
                              sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.5 }}
                            >
                              <Typography
                                variant="caption"
                                color="text.secondary"
                                sx={{ fontSize: '0.65rem' }}
                              >
                                {round.model}
                              </Typography>
                              {round.is_streaming && (
                                <Chip
                                  label="stream"
                                  size="small"
                                  sx={{
                                    height: 16,
                                    fontSize: '0.55rem',
                                    bgcolor: 'info.light',
                                    color: 'info.dark',
                                  }}
                                />
                              )}
                              {round.has_tool_use && (
                                <Chip
                                  label="tools"
                                  size="small"
                                  sx={{
                                    height: 16,
                                    fontSize: '0.55rem',
                                    bgcolor: 'warning.light',
                                    color: 'warning.dark',
                                  }}
                                />
                              )}
                            </Box>
                          </Box>

                          {/* Expand Icon */}
                          <ChevronRight
                            sx={{
                              fontSize: 16,
                              color: 'text.secondary',
                              transition: 'transform 0.2s',
                              transform: isSelected ? 'rotate(90deg)' : 'rotate(0deg)',
                              flexShrink: 0,
                            }}
                          />
                        </Box>
                      </CardContent>
                    </Card>
                  );
                })}
              </Box>
            </Collapse>
          </Card>
        );
      })}
    </Box>
  );
};

export default RecordingTimeline;
