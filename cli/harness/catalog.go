package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-box/internal/catalog"
)

// CatalogCmd groups the catalog subcommands.
type CatalogCmd struct {
	Check CatalogCheckCmd `kong:"cmd,help='Check the validity of the embedded catalogs, or of the given catalog files'"`
}

// CatalogCheckCmd validates providers.json and claude.models.json (plus the
// consistency between them). It is offline and deterministic, so it can gate
// CI directly: any issue exits non-zero.
type CatalogCheckCmd struct {
	Files []string `kong:"name='file',short='f',sep='none',help='Catalog file to check instead of the embedded ones (repeatable; providers.json / claude.models.json are told apart by content). A catalog not given falls back to the embedded one for the cross-catalog check.'"`
	JSON  bool     `kong:"name='json',help='Emit the result as JSON'"`
}

// Help returns the long usage for `harness catalog check --help`.
func (*CatalogCheckCmd) Help() string {
	return `Checks, for each catalog: it parses, ids are unique, and every entry passes
the same validation the gateway applies (providers.json: ValidateProviderCatalog;
claude.models.json: known dialect/effort values). Then checks consistency across
catalogs: every Claude model offered in providers.json must have an entry in
claude.models.json.

Without --file the catalogs compiled into this binary are checked.`
}

type catalogCheckResult struct {
	Checked []string             `json:"checked"`
	Issues  []catalog.CheckIssue `json:"issues"`
	OK      bool                 `json:"ok"`
	Sources map[string]string    `json:"sources"`
}

func (c *CatalogCheckCmd) Run() error {
	docs := catalog.EmbeddedCatalogJSON()
	sources := map[string]string{
		string(catalog.KindProviders):    "embedded",
		string(catalog.KindClaudeModels): "embedded",
	}

	var issues []catalog.CheckIssue
	for _, path := range c.Files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		kind, err := catalog.DetectCatalogKind(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if sources[string(kind)] != "embedded" {
			return fmt.Errorf("%s: a %s catalog was already given (%s)", path, kind, sources[string(kind)])
		}
		docs[kind] = data
		sources[string(kind)] = path
	}

	issues = append(issues, catalog.CheckProviderCatalogJSON(docs[catalog.KindProviders])...)
	issues = append(issues, catalog.CheckClaudeCatalogJSON(docs[catalog.KindClaudeModels])...)
	issues = append(issues, catalog.CheckCrossCatalog(docs[catalog.KindProviders], docs[catalog.KindClaudeModels])...)

	res := catalogCheckResult{
		Checked: []string{string(catalog.KindProviders), string(catalog.KindClaudeModels)},
		Issues:  issues,
		OK:      len(issues) == 0,
		Sources: sources,
	}
	if res.Issues == nil {
		res.Issues = []catalog.CheckIssue{}
	}

	if c.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			return err
		}
	} else {
		for _, k := range res.Checked {
			fmt.Printf("checked %-14s (%s)\n", k, sources[k])
		}
		for _, i := range issues {
			fmt.Println("  ✗", i)
		}
		if res.OK {
			fmt.Println("catalogs OK")
		}
	}
	if !res.OK {
		return fmt.Errorf("catalog check failed: %d issue(s)", len(issues))
	}
	return nil
}
