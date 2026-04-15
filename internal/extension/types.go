package extension

// Extension represents a code-defined extension type
type Extension struct {
	ID          string            // Unique identifier (e.g., "vmodel", "mcp")
	Name        string            // Display name
	Description string            // Human-readable description
	Icon        string            // Icon identifier
	Metadata    map[string]string // Additional metadata
}

// ExtensionItem represents a code-defined item within an extension
type ExtensionItem struct {
	ID          string                 // Unique identifier (e.g., "compact-thinking")
	ExtensionID string                 // Parent extension ID
	Name        string                 // Display name
	Description string                 // Description
	Icon        string                 // Icon
	Type        string                 // Item type (domain-specific, e.g., "static", "proxy", "tool")
	Metadata    map[string]interface{} // Domain-specific metadata
	Config      map[string]interface{} // Default config schema
}

// ExtensionConfig represents user configuration stored in database
type ExtensionConfig struct {
	ID        string // "vmodel" (extension) or "compact-thinking" (item)
	Type      string // "extension" or "item"
	ParentID  string // For items: parent extension ID
	Enabled   bool   // User toggle
	Config    string // JSON config override (items only)
	Order     int    // Display order
	CreatedAt int64
	UpdatedAt int64
}

// ExtensionView represents an extension with merged code definition and database state
type ExtensionView struct {
	*Extension      // Code definition
	Enabled    bool // From database (or default: true)
	Order      int  // From database (or default: 0)
	Items      []ExtensionItemView
}

// ExtensionItemView represents an item with merged code definition and database state
type ExtensionItemView struct {
	*ExtensionItem        // Code definition
	Enabled        bool   // From database (or default: true)
	Config         string // JSON from database (or default: "")
	Order          int    // From database (or default: 0)
}
