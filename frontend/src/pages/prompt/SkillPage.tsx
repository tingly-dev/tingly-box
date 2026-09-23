import { Add, AutoFixHigh } from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    Stack,
    Typography,
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { type SkillLocation, type Skill, type IDESource } from '@/types/prompt';
import { PageLayout } from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import UnifiedCard from '@/components/UnifiedCard';
import { getIdeSourceLabel } from '@/constants/ideSources';
import { api } from '@/services/api';
import AddSkillLocationDialog from '@/components/prompt/skill/AddSkillLocationDialog';
import AutoDiscoveryDialog from '@/components/prompt/skill/AutoDiscoveryDialog';
import DeleteSkillLocationDialog from '@/components/prompt/skill/DeleteSkillLocationDialog';
import useNotify from '@/hooks/useNotify';
import { useCopyFeedback } from '@/hooks/useCopyFeedback';
import SkillLocationsColumn from './SkillLocationsColumn';
import SkillSkillsColumn from './SkillSkillsColumn';
import SkillDetailColumn from './SkillDetailColumn';

interface AddSkillLocationData {
    name: string;
    path: string;
    ide_source: IDESource;
}

const SkillPage = () => {
    const notify = useNotify();
    const { copy } = useCopyFeedback();
    const [locations, setLocations] = useState<SkillLocation[]>([]);
    const [loading, setLoading] = useState(true);

    // Location list state
    const [locationSearch, setLocationSearch] = useState('');
    const [selectedLocation, setSelectedLocation] = useState<SkillLocation | null>(null);

    // Skill list state
    const [skills, setSkills] = useState<Skill[]>([]);
    const [skillsLoading, setSkillsLoading] = useState(false);
    const [skillSearch, setSkillSearch] = useState('');
    const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);
    const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
    const [isGroupedMode, setIsGroupedMode] = useState(true);

    // Skill detail state
    const [skillContent, setSkillContent] = useState<string>('');
    const [contentLoading, setContentLoading] = useState(false);
    const [viewMode, setViewMode] = useState<'markdown' | 'raw'>('raw');

    // Dialog states
    const [addDialogOpen, setAddDialogOpen] = useState(false);
    const [addDialogMode, setAddDialogMode] = useState<'add' | 'edit'>('add');
    const [editLocation, setEditLocation] = useState<SkillLocation | null>(null);
    const [discoveryDialogOpen, setDiscoveryDialogOpen] = useState(false);
    const [deleteLocation, setDeleteLocation] = useState<SkillLocation | null>(null);
    const [deleteLocationLoading, setDeleteLocationLoading] = useState(false);

    useEffect(() => {
        loadLocations();
    }, []);

    // Load skills when location is selected
    useEffect(() => {
        if (selectedLocation) {
            loadSkills(selectedLocation);
            // Reset expanded groups for new location, but auto-expand first group
            setExpandedGroups(new Set());
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
            setViewMode('raw');
        }
    }, [selectedSkill]);

    const showNotification = (message: string, severity: 'success' | 'error') => {
        notify.notify(severity, message);
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
            // Update the location's skill count in the locations list
            setLocations(prev =>
                prev.map(loc =>
                    loc.id === location.id
                        ? { ...loc, skill_count: result.data.skills?.length || 0 }
                        : loc
                )
            );
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

    const handleDeleteClick = (location: SkillLocation, e: React.MouseEvent) => {
        e.stopPropagation();
        setDeleteLocation(location);
    };

    const handleConfirmDeleteLocation = async () => {
        if (!deleteLocation || deleteLocationLoading) return;

        const locationId = deleteLocation.id;
        setDeleteLocationLoading(true);
        try {
            const result = await api.removeSkillLocation(locationId);
            if (result.success) {
                showNotification('Location deleted successfully!', 'success');
                if (selectedLocation?.id === locationId) {
                    setSelectedLocation(null);
                }
                setDeleteLocation(null);
                await loadLocations();
            } else {
                showNotification(`Failed to delete location: ${result.error}`, 'error');
            }
        } finally {
            setDeleteLocationLoading(false);
        }
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
        copy(skillContent, () => notify.success('Copied to clipboard!'));
    };

    const handleCopyPath = () => {
        if (selectedSkill) {
            copy(selectedSkill.path, () => notify.success('Path copied to clipboard!'));
        }
    };

    // Filter locations
    const filteredLocations = locations.filter((location) => {
        const matchesSearch =
            locationSearch === '' ||
            location.name.toLowerCase().includes(locationSearch.toLowerCase()) ||
            location.path.toLowerCase().includes(locationSearch.toLowerCase());
        return matchesSearch;
    }).sort((a, b) => {
        // Stable sort: first by IDE source, then by name
        const aSource = getIdeSourceLabel(a.ide_source);
        const bSource = getIdeSourceLabel(b.ide_source);
        if (aSource !== bSource) {
            return aSource.localeCompare(bSource);
        }
        return a.name.localeCompare(b.name);
    });

    // Filter skills
    const filteredSkills = useMemo(() => skills.filter((skill) => {
        const matchesSearch =
            skillSearch === '' ||
            skill.name.toLowerCase().includes(skillSearch.toLowerCase()) ||
            skill.filename.toLowerCase().includes(skillSearch.toLowerCase());
        return matchesSearch;
    }), [skills, skillSearch]);

    const toggleGroup = (groupKey: string) => {
        setExpandedGroups(prev => {
            const newSet = new Set(prev);
            if (newSet.has(groupKey)) {
                newSet.delete(groupKey);
            } else {
                newSet.add(groupKey);
            }
            return newSet;
        });
    };

    const isGroupExpanded = (groupKey: string) => {
        // Auto-expand if it's the only group or if search is active
        if (skillSearch !== '') return true;
        return expandedGroups.has(groupKey);
    };

    return (
        <PageLayout loading={loading}>
            <PageHeader
                title="Skill Management"
                subtitle="Manage your AI skill locations from various IDEs and tools"
                actions={
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
                }
                sx={{ mb: 2 }}
            />
            {/* Empty State */}
            {locations.length === 0 && !loading && (
                <UnifiedCard
                    title="No Skill Locations"
                    subtitle="Get started by discovering or adding your first skill location"
                    size="large"
                >
                    <Box
                        sx={{
                            textAlign: "center",
                            py: 3
                        }}>
                        <Alert severity="info" sx={{ mb: 2, display: 'inline-block', textAlign: 'left' }}>
                            <Typography variant="body2">
                                <strong>About Skills</strong><br />
                                Skills are reusable AI prompts stored as markdown files in your IDE
                                configuration directories. Tingly Box can discover and manage these
                                skills from multiple sources.
                            </Typography>
                        </Alert>
                        <Stack
                            direction="row"
                            spacing={2}
                            sx={{
                                justifyContent: "center",
                                mt: 2
                            }}>
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
                    <SkillLocationsColumn
                        locations={filteredLocations}
                        search={locationSearch}
                        onSearchChange={setLocationSearch}
                        selectedLocation={selectedLocation}
                        onSelectLocation={setSelectedLocation}
                        onRefreshLocation={handleRefreshClick}
                        onEditLocation={handleEditClick}
                        onDeleteLocation={handleDeleteClick}
                        refreshDisabled={skillsLoading}
                    />
                    {/* Column 2: Skills List */}
                    <SkillSkillsColumn
                        selectedLocation={selectedLocation}
                        skills={filteredSkills}
                        skillsLoading={skillsLoading}
                        search={skillSearch}
                        onSearchChange={setSkillSearch}
                        selectedSkill={selectedSkill}
                        onSelectSkill={setSelectedSkill}
                        isGroupedMode={isGroupedMode}
                        onToggleGroupedMode={() => setIsGroupedMode(!isGroupedMode)}
                        isGroupExpanded={isGroupExpanded}
                        onToggleGroup={toggleGroup}
                    />
                    {/* Column 3: Skill Detail */}
                    <SkillDetailColumn
                        selectedSkill={selectedSkill}
                        selectedLocation={selectedLocation}
                        content={skillContent}
                        contentLoading={contentLoading}
                        viewMode={viewMode}
                        onViewModeChange={setViewMode}
                        onCopyContent={handleCopyContent}
                        onCopyPath={handleCopyPath}
                    />
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
                              ide_source: editLocation.ide_source as IDESource,
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
            <DeleteSkillLocationDialog
                location={deleteLocation}
                loading={deleteLocationLoading}
                onClose={() => setDeleteLocation(null)}
                onConfirm={handleConfirmDeleteLocation}
            />
        </PageLayout>
    );
};

export default SkillPage;
