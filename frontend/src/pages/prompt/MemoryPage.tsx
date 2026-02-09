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
  Popover,
  CircularProgress,
} from '@mui/material';
import {
  Description,
  FolderOpen,
  Search as SearchIcon,
  Refresh,
  Event,
} from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import {
  RecordingCalendar,
  RecordingTimeline,
  MemoryDetailView,
} from '@/components/prompt';
import type { PromptRoundListItem, SessionGroup } from '@/types/prompt';
import api from '@/services/api';
import { groupBySession, filterSessionGroups } from '@/utils/sessionUtils';

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

  const MemoryPage = () => {
  const [loading, setLoading] = useState(true);

  // List state - lightweight items
  const [memoryList, setMemoryList] = useState<PromptRoundListItem[]>([]);

  // Session grouped state
  const [sessionGroups, setSessionGroups] = useState<SessionGroup[]>([]);
  const [filteredSessionGroups, setFilteredSessionGroups] = useState<SessionGroup[]>([]);

  // Selection state
  const [selectedGroupId, setSelectedGroupId] = useState<string>('');
  const [selectedSession, setSelectedSession] = useState<SessionGroup | null>(null);

  const [selectedDate, setSelectedDate] = useState(new Date());
  const [calendarDate, setCalendarDate] = useState(new Date());
  const [rangeMode, setRangeMode] = useState<number | null>(null);
  const [calendarAnchorEl, setCalendarAnchorEl] = useState<HTMLElement | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [scenarioFilter, setScenarioFilter] = useState<string>('');
  const [protocolFilter, setProtocolFilter] = useState<string | undefined>();
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [memoryToDelete, setMemoryToDelete] = useState<PromptRoundListItem | null>(null);
  const [isRefreshing, setIsRefreshing] = useState(false);

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

  // Initial fetch and refetch when date/filter changes
  useEffect(() => {
    fetchMemoryList();
    // Clear selected session when list changes
    setSelectedSession(null);
    setSelectedGroupId('');
  }, [selectedDate, rangeMode, scenarioFilter, protocolFilter]);

  const handleRefresh = () => {
    setIsRefreshing(true);
    fetchMemoryList();
  };

  // Group memories by session
  useEffect(() => {
    const groups = groupBySession(memoryList);
    setSessionGroups(groups);
  }, [memoryList]);

  // Filter session groups based on search query
  useEffect(() => {
    const filtered = filterSessionGroups(sessionGroups, searchQuery);
    setFilteredSessionGroups(filtered);
  }, [sessionGroups, searchQuery]);

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

  const handleSelectGroup = (groupId: string) => {
    setSelectedGroupId(groupId);
    // Find the session from filtered groups (or all groups if not found)
    const session = filteredSessionGroups.find((g) => g.groupKey === groupId)
      || sessionGroups.find((g) => g.groupKey === groupId);
    if (session) {
      setSelectedSession(session);
    }
  };

  const handleDeleteConfirm = async () => {
    if (!memoryToDelete) return;
    // Note: Individual delete is not implemented in API yet
    // For now, just remove from local state
    setMemoryList(memoryList.filter((m) => m.id !== memoryToDelete.id));
    if (selectedSession) {
      setSelectedSession((prev) => {
        if (!prev) return null;
        return {
          ...prev,
          rounds: prev.rounds.filter((r) => r.id !== memoryToDelete.id),
          stats: {
            ...prev.stats,
            totalRounds: prev.stats.totalRounds - 1,
          },
        };
      });
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

  return (
    <PageLayout loading={loading}>
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        {/* Header */}
        {/* Unified Header Card with Search and Filters */}
        <Paper sx={{ p: 2, mb: 2 }}>
          {/* Top Row: Title + Actions */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1.5 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
              <Typography variant="h5" sx={{ fontWeight: 600 }}>
                Project Memory
              </Typography>
              {/* Active Filters */}
              {(scenarioFilter || protocolFilter || rangeMode !== null) && (
                <Box sx={{ display: 'flex', gap: 0.5 }}>
                  {rangeMode !== null && (
                    <Chip
                      label={`Last ${rangeMode} days`}
                      onDelete={() => setRangeMode(null)}
                      size="small"
                      color="info"
                      variant="outlined"
                    />
                  )}
                  {scenarioFilter && (
                    <Chip
                      label={SCENARIOS.find((s) => s.value === scenarioFilter)?.label || scenarioFilter}
                      onDelete={() => setScenarioFilter('')}
                      size="small"
                      color="primary"
                      variant="outlined"
                    />
                  )}
                  {protocolFilter && (
                    <Chip
                      label={protocolFilter}
                      onDelete={() => setProtocolFilter(undefined)}
                      size="small"
                      color="secondary"
                      variant="outlined"
                    />
                  )}
                </Box>
              )}
            </Box>

            {/* Actions */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              {/* Date Picker Button */}
              <Button
                variant="outlined"
                size="small"
                startIcon={<Event />}
                onClick={(e) => setCalendarAnchorEl(e.currentTarget)}
                sx={{ textTransform: 'none', minWidth: 'auto' }}
              >
                {rangeMode !== null
                  ? `Last ${rangeMode} days`
                  : selectedDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
              </Button>
              <IconButton
                onClick={handleRefresh}
                disabled={isRefreshing}
                size="small"
              >
                <Refresh sx={{ ...(isRefreshing && { animation: 'spin 1s linear infinite' }) }} />
              </IconButton>
            </Box>
          </Box>

          {/* Bottom Row: Search + Filters */}
          <Box sx={{ display: 'flex', gap: 1.5, alignItems: 'center', flexWrap: 'wrap' }}>
            {/* Search Input */}
            <TextField
              placeholder="Search in memories..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              slotProps={{
                input: {
                  startAdornment: <SearchIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 18 }} />,
                },
              }}
              sx={{ minWidth: 200, flex: 1, maxWidth: 320 }}
              size="small"
            />

            {/* Scenario Filter */}
            <FormControl size="small" sx={{ minWidth: 130 }}>
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
            <FormControl size="small" sx={{ minWidth: 130 }}>
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
          </Box>
        </Paper>

        {/* Calendar Popover */}
        <Popover
          open={Boolean(calendarAnchorEl)}
          anchorEl={calendarAnchorEl}
          onClose={() => setCalendarAnchorEl(null)}
          anchorOrigin={{
            vertical: 'bottom',
            horizontal: 'right',
          }}
          transformOrigin={{
            vertical: 'top',
            horizontal: 'right',
          }}
        >
          <Box sx={{ p: 2 }}>
            <RecordingCalendar
              currentDate={calendarDate}
              selectedDate={selectedDate}
              recordingCounts={memoryCounts}
              rangeMode={rangeMode}
              onDateSelect={(date) => {
                setSelectedDate(date);
                setCalendarAnchorEl(null);
              }}
              onMonthChange={setCalendarDate}
              onRangeChange={(days) => {
                setRangeMode(days);
                setCalendarAnchorEl(null);
              }}
            />
          </Box>
        </Popover>

        {/* Global styles for spin animation */}
        <style>{`
          @keyframes spin {
            from { transform: rotate(0deg); }
            to { transform: rotate(360deg); }
          }
        `}</style>

        {/* Two-Column Layout */}
        <Stack direction="row" spacing={1} sx={{ height: 'calc(100vh - 220px)' }}>
          {/* Column 1: Sessions List */}
          <Paper
            sx={{
              width: 400,
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
                {getDateLabel()} ({filteredSessionGroups.length} sessions)
              </Typography>
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto' }}>
              {loading ? (
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
                  <CircularProgress size={32} sx={{ mb: 2 }} />
                  <Typography variant="body2" color="text.secondary">
                    Loading sessions...
                  </Typography>
                </Box>
              ) : filteredSessionGroups.length === 0 ? (
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
                <RecordingTimeline
                  sessionGroups={filteredSessionGroups}
                  selectedGroupId={selectedGroupId}
                  onSelectGroup={handleSelectGroup}
                />
              )}
            </Box>
          </Paper>

          {/* Column 2: Memory Detail */}
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
            <Box sx={{ flex: 1, overflow: 'auto', p: 2 }}>
              {!selectedSession ? (
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
                    Select a session to view the conversation
                  </Typography>
                </Box>
              ) : (
                <MemoryDetailView session={selectedSession} onClose={() => {
                  setSelectedSession(null);
                  setSelectedGroupId('');
                }} />
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

export default MemoryPage;
