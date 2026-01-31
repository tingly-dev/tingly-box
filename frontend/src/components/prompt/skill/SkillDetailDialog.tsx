import {
  Box,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  Chip,
  CircularProgress,
  Paper,
  Divider,
  IconButton,
  Tooltip,
} from '@mui/material';
import { Close, OpenInNew, Description } from '@mui/icons-material';
import { useState, useEffect } from 'react';
import type { Skill, SkillLocation } from '@/types/prompt';

interface SkillDetailDialogProps {
  open: boolean;
  skill?: Skill;
  location?: SkillLocation;
  onClose: () => void;
  onOpen: (skill: Skill) => void;
}

const formatFileSize = (bytes?: number): string => {
  if (!bytes) return 'Unknown';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
};

const formatDate = (date?: Date): string => {
  if (!date) return 'Unknown';
  return new Date(date).toLocaleString();
};

// TODO: Replace with actual backend API call
const mockGetSkillContent = async (skillPath: string): Promise<string> => {
  // Simulate API delay
  await new Promise((resolve) => setTimeout(resolve, 500));

  // Mock content preview
  return `// Skill: ${skillPath.split('/').pop()}
// This is a preview of the skill content
// In the real implementation, this would be fetched from the backend

export default function handler(req, res) {
  // Your skill logic here
  return { success: true };
}`;
};

const getLanguageFromFileType = (fileType: string): string => {
  const langMap: Record<string, string> = {
    '.ts': 'typescript',
    '.tsx': 'typescript',
    '.js': 'javascript',
    '.jsx': 'javascript',
    '.py': 'python',
    '.go': 'go',
    '.rs': 'rust',
    '.md': 'markdown',
    '.json': 'json',
    '.yaml': 'yaml',
    '.yml': 'yaml',
    '.sh': 'bash',
  };
  return langMap[fileType] || 'text';
};

const SkillDetailDialog: React.FC<SkillDetailDialogProps> = ({
  open,
  skill,
  location,
  onClose,
  onOpen,
}) => {
  const [loading, setLoading] = useState(false);
  const [content, setContent] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Load content when skill changes
  useEffect(() => {
    if (!skill || !open) {
      setContent(null);
      setError(null);
      return;
    }

    // Only show preview for text-based files
    const textExtensions = ['.ts', '.tsx', '.js', '.jsx', '.py', '.go', '.rs', '.md', '.json', '.yaml', '.yml', '.sh', '.txt'];
    if (!textExtensions.includes(skill.file_type)) {
      setContent(null);
      return;
    }

    setLoading(true);
    setError(null);

    mockGetSkillContent(skill.path)
      .then((data) => {
        setContent(data);
      })
      .catch((err) => {
        setError(err.message || 'Failed to load skill content');
      })
      .finally(() => {
        setLoading(false);
      });
  }, [skill, open]);

  const handleOpen = () => {
    if (skill) {
      onOpen(skill);
      onClose();
    }
  };

  const canPreview = skill && ['.ts', '.tsx', '.js', '.jsx', '.py', '.go', '.rs', '.md', '.json', '.yaml', '.yml', '.sh', '.txt'].includes(skill.file_type);

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      {skill && (
        <>
          <DialogTitle>
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
              }}
            >
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Description color="primary" />
                <Typography variant="h6" component="span">
                  {skill.name}
                </Typography>
              </Box>
              <IconButton onClick={onClose} size="small">
                <Close />
              </IconButton>
            </Box>
          </DialogTitle>

          <DialogContent>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              {/* Basic Info */}
              <Box
                sx={{
                  display: 'flex',
                  flexWrap: 'wrap',
                  gap: 2,
                  alignItems: 'center',
                }}
              >
                <Chip
                  label={skill.file_type}
                  size="small"
                  color="primary"
                  variant="outlined"
                />
                {location && (
                  <Chip
                    label={location.name}
                    size="small"
                    variant="outlined"
                  />
                )}
                {skill.size && (
                  <Typography variant="caption" color="text.secondary">
                    {formatFileSize(skill.size)}
                  </Typography>
                )}
                {skill.modified_at && (
                  <Typography variant="caption" color="text.secondary">
                    Modified: {formatDate(skill.modified_at)}
                  </Typography>
                )}
              </Box>

              {/* Path */}
              <Box>
                <Typography variant="caption" color="text.secondary">
                  Path
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    fontFamily: 'monospace',
                    wordBreak: 'break-all',
                  }}
                >
                  {skill.path}
                </Typography>
              </Box>

              {/* Description */}
              {skill.description && (
                <Box>
                  <Typography variant="caption" color="text.secondary">
                    Description
                  </Typography>
                  <Typography variant="body2">{skill.description}</Typography>
                </Box>
              )}

              {/* Content Hash */}
              {skill.content_hash && (
                <Box>
                  <Typography variant="caption" color="text.secondary">
                    Content Hash (SHA256)
                  </Typography>
                  <Typography
                    variant="caption"
                    sx={{
                      fontFamily: 'monospace',
                      wordBreak: 'break-all',
                      display: 'block',
                    }}
                  >
                    {skill.content_hash}
                  </Typography>
                </Box>
              )}

              {/* Preview */}
              {canPreview && (
                <Box>
                  <Divider sx={{ my: 2 }} />
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>
                    Preview
                  </Typography>
                  {loading ? (
                    <Box
                      sx={{
                        display: 'flex',
                        justifyContent: 'center',
                        py: 4,
                      }}
                    >
                      <CircularProgress size={32} />
                    </Box>
                  ) : error ? (
                    <Paper
                      variant="outlined"
                      sx={{
                        p: 2,
                        bgcolor: 'error.50',
                        borderColor: 'error.200',
                      }}
                    >
                      <Typography variant="body2" color="error">
                        {error}
                      </Typography>
                    </Paper>
                  ) : content ? (
                    <Paper
                      variant="outlined"
                      sx={{
                        p: 2,
                        bgcolor: 'grey.50',
                        maxHeight: 300,
                        overflow: 'auto',
                      }}
                    >
                      <Typography
                        variant="body2"
                        component="pre"
                        sx={{
                          fontFamily: 'monospace',
                          fontSize: '0.75rem',
                          whiteSpace: 'pre-wrap',
                          m: 0,
                        }}
                      >
                        {content}
                      </Typography>
                    </Paper>
                  ) : null}
                </Box>
              )}
            </Box>
          </DialogContent>

          <DialogActions>
            <Button onClick={onClose}>Close</Button>
            <Tooltip title="Open in default editor">
              <Button
                onClick={handleOpen}
                variant="contained"
                startIcon={<OpenInNew />}
              >
                Open in Editor
              </Button>
            </Tooltip>
          </DialogActions>
        </>
      )}
    </Dialog>
  );
};

export default SkillDetailDialog;
