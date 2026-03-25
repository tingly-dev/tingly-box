package config

// GUIConfig holds GUI-specific settings (slim/gui mode)
type GUIConfig struct {
	// Debug enables debug mode for GUI (gin debug logging, detailed logs)
	Debug bool `json:"debug"`
	// Port specifies the GUI server port. 0 means use ServerPort from global config
	Port int `json:"port"`
	// Verbose enables verbose logging for GUI
	Verbose bool `json:"verbose"`
}

// ============
// GUI Configuration
// ============

// GetGUIDebug returns the GUI debug setting
func (c *Config) GetGUIDebug() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.GUI.Debug
}

// SetGUIDebug updates the GUI debug setting
func (c *Config) SetGUIDebug(debug bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GUI.Debug = debug
	return c.Save()
}

// GetGUIPort returns the GUI port setting (0 means use ServerPort)
func (c *Config) GetGUIPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.GUI.Port
}

// SetGUIPort updates the GUI port setting
func (c *Config) SetGUIPort(port int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GUI.Port = port
	return c.Save()
}

// GetGUIVerbose returns the GUI verbose setting
func (c *Config) GetGUIVerbose() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.GUI.Verbose
}

// SetGUIVerbose updates the GUI verbose setting
func (c *Config) SetGUIVerbose(verbose bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GUI.Verbose = verbose
	return c.Save()
}
