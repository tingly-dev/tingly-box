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
  Collapse,
  CircularProgress,
} from '@mui/material';
import {
  Description,
  FolderOpen,
  Search as SearchIcon,
  Delete,
  Close,
  Refresh,
  ExpandMore,
  ExpandLess,
} from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import { RecordingCalendar } from '@/components/prompt';
import type { PromptRoundItem, PromptRoundListItem } from '@/types/prompt';
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
  const [detailLoading, setDetailLoading] = useState(false);

  // List state - lightweight items
  const [memoryList, setMemoryList] = useState<PromptRoundListItem[]>([]);
  const [filteredMemories, setFilteredMemories] = useState<PromptRoundListItem[]>([]);

  // Detail state - full item data
  const [selectedMemory, setSelectedMemory] = useState<PromptRoundItem | null>(null);

  const [selectedDate, setSelectedDate] = useState(new Date());
  const [calendarDate, setCalendarDate] = useState(new Date());
  const [rangeMode, setRangeMode] = useState<number | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [scenarioFilter, setScenarioFilter] = useState<string>('');
  const [protocolFilter, setProtocolFilter] = useState<string | undefined>();
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [memoryToDelete, setMemoryToDelete] = useState<PromptRoundListItem | null>(null);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [injectionSectionExpanded, setInjectionSectionExpanded] = useState(false);
  const [expandedInjections, setExpandedInjections] = useState<Record<number, boolean>>({});

  // Fetch lightweight list from API
  const fetchMemoryList = async () => {
    setLoading(true);
    try {
      // Calculate date range for API call
      let startDate: Date | undefined;
      let endDate: Date | undefined;

      if (rangeMode !== null) {
        endDate = new Date();
        startDate = new Date();
        startDate.setDate(startDate.getDate() - rangeMode);
        startDate.setHours(0, 0, 0, 0);
      } else {
        startDate = new Date(selectedDate);
        startDate.setHours(0, 0, 0, 0);
        endDate = new Date(selectedDate);
        endDate.setHours(23, 59, 59, 999);
      }

      // TODO: Once backend implements /api/v1/memory/user-inputs/list endpoint,
      // use the following parameters:
      const result = await api.getMemoryUserInputsList({
        scenario: scenarioFilter || undefined,
        protocol: protocolFilter,
        start_date: startDate ? startDate.toISOString() : undefined,
        end_date: endDate ? endDate.toISOString() : undefined,
        limit: 100,
      });

      if (result.success && result.data) {
        // Backend returns { success: true, data: { rounds: [...], total: ... } }
        setMemoryList(result.data.rounds || []);
      } else {
        console.error('Failed to fetch memory list:', result.error);
        setMemoryList([]);
      }
    } catch (error) {
      console.error('Error fetching memory list:', error);
      setMemoryList([]);
    } finally {
      setLoading(false);
      setIsRefreshing(false);
    }
  };

  // Fetch full details for a specific memory
  const fetchMemoryDetail = async (id: number) => {
    setDetailLoading(true);
    try {
      const result = await api.getMemoryRoundDetail(id);
      if (result.success && result.data) {
        setSelectedMemory(result.data);
      } else {
        console.error('Failed to fetch memory detail:', result.error);
        setSelectedMemory(null);
      }
    } catch (error) {
      console.error('Error fetching memory detail:', error);
      setSelectedMemory(null);
    } finally {
      setDetailLoading(false);
    }
  };

  // Initial fetch and refetch when date/filter changes
  useEffect(() => {
    fetchMemoryList();
    // Clear selected memory when list changes
    setSelectedMemory(null);
  }, [selectedDate, rangeMode, scenarioFilter, protocolFilter]);

  const handleRefresh = () => {
    setIsRefreshing(true);
    fetchMemoryList();
  };

  // Filter memories based on selected date/range and filters
  // Note: With the new API flow, most filtering should happen on the backend
  // This is mainly for search query filtering since it's client-side
  useEffect(() => {
    let filtered = [...memoryList];

    // Search filter (client-side since backend doesn't support it yet)
    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      filtered = filtered.filter(
        (item) =>
          item.user_input_preview.toLowerCase().includes(query) ||
          item.model.toLowerCase().includes(query)
      );
    }

    setFilteredMemories(filtered);
  }, [memoryList, searchQuery]);

  // Calculate memory counts per date for calendar
  const memoryCounts = useMemo(() => {
    const counts = new Map<string, number>();
    memoryList.forEach((item) => {
      const date = new Date(item.created_at);
      const dateKey = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
      counts.set(dateKey, (counts.get(dateKey) || 0) + 1);
    });
    return counts;
  }, [memoryList]);

  const handleDateSelect = (date: Date) => {
    setSelectedDate(date);
  };

  const handleRangeChange = (days: number | null) => {
    setRangeMode(days);
  };

  const handleViewDetails = (memory: PromptRoundListItem) => {
    // Fetch full details when clicking an item
    fetchMemoryDetail(memory.id);
  };

  const handleDeleteClick = (memory: PromptRoundListItem) => {
    setMemoryToDelete(memory);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!memoryToDelete) return;
    // Note: Individual delete is not implemented in API yet
    // For now, just remove from local state
    setMemoryList(memoryList.filter((m) => m.id !== memoryToDelete.id));
    if (selectedMemory?.id === memoryToDelete.id) {
      setSelectedMemory(null);
    }
    setDeleteDialogOpen(false);
    setMemoryToDelete(null);
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
    setMemoryToDelete(null);
  };

  // Get date label for header
  const getDateLabel = () => {
    if (rangeMode !== null) {
      return `Last ${rangeMode} days`;
    }
    return selectedDate.toLocaleDateString();
  };

  // Parse Claude Code input to extract XML-like tagged injections
  // Returns { injections: [{tag, content}], remainingInput: string }
  const parseClaudeCodeInput = (input: string) => {
    const injections: { tag: string; content: string }[] = [];
    let remainingInput = input;

    // Match XML-like tags: <tagname>content</tagname>
    // This regex handles multiline content
    const tagRegex = /<([a-zA-Z_][a-zA-Z0-9_-]*)>([\s\S]*?)<\/\1>/g;
    let match;
    let lastIndex = 0;

    while ((match = tagRegex.exec(input)) !== null) {
      injections.push({
        tag: match[1],
        content: match[2].trim(),
      });
      lastIndex = match.index + match[0].length;
    }

    // Remove all tagged sections from the input to get the remaining content
    remainingInput = input.replace(tagRegex, '').trim();

    return { injections, remainingInput };
  };

  return (
    <PageLayout loading={loading}>
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        {/* Header */}
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 3 }}>
          <Box>
            <Typography variant="h3" sx={{ fontWeight: 600, mb: 1 }}>
              Project Memory
            </Typography>
            <Typography variant="body1" color="text.secondary">
              Your AI conversation history and project context
            </Typography>
          </Box>
          <IconButton
            onClick={handleRefresh}
            disabled={isRefreshing}
            sx={{ bgcolor: 'action.hover', '&:hover': { bgcolor: 'action.selected' } }}
          >
            <Refresh sx={{ ...(isRefreshing && { animation: 'spin 1s linear infinite' }) }} />
          </IconButton>
        </Box>

        {/* Global styles for spin animation */}
        <style>{`
          @keyframes spin {
            from { transform: rotate(0deg); }
            to { transform: rotate(360deg); }
          }
        `}</style>

        {/* Search and Filter */}
        <Paper sx={{ p: 2, mb: 2 }}>
          <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
            {/* Search Input */}
            <TextField
              placeholder="Search memories..."
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
                recordingCounts={memoryCounts}
                onDateSelect={handleDateSelect}
                onMonthChange={setCalendarDate}
                onRangeChange={handleRangeChange}
              />
            </Box>
          </Paper>

          {/* Column 2: Memories List */}
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
                {getDateLabel()} ({filteredMemories.length})
              </Typography>
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto' }}>
              {filteredMemories.length === 0 ? (
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
                    {memoryList.length === 0 ? 'No memories found' : 'No memories match your filters'}
                  </Typography>
                </Box>
              ) : (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                  {filteredMemories.map((memory) => (
                    <Paper
                      key={memory.id}
                      onClick={() => handleViewDetails(memory)}
                      sx={{
                        mx: 1,
                        mt: 1,
                        mb: 0,
                        p: 1.5,
                        border: '1px solid',
                        borderColor: selectedMemory?.id === memory.id ? 'primary.main' : 'divider',
                        borderRadius: 2,
                        cursor: 'pointer',
                        transition: 'all 0.2s',
                        backgroundColor: selectedMemory?.id === memory.id ? 'action.selected' : 'background.paper',
                        '&:hover': {
                          borderColor: 'primary.main',
                          boxShadow: 1,
                        },
                      }}
                    >
                      <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                        {/* Time */}
                        <Box sx={{ minWidth: 45, mt: 0.25, flexShrink: 0 }}>
                          <Typography variant="body2" sx={{ fontWeight: 500, color: 'text.secondary', fontSize: '0.7rem' }}>
                            {new Date(memory.created_at).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })}
                          </Typography>
                        </Box>

                        {/* Content */}
                        <Box sx={{ flex: 1, minWidth: 0 }}>
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: 500,
                              whiteSpace: 'pre-wrap',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              display: '-webkit-box',
                              WebkitLineClamp: 3,
                              WebkitBoxOrient: 'vertical',
                              fontSize: '0.8rem',
                              lineHeight: 1.4,
                              mb: 0.5,
                            }}
                          >
                            {memory.scenario === 'claude_code'
                              ? parseClaudeCodeInput(memory.user_input).remainingInput || memory.user_input.slice(0, 100)
                              : memory.user_input}
                          </Typography>

                          {/* Compact Meta Info */}
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexWrap: 'wrap' }}>
                            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem' }}>
                              {memory.model}
                            </Typography>
                            <Box sx={{ display: 'flex', gap: 0.25 }}>
                              {memory.is_streaming && (
                                <Typography variant="caption" sx={{ fontSize: '0.6rem', color: 'info.main', fontWeight: 500 }}>
                                  stream
                                </Typography>
                              )}
                              {memory.has_tool_use && (
                                <Typography variant="caption" sx={{ fontSize: '0.6rem', color: 'warning.main', fontWeight: 500 }}>
                                  tools
                                </Typography>
                              )}
                            </Box>
                          </Box>
                        </Box>

                        {/* Delete Button */}
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDeleteClick(memory);
                          }}
                          sx={{ opacity: 0.6, '&:hover': { opacity: 1 }, flexShrink: 0 }}
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

          {/* Column 3: Memory Detail */}
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
                {selectedMemory ? 'Memory Details' : 'Memory Details'}
              </Typography>
              {selectedMemory && (
                <IconButton size="small" onClick={() => setSelectedMemory(null)}>
                  <Close />
                </IconButton>
              )}
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto', p: 2 }}>
              {detailLoading ? (
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
                  <CircularProgress size={40} sx={{ mb: 2 }} />
                  <Typography variant="body2" color="text.secondary">
                    Loading memory details...
                  </Typography>
                </Box>
              ) : !selectedMemory ? (
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
                    Select a memory to view its details
                  </Typography>
                </Box>
              ) : (
                <Box>
                  {/* Compact Meta Row */}
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2, flexWrap: 'wrap' }}>
                    <Typography variant="caption" color="text.secondary">
                      {new Date(selectedMemory.created_at).toLocaleString()}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">•</Typography>
                    <Typography variant="caption" color="text.secondary">{selectedMemory.model}</Typography>
                    <Typography variant="caption" color="text.secondary">•</Typography>
                    <Typography variant="caption" color="text.secondary">{selectedMemory.provider_name}</Typography>
                    {selectedMemory.is_streaming && (
                      <>
                        <Typography variant="caption" color="text.secondary">•</Typography>
                        <Chip label="Streaming" size="small" sx={{ height: 20, fontSize: '0.65rem' }} />
                      </>
                    )}
                    {selectedMemory.has_tool_use && (
                      <>
                        <Typography variant="caption" color="text.secondary">•</Typography>
                        <Chip label="Tool Use" size="small" sx={{ height: 20, fontSize: '0.65rem' }} />
                      </>
                    )}
                  </Box>

                  {/* Token Info */}
                  <Box sx={{ mb: 3 }}>
                    <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
                      TOKENS
                    </Typography>
                    <Box sx={{ display: 'flex', gap: 3, mt: 0.5 }}>
                      <Box>
                        <Typography variant="caption" color="text.secondary">Input</Typography>
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>
                          {selectedMemory.input_tokens.toLocaleString()}
                        </Typography>
                      </Box>
                      <Box>
                        <Typography variant="caption" color="text.secondary">Output</Typography>
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>
                          {selectedMemory.output_tokens.toLocaleString()}
                        </Typography>
                      </Box>
                      <Box>
                        <Typography variant="caption" color="text.secondary">Total</Typography>
                        <Typography variant="body2" sx={{ fontWeight: 500, color: 'primary.main' }}>
                          {selectedMemory.total_tokens.toLocaleString()}
                        </Typography>
                      </Box>
                    </Box>
                  </Box>

                  {/* Context Info (collapsible) */}
                  <Box sx={{ mb: 3, p: 1.5, bgcolor: 'action.hover', borderRadius: 1 }}>
                    <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, mb: 0.5, display: 'block' }}>
                      CONTEXT
                    </Typography>
                    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2, mt: 0.5 }}>
                      <Box>
                        <Typography variant="caption" color="text.secondary">Protocol</Typography>
                        <Typography variant="caption" sx={{ ml: 0.5, fontWeight: 500 }}>{selectedMemory.protocol}</Typography>
                      </Box>
                      <Box>
                        <Typography variant="caption" color="text.secondary">Scenario</Typography>
                        <Typography variant="caption" sx={{ ml: 0.5, fontWeight: 500 }}>{selectedMemory.scenario}</Typography>
                      </Box>
                      {selectedMemory.project_id && (
                        <Box>
                          <Typography variant="caption" color="text.secondary">Project</Typography>
                          <Typography variant="caption" sx={{ ml: 0.5, fontFamily: 'monospace', fontWeight: 500 }}>
                            {selectedMemory.project_id.slice(0, 8)}...
                          </Typography>
                        </Box>
                      )}
                      {selectedMemory.session_id && (
                        <Box>
                          <Typography variant="caption" color="text.secondary">Session</Typography>
                          <Typography variant="caption" sx={{ ml: 0.5, fontFamily: 'monospace', fontWeight: 500 }}>
                            {selectedMemory.session_id.slice(0, 8)}...
                          </Typography>
                        </Box>
                      )}
                      <Box>
                        <Typography variant="caption" color="text.secondary">Round</Typography>
                        <Typography variant="caption" sx={{ ml: 0.5, fontWeight: 500 }}>#{selectedMemory.round_index}</Typography>
                      </Box>
                    </Box>
                  </Box>

                  {/* Parse input for Claude Code scenario */}
                  {(() => {
                    const isClaudeCode = selectedMemory.scenario === 'claude_code';
                    const { injections, remainingInput } = isClaudeCode
                      ? parseClaudeCodeInput(selectedMemory.user_input)
                      : { injections: [], remainingInput: selectedMemory.user_input };

                    return (
                      <>
                        {/* INPUT INJECTION - Separate section (only for claude_code with injections) */}
                        {injections.length > 0 && (
                          <Box sx={{ mb: 3 }}>
                            {/* Collapsible Header for Injection Section */}
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
                                INPUT INJECTION ({injections.length})
                              </Typography>
                            </Box>

                            {/* Collapsible Injection Cards */}
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
                                    {/* Collapsible Header for Each Injection */}
                                    <Box
                                      sx={{
                                        display: 'flex',
                                        alignItems: 'center',
                                        p: 1,
                                        cursor: 'pointer',
                                        '&:hover': { bgcolor: 'primary.100' },
                                      }}
                                      onClick={() =>
                                        setExpandedInjections((prev) => ({
                                          ...prev,
                                          [idx]: !prev[idx],
                                        }))
                                      }
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

                                    {/* Collapsible Content */}
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

                        {/* YOUR INPUT - Separate section */}
                        <Box sx={{ mb: 3 }}>
                          <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 1 }}>
                            YOUR INPUT
                          </Typography>
                          <Paper
                            variant="outlined"
                            sx={{
                              p: 2,
                              bgcolor: 'background.default',
                              maxHeight: 250,
                              overflow: 'auto',
                              whiteSpace: 'pre-wrap',
                              wordBreak: 'break-word',
                              minHeight: 60,
                            }}
                          >
                            {remainingInput ? (
                              <Typography variant="body1" sx={{ fontFamily: 'inherit', fontSize: '0.9rem', lineHeight: 1.6 }}>
                                {remainingInput}
                              </Typography>
                            ) : (
                              <Typography variant="body2" sx={{ color: 'text.disabled', fontStyle: 'italic' }}>
                                {isClaudeCode ? '(Only injection tags, no additional input)' : selectedMemory.user_input}
                              </Typography>
                            )}
                          </Paper>
                        </Box>
                      </>
                    );
                  })()}

                  {/* AI RESPONSE - Separate section */}
                  <Box>
                    <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 600, mb: 1 }}>
                      AI RESPONSE
                    </Typography>
                    <Paper
                      variant="outlined"
                      sx={{
                        p: 2,
                        bgcolor: 'background.default',
                        maxHeight: 400,
                        overflow: 'auto',
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                        minHeight: 80,
                      }}
                    >
                      {selectedMemory.round_result ? (
                        <Typography variant="body1" sx={{ fontFamily: 'inherit', fontSize: '0.9rem', lineHeight: 1.6 }}>
                          {selectedMemory.round_result}
                        </Typography>
                      ) : (
                        <Typography variant="body2" sx={{ color: 'text.disabled', fontStyle: 'italic' }}>
                          No response text available (may contain only tool calls or be empty)
                        </Typography>
                      )}
                    </Paper>
                  </Box>
                </Box>
              )}
            </Box>
          </Paper>
        </Stack>
      </Box>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onClose={handleDeleteCancel}>
        <DialogTitle>Delete Memory</DialogTitle>
        <DialogContent>
          <Typography variant="body1">
            Are you sure you want to delete this memory? This action cannot be undone.
          </Typography>
          {memoryToDelete && (
            <Paper variant="outlined" sx={{ mt: 2, p: 1.5, bgcolor: 'background.default' }}>
              <Typography
                variant="body2"
                sx={{
                  whiteSpace: 'pre-wrap',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  display: '-webkit-box',
                  WebkitLineClamp: 3,
                  WebkitBoxOrient: 'vertical',
                }}
              >
                {memoryToDelete.user_input}
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
