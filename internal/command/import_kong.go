//go:build kong

package command

// ImportCmdKong imports routing rules
type ImportCmdKong struct {
	File   string `kong:"arg,optional,help='Import file path (default: stdin)'"`
	Format string `kong:"flag,name='format',default='auto',help='Import format (auto, jsonl, base64)'"`
}

func (i *ImportCmdKong) Run(appManager *AppManager) error {
	return runImport(appManager, i.Format, []string{i.File})
}
