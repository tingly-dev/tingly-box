import React from 'react';
import {
  Box,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
  Paper,
  Stack,
  Typography,
  Tooltip,
  alpha,
} from '@mui/material';
import { Language as LanguageIcon } from '@/components/icons';
import { Description as DescriptionIcon } from '@/components/icons';
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

interface ProviderTableProps {
  providers: ProviderData[];
  onCopySuccess?: (message: string) => void;
}

const ProviderTable: React.FC<ProviderTableProps> = ({ providers, onCopySuccess }) => {
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
    <TableContainer
      component={Paper}
      elevation={0}
      sx={{
        border: 1,
        borderColor: 'divider',
        borderRadius: 2,
        boxShadow: 'none',
        overflowX: 'auto',
      }}
    >
      <Table sx={{ minWidth: 980 }}>
        <TableHead>
          <TableRow sx={{ bgcolor: 'action.hover' }}>
            <TableCell width={60} sx={{ pl: 2, py: 1.25, fontWeight: 600 }}>
              Icon
            </TableCell>
            <TableCell sx={{ py: 1.25, fontWeight: 600 }}>Provider</TableCell>
            <TableCell width={160} sx={{ py: 1.25, fontWeight: 600 }}>Protocol</TableCell>
            <TableCell sx={{ py: 1.25, fontWeight: 600 }}>Description</TableCell>
            <TableCell width={120} align="right" sx={{ pr: 2, py: 1.25, fontWeight: 600 }}>
              Actions
            </TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {providers.map((provider) => (
            <TableRow
              key={provider.id}
              sx={{
                '& > .MuiTableCell-root': {
                  py: 1.25,
                },
                '&:last-child td, &:last-child th': { border: 0 },
                '&:hover': {
                  bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04),
                },
              }}
            >
              <TableCell sx={{ pl: 2 }}>
                <ProviderIcon identifier={provider.icon} size={32} />
              </TableCell>
              <TableCell>
                <Box>
                  <Typography variant="subtitle2" fontWeight={500}>
                    {provider.name}
                  </Typography>
                  {provider.name_zh && (
                    <Typography variant="caption" color="text.secondary">
                      {provider.name_zh}
                    </Typography>
                  )}
                </Box>
              </TableCell>
              <TableCell>
                <Stack direction="row" spacing={1} useFlexGap>
                  {provider.base_url_openai && (
                    <Tooltip title={provider.base_url_openai} arrow>
                      <Box>
                        <ProtocolBadge
                          protocol="OpenAI"
                          onClick={() => {
                            navigator.clipboard.writeText(provider.base_url_openai!);
                            onCopySuccess?.('OpenAI base URL copied to clipboard');
                          }}
                        />
                      </Box>
                    </Tooltip>
                  )}
                  {provider.base_url_anthropic && (
                    <Tooltip title={provider.base_url_anthropic} arrow>
                      <Box>
                        <ProtocolBadge
                          protocol="Anthropic"
                          onClick={() => {
                            navigator.clipboard.writeText(provider.base_url_anthropic!);
                            onCopySuccess?.('Anthropic base URL copied to clipboard');
                          }}
                        />
                      </Box>
                    </Tooltip>
                  )}
                </Stack>
              </TableCell>
              <TableCell>
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{
                    maxWidth: 400,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {provider.description}
                </Typography>
              </TableCell>
              <TableCell align="right" sx={{ pr: 2 }}>
                <Stack direction="row" spacing={0.5} justifyContent="flex-end">
                  <IconButton
                    size="small"
                    href={provider.website}
                    target="_blank"
                    rel="noopener noreferrer"
                    title="Website"
                  >
                    <LanguageIcon fontSize="small" />
                  </IconButton>
                  <IconButton
                    size="small"
                    href={provider.api_doc}
                    target="_blank"
                    rel="noopener noreferrer"
                    title="API Documentation"
                  >
                    <DescriptionIcon fontSize="small" />
                  </IconButton>
                </Stack>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

export default ProviderTable;
