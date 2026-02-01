import {
    Add,
    AutoFixHigh,
    Code,
    ContentCopy,
    Delete,
    Description,
    Edit,
    FolderOpen,
    Refresh,
    Search,
    Visibility,
} from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Divider,
    IconButton,
    InputAdornment,
    List,
    ListItem,
    ListItemButton,
    ListItemText,
    Paper,
    Stack,
    TextField,
    Typography,
    Chip as MuiChip,
} from '@mui/material';
import { useEffect, useState } from 'react';
import XMarkdown from '@ant-design/x-markdown';
import { type SkillLocation, type Skill, type IDESource } from '@/types/prompt';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { getIdeSourceLabel } from '@/constants/ideSources';
import { api } from '@/services/api';
import AddSkillLocationDialog from '@/components/prompt/skill/AddSkillLocationDialog';
import AutoDiscoveryDialog from '@/components/prompt/skill/AutoDiscoveryDialog';

interface AddSkillLocationData {
    name: string;
    path: string;
    ide_source: IDESource;
}

const SkillPage = () => {
    const [locations, setLocations] = useState<SkillLocation[]>([]);
    const [loading, setLoading] = useState(true);
    const [notification, setNotification] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error';
    }>({ open: false, message: '', severity: 'success' });

    // Location list state
    const [locationSearch, setLocationSearch] = useState('');
    const [selectedLocation, setSelectedLocation] = useState<SkillLocation | null>(null);

    // Skill list state
    const [skills, setSkills] = useState<Skill[]>([]);
    const [skillsLoading, setSkillsLoading] = useState(false);
    const [skillSearch, setSkillSearch] = useState('');
    const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);

    // Skill detail state
    const [skillContent, setSkillContent] = useState<string>('');
    const [contentLoading, setContentLoading] = useState(false);
    const [viewMode, setViewMode] = useState<'markdown' | 'raw'>('markdown');

    // Dialog states
    const [addDialogOpen, setAddDialogOpen] = useState(false);
    const [addDialogMode, setAddDialogMode] = useState<'add' | 'edit'>('add');
    const [editLocation, setEditLocation] = useState<SkillLocation | null>(null);
    const [discoveryDialogOpen, setDiscoveryDialogOpen] = useState(false);

    useEffect(() => {
        loadLocations();
    }, []);

    // Load skills when location is selected
    useEffect(() => {
        if (selectedLocation) {
            loadSkills(selectedLocation);
        } else {
            setSkills([]);
            setSelectedSkill(null);
            setSkillContent('');
        }
    }, [selectedLocation]);

    // Load skill content when skill is selected
    useEffect(() => {
        if (selectedSkill && selectedLocation) {
            loadSkillContent(selectedSkill);
        } else {
            setSkillContent('');
            setViewMode('markdown');
        }
    }, [selectedSkill]);

    const showNotification = (message: string, severity: 'success' | 'error') => {
        setNotification({ open: true, message, severity });
    };

    const loadLocations = async () => {
        setLoading(true);
        const result = await api.getSkillLocations();
        if (result.success) {
            setLocations(result.data || []);
        } else {
            showNotification(`Failed to load locations: ${result.error}`, 'error');
        }
        setLoading(false);
    };

    const loadSkills = async (location: SkillLocation) => {
        setSkillsLoading(true);
        const result = await api.refreshSkillLocation(location.id);
        if (result.success && result.data) {
            setSkills(result.data.skills || []);
        } else {
            showNotification(`Failed to load skills: ${result.error}`, 'error');
        }
        setSkillsLoading(false);
    };

    const loadSkillContent = async (skill: Skill) => {
        if (!selectedLocation) return;

        setContentLoading(true);
        const result = await api.getSkillContent(
            selectedLocation.id,
            skill.id,
            skill.path
        );
        if (result.success && result.data) {
            setSkillContent(result.data.content || '');
        } else {
            showNotification(`Failed to load skill content: ${result.error}`, 'error');
        }
        setContentLoading(false);
    };

    const handleAddClick = () => {
        setAddDialogMode('add');
        setEditLocation(null);
        setAddDialogOpen(true);
    };

    const handleEditClick = (location: SkillLocation, e: React.MouseEvent) => {
        e.stopPropagation();
        setAddDialogMode('edit');
        setEditLocation(location);
        setAddDialogOpen(true);
    };

    const handleDeleteClick = (id: string, e: React.MouseEvent) => {
        e.stopPropagation();
        if (!confirm('Are you sure you want to delete this location?')) {
            return;
        }

        api.removeSkillLocation(id).then((result) => {
            if (result.success) {
                showNotification('Location deleted successfully!', 'success');
                if (selectedLocation?.id === id) {
                    setSelectedLocation(null);
                }
                loadLocations();
            } else {
                showNotification(`Failed to delete location: ${result.error}`, 'error');
            }
        });
    };

    const handleRefreshClick = (id: string, e: React.MouseEvent) => {
        e.stopPropagation();
        api.refreshSkillLocation(id).then((result) => {
            if (result.success) {
                showNotification('Location refreshed successfully!', 'success');
                loadLocations();
            } else {
                showNotification(`Failed to refresh location: ${result.error}`, 'error');
            }
        });
    };

    const handleAddSubmit = async (data: AddSkillLocationData) => {
        if (addDialogMode === 'add') {
            const result = await api.addSkillLocation({
                name: data.name,
                path: data.path,
                ide_source: data.ide_source,
            });
            if (result.success) {
                showNotification('Location added successfully!', 'success');
                loadLocations();
            } else {
                showNotification(`Failed to add location: ${result.error}`, 'error');
            }
        } else if (editLocation) {
            const deleteResult = await api.removeSkillLocation(editLocation.id);
            if (deleteResult.success) {
                const addResult = await api.addSkillLocation({
                    name: data.name,
                    path: data.path,
                    ide_source: data.ide_source,
                });
                if (addResult.success) {
                    showNotification('Location updated successfully!', 'success');
                    loadLocations();
                } else {
                    showNotification(`Failed to update location: ${addResult.error}`, 'error');
                }
            } else {
                showNotification(`Failed to update location: ${deleteResult.error}`, 'error');
            }
        }
    };

    const handleImportLocations = async (locs: SkillLocation[]) => {
        const result = await api.importSkillLocations(locs);
        if (result.success) {
            showNotification(
                `Imported ${result.data?.length || 0} location(s) successfully!`,
                'success'
            );
            loadLocations();
        } else {
            showNotification(`Failed to import locations: ${result.error}`, 'error');
        }
    };

    const handleCopyContent = () => {
        navigator.clipboard.writeText(skillContent);
        showNotification('Copied to clipboard!', 'success');
    };

    // Filter locations
    const filteredLocations = locations.filter((location) => {
        const matchesSearch =
            locationSearch === '' ||
            location.name.toLowerCase().includes(locationSearch.toLowerCase()) ||
            location.path.toLowerCase().includes(locationSearch.toLowerCase());
        return matchesSearch;
    });

    // Filter skills
    const filteredSkills = skills.filter((skill) => {
        const matchesSearch =
            skillSearch === '' ||
            skill.name.toLowerCase().includes(skillSearch.toLowerCase()) ||
            skill.filename.toLowerCase().includes(skillSearch.toLowerCase());
        return matchesSearch;
    });

    const formatFileSize = (bytes?: number): string => {
        if (!bytes) return '-';
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    };

    const getRelativePath = (skill: Skill, location: SkillLocation): string => {
        const basePath = location.path.endsWith('/') ? location.path : location.path + '/';
        if (skill.path.startsWith(basePath)) {
            return skill.path.substring(basePath.length);
        }
        return skill.filename;
    };

    return (
        <PageLayout loading={loading} notification={notification}>
            {/* Header */}
            <Box sx={{ mb: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Box>
                    <Typography variant="h4" sx={{ fontWeight: 600, mb: 0.5 }}>
                        Skill Management
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Manage your AI skill locations from various IDEs and tools
                    </Typography>
                </Box>
                <Stack direction="row" spacing={1}>
                    <Button
                        variant="outlined"
                        startIcon={<AutoFixHigh />}
                        onClick={() => setDiscoveryDialogOpen(true)}
                        size="small"
                    >
                        Auto Discover
                    </Button>
                    <Button
                        variant="contained"
                        startIcon={<Add />}
                        onClick={handleAddClick}
                        size="small"
                    >
                        Add Location
                    </Button>
                </Stack>
            </Box>

            {/* Empty State */}
            {locations.length === 0 && !loading && (
                <UnifiedCard
                    title="No Skill Locations"
                    subtitle="Get started by discovering or adding your first skill location"
                    size="large"
                >
                    <Box textAlign="center" py={3}>
                        <Alert severity="info" sx={{ mb: 2, display: 'inline-block', textAlign: 'left' }}>
                            <Typography variant="body2">
                                <strong>About Skills</strong><br />
                                Skills are reusable AI prompts stored as markdown files in your IDE
                                configuration directories. Tingly Box can discover and manage these
                                skills from multiple sources.
                            </Typography>
                        </Alert>
                        <Stack direction="row" spacing={2} justifyContent="center" sx={{ mt: 2 }}>
                            <Button
                                variant="outlined"
                                onClick={() => setDiscoveryDialogOpen(true)}
                            >
                                Auto Discover
                            </Button>
                            <Button variant="contained" onClick={handleAddClick}>
                                Add Location Manually
                            </Button>
                        </Stack>
                    </Box>
                </UnifiedCard>
            )}

            {/* Three-Column Layout */}
            {locations.length > 0 && (
                <Stack direction="row" spacing={1} sx={{ height: 'calc(100vh - 180px)' }}>
                    {/* Column 1: Locations List */}
                    <Paper
                        sx={{
                            width: 300,
                            display: 'flex',
                            flexDirection: 'column',
                            border: 1,
                            borderColor: 'divider',
                            borderRadius: 2,
                            overflow: 'hidden',
                        }}
                    >
                        <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                            <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                                Locations ({locations.length})
                            </Typography>
                            <TextField
                                placeholder="Search..."
                                value={locationSearch}
                                onChange={(e) => setLocationSearch(e.target.value)}
                                size="small"
                                fullWidth
                                InputProps={{
                                    startAdornment: (
                                        <InputAdornment position="start">
                                            <Search fontSize="small" />
                                        </InputAdornment>
                                    ),
                                }}
                            />
                        </Box>
                        <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                            {filteredLocations.map((location) => {
                                const isSelected = selectedLocation?.id === location.id;
                                return (
                                    <ListItem
                                        key={location.id}
                                        disablePadding
                                        divider
                                        sx={{
                                            bgcolor: isSelected ? 'primary.50' : 'transparent',
                                        }}
                                    >
                                        <ListItemButton
                                            onClick={() => setSelectedLocation(location)}
                                            dense
                                            sx={{ py: 1.5 }}
                                        >
                                            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5, flex: 1, minWidth: 0 }}>
                                                <Typography
                                                    variant="subtitle2"
                                                    sx={{ fontWeight: 500 }}
                                                >
                                                    {location.name}
                                                </Typography>
                                                <Typography
                                                    variant="caption"
                                                    color="text.secondary"
                                                    sx={{
                                                        overflow: 'hidden',
                                                        textOverflow: 'ellipsis',
                                                        whiteSpace: 'nowrap',
                                                        display: 'block',
                                                    }}
                                                >
                                                    {location.path}
                                                </Typography>
                                                <MuiChip
                                                    label={getIdeSourceLabel(location.ide_source)}
                                                    size="small"
                                                    variant="outlined"
                                                    sx={{ alignSelf: 'flex-start', height: 20, fontSize: '0.7rem' }}
                                                />
                                            </Box>
                                            <Stack direction="row" spacing={0.25} alignItems="center">
                                                <Typography variant="caption" color="text.secondary" sx={{ mr: 0.5 }}>
                                                    {location.skill_count}
                                                </Typography>
                                                <IconButton
                                                    size="small"
                                                    onClick={(e) => handleRefreshClick(location.id, e)}
                                                    disabled={skillsLoading}
                                                >
                                                    <Refresh fontSize="small" />
                                                </IconButton>
                                                <IconButton
                                                    size="small"
                                                    onClick={(e) => handleEditClick(location, e)}
                                                >
                                                    <Edit fontSize="small" />
                                                </IconButton>
                                                <IconButton
                                                    size="small"
                                                    color="error"
                                                    onClick={(e) => handleDeleteClick(location.id, e)}
                                                >
                                                    <Delete fontSize="small" />
                                                </IconButton>
                                            </Stack>
                                        </ListItemButton>
                                    </ListItem>
                                );
                            })}
                        </List>
                    </Paper>

                    {/* Column 2: Skills List */}
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
                            <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                                {selectedLocation ? selectedLocation.name : 'Skills'}
                                {selectedLocation && ` (${skills.length})`}
                            </Typography>
                            <TextField
                                placeholder="Search skills..."
                                value={skillSearch}
                                onChange={(e) => setSkillSearch(e.target.value)}
                                size="small"
                                fullWidth
                                disabled={!selectedLocation}
                                InputProps={{
                                    startAdornment: (
                                        <InputAdornment position="start">
                                            <Search fontSize="small" />
                                        </InputAdornment>
                                    ),
                                }}
                            />
                        </Box>
                        <Box sx={{ flex: 1, overflow: 'auto' }}>
                            {!selectedLocation ? (
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
                                        Select a location to view skills
                                    </Typography>
                                </Box>
                            ) : skillsLoading ? (
                                <Box
                                    sx={{
                                        display: 'flex',
                                        flexDirection: 'column',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                        height: '100%',
                                    }}
                                >
                                    <CircularProgress size={32} />
                                </Box>
                            ) : filteredSkills.length === 0 ? (
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
                                        sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }}
                                    />
                                    <Typography variant="body2" color="text.secondary">
                                        {skillSearch
                                            ? 'No skills match your search'
                                            : 'No skills found in this location'}
                                    </Typography>
                                </Box>
                            ) : (
                                <List sx={{ p: 0 }}>
                                    {filteredSkills.map((skill) => {
                                        const isSelected = selectedSkill?.id === skill.id;
                                        const relativePath = selectedLocation ? getRelativePath(skill, selectedLocation) : skill.filename;
                                        return (
                                            <ListItem
                                                key={skill.id}
                                                disablePadding
                                                divider
                                                sx={{
                                                    bgcolor: isSelected
                                                        ? 'action.selected'
                                                        : 'transparent',
                                                }}
                                            >
                                                <ListItemButton
                                                    onClick={() => setSelectedSkill(skill)}
                                                    dense
                                                >
                                                    <Description
                                                        fontSize="small"
                                                        sx={{ mr: 1.5, color: 'action.active' }}
                                                    />
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
                                                            <Typography
                                                                variant="caption"
                                                                color="text.secondary"
                                                                sx={{
                                                                    overflow: 'hidden',
                                                                    textOverflow: 'ellipsis',
                                                                    whiteSpace: 'nowrap',
                                                                    display: 'block',
                                                                }}
                                                            >
                                                                {relativePath}
                                                            </Typography>
                                                        }
                                                    />
                                                </ListItemButton>
                                            </ListItem>
                                        );
                                    })}
                                </List>
                            )}
                        </Box>
                    </Paper>

                    {/* Column 3: Skill Detail */}
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
                                alignItems: 'flex-start',
                            }}
                        >
                            <Box sx={{ minWidth: 0, flex: 1 }}>
                                <Typography
                                    variant="subtitle1"
                                    sx={{
                                        fontWeight: 600,
                                        overflow: 'hidden',
                                        textOverflow: 'ellipsis',
                                        whiteSpace: 'nowrap',
                                    }}
                                >
                                    {selectedSkill ? selectedSkill.name : 'Skill Details'}
                                </Typography>
                                {selectedSkill && (
                                    <>
                                        <Typography
                                            variant="caption"
                                            color="text.secondary"
                                            sx={{
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                whiteSpace: 'nowrap',
                                                display: 'block',
                                            }}
                                        >
                                            {selectedSkill.path}
                                        </Typography>
                                        <Typography
                                            variant="caption"
                                            color="text.secondary"
                                            sx={{
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                whiteSpace: 'nowrap',
                                                display: 'block',
                                            }}
                                        >
                                            {formatFileSize(selectedSkill.size)}
                                        </Typography>
                                    </>
                                )}
                            </Box>
                            <Stack direction="row" spacing={0.5} alignItems="center">
                                {skillContent && (
                                    <>
                                        <Button
                                            size="small"
                                            variant={viewMode === 'markdown' ? 'contained' : 'outlined'}
                                            startIcon={<Visibility />}
                                            onClick={() => setViewMode('markdown')}
                                            sx={{ minWidth: 32, px: 1 }}
                                        >
                                            Markdown
                                        </Button>
                                        <Button
                                            size="small"
                                            variant={viewMode === 'raw' ? 'contained' : 'outlined'}
                                            startIcon={<Code />}
                                            onClick={() => setViewMode('raw')}
                                            sx={{ minWidth: 32, px: 1 }}
                                        >
                                            Raw
                                        </Button>
                                        <IconButton
                                            size="small"
                                            onClick={handleCopyContent}
                                            disabled={contentLoading}
                                        >
                                            <ContentCopy fontSize="small" />
                                        </IconButton>
                                    </>
                                )}
                            </Stack>
                        </Box>
                        <Box sx={{ flex: 1, overflow: 'auto', bgcolor: 'background.default' }}>
                            {!selectedSkill ? (
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
                                        Select a skill to view its content
                                    </Typography>
                                </Box>
                            ) : contentLoading ? (
                                <Box
                                    sx={{
                                        display: 'flex',
                                        flexDirection: 'column',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                        height: '100%',
                                    }}
                                >
                                    <CircularProgress size={32} />
                                </Box>
                            ) : skillContent ? (
                                <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                                    {viewMode === 'markdown' ? (
                                        <Box
                                            sx={{
                                                flex: 1,
                                                overflow: 'auto',
                                                '& .ant-md': {
                                                    bgcolor: 'background.paper',
                                                    p: 2,
                                                    minHeight: '100%',
                                                },
                                                '& .ant-markdown': {
                                                    fontSize: '0.875rem',
                                                    lineHeight: 1.6,
                                                },
                                            }}
                                        >
                                            <XMarkdown
                                                style={{
                                                    height: '100%',
                                                }}
                                            >
                                                {skillContent}
                                            </XMarkdown>
                                        </Box>
                                    ) : (
                                        <Box
                                            sx={{
                                                p: 2,
                                                fontFamily: 'monospace',
                                                fontSize: '0.875rem',
                                                whiteSpace: 'pre-wrap',
                                                wordBreak: 'break-word',
                                                lineHeight: 1.6,
                                                flex: 1,
                                                overflow: 'auto',
                                            }}
                                        >
                                            {skillContent}
                                        </Box>
                                    )}
                                </Box>
                            ) : (
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
                                    <Alert severity="info">
                                        <Typography variant="body2">
                                            No content available for this skill
                                        </Typography>
                                    </Alert>
                                </Box>
                            )}
                        </Box>
                    </Paper>
                </Stack>
            )}

            {/* Add/Edit Location Dialog */}
            <AddSkillLocationDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddSubmit}
                initialData={
                    editLocation
                        ? {
                              name: editLocation.name,
                              path: editLocation.path,
                              ide_source: editLocation.ide_source,
                          }
                        : undefined
                }
                mode={addDialogMode}
            />

            {/* Auto Discovery Dialog */}
            <AutoDiscoveryDialog
                open={discoveryDialogOpen}
                onClose={() => setDiscoveryDialogOpen(false)}
                onImport={handleImportLocations}
            />
        </PageLayout>
    );
};

export default SkillPage;
