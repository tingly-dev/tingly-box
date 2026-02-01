import { Box, Card, CardContent, Typography, Chip } from '@mui/material';
import type { Skill } from '@/types/prompt';

interface SkillCardProps {
  skill: Skill;
  onOpen: () => void;
}

const SkillCard: React.FC<SkillCardProps> = ({ skill, onOpen }) => {
  return (
    <Card
      onClick={onOpen}
      sx={{
        height: '100%',
        cursor: 'pointer',
        transition: 'all 0.2s ease-in-out',
        '&:hover': {
          transform: 'translateY(-2px)',
          boxShadow: 3,
        },
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 2,
      }}
    >
      <CardContent sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        {/* File Type Chip and Name */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
          <Chip
            label={skill.file_type}
            size="small"
            variant="filled"
            color="primary"
            sx={{ fontSize: '0.7rem', fontWeight: 500 }}
          />
          <Typography
            variant="body1"
            sx={{
              fontWeight: 500,
              flex: 1,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {skill.name}
          </Typography>
        </Box>

        {/* Description */}
        {skill.description && (
          <Typography
            variant="caption"
            sx={{
              color: 'text.secondary',
              display: '-webkit-box',
              WebkitLineClamp: 2,
              WebkitBoxOrient: 'vertical',
              overflow: 'hidden',
              flex: 1,
            }}
          >
            {skill.description}
          </Typography>
        )}

        {/* Filename */}
        <Typography
          variant="caption"
          sx={{
            color: 'text.disabled',
            mt: 'auto',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {skill.filename}
        </Typography>
      </CardContent>
    </Card>
  );
};

export default SkillCard;
