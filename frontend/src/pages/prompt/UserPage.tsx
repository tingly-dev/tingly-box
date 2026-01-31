import { useState, useMemo } from 'react';
import { Box, Typography, Grid, Paper } from '@mui/material';
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
  const [searchQuery, setSearchQuery] = useState('');
  const [userFilter, setUserFilter] = useState<string>();
  const [projectFilter, setProjectFilter] = useState<string>();
  const [typeFilter, setTypeFilter] = useState<RecordingType>();
  const [dateRange, setDateRange] = useState<[Date | null, Date | null]>();
  const [recordings, setRecordings] = useState<Recording[]>(mockRecordings);

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

  // Filter recordings based on selected date and filters
  const filteredRecordings = useMemo(() => {
    return recordings.filter((recording) => {
      const matchesDate =
        recording.timestamp.getDate() === selectedDate.getDate() &&
        recording.timestamp.getMonth() === selectedDate.getMonth() &&
        recording.timestamp.getFullYear() === selectedDate.getFullYear();

      const matchesSearch = searchQuery === '' ||
        recording.content.toLowerCase().includes(searchQuery.toLowerCase()) ||
        recording.summary?.toLowerCase().includes(searchQuery.toLowerCase());

      const matchesUser = !userFilter || recording.user.name === userFilter;
      const matchesProject = !projectFilter || recording.project === projectFilter;
      const matchesType = !typeFilter || recording.type === typeFilter;

      return matchesDate && matchesSearch && matchesUser && matchesProject && matchesType;
    });
  }, [recordings, selectedDate, searchQuery, userFilter, projectFilter, typeFilter]);

  const handleDateSelect = (date: Date) => {
    setSelectedDate(date);
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

        {/* Dual Panel Layout */}
        <Grid container spacing={2} sx={{ flex: 1, overflow: 'hidden' }}>
          <Grid item xs={12} md={4} sx={{ height: '100%' }}>
            <Paper sx={{ height: '100%', p: 2, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
              <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
                Calendar
              </Typography>
              <Box sx={{ flex: 1, overflow: 'auto' }}>
                <RecordingCalendar
                  currentDate={calendarDate}
                  selectedDate={selectedDate}
                  recordingCounts={recordingCounts}
                  onDateSelect={handleDateSelect}
                  onMonthChange={setCalendarDate}
                />
              </Box>
            </Paper>
          </Grid>
          <Grid item xs={12} md={8} sx={{ height: '100%' }}>
            <Paper sx={{ height: '100%', p: 2, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                <Typography variant="h6" sx={{ fontWeight: 600 }}>
                  {selectedDate.toLocaleDateString()}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {filteredRecordings.length} recording{filteredRecordings.length !== 1 ? 's' : ''}
                </Typography>
              </Box>
              <Box
                sx={{
                  flex: 1,
                  overflow: 'auto',
                  '&::-webkit-scrollbar': {
                    width: 8,
                    height: 8,
                  },
                  '&::-webkit-scrollbar-track': {
                    backgroundColor: 'grey.100',
                    borderRadius: 1,
                  },
                  '&::-webkit-scrollbar-thumb': {
                    backgroundColor: 'grey.300',
                    borderRadius: 1,
                    '&:hover': {
                      backgroundColor: 'grey.400',
                    },
                  },
                }}
              >
                <RecordingTimeline
                  recordings={filteredRecordings}
                  onPlay={handlePlay}
                  onViewDetails={handleViewDetails}
                  onDelete={handleDelete}
                />
              </Box>
            </Paper>
          </Grid>
        </Grid>
      </Box>
    </PageLayout>
  );
};

export default UserPage;
