import { useState, useEffect, useCallback, useMemo } from 'react';
import {
  Box,
  Typography,
  Paper,
  Stack,
  CircularProgress,
  IconButton,
  Tooltip,
} from '@mui/material';
import {
  Refresh,
  Description,
  FolderOpen,
} from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import {
  MemorySearchBar,
  FilterPanel,
  SearchResultsList,
} from '@/components/prompt/memory';
import { MemoryDetailView } from '@/components/prompt';
import type {
  PromptRoundListItem,
  MemorySessionItem,
} from '@/types/prompt';
import api from '@/services/api';

const MemoryPage = () => {
  // Loading states
  const [loading, setLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [loadingRounds, setLoadingRounds] = useState(false);

  // Search state
  const [searchQuery, setSearchQuery] = useState('');
  const [recentSearches, setRecentSearches] = useState<string[]>(() => {
    try {
      const saved = localStorage.getItem('memory_recent_searches');
      return saved ? JSON.parse(saved) : [];
    } catch {
      return [];
    }
  });

  // Filter state
  const [filters, setFilters] = useState({
    scenario: '',
    protocol: undefined as string | undefined,
    dateRange: {
      mode: 'range' as 'all' | 'range' | 'date',
      rangeDays: 30,
    },
  });

  // Session list state
  const [sessionList, setSessionList] = useState<MemorySessionItem[]>([]);

  // Rounds cache (session_id -> rounds)
  const [roundsBySession, setRoundsBySession] = useState<Map<string, PromptRoundListItem[]>>(new Map());

  // Selected session state
  const [selectedSessionId, setSelectedSessionId] = useState<string>('');
  const [selectedSessionItem, setSelectedSessionItem] = useState<MemorySessionItem | null>(null);
  const [sessionRounds, setSessionRounds] = useState<PromptRoundListItem[]>([]);

  // Fetch session list from API
  const fetchSessionList = useCallback(async () => {
    setLoading(true);
    try {
      let startDate: Date | undefined;
      let endDate: Date | undefined;

      if (filters.dateRange.mode === 'range' && filters.dateRange.rangeDays !== undefined) {
        endDate = new Date();
        startDate = new Date();
        startDate.setDate(startDate.getDate() - filters.dateRange.rangeDays);
        startDate.setHours(0, 0, 0, 0);
      } else if (filters.dateRange.mode === 'date' && filters.dateRange.selectedDate) {
        startDate = new Date(filters.dateRange.selectedDate);
        startDate.setHours(0, 0, 0, 0);
        endDate = new Date(filters.dateRange.selectedDate);
        endDate.setHours(23, 59, 59, 999);
      }

      const result = await api.getMemorySessions({
        start_date: startDate ? startDate.toISOString() : undefined,
        end_date: endDate ? endDate.toISOString() : undefined,
        limit: 200,
      });

      if (result.success && result.data) {
        setSessionList(result.data.sessions || []);
        // Pre-fetch rounds for search
        await prefetchRoundsForSearch(result.data.sessions || []);
      } else {
        console.error('Failed to fetch session list:', result.error);
        setSessionList([]);
      }
    } catch (error) {
      console.error('Error fetching session list:', error);
      setSessionList([]);
    } finally {
      setLoading(false);
      setIsRefreshing(false);
    }
  }, [filters.dateRange]);

  // Pre-fetch rounds for all sessions (for client-side search)
  const prefetchRoundsForSearch = async (sessions: MemorySessionItem[]) => {
    const newRoundsMap = new Map<string, PromptRoundListItem[]>();
    const promises = sessions.slice(0, 50).map(async (session) => {
      try {
        const result = await api.getMemorySessionRounds(session.session_id, { limit: 100 });
        if (result.success && result.data) {
          newRoundsMap.set(session.session_id, result.data);
        }
      } catch (e) {
        // Silent fail for prefetch
      }
    });
    await Promise.all(promises);
    setRoundsBySession(newRoundsMap);
  };

  // Fetch rounds for a specific session
  const fetchSessionRounds = useCallback(async (sessionId: string) => {
    setLoadingRounds(true);
    try {
      const result = await api.getMemorySessionRounds(sessionId, { limit: 100 });

      if (result.success && result.data) {
        setSessionRounds(result.data);
        // Update cache
        setRoundsBySession((prev) => {
          const next = new Map(prev);
          next.set(sessionId, result.data);
          return next;
        });
      } else {
        console.error('Failed to fetch session rounds:', result.error);
        setSessionRounds([]);
      }
    } catch (error) {
      console.error('Error fetching session rounds:', error);
      setSessionRounds([]);
    } finally {
      setLoadingRounds(false);
    }
  }, []);

  // Initial fetch and refetch when filters change
  useEffect(() => {
    fetchSessionList();
    // Clear selected session when list changes
    setSelectedSessionItem(null);
    setSelectedSessionId('');
    setSessionRounds([]);
  }, [filters.dateRange]);

  const handleRefresh = () => {
    setIsRefreshing(true);
    fetchSessionList();
  };

  // Handle search
  const handleSearch = useCallback((query: string) => {
    setSearchQuery(query);
    // Save to recent searches
    if (query.trim()) {
      setRecentSearches((prev) => {
        const next = [query, ...prev.filter((s) => s !== query)].slice(0, 10);
        localStorage.setItem('memory_recent_searches', JSON.stringify(next));
        return next;
      });
    }
  }, []);

  // Clear recent searches
  const handleClearRecent = useCallback(() => {
    setRecentSearches([]);
    localStorage.removeItem('memory_recent_searches');
  }, []);

  // Handle session selection
  const handleSelectSession = useCallback((session: MemorySessionItem) => {
    setSelectedSessionId(session.id);
    setSelectedSessionItem(session);
    fetchSessionRounds(session.session_id);
  }, [fetchSessionRounds]);

  // Filter sessions by scenario/protocol
  const filteredSessions = useMemo(() => {
    let filtered = sessionList;

    if (filters.scenario) {
      filtered = filtered.filter((s) => s.scenario === filters.scenario);
    }

    if (filters.protocol) {
      filtered = filtered.filter((s) => s.protocol === filters.protocol);
    }

    return filtered;
  }, [sessionList, filters.scenario, filters.protocol]);

  return (
    <PageLayout loading={loading}>
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        {/* Search Header */}
        <Paper sx={{ p: 2, mb: 2 }}>
          <Stack spacing={1.5}>
            {/* Title Row */}
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <Typography variant="h5" sx={{ fontWeight: 600 }}>
                Project Memory
              </Typography>
              <Tooltip title="Refresh">
                <IconButton
                  onClick={handleRefresh}
                  disabled={isRefreshing}
                  size="small"
                >
                  <Refresh sx={{ ...(isRefreshing && { animation: 'spin 1s linear infinite' }) }} />
                </IconButton>
              </Tooltip>
            </Box>

            {/* Search Bar */}
            <MemorySearchBar
              value={searchQuery}
              onChange={setSearchQuery}
              onSearch={handleSearch}
              isLoading={loading}
              placeholder="Search in memories... (press / to focus)"
              recentSearches={recentSearches}
              onClearRecent={handleClearRecent}
            />

            {/* Filters */}
            <FilterPanel
              values={filters}
              onChange={setFilters}
            />
          </Stack>
        </Paper>

        {/* Global styles for spin animation */}
        <style>{`
          @keyframes spin {
            from { transform: rotate(0deg); }
            to { transform: rotate(360deg); }
          }
        `}</style>

        {/* Two-Column Layout */}
        <Stack direction="row" spacing={1} sx={{ height: 'calc(100vh - 220px)' }}>
          {/* Column 1: Search Results */}
          <Paper
            sx={{
              width: 420,
              display: 'flex',
              flexDirection: 'column',
              border: 1,
              borderColor: 'divider',
              borderRadius: 2,
              overflow: 'hidden',
            }}
          >
            <Box sx={{ flex: 1, overflow: 'auto' }}>
              <SearchResultsList
                searchQuery={searchQuery}
                sessions={filteredSessions}
                roundsBySession={roundsBySession}
                selectedSessionId={selectedSessionId}
                onSelectSession={handleSelectSession}
                isLoading={loading}
                loadingSessionId={loadingRounds ? selectedSessionItem?.session_id : undefined}
              />
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
              {!selectedSessionItem ? (
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
                    {searchQuery
                      ? 'Select a search result to view the conversation'
                      : 'Search or select a session to view the conversation'}
                  </Typography>
                </Box>
              ) : loadingRounds ? (
                <Box
                  sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    height: '100%',
                    p: 3,
                  }}
                >
                  <CircularProgress size={32} sx={{ mb: 2 }} />
                  <Typography variant="body2" color="text.secondary">
                    Loading conversation...
                  </Typography>
                </Box>
              ) : (
                <MemoryDetailView
                  sessionItem={selectedSessionItem}
                  rounds={sessionRounds}
                  onClose={() => {
                    setSelectedSessionItem(null);
                    setSelectedSessionId('');
                    setSessionRounds([]);
                  }}
                />
              )}
            </Box>
          </Paper>
        </Stack>
      </Box>
    </PageLayout>
  );
};

export default MemoryPage;
