import { useState, useMemo } from 'react';
import {
  Box,
  Typography,
  Paper,
  Stack,
} from '@mui/material';
import { Description, FolderOpen } from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import { RecordingCalendar, FilterBar, RecordingTimeline } from '@/components/prompt';
import type { Recording, RecordingType } from '@/types/prompt';
import { useTranslation } from 'react-i18next';

// Mock data - TODO: Replace with actual API calls
const mockRecordings: Recording[] = [
  {
    id: '1',
    timestamp: new Date(),
    user: { id: '1', name: 'John Doe' },
    project: 'tingly-box',
    type: 'code-review',
    content: 'Code review session for authentication module',
    duration: 120,
    model: 'claude-3-5-sonnet-20241022',
    summary: 'Reviewed auth implementation',
  },
  {
    id: '2',
    timestamp: new Date(Date.now() - 3600000),
    user: { id: '2', name: 'Jane Smith' },
    project: 'tingly-box',
    type: 'debug',
    content: 'Debug session for proxy routing',
    duration: 300,
    model: 'claude-3-5-sonnet-20241022',
    summary: 'Fixed routing bug',
  },
  {
    id: '3',
    timestamp: new Date(Date.now() - 86400000),
    user: { id: '1', name: 'John Doe' },
    project: 'tingly-box',
    type: 'refactor',
    content: 'Refactored load balancer logic',
    duration: 180,
    model: 'claude-3-5-sonnet-20241022',
    summary: 'Improved load balancer',
  },
  {
    id: '4',
    timestamp: new Date(Date.now() - 172800000),
    user: { id: '2', name: 'Jane Smith' },
    project: 'tingly-box',
    type: 'test',
    content: 'Added unit tests for smart routing',
    duration: 240,
    model: 'claude-3-5-sonnet-20241022',
    summary: 'Unit tests for routing',
  },
];

const UserPage = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [selectedDate, setSelectedDate] = useState(new Date());
  const [calendarDate, setCalendarDate] = useState(new Date());
  const [rangeMode, setRangeMode] = useState<number | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [userFilter, setUserFilter] = useState<string>();
  const [projectFilter, setProjectFilter] = useState<string>();
  const [typeFilter, setTypeFilter] = useState<RecordingType>();
  const [recordings, setRecordings] = useState<Recording[]>(mockRecordings);
  const [selectedRecording, setSelectedRecording] = useState<Recording | null>(null);

  // Get unique users and projects from recordings
  const { users, projects } = useMemo(() => {
    const uniqueUsers = Array.from(new Set(recordings.map((r) => r.user.name)));
    const uniqueProjects = Array.from(new Set(recordings.map((r) => r.project)));
    return { users: uniqueUsers, projects: uniqueProjects };
  }, [recordings]);

  // Calculate recording counts per date for calendar
  const recordingCounts = useMemo(() => {
    const counts = new Map<string, number>();
    recordings.forEach((recording) => {
      const dateKey = `${recording.timestamp.getFullYear()}-${String(
        recording.timestamp.getMonth() + 1
      ).padStart(2, '0')}-${String(recording.timestamp.getDate()).padStart(2, '0')}`;
      counts.set(dateKey, (counts.get(dateKey) || 0) + 1);
    });
    return counts;
  }, [recordings]);


  // Filter recordings based on selected date/range and filters
  const filteredRecordings = useMemo(() => {
    return recordings.filter((recording) => {
      // Date range or single date filter
      let matchesDate = true;
      if (rangeMode !== null) {
        const today = new Date();
        today.setHours(23, 59, 59, 999);
        const startDate = new Date(today);
        startDate.setDate(startDate.getDate() - rangeMode);
        startDate.setHours(0, 0, 0, 0);
        matchesDate = recording.timestamp >= startDate && recording.timestamp <= today;
      } else {
        matchesDate =
          recording.timestamp.getDate() === selectedDate.getDate() &&
          recording.timestamp.getMonth() === selectedDate.getMonth() &&
          recording.timestamp.getFullYear() === selectedDate.getFullYear();
      }

      const matchesSearch = searchQuery === '' ||
        recording.content.toLowerCase().includes(searchQuery.toLowerCase()) ||
        recording.summary?.toLowerCase().includes(searchQuery.toLowerCase());

      const matchesUser = !userFilter || recording.user.name === userFilter;
      const matchesProject = !projectFilter || recording.project === projectFilter;
      const matchesType = !typeFilter || recording.type === typeFilter;

      return matchesDate && matchesSearch && matchesUser && matchesProject && matchesType;
    });
  }, [recordings, selectedDate, rangeMode, searchQuery, userFilter, projectFilter, typeFilter]);

  const handleDateSelect = (date: Date) => {
    setSelectedDate(date);
  };

  const handleRangeChange = (days: number | null) => {
    setRangeMode(days);
  };

  const handlePlay = (recording: Recording) => {
    console.log('Play recording:', recording);
    // TODO: Implement playback functionality
  };

  const handleViewDetails = (recording: Recording) => {
    console.log('View details:', recording);
    // TODO: Implement details view
  };

  const handleDelete = (recording: Recording) => {
    console.log('Delete recording:', recording);
    // TODO: Implement delete with confirmation
    setRecordings(recordings.filter((r) => r.id !== recording.id));
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
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 3 }}>
          <Box>
            <Typography variant="h3" sx={{ fontWeight: 600, mb: 1 }}>
              User Recordings
            </Typography>
            <Typography variant="body1" color="text.secondary">
              Browse and manage your IDE recordings
            </Typography>
          </Box>
        </Box>

        {/* Search and Filter */}
        <Paper sx={{ p: 2, mb: 2 }}>
          <FilterBar
            searchQuery={searchQuery}
            onSearchChange={setSearchQuery}
            userFilter={userFilter}
            onUserFilterChange={setUserFilter}
            projectFilter={projectFilter}
            onProjectFilterChange={setProjectFilter}
            typeFilter={typeFilter}
            onTypeFilterChange={setTypeFilter}
            users={users}
            projects={projects}
          />
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

          {/* Column 2: Recordings List */}
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
                {getDateLabel()} ({filteredRecordings.length})
              </Typography>
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto' }}>
              {filteredRecordings.length === 0 ? (
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
                  <FolderOpen
                    sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }}
                  />
                  <Typography variant="body2" color="text.secondary">
                    No recordings found
                  </Typography>
                </Box>
              ) : (
                <RecordingTimeline
                  recordings={filteredRecordings}
                  onPlay={handlePlay}
                  onViewDetails={handleViewDetails}
                  onDelete={handleDelete}
                  onSelectRecording={setSelectedRecording}
                  selectedRecording={selectedRecording}
                />
              )}
            </Box>
          </Paper>

          {/* Column 3: Recording Detail */}
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
                {selectedRecording ? selectedRecording.summary : 'Recording Details'}
              </Typography>
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto', p: 2 }}>
              {!selectedRecording ? (
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
                  <Description
                    sx={{ fontSize: 64, color: 'text.disabled', mb: 2 }}
                  />
                  <Typography variant="body2" color="text.secondary">
                    Select a recording to view its details
                  </Typography>
                </Box>
              ) : (
                <Box>
                  <Typography variant="h6" sx={{ mb: 2 }}>
                    {selectedRecording.summary}
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                    User: {selectedRecording.user.name}
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                    Project: {selectedRecording.project}
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                    Type: {selectedRecording.type}
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                    Duration: {selectedRecording.duration}s
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                    Model: {selectedRecording.model}
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    Time: {selectedRecording.timestamp.toLocaleString()}
                  </Typography>
                  <Typography variant="body1" sx={{ mt: 2 }}>
                    {selectedRecording.content}
                  </Typography>
                </Box>
              )}
            </Box>
          </Paper>
        </Stack>
      </Box>
    </PageLayout>
  );
};

export default UserPage;
