import { Box, List, ListItem, ListItemButton, ListItemText, Badge, IconButton, Typography, Chip } from '@mui/material';
import { Delete as DeleteIcon, Refresh } from '@mui/icons-material';
import type { SkillLocation } from '@/types/prompt';

interface SkillLocationListProps {
  locations: SkillLocation[];
  selectedLocationId?: string;
  onSelectLocation: (location: SkillLocation) => void;
  onRemoveLocation: (locationId: string) => void;
  onRefreshLocation: (locationId: string) => void;
}

const SkillLocationList: React.FC<SkillLocationListProps> = ({
  locations,
  selectedLocationId,
  onSelectLocation,
  onRemoveLocation,
  onRefreshLocation,
}) => {
  return (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
        Locations ({locations.length})
      </Typography>
      <List sx={{ flex: 1, overflowY: 'auto', py: 0 }}>
        {locations.length === 0 ? (
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              py: 4,
              color: 'text.secondary',
            }}
          >
            <Typography variant="body2">No skill locations added</Typography>
          </Box>
        ) : (
          locations.map((location) => (
            <ListItem
              key={location.id}
              disablePadding
              sx={{ mb: 0.5 }}
            >
              <ListItemButton
                selected={selectedLocationId === location.id}
                onClick={() => onSelectLocation(location)}
                sx={{
                  borderRadius: 2,
                  border: '1px solid',
                  borderColor: selectedLocationId === location.id ? 'primary.main' : 'divider',
                  backgroundColor: selectedLocationId === location.id ? 'primary.50' : 'background.paper',
                  '&:hover': {
                    backgroundColor: selectedLocationId === location.id ? 'primary.100' : 'action.hover',
                  },
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                  <Chip
                    label={location.ide_source}
                    size="small"
                    variant="outlined"
                    sx={{ fontSize: '0.7rem', textTransform: 'uppercase' }}
                  />
                  <ListItemText
                    primary={
                      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                        <Typography variant="body1" sx={{ fontWeight: 500 }}>
                          {location.name}
                        </Typography>
                        <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                          <Badge
                            badgeContent={location.skill_count}
                            color="primary"
                            sx={{ mr: 1 }}
                          />
                          <IconButton
                            size="small"
                            onClick={(e) => {
                              e.stopPropagation();
                              onRefreshLocation(location.id);
                            }}
                            title="Refresh"
                          >
                            <Refresh fontSize="small" />
                          </IconButton>
                          <IconButton
                            size="small"
                            onClick={(e) => {
                              e.stopPropagation();
                              onRemoveLocation(location.id);
                            }}
                            title="Remove"
                            color="error"
                          >
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </Box>
                      </Box>
                    }
                    secondary={
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
                    }
                  />
                </Box>
              </ListItemButton>
            </ListItem>
          ))
        )}
      </List>
    </Box>
  );
};

export default SkillLocationList;
