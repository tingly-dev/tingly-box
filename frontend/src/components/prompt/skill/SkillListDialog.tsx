import {
    Close,
    Description,
    FolderOpen,
    Search,
} from '@mui/icons-material';
import {
    Box,
    CircularProgress,
    Chip,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    LinearProgress,
    List,
    ListItem,
    ListItemButton,
    ListItemText,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { type SkillLocation, type Skill, type ScanResult } from '@/types/prompt';
import { getIdeSourceLabel } from '@/constants/ideSources';
import { api } from '@/services/api';

interface SkillListDialogProps {
    open: boolean;
    location: SkillLocation | null;
    onClose: () => void;
    onSkillClick: (skill: Skill) => void;
}

const SkillListDialog = ({ open, location, onClose, onSkillClick }: SkillListDialogProps) => {
    const [loading, setLoading] = useState(false);
    const [refreshing, setRefreshing] = useState(false);
    const [skills, setSkills] = useState<Skill[]>([]);
    const [searchQuery, setSearchQuery] = useState('');
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (open && location) {
            loadSkills();
        }
    }, [open, location]);

    const loadSkills = async () => {
        if (!location) return;

        setLoading(true);
        setError(null);

        try {
            const result = await api.refreshSkillLocation(location.id);
            if (result.success && result.data) {
                setSkills(result.data.skills || []);
            } else {
                setError(result.error || 'Failed to load skills');
            }
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Unknown error occurred');
        } finally {
            setLoading(false);
        }
    };

    const handleRefresh = async () => {
        if (!location) return;

        setRefreshing(true);
        setError(null);

        try {
            const result = await api.refreshSkillLocation(location.id);
            if (result.success && result.data) {
                setSkills(result.data.skills || []);
            } else {
                setError(result.error || 'Failed to refresh skills');
            }
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Unknown error occurred');
        } finally {
            setRefreshing(false);
        }
    };

    const handleClose = () => {
        setSearchQuery('');
        setError(null);
        onClose();
    };

    const formatFileSize = (bytes?: number): string => {
        if (!bytes) return '-';
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    };

    const filteredSkills = skills.filter(
        (skill) =>
            skill.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
            skill.filename.toLowerCase().includes(searchQuery.toLowerCase())
    );

    if (!location) return null;

    const sourceLabel = getIdeSourceLabel(location.ide_source);

    return (
        <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
            <DialogTitle>
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                    }}
                >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Chip
                            size="small"
                            label={sourceLabel}
                            variant="outlined"
                            sx={{ height: 24, fontSize: '0.75rem' }}
                        />
                        <Box>
                            <Typography variant="h6">{location.name}</Typography>
                            <Typography variant="caption" color="text.secondary">
                                {location.path}
                            </Typography>
                        </Box>
                    </Box>
                    <IconButton aria-label="close" onClick={handleClose} size="small">
                        <Close />
                    </IconButton>
                </Box>
            </DialogTitle>
            <DialogContent>
                <Stack spacing={2}>
                    {/* Search Bar */}
                    <Box sx={{ display: 'flex', gap: 2, alignItems: 'center' }}>
                        <TextField
                            placeholder="Search skills..."
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            InputProps={{
                                startAdornment: <Search sx={{ mr: 1, color: 'text.secondary' }} />,
                            }}
                            size="small"
                            fullWidth
                        />
                        <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
                            {filteredSkills.length} / {skills.length}
                        </Typography>
                    </Box>

                    {/* Loading State */}
                    {loading && !refreshing && (
                        <Box sx={{ textAlign: 'center', py: 3 }}>
                            <CircularProgress size={40} sx={{ mb: 2 }} />
                            <Typography variant="body2" color="text.secondary">
                                Loading skills...
                            </Typography>
                        </Box>
                    )}

                    {/* Refreshing Overlay */}
                    {refreshing && <LinearProgress sx={{ mt: 1 }} />}

                    {/* Error State */}
                    {error && !loading && (
                        <Box sx={{ py: 2 }}>
                            <Typography variant="body2" color="error">
                                {error}
                            </Typography>
                        </Box>
                    )}

                    {/* Skills List */}
                    {!loading && (
                        <Box>
                            {filteredSkills.length > 0 ? (
                                <List
                                    sx={{
                                        border: 1,
                                        borderColor: 'divider',
                                        borderRadius: 1,
                                        maxHeight: 400,
                                        overflow: 'auto',
                                    }}
                                >
                                    {filteredSkills.map((skill) => (
                                        <ListItem
                                            key={skill.id}
                                            disablePadding
                                            divider
                                            component="div"
                                        >
                                            <ListItemButton
                                                onClick={() => onSkillClick(skill)}
                                                dense
                                            >
                                                <Box
                                                    sx={{
                                                        width: 32,
                                                        display: 'flex',
                                                        justifyContent: 'center',
                                                        mr: 1,
                                                    }}
                                                >
                                                    <Description
                                                        fontSize="small"
                                                        color="action"
                                                    />
                                                </Box>
                                                <ListItemText
                                                    primary={
                                                        <Typography
                                                            variant="subtitle2"
                                                            sx={{ fontWeight: 500 }}
                                                        >
                                                            {skill.name}
                                                        </Typography>
                                                    }
                                                    secondary={
                                                        <Stack
                                                            direction="row"
                                                            spacing={2}
                                                            alignItems="center"
                                                        >
                                                            <Typography
                                                                variant="caption"
                                                                color="text.secondary"
                                                            >
                                                                {skill.filename}
                                                            </Typography>
                                                            <Typography
                                                                variant="caption"
                                                                color="text.secondary"
                                                            >
                                                                {formatFileSize(skill.size)}
                                                            </Typography>
                                                        </Stack>
                                                    }
                                                />
                                            </ListItemButton>
                                        </ListItem>
                                    ))}
                                </List>
                            ) : (
                                <Box
                                    sx={{
                                        textAlign: 'center',
                                        py: 4,
                                        border: 1,
                                        borderColor: 'divider',
                                        borderRadius: 1,
                                    }}
                                >
                                    <FolderOpen
                                        sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }}
                                    />
                                    <Typography variant="body2" color="text.secondary">
                                        {searchQuery
                                            ? 'No skills match your search.'
                                            : 'No skills found in this location.'}
                                    </Typography>
                                </Box>
                            )}
                        </Box>
                    )}
                </Stack>
            </DialogContent>
        </Dialog>
    );
};

export default SkillListDialog;
