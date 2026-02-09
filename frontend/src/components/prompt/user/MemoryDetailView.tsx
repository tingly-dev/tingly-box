import { useState, useCallback } from 'react';
import {
  Box,
  Typography,
  Paper,
  Grid,
  IconButton,
  Collapse,
  Button,
  Tooltip,
} from '@mui/material';
import {
  Close,
  ContentCopy,
  Check,
  ExpandMore,
  ExpandLess,
  Person,
  SmartToy,
} from '@mui/icons-material';
import type { PromptRoundItem } from '@/types/prompt';

interface MemoryDetailViewProps {
  memory: PromptRoundItem;
  onClose: () => void;
}

// Utility functions
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

// Reusable Content Section Component
interface ContentSectionProps {
  title: string;
  icon: React.ReactElement;
  content: string;
  color: 'primary' | 'secondary';
  copyLabel: string;
  maxHeight?: number;
}

const ContentSection: React.FC<ContentSectionProps> = ({
  title,
  icon,
  content,
  color,
  copyLabel,
  maxHeight = 400,
}) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [content]);

  const stats = getContentStats(content);

  return (
    <Box>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          mb: 1,
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <Box sx={{ color: `${color}.main`, fontSize: 18 }}>{icon}</Box>
          <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
            {title}
          </Typography>
        </Box>
        <Tooltip title={copied ? 'Copied!' : copyLabel}>
          <IconButton size="small" onClick={handleCopy} sx={{ p: 0.5 }}>
            {copied ? (
              <Check fontSize="small" color="success" />
            ) : (
              <ContentCopy fontSize="small" />
            )}
          </IconButton>
        </Tooltip>
      </Box>
      <Paper
        variant="outlined"
        sx={{
          p: 2,
          bgcolor:
            color === 'primary'
              ? 'primary.50'
              : 'secondary.50',
          borderLeft: 3,
          borderLeftColor: `${color}.main`,
          borderRadius: 1.5,
          maxHeight,
          overflow: 'auto',
        }}
      >
        <Typography
          variant="body1"
          sx={{
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            lineHeight: 1.6,
            fontSize: '0.9rem',
          }}
        >
          {content || (
            <Typography
              variant="body2"
              sx={{ color: 'text.disabled', fontStyle: 'italic' }}
            >
              No content
            </Typography>
          )}
        </Typography>
      </Paper>
      <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
        {stats.words} words · {stats.chars} characters
      </Typography>
    </Box>
  );
};

// Parse Claude Code input to extract XML-like tagged injections
const parseClaudeCodeInput = (
  input: string
): { injections: { tag: string; content: string }[]; remainingInput: string } => {
  const injections: { tag: string; content: string }[] = [];
  let remainingInput = input;

  const tagRegex = /<([a-zA-Z_][a-zA-Z0-9_-]*)>([\s\S]*?)<\/\1>/g;
  let match;

  while ((match = tagRegex.exec(input)) !== null) {
    injections.push({
      tag: match[1],
      content: match[2].trim(),
    });
  }

  remainingInput = input.replace(tagRegex, '').trim();

  return { injections, remainingInput };
};

const MemoryDetailView: React.FC<MemoryDetailViewProps> = ({ memory, onClose }) => {
  const [injectionSectionExpanded, setInjectionSectionExpanded] = useState(false);
  const [expandedInjections, setExpandedInjections] = useState<
    Record<number, boolean>
  >({});

  // Toggle injection expansion
  const toggleInjection = useCallback((idx: number) => {
    setExpandedInjections((prev) => ({
      ...prev,
      [idx]: !prev[idx],
    }));
  }, []);

  // Scenario badge color
  const getScenarioColor = (scenario: string): 'primary' | 'secondary' | 'success' | 'warning' | 'error' => {
    const colors: Record<string, 'primary' | 'secondary' | 'success' | 'warning' | 'error'> = {
      claude_code: 'primary',
      opencode: 'success',
      anthropic: 'secondary',
      openai: 'warning',
      google: 'error',
    };
    return colors[scenario] || 'primary';
  };

  const isClaudeCode = memory.scenario === 'claude_code';
  const { injections, remainingInput } = isClaudeCode
    ? parseClaudeCodeInput(memory.user_input)
    : { injections: [], remainingInput: memory.user_input };

  return (
    <Box>
      {/* Header with Actions */}
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
        <Box>
          <Typography variant="h6" sx={{ fontWeight: 600, mb: 0.5 }}>
            Memory Details
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Round #{memory.round_index} · {new Date(memory.created_at).toLocaleString()}
          </Typography>
        </Box>
        <IconButton size="small" onClick={onClose}>
          <Close />
        </IconButton>
      </Box>

      {/* Quick Actions Bar */}
      <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
        <Tooltip title="Copy All">
          <Button
            size="small"
            variant="outlined"
            startIcon={<ContentCopy fontSize="small" />}
            onClick={() => {
              const fullText = `User: ${memory.user_input}\n\nAI: ${memory.round_result || ''}`;
              navigator.clipboard.writeText(fullText);
            }}
          >
            Copy All
          </Button>
        </Tooltip>
      </Box>

      {/* Token Usage Card */}
      <Paper
        variant="outlined"
        sx={{
          p: 1.5,
          mb: 2,
          borderRadius: 2,
          bgcolor: 'background.default',
        }}
      >
        <Grid container spacing={1.5}>
          <Grid item xs={4}>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: 'block', mb: 0.25 }}>
              INPUT
            </Typography>
            <Typography variant="h6" sx={{ fontWeight: 600, color: 'primary.main', fontSize: '1.1rem' }}>
              {formatTokens(memory.input_tokens)}
            </Typography>
          </Grid>
          <Grid item xs={4}>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: 'block', mb: 0.25 }}>
              OUTPUT
            </Typography>
            <Typography variant="h6" sx={{ fontWeight: 600, color: 'success.main', fontSize: '1.1rem' }}>
              {formatTokens(memory.output_tokens)}
            </Typography>
          </Grid>
          <Grid item xs={4}>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: 'block', mb: 0.25 }}>
              TOTAL
            </Typography>
            <Typography variant="h6" sx={{ fontWeight: 600, color: 'info.main', fontSize: '1.1rem' }}>
              {formatTokens(memory.total_tokens)}
            </Typography>
          </Grid>
        </Grid>
      </Paper>

      {/* Metadata Card */}
      <Paper
        variant="outlined"
        sx={{
          p: 1.5,
          mb: 2,
          borderRadius: 2,
          bgcolor: 'background.default',
        }}
      >
        <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, mb: 1, display: 'block' }}>
          CONTEXT
        </Typography>
        <Grid container spacing={1.5}>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
              Protocol
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }}>
              {memory.protocol}
            </Typography>
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
              Scenario
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }}>
              {memory.scenario}
            </Typography>
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
              Provider
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }}>
              {memory.provider_name}
            </Typography>
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
              Model
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }}>
              {memory.model}
            </Typography>
          </Grid>
          {memory.project_id && (
            <Grid item xs={6}>
              <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                Project
              </Typography>
              <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', fontWeight: 500 }}>
                {memory.project_id.slice(0, 12)}...
              </Typography>
            </Grid>
          )}
          {memory.session_id && (
            <Grid item xs={6}>
              <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                Session
              </Typography>
              <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', fontWeight: 500 }}>
                {memory.session_id.slice(0, 12)}...
              </Typography>
            </Grid>
          )}
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
              Streaming
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }}>
              {memory.is_streaming ? 'Yes' : 'No'}
            </Typography>
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
              Tool Use
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }}>
              {memory.has_tool_use ? 'Yes' : 'No'}
            </Typography>
          </Grid>
        </Grid>
      </Paper>

      {/* Input Injections Section (Claude Code only) */}
      {injections.length > 0 && (
        <Box sx={{ mb: 2 }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              cursor: 'pointer',
              mb: 1,
            }}
            onClick={() => setInjectionSectionExpanded(!injectionSectionExpanded)}
          >
            <IconButton size="small" sx={{ p: 0, mr: 0.5 }}>
              {injectionSectionExpanded ? <ExpandLess /> : <ExpandMore />}
            </IconButton>
            <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600 }}>
              INPUT INJECTIONS ({injections.length})
            </Typography>
          </Box>

          <Collapse in={injectionSectionExpanded}>
            {injections.map((inj, idx) => {
              const isExpanded = expandedInjections[idx] ?? false;
              return (
                <Paper
                  key={idx}
                  variant="outlined"
                  sx={{
                    mb: idx < injections.length - 1 ? 1 : 0,
                    bgcolor: 'primary.50',
                    border: '1px solid',
                    borderColor: 'primary.200',
                    borderRadius: 1,
                    overflow: 'hidden',
                  }}
                >
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      p: 1,
                      cursor: 'pointer',
                      '&:hover': { bgcolor: 'primary.100' },
                    }}
                    onClick={() => toggleInjection(idx)}
                  >
                    <IconButton size="small" sx={{ p: 0, mr: 0.5 }}>
                      {isExpanded ? <ExpandLess /> : <ExpandMore />}
                    </IconButton>
                    <Typography
                      variant="caption"
                      sx={{
                        color: 'primary.main',
                        fontWeight: 600,
                        fontSize: '0.7rem',
                        textTransform: 'uppercase',
                      }}
                    >
                      &lt;{inj.tag}&gt;
                    </Typography>
                  </Box>

                  <Collapse in={isExpanded}>
                    <Box sx={{ px: 1.5, pb: 1.5 }}>
                      <Typography
                        variant="body2"
                        sx={{
                          fontFamily: 'monospace',
                          fontSize: '0.8rem',
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-word',
                          color: 'text.primary',
                          lineHeight: 1.5,
                        }}
                      >
                        {inj.content}
                      </Typography>
                    </Box>
                  </Collapse>
                </Paper>
              );
            })}
          </Collapse>
        </Box>
      )}

      {/* Your Input Section */}
      <ContentSection
        title="YOUR INPUT"
        icon={<Person />}
        content={remainingInput || (isClaudeCode ? '(Only injection tags, no additional input)' : memory.user_input)}
        color="primary"
        copyLabel="Copy input"
        maxHeight={250}
      />

      {/* AI Response Section */}
      <ContentSection
        title="AI RESPONSE"
        icon={<SmartToy />}
        content={memory.round_result || 'No response text available (may contain only tool calls or be empty)'}
        color="secondary"
        copyLabel="Copy response"
        maxHeight={400}
        sx={{ mt: 2 }}
      />
    </Box>
  );
};

export default MemoryDetailView;
