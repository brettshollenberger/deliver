package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

const (
	PACKAGE_FILE   string = "packages.json"
	LOCK_FILE      string = "packages.lock"
	WORKSPACES_DIR string = "deliver_workspaces"
)

var noRun *bool = flag.Bool("n", false, "print the commands but do not run them")
var verbose *bool = flag.Bool("v", false, "print the commands while running them")
var rootWorkspaceDir *string = flag.String("root", "", "where to create the deliver workspaces directory. If empty, uses home directory")
var useDeliverWorkspace *bool = flag.Bool("deliver_workspace", false, "If true, use the project-specific Go workspace. If false, use $GOPATH")

type Manifest struct {
	Repository string `json:",omitempty"`
	Packages   map[string]Package
}

func (m *Manifest) writeToFile(fileName string) {
	data, err := json.Marshal(*m)
	if err != nil {
		panic(err)
	}
	ioutil.WriteFile(fileName, data, 0644)
}

func (m *Manifest) hasRepository() bool {
	return m.Repository != ""
}

// Packages defined in the manifest
type Package struct {
	Source   string
	Branch   string `json:",omitempty"`
	Revision string
}

func (p *Package) getBranch() string {
	if p.Branch == "" {
		return "master"
	} else {
		return p.Branch
	}
}

func (p *Package) hasRevision() bool {
	return p.Revision != ""
}

// Parses the manifest from into a Manifest struct.
func NewManifestFromFile(manifestFile string) (manifest *Manifest) {
	manifest = &Manifest{}
	// Package manifest must exist.
	fileBytes, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(fileBytes, manifest)
	if err != nil {
		panic(err)
	}
	return
}

// Encapsulates commands to run on a git repository.
type GitRepository struct {
	repoUrl  string
	repoPath string
}

func (g *GitRepository) getCurrentRevision() string {
	revisionString := runInDirectory(g.repoPath, func() (string, error) {
		return executeCommand("git", "rev-parse", "HEAD")
	})
	// Strip newline character at the end
	if len(revisionString) > 0 {
		return revisionString[:len(revisionString)-1]
	} else {
		return "<REV>"
	}
}

func (g *GitRepository) checkoutRevision(revision string) {
	runInDirectory(g.repoPath, func() (string, error) {
		return executeCommand("git", "checkout", revision)
	})
}

func (g *GitRepository) checkoutBranchTip(branch string) {
	g.checkoutRevision(branch)
	g.pullBranch(branch)
}

// Pulls the git repo from origin in the given repo path.
func (g *GitRepository) pullBranch(branch string) {
	runInDirectory(g.repoPath, func() (string, error) {
		return executeCommand("git", "pull", "origin", branch)
	})
}

// Clones the git repo into the given directory.
func (g *GitRepository) clone(destinationPath, branch string) {
	_, err := executeCommand("git", "clone", "-b", branch, g.repoUrl, destinationPath)
	if err != nil {
		panic(err)
	}
}

// Fetches the current repository.
func (g *GitRepository) fetch() {
	runInDirectory(g.repoPath, func() (string, error) {
		return executeCommand("git", "fetch")
	})
}

// Function signature used in runInDirectory().
type CommandFunction func() (string, error)

// Runs the given command function after changing the current directory to dir.
// After the command function runs, change the directory back to the original
// directory. Returns the output of the command function.
func runInDirectory(dir string, command CommandFunction) string {
	currentDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	err = os.Chdir(dir)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err = os.Chdir(currentDir); err != nil {
			panic(err)
		}
	}()

	out, err := command()
	if err != nil {
		panic(err)
	}

	return out
}

// Executes a shell command. Depending on the flags,
// it may just print the command to run, or both print and
// run the command.
func executeCommand(args ...string) (out string, err error) {
	var outBytes []byte
	if *noRun || *verbose {
		logArgs := make([]interface{}, len(args))
		for i, arg := range args {
			logArgs[i] = interface{}(arg)
		}
		fmt.Fprintln(os.Stdout, logArgs...)
	}

	if !*noRun {
		cmd := exec.Command(args[0], args[1:]...)
		outBytes, err = cmd.Output()
	}
	return string(outBytes), err
}

func pathCompare(a string, b string) bool {
	realA, err := filepath.EvalSymlinks(a)
	if err != nil {
		panic(err)
	}
	realB, err := filepath.EvalSymlinks(b)
	if err != nil {
		panic(err)
	}
	return realA == realB
}

func createWorkspaceSymlink(repositoryPath string) {
	currentDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	linkPath := path.Join(getWorkspacePath(), "src", repositoryPath)

	if pathCompare(linkPath, currentDir) {
		fmt.Fprintln(os.Stdout, "skipping symlink...")
		return
	}

	linkDir := path.Join(linkPath, "..")
	_, err = executeCommand("mkdir", "-p", linkDir)
	if err != nil {
		panic(err)
	}

	// Remove existing symlink
	_, err = executeCommand("rm", "-f", linkPath)
	if err != nil {
		panic(err)
	}

	_, err = executeCommand("ln", "-s", currentDir, linkPath)
	if err != nil {
		panic(err)
	}
}

// Traverse the path up towards the root. If a directory has a packages.json file,
// then workspace/ in the same directory is the workspace.
// If we get to the root directory, return the env GOPATH.
func getWorkspacePath() string {
	if !*useDeliverWorkspace {
		return strings.Split(os.Getenv("GOPATH"), ":")[0]
	}

	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	for {
		possibleManifest := path.Join(dir, PACKAGE_FILE)
		_, err := os.Stat(possibleManifest)
		if err == nil {
			// packages.json exists. Crete workspace
			var workspaceRoot string
			if len(*rootWorkspaceDir) == 0 {
				workspaceRoot = os.Getenv("HOME")
			} else {
				workspaceRoot = *rootWorkspaceDir
			}
			return path.Join(workspaceRoot, WORKSPACES_DIR, dir)
		}

		if os.IsNotExist(err) {
			// packages.json does not exist.
			if dir == "/" {
				// If we're already at the root, return
				// the GOPATH environment variable.
				return os.Getenv("GOPATH")
			} else {
				// Check the parent directory.
				dir = path.Join(dir, "..")
			}
		} else {
			// some other error occured during os.Stat.
			panic(err)
		}
	}
}

// Gets or updates all packages specified in the given file.
// Fetches packages recursively if one of the referenced packages
// has a manifest.
func downloadPackages(manifest *Manifest) {
	packages := manifest.Packages
	for packageName, packageInfo := range packages {
		downloadPackage(packageName, &packageInfo)
		// Package revision may have been changed.
		packages[packageName] = packageInfo

	}
}

// Installs the given package. If the package has a locked revision,
// use the locked revision. Otherwise, update the package to the latest revision
// by checking out the tip of the specified branch, and save the new revision to packageInfo.
// If the package itself has dependencies specified in a lockfile, recursively download
// them as well.
func downloadPackage(packageName string, packageInfo *Package) {
	packageDir := path.Join(getWorkspacePath(), "src", packageName)
	fmt.Fprintf(os.Stdout, "downloading %s\n", packageName)
	fmt.Fprintf(os.Stdout, "package dir %s\n", packageDir)

	git := GitRepository{
		repoUrl:  packageInfo.Source,
		repoPath: packageDir,
	}

	// If package directory does not exist, create the directory.
	if _, err := os.Stat(packageDir); os.IsNotExist(err) {
		_, execErr := executeCommand("mkdir", "-p", packageDir)
		if execErr != nil {
			panic(execErr)
		}
	}

	// Check if repository already exists in package directory.
	gitInfoPath := path.Join(packageDir, ".git")
	if _, err := os.Stat(gitInfoPath); os.IsNotExist(err) {
		// Git repo does not exist. Clone it.
		git.clone(packageDir, packageInfo.getBranch())
	} else {
		// Git repo exists. Pull latest.
		git.fetch()
	}

	if packageInfo.hasRevision() {
		git.checkoutRevision(packageInfo.Revision)
	} else {
		git.checkoutBranchTip(packageInfo.getBranch())
		packageInfo.Revision = git.getCurrentRevision()
	}

	// Check if package has its own dependencies. If so, download them as well.
	packageManifestFile := path.Join(packageDir, LOCK_FILE)
	_, err := os.Stat(packageManifestFile)
	switch {
	case err == nil:
		// No error means lock file exists.
		fmt.Fprintf(os.Stdout, "getting dependencies of %s...\n", packageName)
		packageManifest := NewManifestFromFile(packageManifestFile)
		downloadPackages(packageManifest)
		fmt.Fprintf(os.Stdout, "done with dependencies of %s\n", packageName)

	case !os.IsNotExist(err):
		panic(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Deliver is a package manager for Go\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n\n  deliver [flags] [command] [arguments]\n\n")
	fmt.Fprintf(os.Stderr, "The commands are:\n\n")
	fmt.Fprintf(os.Stderr, "  install [package]\tInstalls all packages in packages.lock.\n"+
		"                   \tIf a package name is provided, installs only a single package.\n")
	fmt.Fprintf(os.Stderr, "  update [package] \tUpdates all packages in packages.json to the latest versions, and\n"+
		"                   \tsaves the versions to packages.lock.\n"+
		"                   \tIf a package name is provided, updates only a single package.\n")
	fmt.Fprintf(os.Stderr, "The flags are:\n\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(0)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "%v\n", r)
			os.Exit(1)
		}
	}()

	switch args[0] {
	case "path":
		// Return the deliver gopath.
		workspacePath := getWorkspacePath()
		fmt.Fprintf(os.Stdout, "%s", workspacePath)
		os.Exit(0)

	case "install":
		// Downloads packages from the lockfile.
		lockManifest := NewManifestFromFile(LOCK_FILE)
		if len(args) == 2 {
			packageName := args[1]
			packageInfo, ok := lockManifest.Packages[packageName]
			if !ok {
				panic(errors.New(fmt.Sprintf("Package %s not found in %s", packageName, LOCK_FILE)))
			}
			downloadPackage(packageName, &packageInfo)
		} else {
			downloadPackages(lockManifest)
			if lockManifest.hasRepository() {
				createWorkspaceSymlink(lockManifest.Repository)
			}
		}

	case "update":
		// Downloads packages from the package file and updates the lockfile.
		manifest := NewManifestFromFile(PACKAGE_FILE)
		if len(args) == 2 {
			packageName := args[1]
			packageInfo, ok := manifest.Packages[packageName]
			if !ok {
				panic(errors.New(fmt.Sprintf("Package not found: %s", packageName)))
			}
			downloadPackage(packageName, &packageInfo)
			// Replace a single package in the lockfile.
			// This will create a new lockfile if one doesn't exist.
			lockManifest := NewManifestFromFile(LOCK_FILE)
			lockManifest.Packages[packageName] = packageInfo
			lockManifest.writeToFile(LOCK_FILE)
		} else {
			downloadPackages(manifest)
			if manifest.hasRepository() {
				createWorkspaceSymlink(manifest.Repository)
			}
			// Replace the entire lockfile.
			// This will create a new lockfile if one doesn't exist.
			manifest.writeToFile(LOCK_FILE)
		}
	}
}
