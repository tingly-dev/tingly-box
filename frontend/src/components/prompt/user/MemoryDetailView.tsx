import { useState, useCallback } from 'react';
import {
  Box,
  Typography,
  Paper,
  Grid,
  IconButton,
  Button,
  Tooltip,
  Chip,
  Avatar,
  Divider,
} from '@mui/material';
import {
  Close,
  ContentCopy,
  Check,
  Person,
  SmartToy,
} from '@mui/icons-material';
import type { SessionGroup } from '@/types/prompt';

interface SessionDetailViewProps {
  session: SessionGroup;
  onClose: () => void;
}

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

const formatTokens = (count: number): string => {
  if (count >= 1000000) return `${(count / 1000000).toFixed(1)}M`;
  if (count >= 1000) return `${(count / 1000).toFixed(1)}K`;
  return count.toString();
};

const getContentStats = (content: string): { words: number; chars: number } => {
  const words = content.trim().split(/\s+/).filter(Boolean).length;
  const chars = content.length;
  return { words, chars };
};

const getInitials = (name: string): string => {
  return name
    .split(' ')
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
};

// Scenario configuration
const SCENARIO_CONFIG: Record<string, { color: string; label: string }> = {
  claude_code: { color: 'primary', label: 'Claude Code' },
  opencode: { color: 'success', label: 'OpenCode' },
  anthropic: { color: 'secondary', label: 'Anthropic' },
  openai: { color: 'warning', label: 'OpenAI' },
  google: { color: 'error', label: 'Google' },
};

const SessionDetailView: React.FC<SessionDetailViewProps> = ({ session, onClose }) => {
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const handleCopy = useCallback((content: string, id: string) => {
    navigator.clipboard.writeText(content);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  }, []);

  const scenarioConfig = SCENARIO_CONFIG[session.stats.scenario] || {
    color: 'default',
    label: session.stats.scenario,
  };

  return (
    <Box>
      {/* Header with Session Info */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          mb: 2,
          pb: 1.5,
          borderBottom: 1,
          borderColor: 'divider',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, minWidth: 0 }}>
          {/* Avatar */}
          <Avatar
            sx={{
              width: 36,
              height: 36,
              bgcolor: `${scenarioConfig.color}.main`,
              fontSize: '0.8rem',
              fontWeight: 600,
            }}
          >
            {getInitials(session.account.name || session.account.id)}
          </Avatar>

          {/* Session Info */}
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.25 }}>
              <Typography variant="h6" sx={{ fontWeight: 600 }}>
                {session.account.name || session.account.id}
              </Typography>
              <Chip
                label={scenarioConfig.label}
                size="small"
                sx={{
                  height: 18,
                  fontSize: '0.65rem',
                  bgcolor: `${scenarioConfig.color}.light`,
                  color: `${scenarioConfig.color}.dark`,
                }}
              />
            </Box>
            <Typography variant="caption" color="text.secondary">
              {session.sessionId.slice(-12)} · {session.stats.totalRounds} messages · {formatTokens(session.stats.totalTokens)} tokens
            </Typography>
          </Box>
        </Box>

        <IconButton size="small" onClick={onClose}>
          <Close />
        </IconButton>
      </Box>

      {/* Chat-style Messages */}
      <Box
        sx={{
          flex: 1,
          overflow: 'auto',
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
        }}
      >
        {session.rounds.map((round, index) => {
          const inputStats = getContentStats(round.user_input);
          const outputStats = round.round_result_preview
            ? getContentStats(round.round_result_preview)
            : null;

          return (
            <Box key={round.id}>
              {/* Round Header */}
              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
                  Round {index + 1}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {formatDateTime(round.created_at)}
                </Typography>
              </Box>

              {/* User Message */}
              <Box
                sx={{
                  display: 'flex',
                  justifyContent: 'flex-end',
                  mb: 1.5,
                }}
              >
                <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', maxWidth: '85%' }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
                    <Typography variant="caption" sx={{ fontWeight: 600, color: 'primary.main', fontSize: '0.7rem' }}>
                      You
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                      {formatTime(round.created_at)}
                    </Typography>
                    {round.is_streaming && (
                      <Chip label="streaming" size="small" sx={{ height: 16, fontSize: '0.6rem' }} />
                    )}
                    <Tooltip title={copiedId === `input-${round.id}` ? 'Copied!' : 'Copy'}>
                      <IconButton
                        size="small"
                        onClick={() => handleCopy(round.user_input, `input-${round.id}`)}
                        sx={{ p: 0.5 }}
                      >
                        {copiedId === `input-${round.id}` ? (
                          <Check fontSize="small" color="success" sx={{ fontSize: 14 }} />
                        ) : (
                          <ContentCopy fontSize="small" sx={{ fontSize: 14 }} />
                        )}
                      </IconButton>
                    </Tooltip>
                  </Box>

                  <Paper
                    sx={{
                      p: 1.5,
                      px: 2,
                      bgcolor: 'primary.main',
                      color: 'white',
                      borderRadius: '12px 12px 0 12px',
                      maxWidth: '100%',
                    }}
                  >
                    <Typography
                      variant="body1"
                      sx={{
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                        fontSize: '0.9rem',
                        lineHeight: 1.5,
                      }}
                    >
                      {round.user_input}
                    </Typography>
                    <Typography variant="caption" sx={{ fontSize: '0.65rem', opacity: 0.8, mt: 0.5, display: 'block' }}>
                      {inputStats.words} words · {inputStats.chars} chars
                    </Typography>
                  </Paper>
                </Box>
              </Box>

              {/* AI Response */}
              {round.round_result_preview && (
                <Box
                  sx={{
                    display: 'flex',
                    justifyContent: 'flex-start',
                  }}
                >
                  <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start', maxWidth: '85%' }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
                      {/* AI Avatar */}
                      <Avatar
                        sx={{
                          width: 20,
                          height: 20,
                          bgcolor: 'success.main',
                          fontSize: '0.65rem',
                        }}
                      >
                        <SmartToy sx={{ fontSize: 12 }} />
                      </Avatar>
                      <Typography variant="caption" sx={{ fontWeight: 600, color: 'success.main', fontSize: '0.7rem' }}>
                        AI
                      </Typography>
                      <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                        {round.model}
                      </Typography>
                      {round.has_tool_use && (
                        <Chip label="tool use" size="small" sx={{ height: 16, fontSize: '0.6rem', bgcolor: 'warning.light', color: 'warning.dark' }} />
                      )}
                      <Tooltip title={copiedId === `output-${round.id}` ? 'Copied!' : 'Copy'}>
                        <IconButton
                          size="small"
                          onClick={() => handleCopy(round.round_result_preview || '', `output-${round.id}`)}
                          sx={{ p: 0.5 }}
                        >
                          {copiedId === `output-${round.id}` ? (
                            <Check fontSize="small" color="success" sx={{ fontSize: 14 }} />
                          ) : (
                            <ContentCopy fontSize="small" sx={{ fontSize: 14 }} />
                          )}
                        </IconButton>
                      </Tooltip>
                    </Box>

                    <Paper
                      sx={{
                        p: 1.5,
                        px: 2,
                        bgcolor: 'background.paper',
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: '0 12px 12px 12px',
                        maxWidth: '100%',
                      }}
                    >
                      <Typography
                        variant="body1"
                        sx={{
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-word',
                          fontSize: '0.9rem',
                          lineHeight: 1.5,
                          color: 'text.primary',
                        }}
                      >
                        {round.round_result_preview}
                      </Typography>
                      <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem', mt: 0.5, display: 'block' }}>
                        {outputStats?.words || 0} words · {outputStats?.chars || 0} chars
                      </Typography>
                    </Paper>
                  </Box>
                </Box>
              )}

              {/* Divider between rounds */}
              {index < session.rounds.length - 1 && (
                <Divider sx={{ my: 1, opacity: 0.5 }} />
              )}
            </Box>
          );
        })}
      </Box>
    </Box>
  );
};

export default SessionDetailView;
