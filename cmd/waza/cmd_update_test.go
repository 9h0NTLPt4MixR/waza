package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateCommand_ConfirmedRunsInstaller(t *testing.T) {
	var stdout bytes.Buffer
	var ran bool

	cmd := newUpdateCommandWithOptions(&updateCommandOptions{
		InstallerURL: "https://example.com/install.sh",
		LookPath: func(name string) (string, error) {
			require.Equal(t, "bash", name)
			return "/usr/bin/bash", nil
		},
		RunCommand: func(ctx context.Context, name string, args []string, stdin io.Reader, out, errOut io.Writer) error {
			ran = true
			assert.Equal(t, "/usr/bin/bash", name)
			require.Len(t, args, 4)
			assert.Equal(t, "-c", args[0])
			assert.Contains(t, args[1], "curl -fsSL")
			assert.Equal(t, "https://example.com/install.sh", args[3])
			return nil
		},
	})
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	require.NoError(t, cmd.Execute())
	assert.True(t, ran)
	assert.Contains(t, stdout.String(), "Continue? [y/N]:")
	assert.Contains(t, stdout.String(), "Updating waza")
	assert.Contains(t, stdout.String(), "Update complete")
}

func TestUpdateCommand_DeclinedDoesNotRunInstaller(t *testing.T) {
	var stdout bytes.Buffer

	cmd := newUpdateCommandWithOptions(&updateCommandOptions{
		RunCommand: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
			t.Fatal("installer should not run when update is declined")
			return nil
		},
	})
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	require.NoError(t, cmd.Execute())
	assert.Contains(t, stdout.String(), "Update cancelled.")
}

func TestUpdateCommand_YesFlagSkipsConfirmation(t *testing.T) {
	var stdout bytes.Buffer
	var ran bool

	cmd := newUpdateCommandWithOptions(&updateCommandOptions{
		LookPath: func(name string) (string, error) {
			return "/usr/bin/bash", nil
		},
		RunCommand: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
			ran = true
			return nil
		},
	})
	cmd.SetArgs([]string{"--yes"})
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	require.NoError(t, cmd.Execute())
	assert.True(t, ran)
	assert.NotContains(t, stdout.String(), "Continue? [y/N]:")
}

func TestUpdateCommand_MissingBashReturnsGuidance(t *testing.T) {
	cmd := newUpdateCommandWithOptions(&updateCommandOptions{
		LookPath: func(name string) (string, error) {
			return "", errors.New("not found")
		},
		RunCommand: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
			t.Fatal("installer should not run when bash is missing")
			return nil
		},
	})
	cmd.SetArgs([]string{"--yes"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bash is required")
	assert.Contains(t, err.Error(), latestReleaseURL)
}

func TestUpdateCommand_RunFailureIncludesContext(t *testing.T) {
	cmd := newUpdateCommandWithOptions(&updateCommandOptions{
		LookPath: func(name string) (string, error) {
			return "/usr/bin/bash", nil
		},
		RunCommand: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
			return errors.New("boom")
		},
	})
	cmd.SetArgs([]string{"--yes"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "running waza installer")
	assert.Contains(t, err.Error(), "boom")
}

func TestRootCommand_RegistersUpdateCommand(t *testing.T) {
	cmd := newRootCommand()
	found, _, err := cmd.Find([]string{"update"})
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "update", found.Name())
}

func TestShouldRunUpdateCheck_SkipsUpdateCommand(t *testing.T) {
	root := newRootCommand()
	updateCmd, _, err := root.Find([]string{"update"})
	require.NoError(t, err)

	assert.False(t, shouldRunUpdateCheck(updateCmd, false))
}

func TestShouldRunUpdateCheck_RespectsOptOuts(t *testing.T) {
	root := newRootCommand()

	assert.False(t, shouldRunUpdateCheck(root, true))

	t.Setenv("WAZA_NO_UPDATE_CHECK", "1")
	assert.False(t, shouldRunUpdateCheck(root, false))

	require.NoError(t, os.Unsetenv("WAZA_NO_UPDATE_CHECK"))
	assert.True(t, shouldRunUpdateCheck(root, false))
}
