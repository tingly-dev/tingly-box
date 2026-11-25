package command

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/app"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
)

type DBCmdKong struct {
	Export DBExportCmdKong `kong:"cmd,help='Export the entire tingly.db as a SQLite database'"`
}

type DBExportCmdKong struct {
	Output       string `kong:"required,short='o',type='path',help='Path for the new SQLite database file. The file may contain credentials.'"`
	ExcludeUsage bool   `kong:"help='Omit request usage, service statistics, provider quota usage, and token last-used timestamps'"`
}

func (c *DBExportCmdKong) Run(appManager *app.AppManager) error {
	if err := db.ExportSQLite(constant.GetDBFile(appManager.ConfigDir()), c.Output, c.ExcludeUsage); err != nil {
		return err
	}
	fmt.Printf("Database exported to %s\n", c.Output)
	return nil
}
