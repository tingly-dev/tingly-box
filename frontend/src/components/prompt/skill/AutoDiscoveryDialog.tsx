import {
  Box,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  CircularProgress,
  Checkbox,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Divider,
  Alert,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { Search, CheckCircle } from '@mui/icons-material';
import type { SkillLocation } from '@/types/prompt';
import api from '@/services/api';
import React from "react";

interface DiscoveredIDE {
  key: string;
  displayName: string;
  icon: string;
  skillsCount: number;
  path: string;
  isInstalled: boolean;
  location: SkillLocation;
}

interface AutoDiscoveryDialogProps {
  open: boolean;
  onClose: () => void;
  onImport: (locations: SkillLocation[]) => void;
  autoImport?: boolean;  // If true, automatically import all discovered IDEs without showing dialog
}

const AutoDiscoveryDialog: React.FC<AutoDiscoveryDialogProps> = ({
  open,
  onClose,
  onImport,
  autoImport = false,
}) => {
  const [scanning, setScanning] = useState(false);
  const [discoveredIdes, setDiscoveredIdes] = useState<DiscoveredIDE[]>([]);
  const [selectedIdes, setSelectedIdes] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);

  // Debug state changes
  useEffect(() => {
    console.log('discoveredIdes state changed:', discoveredIdes);
    console.log('discoveredIdes.length:', discoveredIdes.length);
  }, [discoveredIdes]);

  // Trigger handleOpen when dialog opens
  useEffect(() => {
    if (open) {
      handleOpen();
    }
    // Only run when open changes, not on handleOpen changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  // Reset state when dialog opens
  const handleOpen = async () => {
    console.log('AutoDiscoveryDialog handleOpen called');
    console.log('scanResult exists?', !!(window as any).scanResult);
    if ((window as any).scanResult) {
      console.log('scanResult:', (window as any).scanResult);
    }

    setScanning(true);
    setDiscoveredIdes([]);
    setSelectedIdes(new Set());
    setError(null);

    try {
      let result: any;

      // Check if there's a pre-existing scan result from "Scan All" button
      if ((window as any).scanResult) {
        result = (window as any).scanResult;
        // Clear the stored result after using it
        delete (window as any).scanResult;
        setScanning(false); // No need to scan again
        console.log('Using stored scanResult');
      } else {
        // Perform new scan
        console.log('No stored scanResult, calling discoverIdes API');
        const response = await api.discoverIdes();
        console.log('discoverIdes response:', response);
        if (!response.success || !response.data) {
          setError('Failed to discover IDEs');
          setScanning(false);
          return;
        }
        result = response.data;
      }

      console.log('Result locations:', result.locations);
      console.log('Result locations count:', result.locations?.length);

      // Convert discovered locations to DiscoveredIDE format using backend data
      const ides: DiscoveredIDE[] = result.locations.map((loc: SkillLocation) => ({
        key: loc.ide_source,
        displayName: loc.name.replace(' Skills', ''), // Remove " Skills" suffix for cleaner display
        icon: loc.icon || '📂',
        skillsCount: loc.skill_count || 0,
        path: loc.path,
        isInstalled: loc.is_installed ?? true,
        location: loc,
      }));
      console.log('Mapped ides:', ides);
      console.log('Setting discoveredIdes with count:', ides.length);
      setDiscoveredIdes(ides);

      // Auto-import mode: automatically import all discovered IDEs
      if (autoImport && ides.length > 0) {
        setSelectedIdes(new Set(result.locations.map((l: SkillLocation) => l.id)));
        // Trigger import after a short delay for UX
        setTimeout(() => {
          handleImport();
        }, 500);
      } else {
        // Manual selection mode: auto-select all discovered IDEs
        setSelectedIdes(new Set(result.locations.map((l: SkillLocation) => l.id)));
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to scan for IDEs');
    } finally {
      setScanning(false);
    }
  };

  const handleToggleIde = (locationId: string) => {
    const newSelected = new Set(selectedIdes);
    if (newSelected.has(locationId)) {
      newSelected.delete(locationId);
    } else {
      newSelected.add(locationId);
    }
    setSelectedIdes(newSelected);
  };

  const handleToggleAll = () => {
    if (selectedIdes.size === discoveredIdes.length) {
      setSelectedIdes(new Set());
    } else {
      setSelectedIdes(new Set(discoveredIdes.map((ide) => ide.location.id)));
    }
  };

  const handleImport = () => {
    const locationsToImport = discoveredIdes
      .filter((ide) => selectedIdes.has(ide.location.id))
      .map((ide) => ide.location);
    onImport(locationsToImport);
    handleClose();
  };

  const handleClose = () => {
    onClose();
  };

  const allSelected =
    discoveredIdes.length > 0 && selectedIdes.size === discoveredIdes.length;
  const someSelected = selectedIdes.size > 0 && !allSelected;

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>Discover IDE Skills</DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mt: 1 }}>
          {/* Instructions */}
          <Typography variant="body2" color="text.secondary">
            Scan your home directory for supported IDEs and import their skills.
          </Typography>

          {/* Scanning State */}
          {scanning && (
            <Box
              sx={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                gap: 2,
                py: 4,
              }}
            >
              <CircularProgress size={48} />
              <Typography variant="body1" color="text.secondary">
                Scanning for installed IDEs...
              </Typography>
            </Box>
          )}

          {/* Error State */}
          {error && !scanning && (
            <Alert severity="error">{error}</Alert>
          )}

          {/* No IDEs Found */}
          {!scanning && discoveredIdes.length === 0 && (
            <Alert severity="info">
              No supported IDEs found. Add skill paths manually.
            </Alert>
          )}

          {/* Discovered IDEs List */}
          {!scanning && discoveredIdes.length > 0 && (
            <Box>
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  mb: 1,
                }}
              >
                <Typography variant="body2" color="text.secondary">
                  Found {discoveredIdes.length} IDE(s)
                </Typography>
                <Button
                  size="small"
                  onClick={handleToggleAll}
                  disabled={discoveredIdes.length === 0}
                >
                  {allSelected ? 'Deselect All' : 'Select All'}
                </Button>
              </Box>

              <Box
                sx={{
                  border: '1px solid',
                  borderColor: 'divider',
                  borderRadius: 2,
                  overflow: 'hidden',
                }}
              >
                {discoveredIdes.map((ide, index) => (
                  <React.Fragment key={ide.key}>
                    <ListItem disablePadding>
                      <ListItemButton
                        dense
                        onClick={() => handleToggleIde(ide.location.id)}
                      >
                        <ListItemIcon>
                          <Checkbox
                            edge="start"
                            checked={selectedIdes.has(ide.location.id)}
                            tabIndex={-1}
                            disableRipple
                          />
                        </ListItemIcon>
                        <Box
                          sx={{ fontSize: '1.5rem', mr: 1, ml: -1 }}
                        >
                          {ide.icon}
                        </Box>
                        <ListItemText
                          primary={
                            <Box
                              sx={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: 1,
                              }}
                            >
                              <Typography variant="body1" sx={{ fontWeight: 500 }}>
                                {ide.displayName}
                              </Typography>
                              {ide.isInstalled && (
                                <CheckCircle color="success" sx={{ fontSize: 16 }} />
                              )}
                            </Box>
                          }
                          secondary={`${ide.skillsCount} skill(s) • ${ide.path}`}
                        />
                      </ListItemButton>
                    </ListItem>
                    {index < discoveredIdes.length - 1 && <Divider />}
                  </React.Fragment>
                ))}
              </Box>
            </Box>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={scanning}>
          Cancel
        </Button>
        <Button
          onClick={handleImport}
          variant="contained"
          disabled={scanning || selectedIdes.size === 0}
          startIcon={<Search />}
        >
          Import Selected ({selectedIdes.size})
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AutoDiscoveryDialog;
