import { useState, useCallback, useRef, useEffect } from 'react';
import {
  Box,
  Typography,
  Paper,
  IconButton,
  Tooltip,
  Chip,
  Avatar,
  Divider,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Collapse,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
} from '@mui/material';
import {
  Close,
  ContentCopy,
  Check,
  SmartToy,
  OpenInFull,
  ExpandMore,
  ExpandLess,
} from '@mui/icons-material';
import type { SessionGroup, MemorySessionItem } from '@/types/prompt';
import type { PromptRoundListItem } from '@/types/prompt';

interface SessionDetailViewProps {
  session?: SessionGroup;  // Legacy support
  sessionItem?: MemorySessionItem;  // New session-based API
  rounds?: PromptRoundListItem[];  // Rounds array when using sessionItem
  onClose: () => void;
}

interface RoundPairModalProps {
  round: PromptRoundListItem;
  index: number;
  open: boolean;
  onClose: () => void;
}

// Parse Claude Code input to extract XML-like tagged injections
const parseClaudeCodeInput = (
  input: string
): { injections: { tag: string; content: string }[]; remainingInput: string } => {
  const injections: { tag: string; content: string }[] = [];
  const tagRegex = /<([a-zA-Z_][a-zA-Z0-9_-]*)>([\s\S]*?)<\/\1>/g;
  let match;

  while ((match = tagRegex.exec(input)) !== null) {
    injections.push({
      tag: match[1],
      content: match[2].trim(),
    });
  }

  const remainingInput = input.replace(tagRegex, '').trim();

  return { injections, remainingInput };
};

// Injection Section Component for displaying parsed injections
interface InjectionSectionProps {
  injections: { tag: string; content: string }[];
}

const InjectionSection: React.FC<InjectionSectionProps> = ({ injections }) => {
  const [expandedInjections, setExpandedInjections] = useState<Record<number, boolean>>({});

  const toggleInjection = useCallback((idx: number) => {
    setExpandedInjections((prev) => ({
      ...prev,
      [idx]: !prev[idx],
    }));
  }, []);

  if (injections.length === 0) return null;

  return (
    <Box sx={{ mb: 1.5 }}>
      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, mb: 1, display: 'block' }}>
        INPUT INJECTIONS ({injections.length})
      </Typography>
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
    </Box>
  );
};

const RoundPairModal: React.FC<RoundPairModalProps> = ({ round, index, open, onClose }) => {
  const [copiedSection, setCopiedSection] = useState<string | null>(null);

  const handleCopy = useCallback((content: string, section: string) => {
    navigator.clipboard.writeText(content);
    setCopiedSection(section);
    setTimeout(() => setCopiedSection(null), 2000);
  }, []);

  const getContentStats = (content: string): { words: number; chars: number } => {
    const words = content.trim().split(/\s+/).filter(Boolean).length;
    const chars = content.length;
    return { words, chars };
  };

  const inputStats = getContentStats(round.user_input);
  // Use round_result if available (from session rounds API), otherwise fall back to round_result_preview
  const roundResult = round.round_result || round.round_result_preview;
  const outputStats = roundResult ? getContentStats(roundResult) : null;

  // Parse injections from user input (for claude_code scenario)
  const isClaudeCode = round.scenario === 'claude_code';
  const { injections, remainingInput } = isClaudeCode
    ? parseClaudeCodeInput(round.user_input)
    : { injections: [], remainingInput: round.user_input };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      PaperProps={{
        sx: { maxHeight: '85vh', display: 'flex', flexDirection: 'column' },
      }}
    >
      <DialogTitle>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          {/* First row: Round number and close button */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Typography variant="h6" sx={{ fontWeight: 600 }}>
              Round {index + 1}
            </Typography>
            <IconButton size="small" onClick={onClose}>
              <Close />
            </IconButton>
          </Box>
          {/* Second row: Session and scenario info */}
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
            {/* Session ID */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Typography variant="caption" color="text.secondary">
                Session:
              </Typography>
              <Typography
                variant="caption"
                sx={{
                  fontFamily: 'monospace',
                  color: 'primary.main',
                  fontWeight: 500,
                }}
              >
                {round.session_id || 'N/A'}
              </Typography>
            </Box>
            {/* Scenario */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Typography variant="caption" color="text.secondary">
                Scenario:
              </Typography>
              <Chip
                label={round.scenario || 'N/A'}
                size="small"
                sx={{ height: 18, fontSize: '0.65rem' }}
              />
            </Box>
            {/* Protocol */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Typography variant="caption" color="text.secondary">
                Protocol:
              </Typography>
              <Typography variant="caption" sx={{ fontWeight: 500 }}>
                {round.protocol || 'N/A'}
              </Typography>
            </Box>
            {/* Flags */}
            {round.is_streaming && (
              <Chip label="streaming" size="small" sx={{ height: 18, fontSize: '0.65rem' }} />
            )}
            {round.has_tool_use && (
              <Chip label="tool use" size="small" color="warning" sx={{ height: 18, fontSize: '0.65rem' }} />
            )}
            {/* Model */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Typography variant="caption" color="text.secondary">
                Model:
              </Typography>
              <Typography variant="caption" sx={{ fontWeight: 500 }}>
                {round.model || 'N/A'}
              </Typography>
            </Box>
          </Box>
        </Box>
      </DialogTitle>
      <DialogContent sx={{ flex: 1, overflow: 'auto' }}>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {/* User Input */}
          <Box>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600, color: 'primary.main' }}>
                  You
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {inputStats.words} words · {inputStats.chars} chars
                </Typography>
              </Box>
              <Tooltip title={copiedSection === 'input' ? 'Copied!' : 'Copy'}>
                <IconButton
                  size="small"
                  onClick={() => handleCopy(round.user_input, 'input')}
                >
                  {copiedSection === 'input' ? (
                    <Check fontSize="small" color="success" />
                  ) : (
                    <ContentCopy fontSize="small" />
                  )}
                </IconButton>
              </Tooltip>
            </Box>

            {/* Injections Section (for claude_code) */}
            {injections.length > 0 && (
              <InjectionSection injections={injections} />
            )}

            {/* Remaining Input */}
            <Paper
              sx={{
                p: 2,
                bgcolor: 'grey.50',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 2,
              }}
            >
              <Typography
                variant="body1"
                sx={{
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  lineHeight: 1.6,
                  color: 'text.primary',
                }}
              >
                {remainingInput || (isClaudeCode && injections.length > 0 ? '(Only injection tags, no additional input)' : round.user_input)}
              </Typography>
            </Paper>
          </Box>

          {/* AI Response */}
          {roundResult && (
            <Box>
              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                  <Avatar
                    sx={{
                      width: 20,
                      height: 20,
                      fontSize: '0.65rem',
                    }}
                  >
                    <SmartToy sx={{ fontSize: 12 }} />
                  </Avatar>
                  <Typography variant="subtitle2" sx={{ fontWeight: 600, color: 'success.main' }}>
                    AI
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {outputStats?.words || 0} words · {outputStats?.chars || 0} chars
                  </Typography>
                </Box>
                <Tooltip title={copiedSection === 'output' ? 'Copied!' : 'Copy'}>
                  <IconButton
                    size="small"
                    onClick={() => handleCopy(roundResult || '', 'output')}
                  >
                    {copiedSection === 'output' ? (
                      <Check fontSize="small" color="success" />
                    ) : (
                      <ContentCopy fontSize="small" />
                    )}
                  </IconButton>
                </Tooltip>
              </Box>
              <Paper
                sx={{
                  p: 2,
                  bgcolor: 'grey.50',
                  border: '1px solid',
                  borderColor: 'divider',
                  borderRadius: 2,
                }}
              >
                <Typography
                  variant="body1"
                  sx={{
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                    lineHeight: 1.6,
                    color: 'text.primary',
                  }}
                >
                  {roundResult}
                </Typography>
              </Paper>
            </Box>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
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

const SessionDetailView: React.FC<SessionDetailViewProps> = ({ session, sessionItem, rounds, onClose }) => {
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [selectedRoundIndex, setSelectedRoundIndex] = useState<number | null>(null);
  const [activeRoundIndex, setActiveRoundIndex] = useState<number | null>(null);
  const roundRefs = useRef<(HTMLDivElement | null)[]>([]);

  // Use new props if available, otherwise fall back to legacy props
  const isLegacy = !sessionItem && !!session;
  const currentSessionItem = sessionItem;
  const currentRounds = rounds || session?.rounds || [];

  // Build a compatible session-like object for rendering
  const sessionData: SessionGroup = session || {
    groupKey: sessionItem?.id || '',
    account: {
      id: sessionItem?.account_id || '',
      name: sessionItem?.account_name || '',
    },
    sessionId: sessionItem?.session_id || '',
    projectId: undefined,
    rounds: currentRounds,
    stats: sessionItem ? {
      totalRounds: sessionItem.total_rounds,
      totalTokens: sessionItem.total_tokens,
      firstMessageTime: sessionItem.created_at,
      lastMessageTime: sessionItem.created_at,
      models: [sessionItem.model],
      scenario: sessionItem.scenario,
    } : session?.stats || {
      totalRounds: 0,
      totalTokens: 0,
      firstMessageTime: '',
      lastMessageTime: '',
      models: [],
      scenario: '',
    },
  };

  const handleCopy = useCallback((content: string, id: string) => {
    navigator.clipboard.writeText(content);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  }, []);

  const handleRoundClick = useCallback((index: number) => {
    setSelectedRoundIndex(index);
  }, []);

  const handleCloseModal = useCallback(() => {
    setSelectedRoundIndex(null);
  }, []);

  // Scroll to a specific round
  const scrollToRound = useCallback((index: number) => {
    const element = roundRefs.current[index];
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'start' });
      setSelectedRoundIndex(null);
    }
  }, []);

  // Track visible round for outline highlighting
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            const index = Number(entry.target.dataset.index);
            if (!isNaN(index)) {
              setActiveRoundIndex(index);
            }
          }
        });
      },
      { threshold: 0.5, rootMargin: '-10% 0px -60% 0px' }
    );

    roundRefs.current.forEach((ref) => {
      if (ref) observer.observe(ref);
    });

    return () => observer.disconnect();
  }, [currentRounds]);

  const scenarioConfig = SCENARIO_CONFIG[sessionData.stats.scenario] || {
    color: 'default',
    label: sessionData.stats.scenario,
  };

  const selectedRound = selectedRoundIndex !== null ? currentRounds[selectedRoundIndex] : null;

  return (
    <Box>
      {/* Round Pair Modal */}
      {selectedRound && (
        <RoundPairModal
          round={selectedRound}
          index={selectedRoundIndex!}
          open={true}
          onClose={handleCloseModal}
        />
      )}

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
            {getInitials(sessionData.account.name || sessionData.account.id)}
          </Avatar>

          {/* Session Info */}
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.25 }}>
              <Typography variant="h6" sx={{ fontWeight: 600 }}>
                {sessionData.account.name || sessionData.account.id}
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
              {sessionData.sessionId.slice(-12)} · {sessionData.stats.totalRounds} messages · {formatTokens(sessionData.stats.totalTokens)} tokens
            </Typography>
          </Box>
        </Box>

        <IconButton size="small" onClick={onClose}>
          <Close />
        </IconButton>
      </Box>

      {/* Chat-style Messages with Outline */}
      <Box
        sx={{
          flex: 1,
          display: 'flex',
          gap: 2,
          overflow: 'hidden',
        }}
      >
        {/* Outline Sidebar */}
        <Paper
          variant="outlined"
          sx={{
            width: 200,
            flexShrink: 0,
            overflow: 'hidden',
            display: 'flex',
            flexDirection: 'column',
            borderColor: 'divider',
          }}
        >
          <Box sx={{ p: 1.5, borderBottom: 1, borderColor: 'divider', bgcolor: 'grey.50' }}>
            <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
              OUTLINE
            </Typography>
          </Box>
          <List sx={{ flex: 1, overflow: 'auto', py: 0.5 }}>
            {currentRounds.map((round, index) => (
              <ListItem
                key={round.id}
                disablePadding
                sx={{ display: 'block' }}
              >
                <ListItemButton
                  dense
                  selected={activeRoundIndex === index}
                  onClick={() => scrollToRound(index)}
                  sx={{
                    py: 1,
                    px: 1.5,
                    borderRadius: 1,
                    mx: 0.5,
                    mb: 0.25,
                    '&.Mui-selected': {
                      bgcolor: 'primary.50',
                      '&:hover': {
                        bgcolor: 'primary.100',
                      },
                    },
                  }}
                >
                  <Box sx={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.25 }}>
                      <Typography
                        variant="caption"
                        sx={{
                          fontWeight: activeRoundIndex === index ? 600 : 500,
                          fontSize: '0.65rem',
                          color: activeRoundIndex === index ? 'primary.main' : 'text.secondary',
                          minWidth: 20,
                        }}
                      >
                        {index + 1}
                      </Typography>
                      {round.is_streaming && (
                        <Typography variant="caption" sx={{ fontSize: '0.6rem', color: 'info.main' }}>
                          stream
                        </Typography>
                      )}
                    </Box>
                    <Typography
                      variant="body2"
                      sx={{
                        fontSize: '0.75rem',
                        color: 'text.secondary',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                        display: 'block',
                      }}
                      title={round.user_input}
                    >
                      {round.user_input}
                    </Typography>
                  </Box>
                </ListItemButton>
              </ListItem>
            ))}
          </List>
        </Paper>

        {/* Messages */}
        <Box
          sx={{
            flex: 1,
            overflow: 'auto',
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
          }}
        >
        {currentRounds.map((round, index) => {
          const inputStats = getContentStats(round.user_input);
          // Use round_result if available (from session rounds API), otherwise fall back to round_result_preview
          const roundResult = round.round_result || round.round_result_preview;
          const outputStats = roundResult ? getContentStats(roundResult) : null;

          // Parse injections for preview display
          const isClaudeCode = round.scenario === 'claude_code';
          const { injections, remainingInput } = isClaudeCode
            ? parseClaudeCodeInput(round.user_input)
            : { injections: [], remainingInput: round.user_input };
          const hasInjections = injections.length > 0;

          return (
            <Box
              key={round.id}
              ref={(el) => { roundRefs.current[index] = el; }}
              data-index={index}
            >
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
                    {hasInjections && (
                      <Chip label={`${injections.length} injections`} size="small" sx={{ height: 16, fontSize: '0.6rem', bgcolor: 'primary.light', color: 'primary.dark' }} />
                    )}
                    {round.is_streaming && (
                      <Chip label="streaming" size="small" sx={{ height: 16, fontSize: '0.6rem' }} />
                    )}
                    <Tooltip title="Open full conversation">
                      <IconButton
                        size="small"
                        onClick={() => handleRoundClick(index)}
                        sx={{ p: 0.5 }}
                      >
                        <OpenInFull sx={{ fontSize: 14 }} />
                      </IconButton>
                    </Tooltip>
                  </Box>

                  <Paper
                    sx={{
                      p: 1.5,
                      px: 2,
                      bgcolor: 'grey.50',
                      border: '1px solid',
                      borderColor: 'divider',
                      borderRadius: '12px 12px 0 12px',
                      maxWidth: '100%',
                      cursor: 'pointer',
                      transition: 'filter 0.2s',
                      '&:hover': {
                        borderColor: 'primary.main',
                      },
                    }}
                    onClick={() => handleRoundClick(index)}
                  >
                    {/* Show injection indicator */}
                    {hasInjections && (
                      <Box sx={{ mb: 0.5 }}>
                        <Typography variant="caption" sx={{ color: 'primary.main', fontSize: '0.65rem', fontWeight: 600 }}>
                          &lt;{injections.map(i => i.tag).join('&gt; &lt;')}&gt;
                        </Typography>
                      </Box>
                    )}
                    <Typography
                      variant="body1"
                      sx={{
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                        fontSize: '0.9rem',
                        lineHeight: 1.5,
                        color: 'text.primary',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        display: '-webkit-box',
                        WebkitLineClamp: 6,
                        WebkitBoxOrient: 'vertical',
                      }}
                    >
                      {remainingInput || round.user_input}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem', mt: 0.5, display: 'block' }}>
                      {inputStats.words} words · {inputStats.chars} chars
                    </Typography>
                  </Paper>
                </Box>
              </Box>

              {/* AI Response */}
              {roundResult && (
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
                      <Tooltip title="Open full conversation">
                        <IconButton
                          size="small"
                          onClick={() => handleRoundClick(index)}
                          sx={{ p: 0.5 }}
                        >
                          <OpenInFull sx={{ fontSize: 14 }} />
                        </IconButton>
                      </Tooltip>
                    </Box>

                    <Paper
                      sx={{
                        p: 1.5,
                        px: 2,
                        bgcolor: 'grey.50',
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: '0 12px 12px 12px',
                        maxWidth: '100%',
                        cursor: 'pointer',
                        transition: 'filter 0.2s',
                        '&:hover': {
                          borderColor: 'primary.main',
                        },
                      }}
                      onClick={() => handleRoundClick(index)}
                    >
                      <Typography
                        variant="body1"
                        sx={{
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-word',
                          fontSize: '0.9rem',
                          lineHeight: 1.5,
                          color: 'text.primary',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          display: '-webkit-box',
                          WebkitLineClamp: 6,
                          WebkitBoxOrient: 'vertical',
                        }}
                      >
                        {roundResult}
                      </Typography>
                      <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem', mt: 0.5, display: 'block' }}>
                        {outputStats?.words || 0} words · {outputStats?.chars || 0} chars
                      </Typography>
                    </Paper>
                  </Box>
                </Box>
              )}

              {/* Divider between rounds */}
              {index < currentRounds.length - 1 && (
                <Divider sx={{ my: 1, opacity: 0.5 }} />
              )}
            </Box>
          );
        })}
        </Box>
      </Box>
    </Box>
  );
};

export default SessionDetailView;
