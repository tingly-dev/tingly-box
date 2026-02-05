import { useState, useMemo, useEffect } from 'react';
import {
  Box,
  Typography,
  Paper,
  Stack,
  Chip,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  TextField,
} from '@mui/material';
import {
  Description,
  FolderOpen,
  Search as SearchIcon,
  Delete,
  Close,
} from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import { RecordingCalendar } from '@/components/prompt';
import type { PromptRoundItem, ProtocolType } from '@/types/prompt';
import { useTranslation } from 'react-i18next';
import api from '@/services/api';

// Available scenarios for filtering
const SCENARIOS = [
  { value: '', label: 'All Scenarios' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'claude_code', label: 'Claude Code' },
  { value: 'opencode', label: 'OpenCode' },
  { value: 'openai', label: 'OpenAI' },
];

// Available protocols for filtering
const PROTOCOLS: { value: string | undefined; label: string }[] = [
  { value: undefined, label: 'All Protocols' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'google', label: 'Google' },
];

const UserPage = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [selectedDate, setSelectedDate] = useState(new Date());
  const [calendarDate, setCalendarDate] = useState(new Date());
  const [rangeMode, setRangeMode] = useState<number | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [scenarioFilter, setScenarioFilter] = useState<string>('');
  const [protocolFilter, setProtocolFilter] = useState<string | undefined>();
  const [promptRounds, setPromptRounds] = useState<PromptRoundItem[]>([]);
  const [filteredRounds, setFilteredRounds] = useState<PromptRoundItem[]>([]);
  const [selectedRound, setSelectedRound] = useState<PromptRoundItem | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [roundToDelete, setRoundToDelete] = useState<PromptRoundItem | null>(null);

  // Fetch prompt rounds from API
  useEffect(() => {
    const fetchPromptRounds = async () => {
      setLoading(true);
      try {
        const result = await api.getPromptUserInputs({ limit: 100 });
        if (result.success && result.data) {
          setPromptRounds(result.data);
        } else {
          console.error('Failed to fetch prompt rounds:', result.error);
          setPromptRounds([]);
        }
      } catch (error) {
        console.error('Error fetching prompt rounds:', error);
        setPromptRounds([]);
      } finally {
        setLoading(false);
      }
    };

    fetchPromptRounds();
  }, []);

  // Filter rounds based on selected date/range and filters
  useEffect(() => {
    let filtered = [...promptRounds];

    // Date range or single date filter
    if (rangeMode !== null) {
      const today = new Date();
      today.setHours(23, 59, 59, 999);
      const startDate = new Date(today);
      startDate.setDate(startDate.getDate() - rangeMode);
      startDate.setHours(0, 0, 0, 0);
      filtered = filtered.filter((round) => {
        const roundDate = new Date(round.created_at);
        return roundDate >= startDate && roundDate <= today;
      });
    } else {
      filtered = filtered.filter((round) => {
        const roundDate = new Date(round.created_at);
        return (
          roundDate.getDate() === selectedDate.getDate() &&
          roundDate.getMonth() === selectedDate.getMonth() &&
          roundDate.getFullYear() === selectedDate.getFullYear()
        );
      });
    }

    // Search filter
    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      filtered = filtered.filter(
        (round) =>
          round.user_input.toLowerCase().includes(query) ||
          round.round_result?.toLowerCase().includes(query) ||
          round.model.toLowerCase().includes(query)
      );
    }

    // Scenario filter
    if (scenarioFilter) {
      filtered = filtered.filter((round) => round.scenario === scenarioFilter);
    }

    // Protocol filter
    if (protocolFilter) {
      filtered = filtered.filter((round) => round.protocol === protocolFilter);
    }

    setFilteredRounds(filtered);
  }, [promptRounds, selectedDate, rangeMode, searchQuery, scenarioFilter, protocolFilter]);

  // Calculate recording counts per date for calendar
  const recordingCounts = useMemo(() => {
    const counts = new Map<string, number>();
    promptRounds.forEach((round) => {
      const date = new Date(round.created_at);
      const dateKey = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
      counts.set(dateKey, (counts.get(dateKey) || 0) + 1);
    });
    return counts;
  }, [promptRounds]);

  const handleDateSelect = (date: Date) => {
    setSelectedDate(date);
  };

  const handleRangeChange = (days: number | null) => {
    setRangeMode(days);
  };

  const handleViewDetails = (round: PromptRoundItem) => {
    setSelectedRound(round);
  };

  const handleDeleteClick = (round: PromptRoundItem) => {
    setRoundToDelete(round);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!roundToDelete) return;
    // Note: Individual delete is not implemented in API yet
    // For now, just remove from local state
    setPromptRounds(promptRounds.filter((r) => r.id !== roundToDelete.id));
    if (selectedRound?.id === roundToDelete.id) {
      setSelectedRound(null);
    }
    setDeleteDialogOpen(false);
    setRoundToDelete(null);
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
    setRoundToDelete(null);
  };

  // Get date label for header
  const getDateLabel = () => {
    if (rangeMode !== null) {
      return `Last ${rangeMode} days`;
    }
    return selectedDate.toLocaleDateString();
  };

  // Format user input for display
  const formatUserInput = (input: string, maxLength: number = 80) => {
    if (input.length <= maxLength) return input;
    return input.substring(0, maxLength) + '...';
  };

  return (
    <PageLayout loading={loading}>
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        {/* Header */}
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 3 }}>
          <Box>
            <Typography variant="h3" sx={{ fontWeight: 600, mb: 1 }}>
              Prompt Recordings
            </Typography>
            <Typography variant="body1" color="text.secondary">
              Browse and search your AI conversation history
            </Typography>
          </Box>
        </Box>

        {/* Search and Filter */}
        <Paper sx={{ p: 2, mb: 2 }}>
          <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
            {/* Search Input */}
            <TextField
              placeholder="Search prompts..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              InputProps={{
                startAdornment: <SearchIcon sx={{ mr: 1, color: 'text.secondary' }} />,
              }}
              sx={{ minWidth: 200, flex: 1, maxWidth: 300 }}
              size="small"
            />

            {/* Scenario Filter */}
            <FormControl size="small" sx={{ minWidth: 150 }}>
              <InputLabel>Scenario</InputLabel>
              <Select
                value={scenarioFilter}
                label="Scenario"
                onChange={(e) => setScenarioFilter(e.target.value)}
              >
                {SCENARIOS.map((scenario) => (
                  <MenuItem key={scenario.value} value={scenario.value}>
                    {scenario.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            {/* Protocol Filter */}
            <FormControl size="small" sx={{ minWidth: 150 }}>
              <InputLabel>Protocol</InputLabel>
              <Select
                value={protocolFilter || ''}
                label="Protocol"
                onChange={(e) => setProtocolFilter(e.target.value || undefined)}
              >
                {PROTOCOLS.map((protocol) => (
                  <MenuItem key={protocol.value || 'all'} value={protocol.value || ''}>
                    {protocol.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            {/* Active Filters Display */}
            {(scenarioFilter || protocolFilter) && (
              <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', ml: 'auto' }}>
                {scenarioFilter && (
                  <Chip
                    label={`Scenario: ${SCENARIOS.find((s) => s.value === scenarioFilter)?.label || scenarioFilter}`}
                    onDelete={() => setScenarioFilter('')}
                    size="small"
                    color="primary"
                    variant="outlined"
                  />
                )}
                {protocolFilter && (
                  <Chip
                    label={`Protocol: ${protocolFilter}`}
                    onDelete={() => setProtocolFilter(undefined)}
                    size="small"
                    color="primary"
                    variant="outlined"
                  />
                )}
                <Chip
                  label="Clear all"
                  onClick={() => {
                    setScenarioFilter('');
                    setProtocolFilter(undefined);
                  }}
                  size="small"
                  color="error"
                  variant="outlined"
                  sx={{ cursor: 'pointer' }}
                />
              </Box>
            )}
          </Box>
        </Paper>

        {/* Three-Column Layout */}
        <Stack direction="row" spacing={1} sx={{ height: 'calc(100vh - 220px)' }}>
          {/* Column 1: Calendar */}
          <Paper
            sx={{
              width: 320,
              display: 'flex',
              flexDirection: 'column',
              border: 1,
              borderColor: 'divider',
              borderRadius: 2,
              overflow: 'hidden',
            }}
          >
            <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                Calendar
              </Typography>
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto', p: 2 }}>
              <RecordingCalendar
                currentDate={calendarDate}
                selectedDate={selectedDate}
                recordingCounts={recordingCounts}
                onDateSelect={handleDateSelect}
                onMonthChange={setCalendarDate}
                onRangeChange={handleRangeChange}
              />
            </Box>
          </Paper>

          {/* Column 2: Rounds List */}
          <Paper
            sx={{
              width: 380,
              display: 'flex',
              flexDirection: 'column',
              border: 1,
              borderColor: 'divider',
              borderRadius: 2,
              overflow: 'hidden',
            }}
          >
            <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                {getDateLabel()} ({filteredRounds.length})
              </Typography>
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto' }}>
              {filteredRounds.length === 0 ? (
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
                  <FolderOpen sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
                  <Typography variant="body2" color="text.secondary">
                    {promptRounds.length === 0 ? 'No prompt recordings found' : 'No recordings match your filters'}
                  </Typography>
                </Box>
              ) : (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                  {filteredRounds.map((round) => (
                    <Paper
                      key={round.id}
                      onClick={() => handleViewDetails(round)}
                      sx={{
                        mx: 1,
                        mt: 1,
                        mb: 0,
                        p: 1.5,
                        border: '1px solid',
                        borderColor: selectedRound?.id === round.id ? 'primary.main' : 'divider',
                        borderRadius: 2,
                        cursor: 'pointer',
                        transition: 'all 0.2s',
                        backgroundColor: selectedRound?.id === round.id ? 'action.selected' : 'background.paper',
                        '&:hover': {
                          borderColor: 'primary.main',
                          boxShadow: 1,
                        },
                      }}
                    >
                      <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                        {/* Time */}
                        <Box sx={{ minWidth: 50, mt: 0.25 }}>
                          <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.primary', fontSize: '0.7rem' }}>
                            {new Date(round.created_at).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })}
                          </Typography>
                        </Box>

                        {/* Content */}
                        <Box sx={{ flex: 1, minWidth: 0 }}>
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: 500,
                              whiteSpace: 'nowrap',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              fontSize: '0.8rem',
                            }}
                          >
                            {formatUserInput(round.user_input)}
                          </Typography>
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.25, flexWrap: 'wrap' }}>
                            <Chip
                              label={round.protocol}
                              size="tiny"
                              sx={{
                                height: 16,
                                fontSize: '0.6rem',
                                borderRadius: 0.5,
                                backgroundColor: 'primary.100',
                                color: 'primary.dark',
                                fontWeight: 500,
                              }}
                            />
                            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                              {round.model}
                            </Typography>
                            {round.is_streaming && (
                              <Chip
                                label="stream"
                                size="tiny"
                                sx={{
                                  height: 16,
                                  fontSize: '0.6rem',
                                  borderRadius: 0.5,
                                  backgroundColor: 'info.100',
                                  color: 'info.dark',
                                  fontWeight: 500,
                                }}
                              />
                            )}
                            {round.has_tool_use && (
                              <Chip
                                label="tools"
                                size="tiny"
                                sx={{
                                  height: 16,
                                  fontSize: '0.6rem',
                                  borderRadius: 0.5,
                                  backgroundColor: 'warning.100',
                                  color: 'warning.dark',
                                  fontWeight: 500,
                                }}
                              />
                            )}
                          </Box>
                        </Box>

                        {/* Delete Button */}
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDeleteClick(round);
                          }}
                          sx={{ opacity: 0.6, '&:hover': { opacity: 1 } }}
                        >
                          <Delete sx={{ fontSize: 16 }} />
                        </IconButton>
                      </Box>
                    </Paper>
                  ))}
                </Box>
              )}
            </Box>
          </Paper>

          {/* Column 3: Round Detail */}
          <Paper
            sx={{
              flex: 1,
              display: 'flex',
              flexDirection: 'column',
              border: 1,
              borderColor: 'divider',
              borderRadius: 2,
              overflow: 'hidden',
            }}
          >
            <Box
              sx={{
                p: 2,
                borderBottom: 1,
                borderColor: 'divider',
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
              }}
            >
              <Typography
                variant="subtitle1"
                sx={{
                  fontWeight: 600,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {selectedRound ? 'Prompt Details' : 'Prompt Details'}
              </Typography>
              {selectedRound && (
                <IconButton size="small" onClick={() => setSelectedRound(null)}>
                  <Close />
                </IconButton>
              )}
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto', p: 2 }}>
              {!selectedRound ? (
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
                  <Description sx={{ fontSize: 64, color: 'text.disabled', mb: 2 }} />
                  <Typography variant="body2" color="text.secondary">
                    Select a prompt recording to view its details
                  </Typography>
                </Box>
              ) : (
                <Box>
                  <Box sx={{ display: 'flex', gap: 0.5, mb: 2 }}>
                    <Chip label={selectedRound.protocol} size="small" color="primary" variant="outlined" />
                    <Chip label={selectedRound.scenario} size="small" color="secondary" variant="outlined" />
                    {selectedRound.is_streaming && (
                      <Chip label="Streaming" size="small" color="info" variant="outlined" />
                    )}
                    {selectedRound.has_tool_use && (
                      <Chip label="Tool Use" size="small" color="warning" variant="outlined" />
                    )}
                  </Box>

                  <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5 }}>
                      Model
                    </Typography>
                    <Typography variant="body1">{selectedRound.model}</Typography>
                  </Box>

                  <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5 }}>
                      Provider
                    </Typography>
                    <Typography variant="body1">{selectedRound.provider_name}</Typography>
                  </Box>

                  {selectedRound.project_id && (
                    <Box sx={{ mb: 2 }}>
                      <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5 }}>
                        Project ID
                      </Typography>
                      <Typography variant="body1" sx={{ fontFamily: 'monospace', fontSize: '0.85rem' }}>
                        {selectedRound.project_id}
                      </Typography>
                    </Box>
                  )}

                  {selectedRound.session_id && (
                    <Box sx={{ mb: 2 }}>
                      <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5 }}>
                        Session ID
                      </Typography>
                      <Typography variant="body1" sx={{ fontFamily: 'monospace', fontSize: '0.85rem' }}>
                        {selectedRound.session_id}
                      </Typography>
                    </Box>
                  )}

                  <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5 }}>
                      Tokens
                    </Typography>
                    <Typography variant="body1">
                      Input: {selectedRound.input_tokens.toLocaleString()} | Output: {selectedRound.output_tokens.toLocaleString()} | Total:{' '}
                      {selectedRound.total_tokens.toLocaleString()}
                    </Typography>
                  </Box>

                  <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5 }}>
                      Time
                    </Typography>
                    <Typography variant="body1">{new Date(selectedRound.created_at).toLocaleString()}</Typography>
                  </Box>

                  <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5 }}>
                      Round Index
                    </Typography>
                    <Typography variant="body1">{selectedRound.round_index}</Typography>
                  </Box>

                  <Box sx={{ mb: 2 }}>
                    <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5 }}>
                      User Input
                    </Typography>
                    <Paper
                      variant="outlined"
                      sx={{
                        p: 1.5,
                        bgcolor: 'background.default',
                        maxHeight: 200,
                        overflow: 'auto',
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                      }}
                    >
                      <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                        {selectedRound.user_input}
                      </Typography>
                    </Paper>
                  </Box>

                  {selectedRound.round_result && (
                    <Box>
                      <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5 }}>
                        Round Result
                      </Typography>
                      <Paper
                        variant="outlined"
                        sx={{
                          p: 1.5,
                          bgcolor: 'background.default',
                          maxHeight: 300,
                          overflow: 'auto',
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-word',
                        }}
                      >
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                          {selectedRound.round_result}
                        </Typography>
                      </Paper>
                    </Box>
                  )}
                </Box>
              )}
            </Box>
          </Paper>
        </Stack>
      </Box>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onClose={handleDeleteCancel}>
        <DialogTitle>Delete Recording</DialogTitle>
        <DialogContent>
          <Typography variant="body1">
            Are you sure you want to delete this prompt recording? This action cannot be undone.
          </Typography>
          {roundToDelete && (
            <Paper variant="outlined" sx={{ mt: 2, p: 1.5, bgcolor: 'background.default' }}>
              <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                {formatUserInput(roundToDelete.user_input, 100)}
              </Typography>
            </Paper>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={handleDeleteCancel}>Cancel</Button>
          <Button onClick={handleDeleteConfirm} color="error" variant="contained">
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </PageLayout>
  );
};

export default UserPage;
