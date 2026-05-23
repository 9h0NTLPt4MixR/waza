package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"

	versionpkg "github.com/microsoft/waza/internal/version"
	"github.com/spf13/cobra"
)

const latestReleaseURL = "https://github.com/microsoft/waza/releases/latest"

type updateCommandOptions struct {
	InstallerURL string
	LookPath     func(string) (string, error)
	RunCommand   func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error
}

func newUpdateCommand() *cobra.Command {
	return newUpdateCommandWithOptions(nil)
}

func newUpdateCommandWithOptions(options *updateCommandOptions) *cobra.Command {
	opts := normalizeUpdateCommandOptions(options)
	var yes bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update waza to the latest release",
		Long: fmt.Sprintf(`Update waza to the latest release.

This command downloads and runs the official installer:
  %s

The installer detects the OS and architecture for the current shell environment,
downloads the matching release asset, verifies its checksum, and replaces the
waza binary.`, opts.InstallerURL),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateCommand(cmd, opts, yes)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Run the update without prompting for confirmation")

	return cmd
}

func normalizeUpdateCommandOptions(options *updateCommandOptions) updateCommandOptions {
	opts := updateCommandOptions{
		InstallerURL: versionpkg.InstallScriptURL,
		LookPath:     exec.LookPath,
		RunCommand: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdin = stdin
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			return cmd.Run()
		},
	}
	if options == nil {
		return opts
	}
	if options.InstallerURL != "" {
		opts.InstallerURL = options.InstallerURL
	}
	if options.LookPath != nil {
		opts.LookPath = options.LookPath
	}
	if options.RunCommand != nil {
		opts.RunCommand = options.RunCommand
	}
	return opts
}

func runUpdateCommand(cmd *cobra.Command, opts updateCommandOptions, yes bool) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	if !yes {
		ok, err := confirmUpdate(cmd.InOrStdin(), out, opts.InstallerURL)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Update cancelled.")
			return nil
		}
	}

	bashPath, err := opts.LookPath("bash")
	if err != nil {
		return missingBashError()
	}

	fmt.Fprintln(out, "Updating waza...")
	args := []string{"-c", `set -euo pipefail; curl -fsSL "$1" | bash`, "waza-installer", opts.InstallerURL}
	if err := opts.RunCommand(cmd.Context(), bashPath, args, cmd.InOrStdin(), out, errOut); err != nil {
		return fmt.Errorf("running waza installer: %w", err)
	}

	fmt.Fprintln(out, "Update complete.")
	return nil
}

func confirmUpdate(in io.Reader, out io.Writer, installerURL string) (bool, error) {
	fmt.Fprintf(out, "waza update will download and run the official installer:\n  %s\n\nContinue? [y/N]: ", installerURL)
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("reading confirmation: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func missingBashError() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("bash is required to run the waza installer; install Git Bash, MSYS2, or Cygwin, or download the native Windows binary from %s", latestReleaseURL)
	}
	return fmt.Errorf("bash is required to run the waza installer; install bash or download a release binary from %s", latestReleaseURL)
}
