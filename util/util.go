// Copyright (c) 2022 Arista Networks, Inc.  All rights reserved.
// Arista Networks, Inc. Confidential and Proprietary.

package util

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"golang.org/x/sys/unix"
)

// Globals type struct exported for global flags
type Globals struct {
	Quiet bool
}

// GlobalVar global variable exported for all global variables
var GlobalVar Globals

// ErrPrefix is a container type for error prefix strings.
type ErrPrefix string

type GitSpec struct {
	Revision  string
	ClonedDir string
}

// Returns if the provided revision is a "COMMIT" or a "TAG"
func (spec *GitSpec) typeOfGitRevision() string {
	// Check 1st line of git show
	return ""
}

// Returns a unique version number based on the commit/tag
func (spec *GitSpec) GetVersionFromRevision() string {
	// If type is TAG, return as is

	// If type is commit
	// If short commit, return as is

	// If long commit, reduce size
	return ""
}

// RunSystemCmd runs a command on the shell and pipes to stdout and stderr
func RunSystemCmd(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Stderr = os.Stderr
	if !GlobalVar.Quiet {
		cmd.Stdout = os.Stdout
	} else {
		cmd.Stdout = io.Discard
	}
	err := cmd.Run()
	return err
}

// Runs the system command from a specified directory
func RunSystemCmdInDir(dir string, name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if !GlobalVar.Quiet {
		cmd.Stdout = os.Stdout
	} else {
		cmd.Stdout = io.Discard
	}
	err := cmd.Run()
	return err
}

// CheckOutput runs a command on the shell and returns stdout if it is successful
// else it return the error
func CheckOutput(name string, arg ...string) (
	string, error) {
	cmd := exec.Command(name, arg...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(output),
				fmt.Errorf("Running '%s %s': exited with exit-code %d\nstderr:\n%s",
					name, strings.Join(arg, " "), exitErr.ExitCode(), exitErr.Stderr)
		}
		return string(output),
			fmt.Errorf("Running '%s %s' failed with '%s'",
				name, strings.Join(arg, " "), err)
	}
	return string(output), nil
}

// CheckPath checks if path exists. It also optionally checks if it is a directory,
// or if the path is writable
func CheckPath(path string, checkDir bool, checkWritable bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if checkDir && !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	if checkWritable && unix.Access(path, unix.W_OK) != nil {
		return fmt.Errorf("%s is not writable", path)
	}
	return nil
}

// MaybeCreateDir creates a directory with permissions 0775
// Pre-existing directories are left untouched.
func MaybeCreateDir(dirPath string, errPrefix ErrPrefix) error {
	err := os.Mkdir(dirPath, 0775)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("%s: Error '%s' creating %s", errPrefix, err, dirPath)
	}
	return nil
}

// MaybeCreateDirWithParents creates a directory at dirPath if one
// doesn't already exist. It also creates any parent directories.
func MaybeCreateDirWithParents(dirPath string, errPrefix ErrPrefix) error {
	if err := RunSystemCmd("mkdir", "-p", dirPath); err != nil {
		return fmt.Errorf("%sError '%s' trying to create directory %s with parents",
			errPrefix, err, dirPath)
	}
	return nil
}

// RemoveDirs removes the directories dirs
func RemoveDirs(dirs []string, errPrefix ErrPrefix) error {
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("%sError '%s' while removing %s",
				errPrefix, err, dir)
		}
	}
	return nil
}

// CopyToDestDir copies files/dirs specified by srcGlob to destDir
// It is assumed that destDir is present and writable
func CopyToDestDir(
	srcGlob string,
	destDir string,
	errPrefix ErrPrefix) error {

	if err := CheckPath(destDir, true, true); err != nil {
		return fmt.Errorf("%sDirectory %s should be present and writable: %s",
			errPrefix, destDir, err)
	}

	filesToCopy, patternErr := filepath.Glob(srcGlob)
	if patternErr != nil {
		return fmt.Errorf("%sGlob %s returned %s", errPrefix, srcGlob, patternErr)
	}

	for _, file := range filesToCopy {
		insideDestDir := destDir + "/"
		if err := RunSystemCmd("cp", "-rf", file, insideDestDir); err != nil {
			return fmt.Errorf("%scopying %s to %s errored out with '%s'",
				errPrefix, file, insideDestDir, err)
		}
	}
	return nil
}

// GetRepoDir returns the path of the cloned source repo.
// If repo is specified, it's subpath under SrcDir config is
// returned.
// If no repo is specfied, we return current working directory.
func GetRepoDir(repo string) string {
	var repoDir string
	if repo != "" {
		srcDir := viper.GetString("SrcDir")
		repoDir = filepath.Join(srcDir, repo)
	} else {
		repoDir = "."
	}
	return repoDir
}

// VerifyRpmSignature verifies that the RPM specified at rpmPath
// is signed with a valid key in the key ring and that the signatures
// are valid.
func VerifyRpmSignature(rpmPath string, errPrefix ErrPrefix) error {
	output, err := CheckOutput("rpm", "-K", rpmPath)
	if err != nil {
		return fmt.Errorf("%s:%s", errPrefix, err)
	}
	if !strings.Contains(output, "digests signatures OK") {
		return fmt.Errorf("%sSignature check of %s failed. rpm -K output:\n%s",
			errPrefix, rpmPath, output)
	}
	return nil
}

// VerifyTarballSignature verifies that the detached signature of the tarball
// is valid.
func VerifyTarballSignature(
	tarballPath string, tarballSigPath string, pubKeyPath string,
	errPrefix ErrPrefix) error {
	tmpDir, mkdtErr := os.MkdirTemp("", "eext-keyring")
	if mkdtErr != nil {
		return fmt.Errorf("%sError '%s'creating temp dir for keyring",
			errPrefix, mkdtErr)
	}
	defer os.RemoveAll(tmpDir)

	keyRingPath := filepath.Join(tmpDir, "eext.gpg")
	baseArgs := []string{
		"--homedir", tmpDir,
		"--no-default-keyring", "--keyring", keyRingPath}
	gpgCmd := "gpg"

	// Create keyring
	createKeyRingCmdArgs := append(baseArgs, "--fingerprint")
	if err := RunSystemCmd(gpgCmd, createKeyRingCmdArgs...); err != nil {
		return fmt.Errorf("%sError '%s'creating keyring",
			errPrefix, err)
	}

	// Import public key
	importKeyCmdArgs := append(baseArgs, "--import", pubKeyPath)
	if err := RunSystemCmd(gpgCmd, importKeyCmdArgs...); err != nil {
		return fmt.Errorf("%sError '%s' importing public-key %s",
			errPrefix, err, pubKeyPath)
	}

	verifySigArgs := append(baseArgs, "--verify", tarballSigPath, tarballPath)
	if output, err := CheckOutput(gpgCmd, verifySigArgs...); err != nil {
		return fmt.Errorf("%sError verifying signature %s for tarball %s with pubkey %s."+
			"\ngpg --verify err: %sstdout:%s",
			errPrefix, tarballSigPath, tarballPath, pubKeyPath, err, output)
	}

	return nil
}

// VerifyGitSignature verifies that the git repo commit/tag is signed.
func VerifyGitSignature(pubKeyPath string, gitSpec GitSpec, errPrefix ErrPrefix) error {
	tmpDir, mkdtErr := os.MkdirTemp("", "eext-keyring")
	if mkdtErr != nil {
		return fmt.Errorf("%sError '%s'creating temp dir for keyring",
			errPrefix, mkdtErr)
	}
	defer os.RemoveAll(tmpDir)

	keyRingPath := filepath.Join(tmpDir, "eext.gpg")
	baseArgs := []string{
		"--homedir", tmpDir,
		"--no-default-keyring", "--keyring", keyRingPath}
	gpgCmd := "gpg"

	// Create keyring
	createKeyRingCmdArgs := append(baseArgs, "--fingerprint")
	if err := RunSystemCmd(gpgCmd, createKeyRingCmdArgs...); err != nil {
		return fmt.Errorf("%sError '%s'creating keyring",
			errPrefix, err)
	}

	// Import public key
	importKeyCmdArgs := append(baseArgs, "--import", pubKeyPath)
	if err := RunSystemCmd(gpgCmd, importKeyCmdArgs...); err != nil {
		return fmt.Errorf("%sError '%s' importing public-key %s",
			errPrefix, err, pubKeyPath)
	}

	var verifyRepoCmd []string
	revision := gitSpec.Revision
	revisionType := gitSpec.typeOfGitRevision()
	if revisionType == "COMMIT" {
		verifyRepoCmd = []string{"verify-commit", "-v", revision}
	} else if revisionType == "TAG" {
		verifyRepoCmd = []string{"verify-tag", "-v", revision}
	} else {
		return fmt.Errorf("%sinvalid revision %s provided, provide either a COMMIT or TAG", errPrefix, revision)
	}
	clonedDir := gitSpec.ClonedDir
	err := RunSystemCmdInDir(clonedDir, "git", verifyRepoCmd...)
	if err != nil {
		return fmt.Errorf("%serror during verifying git repo at %s: %s", errPrefix, clonedDir, err)
	}

	return nil
}
