import { useState, useMemo, useEffect } from 'react';
import { Box, Typography, Grid, Paper, Button, Alert, IconButton } from '@mui/material';
import { Add as AddIcon, Search as SearchIcon, Close as CloseIcon, Refresh as RefreshIcon } from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import {
  SkillSearchBar,
  SkillLocationList,
  SkillLocationPanel,
  SkillDetailPanel,
  AddPathDialog,
} from '@/components/prompt';
import AutoDiscoveryDialog from '@/components/prompt/skill/AutoDiscoveryDialog';
import type { Skill, SkillLocation, IDESource } from '@/types/prompt';
import { useTranslation } from 'react-i18next';
import api from '@/services/api';

// Auto-discovery on first visit
const SKILL_ONBOARDING_KEY = 'tingly_skill_onboarded';

const SkillPage = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [locations, setLocations] = useState<SkillLocation[]>([]);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [selectedLocationId, setSelectedLocationId] = useState<string>();
  const [searchQuery, setSearchQuery] = useState('');
  const [ideFilter, setIdeFilter] = useState<IDESource>();
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [discoverDialogOpen, setDiscoverDialogOpen] = useState(false);
  const [selectedSkill, setSelectedSkill] = useState<Skill>();
  const [showOnboardingBanner, setShowOnboardingBanner] = useState(false);

  // Check onboarding status and load locations on mount
  useEffect(() => {
    const hasOnboarded = localStorage.getItem(SKILL_ONBOARDING_KEY);
    loadLocations();

    // Show onboarding banner if not yet onboarded
    if (!hasOnboarded) {
      setShowOnboardingBanner(true);
    }
  }, []);

  const loadLocations = async (showLoading = true) => {
    if (showLoading) {
      setLoading(true);
    }
    try {
      const response = await api.getSkillLocations();
      if (response.success && response.data) {
        setLocations(response.data);
      }
    } catch (error) {
      console.error('Failed to load skill locations:', error);
    } finally {
      if (showLoading) {
        setLoading(false);
      }
    }
  };

  const loadSkillsForLocation = async (locationId: string) => {
    try {
      const response = await api.refreshSkillLocation(locationId);
      if (response.success && response.data) {
        // Update skills from the scan result
        if (response.data.skills) {
          setSkills(response.data.skills);
          // Update the location's skill count directly without re-fetching
          setLocations((prev) =>
            prev.map((loc) =>
              loc.id === locationId
                ? { ...loc, skill_count: response.data.skills?.length || 0 }
                : loc
            )
          );
        }
      }
    } catch (error) {
      console.error('Failed to load skills:', error);
    }
  };

  const handleAutoDiscover = () => {
    // Trigger automatic discovery without showing dialog
    // AutoDiscoverDialog will handle the discovery and auto-import
    setDiscoverDialogOpen(true);
  };

  const handleOnboardingComplete = () => {
    localStorage.setItem(SKILL_ONBOARDING_KEY, 'true');
    setShowOnboardingBanner(false);
  };

  const selectedLocation = useMemo(
    () => locations.find((l) => l.id === selectedLocationId),
    [selectedLocationId, locations]
  );

  // For skill detail, use the skill's location if available, otherwise use selected location
  const skillLocation = useMemo(
    () => {
      if (selectedSkill) {
        return locations.find((l) => l.id === selectedSkill.location_id) || selectedLocation;
      }
      return selectedLocation;
    },
    [selectedSkill, selectedLocation, locations]
  );

  const filteredSkills = useMemo(() => {
    return skills.filter((skill) => {
      // If a location is selected, only show skills from that location
      const matchesLocation = !selectedLocationId || skill.location_id === selectedLocationId;

      // Search query filter
      const matchesSearch =
        searchQuery === '' ||
        skill.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        skill.description?.toLowerCase().includes(searchQuery.toLowerCase());

      // If an IDE filter is set and no location is selected, filter by IDE
      const matchesIde = !ideFilter || !selectedLocationId;
      if (ideFilter && !selectedLocationId) {
        // Find the location for this skill and check its IDE
        const skillLocation = locations.find((l) => l.id === skill.location_id);
        const matchesIdeFilter = skillLocation?.ide_source === ideFilter;
        if (!matchesIdeFilter) {
          return false;
        }
      }

      return matchesLocation && matchesSearch;
    });
  }, [skills, selectedLocationId, searchQuery, ideFilter, locations]);

  const handleSelectLocation = (location: SkillLocation) => {
    setSelectedLocationId(location.id);
    // Load skills for the selected location
    loadSkillsForLocation(location.id);
  };

  const handleRemoveLocation = async (locationId: string) => {
    try {
      await api.removeSkillLocation(locationId);
      setLocations(locations.filter((l) => l.id !== locationId));
      if (selectedLocationId === locationId) {
        setSelectedLocationId(undefined);
        setSkills([]);
      }
    } catch (error) {
      console.error('Failed to remove location:', error);
    }
  };

  const handleRefreshLocation = async (locationId: string) => {
    await loadSkillsForLocation(locationId);
  };

  const handleAddLocation = async (data: { name: string; path: string; ideSource: IDESource }) => {
    try {
      const response = await api.addSkillLocation({
        name: data.name,
        path: data.path,
        ide_source: data.ideSource,
      });
      if (response.success) {
        await loadLocations(false);
        setAddDialogOpen(false);
      }
    } catch (error) {
      console.error('Failed to add location:', error);
    }
  };

  const handleImportDiscovered = async (importedLocations: SkillLocation[]) => {
    try {
      const response = await api.importSkillLocations(importedLocations);
      if (response.success) {
        await loadLocations(false);
      }
    } catch (error) {
      console.error('Failed to import locations:', error);
    }
  };

  const handleOpenSkill = (skill: Skill) => {
    setSelectedSkill(skill);
  };

  const handleOpenInEditor = (skill: Skill) => {
    console.log('Open skill in editor:', skill);
    // TODO: Implement open in default editor
  };

  const handleOpenAll = () => {
    console.log('Open all skills in location:', selectedLocationId);
    // TODO: Implement open all
  };

  const handleOpenFolder = () => {
    console.log('Open folder:', selectedLocation?.path);
    // TODO: Implement open folder in file manager
  };

  const handleScanAll = async () => {
    setLoading(true);
    try {
      const response = await api.scanIdes();
      console.log('Scan API response:', response);
      if (response.success && response.data) {
        console.log('Scan result data:', response.data);
        // Store the scan result FIRST (synchronous)
        (window as any).scanResult = response.data;
        console.log('Stored scanResult, now opening dialog');
        // Then show discovered IDEs in the dialog (async state update)
        setDiscoverDialogOpen(true);
      }
    } catch (error) {
      console.error('Failed to scan IDEs:', error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <PageLayout loading={loading}>
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        {/* Onboarding Banner */}
        {showOnboardingBanner && (
          <Alert
            severity="info"
            sx={{ mb: 2 }}
            action={
              <IconButton
                aria-label="close"
                color="inherit"
                size="small"
                onClick={handleOnboardingComplete}
              >
                <CloseIcon fontSize="inherit" />
              </IconButton>
            }
          >
            <Box>
              <Typography variant="body2">
                First time here? Auto-discover IDE skills from your home directory to get started.
              </Typography>
              <Button
                size="small"
                variant="contained"
                startIcon={<SearchIcon />}
                onClick={handleAutoDiscover}
                sx={{ mt: 1 }}
              >
                Auto-Discover Now
              </Button>
            </Box>
          </Alert>
        )}

        {/* Header */}
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 3 }}>
          <Box>
            <Typography variant="h3" sx={{ fontWeight: 600, mb: 1 }}>
              Skills
            </Typography>
            <Typography variant="body1" color="text.secondary">
              Manage skills from your IDE directories
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', gap: 1 }}>
            <Button
              variant="outlined"
              startIcon={<RefreshIcon />}
              onClick={handleScanAll}
              disabled={loading}
            >
              Scan All
            </Button>
            <Button
              variant="outlined"
              startIcon={<SearchIcon />}
              onClick={() => setDiscoverDialogOpen(true)}
            >
              Auto-Discover
            </Button>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => setAddDialogOpen(true)}
            >
              Add Path
            </Button>
          </Box>
        </Box>

        {/* Search and Filter */}
        <Paper sx={{ p: 2, mb: 2 }}>
          <SkillSearchBar
            searchQuery={searchQuery}
            onSearchChange={setSearchQuery}
            ideFilter={ideFilter}
            onIdeFilterChange={setIdeFilter}
          />
        </Paper>

        {/* Triple Panel Layout */}
        <Grid container spacing={2} sx={{ flex: 1, overflow: 'hidden' }}>
          <Grid item xs={12} md={3} sx={{ height: '100%' }}>
            <Paper sx={{ height: '100%', p: 2, overflow: 'hidden' }}>
              <SkillLocationList
                locations={locations}
                selectedLocationId={selectedLocationId}
                onSelectLocation={handleSelectLocation}
                onRemoveLocation={handleRemoveLocation}
                onRefreshLocation={handleRefreshLocation}
              />
            </Paper>
          </Grid>
          <Grid item xs={12} md={5} sx={{ height: '100%' }}>
            <Paper sx={{ height: '100%', p: 2, overflow: 'hidden' }}>
              <SkillLocationPanel
                location={selectedLocation}
                skills={filteredSkills}
                onOpenSkill={handleOpenSkill}
                onOpenAll={handleOpenAll}
                onOpenFolder={handleOpenFolder}
              />
            </Paper>
          </Grid>
          <Grid item xs={12} md={4} sx={{ height: '100%' }}>
            <Paper sx={{ height: '100%', p: 2, overflow: 'hidden' }}>
              <SkillDetailPanel
                skill={selectedSkill}
                location={selectedLocation}
                onOpen={handleOpenInEditor}
              />
            </Paper>
          </Grid>
        </Grid>

        {/* Add Path Dialog */}
        <AddPathDialog
          open={addDialogOpen}
          onClose={() => setAddDialogOpen(false)}
          onAdd={handleAddLocation}
        />

        {/* Auto Discovery Dialog */}
        <AutoDiscoveryDialog
          open={discoverDialogOpen}
          onClose={() => setDiscoverDialogOpen(false)}
          onImport={handleImportDiscovered}
        />
      </Box>
    </PageLayout>
  );
};

export default SkillPage;
