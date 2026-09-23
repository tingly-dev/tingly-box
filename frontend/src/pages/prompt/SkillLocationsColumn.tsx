import { Delete, Edit, Refresh, Search } from '@/components/icons';
import {
    Box,
    Chip as MuiChip,
    IconButton,
    InputAdornment,
    List,
    ListItem,
    ListItemButton,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { type SkillLocation } from '@/types/prompt';
import { getIdeSourceLabel } from '@/constants/ideSources';
import SkillColumnShell from './SkillColumnShell';

interface SkillLocationsColumnProps {
    locations: SkillLocation[];
    search: string;
    onSearchChange: (value: string) => void;
    selectedLocation: SkillLocation | null;
    onSelectLocation: (location: SkillLocation) => void;
    onRefreshLocation: (id: string, e: React.MouseEvent) => void;
    onEditLocation: (location: SkillLocation, e: React.MouseEvent) => void;
    onDeleteLocation: (location: SkillLocation, e: React.MouseEvent) => void;
    refreshDisabled: boolean;
}

const SkillLocationsColumn = ({
    locations,
    search,
    onSearchChange,
    selectedLocation,
    onSelectLocation,
    onRefreshLocation,
    onEditLocation,
    onDeleteLocation,
    refreshDisabled,
}: SkillLocationsColumnProps) => (
    <SkillColumnShell
        width={300}
        header={
            <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                    Locations ({locations.length})
                </Typography>
                <TextField
                    placeholder="Search..."
                    value={search}
                    onChange={(e) => onSearchChange(e.target.value)}
                    size="small"
                    fullWidth
                    slotProps={{
                        input: {
                            startAdornment: (
                                <InputAdornment position="start">
                                    <Search fontSize="small" />
                                </InputAdornment>
                            ),
                        }
                    }}
                />
            </Box>
        }
    >
        <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
            {locations.map((location) => {
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
                            onClick={() => onSelectLocation(location)}
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
                                    sx={{
                                        color: "text.secondary",
                                        overflow: 'hidden',
                                        textOverflow: 'ellipsis',
                                        whiteSpace: 'nowrap',
                                        display: 'block'
                                    }}>
                                    {location.path}
                                </Typography>
                                <MuiChip
                                    label={getIdeSourceLabel(location.ide_source)}
                                    size="small"
                                    variant="outlined"
                                    sx={{ alignSelf: 'flex-start', height: 20, fontSize: '0.7rem' }}
                                />
                            </Box>
                            <Stack direction="row" spacing={0.25} sx={{
                                alignItems: "center"
                            }}>
                                <Typography
                                    variant="caption"
                                    sx={{
                                        color: "text.secondary",
                                        mr: 0.5
                                    }}>
                                    {location.skill_count}
                                </Typography>
                                <IconButton
                                    size="small"
                                    aria-label={`Refresh ${location.name}`}
                                    onClick={(e) => onRefreshLocation(location.id, e)}
                                    disabled={refreshDisabled}
                                >
                                    <Refresh fontSize="small" />
                                </IconButton>
                                <IconButton
                                    size="small"
                                    aria-label={`Edit ${location.name}`}
                                    onClick={(e) => onEditLocation(location, e)}
                                >
                                    <Edit fontSize="small" />
                                </IconButton>
                                <IconButton
                                    size="small"
                                    color="error"
                                    aria-label={`Delete ${location.name}`}
                                    onClick={(e) => onDeleteLocation(location, e)}
                                >
                                    <Delete fontSize="small" />
                                </IconButton>
                            </Stack>
                        </ListItemButton>
                    </ListItem>
                );
            })}
        </List>
    </SkillColumnShell>
);

export default SkillLocationsColumn;
