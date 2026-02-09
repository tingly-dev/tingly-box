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
  Code,
  Terminal,
  Psychology,
  Chat,
  SmartToy,
} from '@mui/icons-material';
import type { MemorySessionItem } from '@/types/prompt';

interface SessionListProps {
  sessions: MemorySessionItem[];
  selectedSessionId?: string;
  onSelectSession: (session: MemorySessionItem) => void;
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

const SessionList: React.FC<SessionListProps> = ({
  sessions,
  selectedSessionId,
  onSelectSession,
}) => {
  if (sessions.length === 0) {
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
      {sessions.map((session) => {
        const isSelected = selectedSessionId === session.id;
        const scenarioConfig = SCENARIO_CONFIG[session.scenario] || {
          icon: <Person fontSize="small" />,
          color: 'default',
          label: session.scenario,
        };

        return (
          <Card
            key={session.id}
            onClick={() => onSelectSession(session)}
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
              {/* Header: Avatar + Account Name + Scenario Chip */}
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
              </Box>

              {/* Model + Provider info */}
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 0.5,
                  mb: 0.75,
                }}
              >
                <Typography
                  variant="body2"
                  sx={{
                    color: 'text.secondary',
                    fontSize: '0.8rem',
                    fontWeight: 500,
                  }}
                >
                  {session.model}
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontSize: '0.7rem' }}
                >
                  · {session.provider_name}
                </Typography>
              </Box>

              {/* Footer: Stats */}
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                  <Typography variant="caption" sx={{ fontWeight: 500, fontSize: '0.7rem' }}>
                    {session.total_rounds}
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
                    {formatTokens(session.total_tokens)}
                  </Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                    tokens
                  </Typography>
                </Box>
                <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem', ml: 'auto' }}>
                  {formatTime(session.created_at)}
                </Typography>
              </Box>
            </Box>
          </Card>
        );
      })}
    </Box>
  );
};

export default SessionList;
