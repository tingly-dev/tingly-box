import { useState, useMemo } from 'react';
import { Box, Typography, Grid, Paper, Button } from '@mui/material';
import { Add as AddIcon } from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import {
  SkillSearchBar,
  SkillLocationList,
  SkillLocationPanel,
  AddPathDialog,
} from '@/components/prompt';
import type { Skill, SkillLocation, IDESource } from '@/types/prompt';
import { useTranslation } from 'react-i18next';

// Mock data - TODO: Replace with actual API calls
const mockLocations: SkillLocation[] = [
  {
    id: '1',
    name: 'Claude Code Skills',
    path: '~/.claude-code/skills',
    ideSource: 'claude-code',
    skillCount: 3,
  },
  {
    id: '2',
    name: 'OpenCode Extensions',
    path: '~/.opencode/extensions',
    ideSource: 'opencode',
    skillCount: 2,
  },
];

const mockSkills: Skill[] = [
  {
    id: '1',
    name: 'code-review',
    filename: 'code-review.ts',
    path: '~/.claude-code/skills/code-review.ts',
    locationId: '1',
    fileType: '.ts',
    description: 'Automated code review skill',
  },
  {
    id: '2',
    name: 'debug-helper',
    filename: 'debug-helper.ts',
    path: '~/.claude-code/skills/debug-helper.ts',
    locationId: '1',
    fileType: '.ts',
    description: 'Debug assistance skill',
  },
  {
    id: '3',
    name: 'refactor',
    filename: 'refactor.ts',
    path: '~/.claude-code/skills/refactor.ts',
    locationId: '1',
    fileType: '.ts',
  },
];

const SkillPage = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [locations, setLocations] = useState<SkillLocation[]>(mockLocations);
  const [skills, setSkills] = useState<Skill[]>(mockSkills);
  const [selectedLocationId, setSelectedLocationId] = useState<string>();
  const [searchQuery, setSearchQuery] = useState('');
  const [ideFilter, setIdeFilter] = useState<IDESource>();
  const [addDialogOpen, setAddDialogOpen] = useState(false);

  const selectedLocation = useMemo(
    () => locations.find((l) => l.id === selectedLocationId),
    [locations, selectedLocationId]
  );

  const filteredSkills = useMemo(() => {
    return skills.filter((skill) => {
      const matchesLocation = !selectedLocationId || skill.locationId === selectedLocationId;
      const matchesSearch =
        searchQuery === '' ||
        skill.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        skill.description?.toLowerCase().includes(searchQuery.toLowerCase());
      const matchesIde = !ideFilter || selectedLocation?.ideSource === ideFilter;

      return matchesLocation && matchesSearch && matchesIde;
    });
  }, [skills, selectedLocationId, searchQuery, ideFilter, selectedLocation]);

  const handleSelectLocation = (location: SkillLocation) => {
    setSelectedLocationId(location.id);
  };

  const handleRemoveLocation = (locationId: string) => {
    setLocations(locations.filter((l) => l.id !== locationId));
    if (selectedLocationId === locationId) {
      setSelectedLocationId(undefined);
    }
  };

  const handleRefreshLocation = (locationId: string) => {
    console.log('Refresh location:', locationId);
    // TODO: Implement refresh functionality
  };

  const handleAddLocation = (data: { name: string; path: string; ideSource: IDESource }) => {
    const newLocation: SkillLocation = {
      id: Date.now().toString(),
      name: data.name,
      path: data.path,
      ideSource: data.ideSource,
      skillCount: 0,
    };
    setLocations([...locations, newLocation]);
    setAddDialogOpen(false);
    // TODO: Trigger scan of new location
  };

  const handleOpenSkill = (skill: Skill) => {
    console.log('Open skill:', skill);
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

  return (
    <PageLayout loading={loading}>
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
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
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setAddDialogOpen(true)}
          >
            Add Path
          </Button>
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

        {/* Dual Panel Layout */}
        <Grid container spacing={2} sx={{ flex: 1, overflow: 'hidden' }}>
          <Grid item xs={12} md={4} sx={{ height: '100%' }}>
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
          <Grid item xs={12} md={8} sx={{ height: '100%' }}>
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
        </Grid>

        {/* Add Path Dialog */}
        <AddPathDialog
          open={addDialogOpen}
          onClose={() => setAddDialogOpen(false)}
          onAdd={handleAddLocation}
        />
      </Box>
    </PageLayout>
  );
};

export default SkillPage;
