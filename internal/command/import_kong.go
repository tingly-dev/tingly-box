//go:build !legacy

package command

// ImportCmdKong imports routing rules
type ImportCmdKong struct {
	File   string `kong:"arg,optional,help='Import file path (default: stdin)'"`
	Format string `kong:"flag,name='format',default='auto',help='Import format (auto, jsonl, base64)'"`
}

func (i *ImportCmdKong) Run(appManager *AppManager) error {
	var args []string
	if i.File != "" {
		args = []string{i.File}
	}
	return runImport(appManager, i.Format, args)
}
