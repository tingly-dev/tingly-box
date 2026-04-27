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
  Stack,
  Typography,
  Link,
  Tooltip,
} from '@mui/material';
import LanguageIcon from '@mui/icons-material/Language';
import DescriptionIcon from '@mui/icons-material/Description';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
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
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell width={60} sx={{ pl: 2 }}>
              Icon
            </TableCell>
            <TableCell>Provider</TableCell>
            <TableCell width={160}>Protocol</TableCell>
            <TableCell>Description</TableCell>
            <TableCell width={120} align="right" sx={{ pr: 2 }}>
              Actions
            </TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {providers.map((provider) => (
            <TableRow
              key={provider.id}
              hover
              sx={{ '&:last-child td, &:last-child th': { border: 0 } }}
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
