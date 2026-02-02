import {
    CheckCircle,
    Close,
    ContentCopy,
    Description,
    ErrorOutline,
} from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Paper,
    Stack,
    Typography,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { type Skill, type SkillLocation } from '@/types/prompt';
import { getIdeSourceLabel } from '@/constants/ideSources';
import { api } from '@/services/api';

interface SkillDetailDialogProps {
    open: boolean;
    skill: Skill | null;
    location: SkillLocation | null;
    onClose: () => void;
}

const SkillDetailDialog = ({ open, skill, location, onClose }: SkillDetailDialogProps) => {
    const [loading, setLoading] = useState(false);
    const [content, setContent] = useState<string>('');
    const [copied, setCopied] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (open && skill && location) {
            loadSkillContent();
        }
        // Reset copied state when dialog closes
        if (!open) {
            setCopied(false);
            setError(null);
        }
    }, [open, skill, location]);

    const loadSkillContent = async () => {
        if (!skill || !location) return;

        setLoading(true);
        setError(null);

        try {
            const result = await api.getSkillContent(
                location.id,
                skill.id,
                skill.path
            );
            if (result.success && result.data) {
                setContent(result.data.content || '');
            } else {
                setError(result.error || 'Failed to load skill content');
            }
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Unknown error occurred');
        } finally {
            setLoading(false);
        }
    };

    const handleCopy = () => {
        navigator.clipboard.writeText(content);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    const handleClose = () => {
        setContent('');
        setError(null);
        setCopied(false);
        onClose();
    };

    const formatFileSize = (bytes?: number): string => {
        if (!bytes) return '-';
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    };

    const formatDate = (dateStr?: string): string => {
        if (!dateStr) return 'Unknown';
        try {
            return new Date(dateStr).toLocaleString();
        } catch {
            return 'Invalid date';
        }
    };

    if (!skill || !location) return null;

    const sourceLabel = getIdeSourceLabel(location.ide_source);

    return (
        <Dialog
            open={open}
            onClose={handleClose}
            maxWidth="lg"
            fullWidth
            PaperProps={{
                sx: { height: '80vh' },
            }}
        >
            <DialogTitle>
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                    }}
                >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                        <Description fontSize="small" color="action" />
                        <Box sx={{ minWidth: 0 }}>
                            <Typography variant="h6" noWrap>
                                {skill.name}
                            </Typography>
                            <Typography variant="caption" color="text.secondary" noWrap>
                                {skill.filename}
                            </Typography>
                        </Box>
                    </Box>
                    <IconButton
                        aria-label="close"
                        onClick={handleClose}
                        size="small"
                        disabled={loading}
                    >
                        <Close />
                    </IconButton>
                </Box>
            </DialogTitle>
            <DialogContent sx={{ p: 0 }}>
                <Stack spacing={0} sx={{ height: '100%' }}>
                    {/* Metadata Bar */}
                    <Box
                        sx={{
                            px: 3,
                            py: 2,
                            borderBottom: 1,
                            borderColor: 'divider',
                            bgcolor: 'action.hover',
                        }}
                    >
                        <Stack direction="row" spacing={3} alignItems="center" flexWrap="wrap">
                            <Stack direction="row" spacing={1} alignItems="center">
                                <Chip
                                    size="small"
                                    label={sourceLabel}
                                    variant="outlined"
                                    sx={{ height: 24, fontSize: '0.75rem' }}
                                />
                                <Typography variant="body2" color="text.secondary">
                                    {location.name}
                                </Typography>
                            </Stack>
                            <Typography variant="body2" color="text.secondary">
                                Size: {formatFileSize(skill.size)}
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                Modified: {formatDate(skill.modified_at as string)}
                            </Typography>
                        </Stack>
                    </Box>

                    {/* Content Area */}
                    <Box sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                        {loading ? (
                            <Box
                                sx={{
                                    display: 'flex',
                                    flexDirection: 'column',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    height: '100%',
                                }}
                            >
                                <CircularProgress size={40} sx={{ mb: 2 }} />
                                <Typography variant="body2" color="text.secondary">
                                    Loading skill content...
                                </Typography>
                            </Box>
                        ) : error ? (
                            <Box
                                sx={{
                                    display: 'flex',
                                    flexDirection: 'column',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    height: '100%',
                                    px: 3,
                                }}
                            >
                                <ErrorOutline
                                    sx={{ fontSize: 48, color: 'error.main', mb: 2 }}
                                />
                                <Typography variant="body2" color="error" align="center">
                                    {error}
                                </Typography>
                                <Button
                                    variant="outlined"
                                    onClick={loadSkillContent}
                                    sx={{ mt: 2 }}
                                    size="small"
                                >
                                    Retry
                                </Button>
                            </Box>
                        ) : content ? (
                            <Box sx={{ p: 3 }}>
                                <Paper
                                    elevation={0}
                                    sx={{
                                        p: 3,
                                        bgcolor: 'background.default',
                                        fontFamily: 'monospace',
                                        fontSize: '0.875rem',
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-word',
                                        border: 1,
                                        borderColor: 'divider',
                                    }}
                                >
                                    {content}
                                </Paper>
                            </Box>
                        ) : (
                            <Box
                                sx={{
                                    display: 'flex',
                                    flexDirection: 'column',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    height: '100%',
                                }}
                            >
                                <Typography variant="body2" color="text.secondary">
                                    No content available
                                </Typography>
                            </Box>
                        )}
                    </Box>

                    {/* Action Bar */}
                    {!loading && content && (
                        <Box
                            sx={{
                                px: 3,
                                py: 2,
                                borderTop: 1,
                                borderColor: 'divider',
                                bgcolor: 'action.hover',
                                display: 'flex',
                                justifyContent: 'flex-end',
                            }}
                        >
                            {copied && (
                                <Alert
                                    severity="success"
                                    icon={<CheckCircle fontSize="inherit" />}
                                    sx={{ mr: 2, py: 0 }}
                                >
                                    Copied to clipboard!
                                </Alert>
                            )}
                            <Button
                                variant="outlined"
                                size="small"
                                startIcon={<ContentCopy />}
                                onClick={handleCopy}
                            >
                                Copy Content
                            </Button>
                        </Box>
                    )}
                </Stack>
            </DialogContent>
        </Dialog>
    );
};

export default SkillDetailDialog;
