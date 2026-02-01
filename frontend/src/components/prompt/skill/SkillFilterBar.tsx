import { Add, Search } from '@mui/icons-material';
import {
    Box,
    Button,
    FormControl,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    Chip,
} from '@mui/material';
import { type IDESource } from '@/types/prompt';
import { IDE_SOURCES } from '@/constants/ideSources';

interface SkillFilterBarProps {
    searchQuery: string;
    onSearchChange: (query: string) => void;
    ideSourceFilter?: IDESource;
    onIdeSourceChange: (source?: IDESource) => void;
    onDiscoverClick: () => void;
    onAddClick: () => void;
}

const SkillFilterBar = ({
    searchQuery,
    onSearchChange,
    ideSourceFilter,
    onIdeSourceChange,
    onDiscoverClick,
    onAddClick,
}: SkillFilterBarProps) => {
    const hasActiveFilters = searchQuery || ideSourceFilter;

    const handleClearFilters = () => {
        onSearchChange('');
        onIdeSourceChange(undefined);
    };

    return (
        <Box
            sx={{
                display: 'flex',
                gap: 2,
                alignItems: 'center',
                flexWrap: 'wrap',
                mb: 3,
            }}
        >
            <TextField
                placeholder="Search locations..."
                value={searchQuery}
                onChange={(e) => onSearchChange(e.target.value)}
                InputProps={{
                    startAdornment: <Search sx={{ mr: 1, color: 'text.secondary' }} />,
                }}
                size="small"
                sx={{ minWidth: 250 }}
            />

            <FormControl size="small" sx={{ minWidth: 180 }}>
                <InputLabel>IDE Source</InputLabel>
                <Select
                    value={ideSourceFilter || ''}
                    label="IDE Source"
                    onChange={(e) => onIdeSourceChange(e.target.value as IDESource || undefined)}
                >
                    <MenuItem value="">All Sources</MenuItem>
                    {(Object.keys(IDE_SOURCES) as Array<keyof typeof IDE_SOURCES>).map((key) => (
                        <MenuItem key={key} value={key}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <span>{IDE_SOURCES[key].icon}</span>
                                <span>{IDE_SOURCES[key].name}</span>
                            </Box>
                        </MenuItem>
                    ))}
                </Select>
            </FormControl>

            <Box sx={{ flex: 1 }} />

            {hasActiveFilters && (
                <Stack direction="row" spacing={1} alignItems="center">
                    {(searchQuery || ideSourceFilter) && (
                        <Chip
                            label="Clear filters"
                            size="small"
                            onDelete={handleClearFilters}
                            color="primary"
                            variant="outlined"
                        />
                    )}
                </Stack>
            )}

            <Stack direction="row" spacing={1}>
                <Button
                    variant="outlined"
                    startIcon={<Search />}
                    onClick={onDiscoverClick}
                    size="small"
                >
                    Auto Discover
                </Button>
                <Button
                    variant="contained"
                    startIcon={<Add />}
                    onClick={onAddClick}
                    size="small"
                >
                    Add Location
                </Button>
            </Stack>
        </Box>
    );
};

export default SkillFilterBar;
