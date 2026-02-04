import React, { useState } from 'react';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  TextField,
  Alert,
  List,
  ListItem,
  ListItemText,
  Divider,
} from '@mui/material';
import {
  Add as AddIcon,
  Key as KeyIcon,
  ContentCopy as CopyIcon,
  Check as CheckIcon,
  Warning as WarningIcon,
  Delete as RevokeIcon,
} from '@mui/icons-material';
import { alpha } from '@mui/material/styles';
import { generateToken, validateToken, revokeToken } from '@/services/api';
import type { TokenInfo } from '@/types';

export const TokensPage: React.FC = () => {
  const [generatedToken, setGeneratedToken] = useState<TokenInfo | null>(null);
  const [clientId, setClientId] = useState('');
  const [expiryHours, setExpiryHours] = useState<number>(24);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [validateTokenInput, setValidateTokenInput] = useState('');
  const [validateResult, setValidateResult] = useState<{ valid: boolean; message?: string; client_id?: string } | null>(null);

  const handleGenerateToken = async () => {
    if (!clientId.trim()) {
      setError('Please enter a client ID');
      return;
    }

    setLoading(true);
    setError(null);
    setSuccess(null);
    setGeneratedToken(null);

    try {
      const result = await generateToken(clientId.trim(), expiryHours);
      setGeneratedToken(result.token);
      setSuccess('Token generated successfully! Make sure to copy it now - you won\'t be able to see it again.');
    } catch (err) {
      setError('Failed to generate token. Please try again.');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const handleCopyToken = () => {
    if (generatedToken) {
      navigator.clipboard.writeText(generatedToken.token);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleValidateToken = async () => {
    if (!validateTokenInput.trim()) {
      setError('Please enter a token to validate');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const result = await validateToken(validateTokenInput.trim());
      setValidateResult(result.result);
    } catch {
      setValidateResult({ valid: false, message: 'Validation failed' });
    } finally {
      setLoading(false);
    }
  };

  const handleRevokeToken = async () => {
    if (!clientId.trim()) {
      setError('Please enter a client ID to revoke');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const result = await revokeToken(clientId.trim());
      setSuccess(result.message);
      setClientId('');
    } catch {
      setError('Failed to revoke token');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Box>
      <Typography variant="h4" fontWeight={700} gutterBottom>
        Token Management
      </Typography>
      <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
        Generate and manage API tokens for client authentication
      </Typography>

      {/* Generate Token */}
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3, mb: 4 }}>
        <Box sx={{ width: { xs: '100%', lg: 'calc(58.333% - 12px)' } }}>
          <Card>
            <CardContent sx={{ p: 2 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
                <AddIcon color="primary" fontSize="small" />
                <Typography variant="subtitle1" fontWeight={600}>
                  Generate New Token
                </Typography>
              </Box>

              {error && (
                <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>
                  {error}
                </Alert>
              )}

              {success && (
                <Alert severity="success" sx={{ mb: 3 }} onClose={() => setSuccess(null)}>
                  {success}
                </Alert>
              )}

              {generatedToken && (
                <Alert
                  severity="warning"
                  sx={{ mb: 3 }}
                  icon={<WarningIcon />}
                >
                  <Typography variant="body2" fontWeight={600} gutterBottom>
                    Token Generated - Copy Now!
                  </Typography>
                  <Box
                    sx={{
                      p: 2,
                      mt: 1,
                      bgcolor: alpha('#f59e0b', 0.1),
                      borderRadius: 1,
                      fontFamily: 'monospace',
                      wordBreak: 'break-all',
                      fontSize: 12,
                    }}
                  >
                    {generatedToken.token}
                  </Box>
                  <Button
                    variant="outlined"
                    size="small"
                    startIcon={copied ? <CheckIcon /> : <CopyIcon />}
                    onClick={handleCopyToken}
                    sx={{ mt: 1 }}
                  >
                    {copied ? 'Copied!' : 'Copy to Clipboard'}
                  </Button>
                </Alert>
              )}

              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mt: 2 }}>
                <Box>
                  <TextField
                    fullWidth
                    label="Client ID"
                    placeholder="e.g., ssh:user@hostname or user-api-key"
                    value={clientId}
                    onChange={(e) => setClientId(e.target.value)}
                    helperText="Unique identifier for the client (e.g., SSH user@host)"
                    size="small"
                  />
                </Box>
                <Box>
                  <TextField
                    fullWidth
                    label="Expiry (hours)"
                    type="number"
                    value={expiryHours}
                    onChange={(e) => setExpiryHours(Number(e.target.value))}
                    inputProps={{ min: 1, max: 8760 }}
                    helperText="0 = no expiry"
                    size="small"
                  />
                </Box>
                <Box sx={{ display: 'flex', gap: 2 }}>
                  <Button
                    variant="contained"
                    startIcon={<KeyIcon />}
                    onClick={handleGenerateToken}
                    disabled={loading}
                    fullWidth
                  >
                    Generate Token
                  </Button>
                </Box>
              </Box>
            </CardContent>
          </Card>
        </Box>

        <Box sx={{ width: { xs: '100%', lg: 'calc(41.666% - 12px)' } }}>
          <Card>
            <CardContent sx={{ p: 3 }}>
              <Typography variant="h6" fontWeight={600} gutterBottom>
                Token Information
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Tokens are generated with the following format:
              </Typography>
              <Box
                sx={{
                  p: 2,
                  bgcolor: alpha('#2563eb', 0.05),
                  borderRadius: 1,
                  fontFamily: 'monospace',
                  fontSize: 12,
                  mb: 2,
                }}
              >
                tingly-box-base64encodedtoken...
              </Box>
              <List dense>
                <ListItem>
                  <ListItemText
                    primary="Format"
                    secondary="Base64-encoded JWT with tingly-box- prefix"
                  />
                </ListItem>
                <Divider />
                <ListItem>
                  <ListItemText
                    primary="Default Expiry"
                    secondary="24 hours from generation"
                  />
                </ListItem>
                <Divider />
                <ListItem>
                  <ListItemText
                    primary="SSH Tokens"
                    secondary="Generated via 'ssh user@server opsx-token'"
                  />
                </ListItem>
              </List>
            </CardContent>
          </Card>
        </Box>
      </Box>

      {/* Validate and Revoke */}
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3 }}>
        <Box sx={{ width: { xs: '100%', md: 'calc(50% - 12px)' } }}>
          <Card>
            <CardContent sx={{ p: 2 }}>
              <Typography variant="h6" fontWeight={600} gutterBottom>
                Validate Token
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Check if a token is valid and view its client ID.
              </Typography>

              {validateResult && (
                <Alert
                  severity={validateResult.valid ? 'success' : 'error'}
                  sx={{ mb: 2 }}
                >
                  {validateResult.message}
                  {validateResult.client_id && (
                    <Typography variant="body2" sx={{ mt: 0.5 }}>
                      Client ID: {validateResult.client_id}
                    </Typography>
                  )}
                </Alert>
              )}

              <TextField
                fullWidth
                label="Token to validate"
                placeholder="tingly-box-xxxxx..."
                value={validateTokenInput}
                onChange={(e) => setValidateTokenInput(e.target.value)}
                multiline
                rows={2}
                sx={{ mb: 2 }}
                size="small"
              />

              <Button
                variant="outlined"
                onClick={handleValidateToken}
                disabled={loading}
                fullWidth
              >
                Validate Token
              </Button>
            </CardContent>
          </Card>
        </Box>

        <Box sx={{ width: { xs: '100%', md: 'calc(50% - 12px)' } }}>
          <Card>
            <CardContent sx={{ p: 2 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1.5 }}>
                <RevokeIcon color="error" fontSize="small" />
                <Typography variant="subtitle1" fontWeight={600}>
                  Revoke Token
                </Typography>
              </Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Note: JWT tokens cannot be immediately revoked without a token blacklist.
                This records the revocation request for auditing.
              </Typography>

              <TextField
                fullWidth
                label="Client ID to revoke"
                placeholder="Enter the client ID"
                value={clientId}
                onChange={(e) => setClientId(e.target.value)}
                sx={{ mb: 2 }}
                size="small"
              />

              <Button
                variant="outlined"
                color="error"
                onClick={handleRevokeToken}
                disabled={loading}
                fullWidth
              >
                Revoke Tokens
              </Button>
            </CardContent>
          </Card>
        </Box>
      </Box>
    </Box>
  );
};
