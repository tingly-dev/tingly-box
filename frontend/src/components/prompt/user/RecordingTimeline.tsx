import { useState } from 'react';
import { Box, Typography, Card, CardContent, Collapse, Chip } from '@mui/material';
import { ExpandMore, ExpandLess, ChevronRight } from '@mui/icons-material';
import type { SessionGroup, PromptRoundListItem } from '@/types/prompt';

interface RecordingTimelineProps {
  sessionGroups: SessionGroup[];
  onPlay: (round: PromptRoundListItem) => void;
  onViewDetails: (round: PromptRoundListItem) => void;
  onDelete: (round: PromptRoundListItem) => void;
  onSelectRound: (round: PromptRoundListItem | null) => void;
  selectedRound: PromptRoundListItem | null;
}

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

  const toggleGroupExpansion = (groupKey: string) => {
    setExpandedGroups((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(groupKey)) {
        newSet.delete(groupKey);
      } else {
        newSet.add(groupKey);
      }
      return newSet;
    });
  };

  const handleRoundClick = (round: PromptRoundListItem) => {
    if (selectedRound?.id === round.id) {
      onSelectRound(null);
    } else {
      onSelectRound(round);
      onViewDetails(round);
    }
  };

  const getScenarioLabel = (scenario: string): string => {
    const labels: Record<string, string> = {
      claude_code: 'Claude Code',
      opencode: 'OpenCode',
      anthropic: 'Anthropic',
      openai: 'OpenAI',
      google: 'Google',
    };
    return labels[scenario] || scenario;
  };

  const getScenarioColor = (scenario: string): string => {
    const colors: Record<string, string> = {
      claude_code: 'info',
      opencode: 'success',
      anthropic: 'secondary',
      openai: 'warning',
      google: 'error',
    };
    return colors[scenario] || 'default';
  };

  if (sessionGroups.length === 0) {
    return null;
  }

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
      }}
    >
      {sessionGroups.map((group) => {
        const isExpanded = expandedGroups.has(group.groupKey);

        return (
          <Card
            key={group.groupKey}
            sx={{
              border: '1px solid',
              borderColor: 'divider',
              borderRadius: 2,
              overflow: 'hidden',
              backgroundColor: 'background.paper',
            }}
          >
            {/* Session Header - Always Visible */}
            <Box
              onClick={() => toggleGroupExpansion(group.groupKey)}
              sx={{
                display: 'flex',
                alignItems: 'center',
                p: 1.5,
                cursor: 'pointer',
                bgcolor: 'action.hover',
                '&:hover': { bgcolor: 'action.selected' },
              }}
            >
              {/* Expand Icon */}
              <Box sx={{ mr: 1 }}>
                {isExpanded ? (
                  <ExpandLess sx={{ fontSize: 18, color: 'text.secondary' }} />
                ) : (
                  <ExpandMore sx={{ fontSize: 18, color: 'text.secondary' }} />
                )}
              </Box>

              {/* Session Info */}
              <Box sx={{ flex: 1, minWidth: 0 }}>
                {/* Account and Session */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    {group.account.name || group.account.id}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    ·
                  </Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                    {group.sessionId.slice(-8)}
                  </Typography>
                </Box>

                {/* Stats */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexWrap: 'wrap' }}>
                  <Chip
                    label={getScenarioLabel(group.stats.scenario)}
                    size="small"
                    color={getScenarioColor(group.stats.scenario) as any}
                    sx={{ height: 18, fontSize: '0.65rem', fontWeight: 500 }}
                  />
                  <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
                    {group.stats.totalRounds} message{group.stats.totalRounds > 1 ? 's' : ''}
                  </Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
                    · {group.stats.totalTokens.toLocaleString()} tokens
                  </Typography>
                  {group.stats.models.length > 0 && (
                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
                      · {group.stats.models[0]}
                    </Typography>
                  )}
                </Box>
              </Box>

              {/* Time Range */}
              <Box sx={{ textAlign: 'right' }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
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
                        mb: index < group.rounds.length - 1 ? 0.5 : 0,
                        border: '1px solid',
                        borderColor: isSelected ? 'primary.main' : 'divider',
                        borderRadius: 1.5,
                        cursor: 'pointer',
                        transition: 'all 0.2s',
                        backgroundColor: isSelected ? 'action.selected' : 'background.paper',
                        '&:hover': {
                          borderColor: 'primary.main',
                          boxShadow: 1,
                        },
                      }}
                    >
                      <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
                        <Box sx={{ display: 'flex', gap: 1 }}>
                          {/* Time */}
                          <Box sx={{ minWidth: 45, flexShrink: 0 }}>
                            <Typography
                              variant="body2"
                              sx={{ fontWeight: 500, color: 'text.secondary', fontSize: '0.7rem' }}
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
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.25 }}>
                              <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                                {round.model}
                              </Typography>
                              {round.is_streaming && (
                                <Chip label="stream" size="small" sx={{ height: 14, fontSize: '0.55rem' }} />
                              )}
                              {round.has_tool_use && (
                                <Chip label="tools" size="small" sx={{ height: 14, fontSize: '0.55rem' }} />
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
