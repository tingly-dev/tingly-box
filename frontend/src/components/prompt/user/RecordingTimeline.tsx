import { useState, useCallback } from 'react';
import {
  Box,
  Typography,
  Card,
  Avatar,
  Tooltip,
  Chip,
} from '@mui/material';
import {
  Person,
  SmartToy,
  Message,
  Code,
  Terminal,
  Psychology,
  Chat,
} from '@mui/icons-material';
import type { SessionGroup } from '@/types/prompt';

interface RecordingTimelineProps {
  sessionGroups: SessionGroup[];
  selectedGroupId?: string;
  onSelectGroup: (groupId: string) => void;
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

const truncateText = (text: string, maxLength: number): string => {
  if (text.length <= maxLength) return text;
  return text.slice(0, maxLength) + '...';
};

const RecordingTimeline: React.FC<RecordingTimelineProps> = ({
  sessionGroups,
  selectedGroupId,
  onSelectGroup,
}) => {
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
        const isSelected = selectedGroupId === group.groupKey;
        const scenarioConfig = SCENARIO_CONFIG[group.stats.scenario] || {
          icon: <Message fontSize="small" />,
          color: 'default',
          label: group.stats.scenario,
        };

        // Get first message preview
        const firstMessage = group.rounds[0];

        return (
          <Card
            key={group.groupKey}
            onClick={() => onSelectGroup(group.groupKey)}
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
                transform: 'translateX(2)',
              },
            }}
          >
            <Box sx={{ p: 1.25 }}>
              {/* Header: Avatar + Name + Chip */}
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, mb: 0.75 }}>
                <Avatar
                  sx={{
                    width: 28,
                    height: 28,
                    bgcolor: `${scenarioConfig.color}.main`,
                    fontSize: '0.7rem',
                    fontWeight: 600,
                  }}
                >
                  {getInitials(group.account.name || group.account.id)}
                </Avatar>
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  {group.account.name || group.account.id}
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
              </Box>

              {/* First message preview */}
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: 0.5,
                  mb: 0.75,
                }}
              >
                <Person sx={{ fontSize: 14, color: 'text.secondary', mt: 0.25 }} />
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
                    fontSize: '0.8rem',
                    lineHeight: 1.3,
                    flex: 1,
                  }}
                >
                  {firstMessage?.user_input || '(empty)'}
                </Typography>
              </Box>

              {/* Footer: Stats */}
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                  <Typography variant="caption" sx={{ fontWeight: 500, fontSize: '0.7rem' }}>
                    {group.stats.totalRounds}
                  </Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                    messages
                  </Typography>
                </Box>
                <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
                  ·
                </Typography>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                  <Typography variant="caption" sx={{ fontWeight: 500, fontSize: '0.7rem' }}>
                    {formatTokens(group.stats.totalTokens)}
                  </Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                    tokens
                  </Typography>
                </Box>
                <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
                  ·
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
                  {formatDuration(group.stats.firstMessageTime, group.stats.lastMessageTime)}
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem', ml: 'auto' }}>
                  {formatTime(group.stats.lastMessageTime)}
                </Typography>
              </Box>
            </Box>
          </Card>
        );
      })}
    </Box>
  );
};

export default RecordingTimeline;
