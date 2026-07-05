package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/catalog"
	"github.com/QaamGo/api2convert-cli/internal/client"
	"github.com/QaamGo/api2convert-cli/internal/config"
)

// completionCatalog resolves config directly (completion may run without the
// persistent pre-run) and loads the cached catalog.
func completionCatalog(cmd *cobra.Command) (*catalog.Catalog, bool) {
	fileCfg, err := config.Load()
	if err != nil {
		return nil, false
	}
	res, err := config.Resolve(fileCfg, config.Flags{
		APIKey:      gf.apiKey,
		BaseURL:     gf.baseURL,
		Timeout:     gf.timeout,
		PollTimeout: gf.pollTimeout,
		MaxRetries:  gf.maxRetries,
		Output:      gf.output,
		Concurrency: gf.concurrency,
	}, os.Getenv)
	if err != nil {
		return nil, false
	}
	cl, err := client.Build(res)
	if err != nil {
		return nil, false
	}
	cat, err := catalog.Load(cmd.Context(), cl, false, res.APIKey+"|"+res.BaseURL)
	if err != nil {
		return nil, false
	}
	return cat, true
}

func completeTargets(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cat, ok := completionCatalog(cmd)
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return cat.Targets(), cobra.ShellCompDirectiveNoFileComp
}

func completeCategories(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cat, ok := completionCatalog(cmd)
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return cat.Categories(), cobra.ShellCompDirectiveNoFileComp
}
