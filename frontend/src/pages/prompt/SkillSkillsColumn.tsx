import {
    Description,
    ExpandLess,
    ExpandMore,
    FolderOpen,
    Search,
    ViewList,
} from '@/components/icons';
import {
    Box,
    Chip as MuiChip,
    Collapse,
    CircularProgress,
    IconButton,
    InputAdornment,
    List,
    ListItem,
    ListItemButton,
    ListItemText,
    TextField,
    Typography,
} from '@mui/material';
import { useMemo } from 'react';
import { type Skill, type SkillLocation } from '@/types/prompt';
import {
    getRelativePath,
    getTwoLevelDisplayName,
    groupSkillsIntelligently,
} from '@/components/prompt/skill/skillGrouping';
import SkillColumnShell from './SkillColumnShell';

interface SkillListItemProps {
    skill: Skill;
    selected: boolean;
    onSelect: (skill: Skill) => void;
    displayName: string;
    displayPath: string;
    indent?: boolean;
}

// Unified skill row renderer shared by grouped and flat modes
const SkillListItem = ({
    skill,
    selected,
    onSelect,
    displayName,
    displayPath,
    indent,
}: SkillListItemProps) => (
    <ListItem
        disablePadding
        divider
        sx={{
            bgcolor: selected ? 'action.selected' : 'transparent',
            ...(indent && { pl: 2 }),
        }}
    >
        <ListItemButton onClick={() => onSelect(skill)} dense sx={{ py: 1 }}>
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
                        {displayName}
                    </Typography>
                }
                secondary={
                    <Typography
                        variant="caption"
                        sx={{
                            color: "text.secondary",
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                            display: 'block'
                        }}>
                        {displayPath}
                    </Typography>
                }
            />
        </ListItemButton>
    </ListItem>
);

interface SkillSkillsColumnProps {
    selectedLocation: SkillLocation | null;
    skills: Skill[];
    skillsLoading: boolean;
    search: string;
    onSearchChange: (value: string) => void;
    selectedSkill: Skill | null;
    onSelectSkill: (skill: Skill) => void;
    isGroupedMode: boolean;
    onToggleGroupedMode: () => void;
    isGroupExpanded: (groupKey: string) => boolean;
    onToggleGroup: (groupKey: string) => void;
}

const SkillSkillsColumn = ({
    selectedLocation,
    skills,
    skillsLoading,
    search,
    onSearchChange,
    selectedSkill,
    onSelectSkill,
    isGroupedMode,
    onToggleGroupedMode,
    isGroupExpanded,
    onToggleGroup,
}: SkillSkillsColumnProps) => {
    const skillGroups = useMemo(
        () => groupSkillsIntelligently(skills, selectedLocation),
        [skills, selectedLocation]
    );

    return (
        <SkillColumnShell
            width={320}
            header={
                <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                        <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                            {selectedLocation ? selectedLocation.name : 'Skills'}
                            {selectedLocation && ` (${skills.length})`}
                        </Typography>
                        <IconButton
                            size="small"
                            onClick={onToggleGroupedMode}
                            disabled={!selectedLocation}
                            title={isGroupedMode ? 'Switch to flat view' : 'Switch to grouped view'}
                        >
                            {isGroupedMode ? <ViewList fontSize="small" /> : <Description fontSize="small" />}
                        </IconButton>
                    </Box>
                    <TextField
                        placeholder="Search skills..."
                        value={search}
                        onChange={(e) => onSearchChange(e.target.value)}
                        size="small"
                        fullWidth
                        disabled={!selectedLocation}
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
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>
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
                ) : skills.length === 0 ? (
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
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>
                            {search
                                ? 'No skills match your search'
                                : 'No skills found in this location'}
                        </Typography>
                    </Box>
                ) : (
                    <Box sx={{ flex: 1, overflow: 'auto' }}>
                        {isGroupedMode ? (
                            // Grouped mode
                            skillGroups.map((group) => {
                                const isExpanded = isGroupExpanded(group.groupKey);
                                const groupLabel = group.groupLabel;

                                return (
                                    <Box key={group.groupKey}>
                                        {/* Group Header */}
                                        <ListItem
                                            disablePadding
                                            sx={{ borderBottom: 1, borderColor: 'divider' }}
                                        >
                                            <ListItemButton
                                                onClick={() => onToggleGroup(group.groupKey)}
                                                dense
                                                sx={{ py: 0.75, px: 2 }}
                                            >
                                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                                                    {isExpanded ? <ExpandLess fontSize="small" /> : <ExpandMore fontSize="small" />}
                                                    <Typography variant="caption" sx={{ fontWeight: 600 }}>
                                                        {groupLabel}
                                                    </Typography>
                                                    <MuiChip
                                                        label={group.skills.length}
                                                        size="small"
                                                        sx={{ height: 18, fontSize: '0.65rem' }}
                                                    />
                                                </Box>
                                            </ListItemButton>
                                        </ListItem>
                                        {/* Group Content */}
                                        <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                                            <List sx={{ p: 0 }}>
                                                {group.skills.map((skill) => {
                                                    const relativePath = selectedLocation ? getRelativePath(skill, selectedLocation) : skill.filename;
                                                    // Display path: remove group prefix if exists
                                                    const displayPath = group.groupKey && relativePath.startsWith(group.groupKey + '/')
                                                        ? relativePath.substring(group.groupKey.length + 1)
                                                        : relativePath;
                                                    // Get two-level display name
                                                    const twoLevelName = selectedLocation
                                                        ? getTwoLevelDisplayName(skill, selectedLocation)
                                                        : skill.filename;
                                                    return (
                                                        <SkillListItem
                                                            key={skill.id}
                                                            skill={skill}
                                                            selected={selectedSkill?.id === skill.id}
                                                            onSelect={onSelectSkill}
                                                            displayName={twoLevelName}
                                                            displayPath={displayPath}
                                                            indent
                                                        />
                                                    );
                                                })}
                                            </List>
                                        </Collapse>
                                    </Box>
                                );
                            })
                        ) : (
                            // Flat mode
                            <List sx={{ p: 0 }}>
                                {skills.map((skill) => {
                                    const twoLevelName = selectedLocation ? getTwoLevelDisplayName(skill, selectedLocation) : skill.filename;
                                    const relativePath = selectedLocation ? getRelativePath(skill, selectedLocation) : skill.filename;
                                    return (
                                        <SkillListItem
                                            key={skill.id}
                                            skill={skill}
                                            selected={selectedSkill?.id === skill.id}
                                            onSelect={onSelectSkill}
                                            displayName={twoLevelName}
                                            displayPath={relativePath}
                                        />
                                    );
                                })}
                            </List>
                        )}
                    </Box>
                )}
            </Box>
        </SkillColumnShell>
    );
};

export default SkillSkillsColumn;
