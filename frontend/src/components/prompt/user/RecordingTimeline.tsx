import { useState, useEffect } from 'react';
import { Box, Typography, IconButton, Card, CardContent } from '@mui/material';
import { PlayArrow, Delete as DeleteIcon, ChevronRight } from '@mui/icons-material';
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
}

const RecordingTimeline: React.FC<RecordingTimelineProps> = ({
  recordings,
  onPlay,
  onViewDetails,
  onDelete,
}) => {
  const [selectedRecording, setSelectedRecording] = useState<Recording | null>(null);

  // Clear selection when recordings change
  useEffect(() => {
    setSelectedRecording(null);
  }, [recordings]);

  const formatTime = (date: Date): string => {
    return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
  };

  const formatDuration = (seconds: number): string => {
    if (seconds < 60) {
      return `${seconds}s`;
    }
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = seconds % 60;
    return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  };

  const handleCardClick = (recording: Recording) => {
    if (selectedRecording?.id === recording.id) {
      setSelectedRecording(null);
    } else {
      setSelectedRecording(recording);
      onViewDetails(recording);
    }
  };

  if (recordings.length === 0) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          py: 8,
          color: 'text.secondary',
        }}
      >
        <Typography variant="body1">No recordings found for this date</Typography>
      </Box>
    );
  }

  return (
    <Box
      sx={{
        display: 'flex',
        gap: 2,
        overflow: 'auto',
        alignItems: 'flex-start',
      }}
    >
      {/* First Column: Recording List */}
      <Box
        sx={{
          minWidth: 320,
          maxWidth: 320,
          flexShrink: 0,
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
              backgroundColor: selectedRecording?.id === recording.id ? 'primary.50' : 'background.paper',
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

      {/* Second Column: Details Panel */}
      {selectedRecording && (
        <Box
          sx={{
            minWidth: 400,
            maxWidth: 500,
            flexShrink: 0,
            animation: 'slideIn 0.2s ease-out',
            '@keyframes slideIn': {
              from: {
                opacity: 0,
                transform: 'translateX(-10px)',
              },
              to: {
                opacity: 1,
                transform: 'translateX(0)',
              },
            },
          }}
        >
          <Card
            sx={{
              border: '1px solid',
              borderColor: 'divider',
              borderRadius: 2,
              height: 'fit-content',
            }}
          >
            <CardContent>
              {/* Header */}
              <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', mb: 2 }}>
                <Box sx={{ flex: 1 }}>
                  <Typography variant="h6" sx={{ fontWeight: 600, mb: 0.5 }}>
                    {selectedRecording.summary || 'Recording'}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {selectedRecording.timestamp.toLocaleString('en-US', {
                      weekday: 'long',
                      month: 'long',
                      day: 'numeric',
                      year: 'numeric',
                      hour: '2-digit',
                      minute: '2-digit',
                    })}
                  </Typography>
                </Box>
                <Box sx={{ display: 'flex', gap: 0.5 }}>
                  <IconButton
                    size="small"
                    onClick={() => onPlay(selectedRecording)}
                    title="Play"
                    color="primary"
                  >
                    <PlayArrow fontSize="small" />
                  </IconButton>
                  <IconButton
                    size="small"
                    onClick={() => onDelete(selectedRecording)}
                    title="Delete"
                    color="error"
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </Box>
              </Box>

              {/* Type Badge */}
              <Box sx={{ mb: 2 }}>
                <Typography
                  variant="caption"
                  sx={{
                    px: 1,
                    py: 0.5,
                    borderRadius: 1,
                    backgroundColor: 'primary.light',
                    color: 'primary.contrastText',
                    fontSize: '0.75rem',
                    fontWeight: 600,
                  }}
                >
                  {RECORDING_TYPE_LABELS[selectedRecording.type]}
                </Typography>
              </Box>

              {/* Metadata Grid */}
              <Box sx={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 2, mb: 2 }}>
                <Box>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.25 }}>
                    User
                  </Typography>
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    {selectedRecording.user.name}
                  </Typography>
                </Box>
                <Box>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.25 }}>
                    Project
                  </Typography>
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    {selectedRecording.project}
                  </Typography>
                </Box>
                <Box>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.25 }}>
                    Duration
                  </Typography>
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    {formatDuration(selectedRecording.duration)}
                  </Typography>
                </Box>
                <Box>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.25 }}>
                    Model
                  </Typography>
                  <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }}>
                    {selectedRecording.model}
                  </Typography>
                </Box>
              </Box>

              {/* Content */}
              <Box>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                  Content
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    p: 1.5,
                    backgroundColor: 'grey.50',
                    borderRadius: 1,
                    maxHeight: 150,
                    overflow: 'auto',
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                  }}
                >
                  {selectedRecording.content}
                </Typography>
              </Box>
            </CardContent>
          </Card>
        </Box>
      )}
    </Box>
  );
};

export default RecordingTimeline;
