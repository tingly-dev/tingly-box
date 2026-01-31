import { Box, Typography, Grid, Button } from '@mui/material';
import { FolderOpen } from '@mui/icons-material';
import type { Skill, SkillLocation } from '@/types/prompt';
import SkillCard from './SkillCard';

interface SkillLocationPanelProps {
  location?: SkillLocation;
  skills: Skill[];
  onOpenSkill: (skill: Skill) => void;
  onOpenAll: () => void;
  onOpenFolder: () => void;
}

const SkillLocationPanel: React.FC<SkillLocationPanelProps> = ({
  location,
  skills,
  onOpenSkill,
  onOpenAll,
  onOpenFolder,
}) => {
  if (!location) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
          color: 'text.secondary',
        }}
      >
        <Typography variant="body1">Select a location to view skills</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      <Box sx={{ mb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
          <Typography variant="h6" sx={{ fontWeight: 600 }}>
            {location.name}
          </Typography>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            ({skills.length} skills)
          </Typography>
        </Box>
        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
            display: 'block',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {location.path}
        </Typography>
      </Box>

      {/* Action Buttons */}
      <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
        <Button
          variant="outlined"
          size="small"
          onClick={onOpenAll}
          disabled={skills.length === 0}
          startIcon={<FolderOpen />}
        >
          Open All
        </Button>
        <Button
          variant="outlined"
          size="small"
          onClick={onOpenFolder}
          startIcon={<FolderOpen />}
        >
          Open Folder
        </Button>
      </Box>

      {/* Skills Grid */}
      <Box sx={{ flex: 1, overflowY: 'auto' }}>
        {skills.length === 0 ? (
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
            <Typography variant="body1">No skills found in this location</Typography>
          </Box>
        ) : (
          <Grid container spacing={2}>
            {skills.map((skill) => (
              <Grid item xs={12} sm={6} md={4} lg={3} key={skill.id}>
                <SkillCard skill={skill} onOpen={() => onOpenSkill(skill)} />
              </Grid>
            ))}
          </Grid>
        )}
      </Box>
    </Box>
  );
};

export default SkillLocationPanel;
