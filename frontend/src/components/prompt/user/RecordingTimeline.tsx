import { Box, Typography, Card, CardContent } from '@mui/material';
import { ChevronRight } from '@mui/icons-material';
import type { Recording, RecordingType } from '@/types/prompt';

const RECORDING_TYPE_LABELS: Record<RecordingType, string> = {
  'code-review': 'Code Review',
  'debug': 'Debug',
  'refactor': 'Refactor',
  'test': 'Test',
  'custom': 'Custom',
};

interface RecordingTimelineProps {
  recordings: Recording[];
  onPlay: (recording: Recording) => void;
  onViewDetails: (recording: Recording) => void;
  onDelete: (recording: Recording) => void;
  onSelectRecording: (recording: Recording | null) => void;
  selectedRecording: Recording | null;
}

const RecordingTimeline: React.FC<RecordingTimelineProps> = ({
  recordings,
  onPlay,
  onViewDetails,
  onDelete,
  onSelectRecording,
  selectedRecording,
}) => {
  const formatTime = (date: Date): string => {
    return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
  };

  const handleCardClick = (recording: Recording) => {
    if (selectedRecording?.id === recording.id) {
      onSelectRecording(null);
    } else {
      onSelectRecording(recording);
      onViewDetails(recording);
    }
  };

  if (recordings.length === 0) {
    return null;
  }

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
      }}
    >
      {recordings.map((recording) => (
        <Card
          key={recording.id}
          onClick={() => handleCardClick(recording)}
          sx={{
            border: '1px solid',
            borderColor: selectedRecording?.id === recording.id ? 'primary.main' : 'divider',
            borderRadius: 2,
            cursor: 'pointer',
            transition: 'all 0.2s',
            backgroundColor: selectedRecording?.id === recording.id ? 'action.selected' : 'background.paper',
            '&:hover': {
              borderColor: 'primary.main',
              boxShadow: 1,
            },
          }}
        >
          <CardContent sx={{ p: 1.5, '&:last-child': { pb: 1.5 } }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              {/* Time */}
              <Box sx={{ minWidth: 50 }}>
                <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.primary' }}>
                  {formatTime(recording.timestamp)}
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
                  }}
                >
                  {recording.summary || recording.content.substring(0, 30) + '...'}
                </Typography>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.25 }}>
                  <Typography
                    variant="caption"
                    sx={{
                      px: 0.5,
                      py: 0.1,
                      borderRadius: 0.5,
                      backgroundColor: 'primary.100',
                      color: 'primary.dark',
                      fontSize: '0.65rem',
                      fontWeight: 500,
                    }}
                  >
                    {RECORDING_TYPE_LABELS[recording.type]}
                  </Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
                    {recording.user.name}
                  </Typography>
                </Box>
              </Box>

              {/* Expand Icon */}
              <ChevronRight
                sx={{
                  fontSize: 18,
                  color: 'text.secondary',
                  transition: 'transform 0.2s',
                  transform: selectedRecording?.id === recording.id ? 'rotate(90deg)' : 'rotate(0deg)',
                }}
              />
            </Box>
          </CardContent>
        </Card>
      ))}
    </Box>
  );
};

export default RecordingTimeline;
