import React from 'react';
import {
  Box,
  Card,
  CardActions,
  CardContent,
  CardHeader,
  Avatar,
  Button,
  Stack,
  Typography,
  Grid,
} from '@mui/material';
import ProviderIcon from '@/components/ProviderIcon';
import ProtocolBadge from './ProtocolBadge';

interface ProviderData {
  id: string;
  name: string;
  name_zh?: string;
  description: string;
  description_zh?: string;
  icon: string;
  website: string;
  api_doc: string;
  base_url_openai?: string;
  base_url_anthropic?: string;
}

interface ProviderGridProps {
  providers: ProviderData[];
}

const ProviderCard: React.FC<{ provider: ProviderData }> = ({ provider }) => {
  return (
    <Card>
      <CardHeader
        avatar={
          <Avatar sx={{ bgcolor: 'transparent' }}>
            <ProviderIcon identifier={provider.icon} size={32} />
          </Avatar>
        }
        title={
          <Typography variant="subtitle2" fontWeight={500}>
            {provider.name}
          </Typography>
        }
        subheader={
          <Typography variant="caption" color="text.secondary">
            {provider.name_zh || ''}
          </Typography>
        }
      />
      <CardContent sx={{ pt: 0 }}>
        <Stack direction="row" spacing={1} useFlexGap sx={{ mb: 1 }}>
          {provider.base_url_openai && <ProtocolBadge protocol="OpenAI" />}
          {provider.base_url_anthropic && <ProtocolBadge protocol="Anthropic" />}
        </Stack>
        <Typography
          variant="body2"
          color="text.secondary"
          sx={{
            display: '-webkit-box',
            WebkitLineClamp: 2,
            WebkitBoxOrient: 'vertical',
            overflow: 'hidden',
          }}
        >
          {provider.description}
        </Typography>
      </CardContent>
      <CardActions sx={{ px: 2, pb: 2 }}>
        <Button
          size="small"
          href={provider.website}
          target="_blank"
          rel="noopener noreferrer"
        >
          Website
        </Button>
        <Button
          size="small"
          href={provider.api_doc}
          target="_blank"
          rel="noopener noreferrer"
        >
          API Docs
        </Button>
      </CardActions>
    </Card>
  );
};

const ProviderGrid: React.FC<ProviderGridProps> = ({ providers }) => {
  if (providers.length === 0) {
    return (
      <Box sx={{ textAlign: 'center', py: 8 }}>
        <Typography color="text.secondary">
          No providers found matching your filters.
        </Typography>
      </Box>
    );
  }

  return (
    <Grid container spacing={2}>
      {providers.map((provider) => (
        <Grid item xs={12} sm={6} md={4} lg={3} key={provider.id}>
          <ProviderCard provider={provider} />
        </Grid>
      ))}
    </Grid>
  );
};

export default ProviderGrid;
