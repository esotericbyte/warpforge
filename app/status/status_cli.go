package statuscli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"

	appbase "github.com/warptools/warpforge/app/base"
	"github.com/warptools/warpforge/app/base/util"
	"github.com/warptools/warpforge/pkg/config"
	"github.com/warptools/warpforge/pkg/dab"
	"github.com/warptools/warpforge/pkg/tracing"
	"github.com/warptools/warpforge/wfapi"
)

func init() {
	appbase.App.Commands = append(appbase.App.Commands, statusCmdDef)
}

var statusCmdDef = &cli.Command{
	Name:  "status",
	Usage: "Get status of workspaces and installation",
	Action: util.ChainCmdMiddleware(cmdStatus,
		util.CmdMiddlewareLogging,
		util.CmdMiddlewareTracingConfig,
		util.CmdMiddlewareTracingSpan,
	),
	Aliases: []string{"info"},
}

func cmdStatus(c *cli.Context) error {
	fmtBold := color.New(color.Bold)
	fmtWarning := color.New(color.FgHiRed, color.Bold)
	verbose := c.Bool("verbose")
	ctx := c.Context

	// display version
	if verbose {
		fmt.Fprintf(c.App.Writer, "Warpforge Version: %s\n\n", appbase.VERSION)
	}

	// check plugins
	pluginsOk := true

	if verbose {
		fmt.Fprintf(c.App.Writer, "\nPlugin Info:\n")
	}

	binPath, err := config.BinPath()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if verbose {
		fmt.Fprintf(c.App.Writer, "binPath = %s\n", binPath)
	}

	rioPath := filepath.Join(binPath, "rio")
	if _, err := os.Stat(rioPath); os.IsNotExist(err) {
		fmt.Fprintf(c.App.Writer, "rio not found (expected at %s)\n", rioPath)
		pluginsOk = false
	} else {
		if verbose {
			fmt.Fprintf(c.App.Writer, "found rio\n")
		}
	}

	runcPath := filepath.Join(binPath, "runc")
	if _, err := os.Stat(runcPath); os.IsNotExist(err) {
		fmt.Fprintf(c.App.Writer, "runc not found (expected at %s)\n", runcPath)
		pluginsOk = false
	} else {
		cmdCtx, cmdSpan := tracing.Start(ctx, "exec", trace.WithAttributes(tracing.AttrFullExecNameRunc))
		defer cmdSpan.End()
		runcVersionCmd := exec.CommandContext(cmdCtx, filepath.Join(binPath, "runc"), "--version")
		var runcVersionOut bytes.Buffer
		runcVersionCmd.Stdout = &runcVersionOut
		err = runcVersionCmd.Run()
		tracing.EndWithStatus(cmdSpan, err)
		if err != nil {
			return fmt.Errorf("failed to get runc version information: %s", err)
		}
		if verbose {
			fmt.Fprintf(c.App.Writer, "found runc\n")
			fmt.Fprintf(c.App.Writer, "%s", &runcVersionOut)
		}
	}

	if !pluginsOk {
		fmtWarning.Fprintf(c.App.Writer, "WARNING: plugins do not appear to be installed correctly.\n\n")
	}

	fsys := os.DirFS("/")

	// check if pwd is a module, read module and set flag
	var module *wfapi.Module
	if _, err := os.Stat(filepath.Join(cwd, dab.MagicFilename_Module)); err == nil {
		module, err = dab.ModuleFromFile(fsys, filepath.Join(cwd, dab.MagicFilename_Module))
		if err != nil {
			return fmt.Errorf("failed to open module file: %s", err)
		}
	}

	if module != nil {
		fmt.Fprintf(c.App.Writer, "Module %q:\n", module.Name)
	} else {
		fmt.Fprintf(c.App.Writer, "No module in this directory.\n")
	}

	// display module and plot info
	var plot wfapi.Plot
	hasPlot := false
	_, err = os.Stat(filepath.Join(cwd, dab.MagicFilename_Plot))
	if module != nil && err == nil {
		// module.wf and plot.wf exists, read the plot
		hasPlot = true
		plot, err = util.PlotFromFile(filepath.Join(cwd, dab.MagicFilename_Plot))
		if err != nil {
			return fmt.Errorf("failed to open plot file: %s", err)
		}
	}

	if hasPlot {
		fmt.Fprintf(c.App.Writer, "\tPlot has %d inputs, %d steps, and %d outputs.\n",
			len(plot.Inputs.Keys),
			len(plot.Steps.Keys),
			len(plot.Outputs.Keys))

		// check for missing catalog refs
		wss, err := util.OpenWorkspaceSet()
		if err != nil {
			return fmt.Errorf("failed to open workspace: %s", err)
		}
		catalogRefCount := 0
		resolvedCatalogRefCount := 0
		ingestCount := 0
		mountCount := 0
		for _, input := range plot.Inputs.Values {
			if input.Basis().Mount != nil {
				mountCount++
				continue
			}
			if input.Basis().Ingest != nil {
				ingestCount++
				continue
			}
			if input.Basis().CatalogRef != nil {
				catalogRefCount++
				ware, _, err := wss.GetCatalogWare(*input.PlotInputSimple.CatalogRef)
				if err != nil {
					return fmt.Errorf("failed to lookup catalog ref: %s", err)
				}
				if ware == nil {
					fmt.Fprintf(c.App.Writer, "\tMissing catalog item: %q.\n", input.Basis().CatalogRef.String())
					continue
				}
				resolvedCatalogRefCount++
				continue
			}
		}
		fmt.Fprintf(c.App.Writer, "\tPlot contains %d catalog inputs. %d/%d catalog inputs resolved successfully.\n", catalogRefCount, resolvedCatalogRefCount, catalogRefCount)
		if resolvedCatalogRefCount < catalogRefCount {
			fmt.Fprintf(c.App.Writer, "\tWarning: plot contains %d unresolved catalog inputs!\n", (catalogRefCount - resolvedCatalogRefCount))
		}
		if ingestCount > 0 {
			fmt.Fprintf(c.App.Writer, "\tWarning: plot contains %d ingest inputs and is not hermetic!\n", ingestCount)
		}
		if mountCount > 0 {
			fmt.Fprintf(c.App.Writer, "\tWarning: plot contains %d mount inputs and is not hermetic!\n", mountCount)
		}
	} else if module != nil {
		// directory is a module, but has no plot
		fmt.Fprintf(c.App.Writer, "\tNo plot file for module.\n")
	}

	// display workspace info
	fmt.Fprintf(c.App.Writer, "\nWorkspace:\n")
	wss, err := util.OpenWorkspaceSet()
	if err != nil {
		return fmt.Errorf("failed to open workspace set: %s", err)
	}

	// handle special case for pwd
	fmt.Fprintf(c.App.Writer, "\t%s (pwd", cwd)
	if module != nil {
		fmt.Fprintf(c.App.Writer, ", module")
	}
	// check if it's a workspace
	if _, err := os.Stat(filepath.Join(cwd, ".warpforge")); !os.IsNotExist(err) {
		fmt.Fprintf(c.App.Writer, ", workspace")
	}
	// check if it's a root workspace
	if _, err := os.Stat(filepath.Join(cwd, ".warpforge/root")); !os.IsNotExist(err) {
		fmt.Fprintf(c.App.Writer, ", root workspace")
	}
	// check if it's a git repo
	if _, err := os.Stat(filepath.Join(cwd, ".git")); !os.IsNotExist(err) {
		fmt.Fprintf(c.App.Writer, ", git repo")
	}

	fmt.Fprintf(c.App.Writer, ")\n")

	// handle all other workspaces
	for _, ws := range wss {
		fs, subPath := ws.Path()
		path := fmt.Sprintf("%s%s", fs, subPath)

		if path == cwd {
			// we handle pwd earlier, ignore
			continue
		}

		labels := []string{}

		// collect workspaces labels
		isRoot := ws.IsRootWorkspace()
		isHome := ws.IsHomeWorkspace()
		if isRoot {
			labels = append(labels, "root workspace")
		}
		if isHome {
			labels = append(labels, "home workspace")
		}
		if !isRoot && !isHome {
			labels = append(labels, "workspace")
		}

		// label if it's a git repo
		if isGitRepo(path) {
			labels = append(labels, "git repo")
		}

		// print a line for this dir
		fmt.Fprintf(c.App.Writer, "\t%s (", path)
		for n, label := range labels {
			fmt.Fprintf(c.App.Writer, "%s", label)
			if n != len(labels)-1 {
				// this is not the last label
				fmt.Fprintf(c.App.Writer, ", ")
			}
		}
		fmt.Fprintf(c.App.Writer, ")\n")
	}

	if module != nil && hasPlot {
		fmtBold.Fprintf(c.App.Writer, "\nYou can evaluate this module with the `%s run` command.\n", os.Args[0])
	}

	return nil
}

func isGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return !os.IsNotExist(err)
}
