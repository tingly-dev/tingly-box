import { Box, Skeleton, Stack } from '@mui/material';

interface SessionListSkeletonProps {
  count?: number;
}

export const SessionListSkeleton: React.FC<SessionListSkeletonProps> = ({ count = 3 }) => (
  <Box sx={{ p: 1 }}>
    {[...Array(count)].map((_, i) => (
      <Box key={i} sx={{ mb: 1.5 }}>
        {/* Session Header Skeleton */}
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            p: 1.5,
            bgcolor: 'action.hover',
            borderRadius: 2,
            mb: 0.75,
          }}
        >
          <Skeleton variant="circular" width={20} height={20} />
          <Skeleton variant="circular" width={32} height={32} />
          <Box sx={{ flex: 1 }}>
            <Skeleton variant="text" width="40%" height={16} sx={{ mb: 0.5 }} />
            <Skeleton variant="text" width="60%" height={14} />
          </Box>
          <Skeleton variant="text" width={60} height={14} />
        </Box>

        {/* Message Cards Skeleton */}
        {[...Array(2)].map((_, j) => (
          <Skeleton
            key={j}
            variant="rectangular"
            height={85}
            sx={{ borderRadius: 1.5, mb: 0.5, ml: 5 }}
          />
        ))}
      </Box>
    ))}
  </Box>
);

interface CalendarSkeletonProps {
  width?: number;
}

export const CalendarSkeleton: React.FC<CalendarSkeletonProps> = ({ width = 320 }) => (
  <Box sx={{ width, p: 2 }}>
    <Skeleton variant="text" width="40%" height={24} sx={{ mb: 2 }} />
    <Skeleton variant="rectangular" height={200} sx={{ borderRadius: 1 }} />
  </Box>
);

interface MemoryDetailSkeletonProps {
  hasHeader?: boolean;
}

export const MemoryDetailSkeleton: React.FC<MemoryDetailSkeletonProps> = ({ hasHeader = true }) => (
  <Box>
    {hasHeader && (
      <>
        <Skeleton variant="text" width="30%" height={24} sx={{ mb: 2 }} />
        <Skeleton variant="rectangular" height={60} sx={{ mb: 2, borderRadius: 1 }} />
      </>
    )}

    {/* Token Stats Skeleton */}
    <Skeleton variant="text" width={80} height={16} sx={{ mb: 1 }} />
    <Stack direction="row" spacing={3} sx={{ mb: 3 }}>
      <Skeleton variant="text" width={60} height={32} />
      <Skeleton variant="text" width={60} height={32} />
      <Skeleton variant="text" width={60} height={32} />
    </Stack>

    {/* Context Skeleton */}
    <Skeleton variant="rectangular" height={80} sx={{ mb: 2, borderRadius: 1 }} />

    {/* Input Section Skeleton */}
    <Skeleton variant="text" width={100} height={16} sx={{ mb: 1 }} />
    <Skeleton variant="rectangular" height={120} sx={{ mb: 2, borderRadius: 1 }} />

    {/* Output Section Skeleton */}
    <Skeleton variant="text" width={100} height={16} sx={{ mb: 1 }} />
    <Skeleton variant="rectangular" height={200} sx={{ borderRadius: 1 }} />
  </Box>
);
