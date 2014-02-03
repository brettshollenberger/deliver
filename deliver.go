package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "io/ioutil"
    "log"
    "os"
    "os/exec"
    "path"
)

const PACKAGE_FILE string = "packages.json"
const LOCK_FILE string = "packages.lock"
const WORKSPACE_DIR string = "workspace"

var noRun *bool = flag.Bool("n", false, "print the commands but do not run them")
var verbose *bool = flag.Bool("v", false, "print the commands while running them")

// Entire package definition
type Manifest struct {
    Repository string `json:",omitempty"`
    Packages map[string]Package
}

func (m *Manifest) writeToFile(fileName string) {
    data, err := json.Marshal(*m)
    if err != nil {
        log.Fatal(err)
    }
    ioutil.WriteFile(fileName, data, 0644)
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

// Parses the JSON manifest into a Manifest struct.
func parseManifest(manifestFile string) (manifest Manifest) {
    // Package manifest must exist.
    if _, err := os.Stat(manifestFile); os.IsNotExist(err) {
        log.Fatal(err)
    } else {
        fileBytes, err := ioutil.ReadFile(manifestFile)
        if err != nil {
            log.Fatal(err)
        }
        err = json.Unmarshal(fileBytes, &manifest)
        if err != nil {
            log.Fatal(err)
        }
    }
    return
}

// Encapsulates commands to run on a git repository.
type GitRepository struct {
    repoUrl string
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
        log.Fatal(err)
    }
}

// Fetches the current repository.
func (g *GitRepository) fetch() {
    runInDirectory(g.repoPath, func() (string, error) {
        return executeCommand("git", "fetch")
    })
}

// Function signature used in runInDirectory().
type commandFunction func() (string, error)

// Runs the given command function after changing the current directory to dir.
// After the command function runs, change the directory back to the original
// directory. Returns the output of the command function.
func runInDirectory(dir string, command commandFunction) string {
    currentDir, err := os.Getwd()
    if err != nil {
        log.Fatal(err)
    }

    err = os.Chdir(dir);
    if err != nil {
        log.Fatal(err)
    }

    out, err := command()
    if err != nil {
        log.Fatal(err)
    }

    err = os.Chdir(currentDir)
    if err != nil {
        log.Fatal(err)
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
        fmt.Fprint(os.Stdout, logArgs...)
    }

    if !*noRun {
        cmd := exec.Command(args[0], args[1:]...)
        outBytes, err = cmd.Output()
    }
    return string(outBytes), err
}

// Traverse the path up towards the root. If a directory has a packages.json file,
// then workspace/ in the same directory is the workspace.
// If we get to the root directory, return the env GOPATH.
func getWorkspacePath() string {
    dir, err := os.Getwd()
    if err != nil {
        log.Fatal(err)
    }

    for {
        possibleManifest := path.Join(dir, PACKAGE_FILE)
        _, err := os.Stat(possibleManifest)
        if err == nil {
            // packages.json exists. Workspace is in this directory.
            return path.Join(dir, WORKSPACE_DIR)
        } else if os.IsNotExist(err) {
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
            log.Fatal(err)
        }
    }
}

// Gets or updates all packages specified in the given file.
// Fetches packages recursively if one of the referenced packages
// has a manifest.
func installPackages(manifest *Manifest, update bool) {
    packages := manifest.Packages
    for packageName, packageInfo := range packages {
        installPackage(packageName, &packageInfo, update)

        // Package revision may have been updated
        if update {
            packages[packageName] = packageInfo
        }

        // Check if package has its own dependencies. If so, install them as well.
        // TODO: move this to installPackage(), so that we get dependencies when updating a single package.
        goPath := os.Getenv("GOPATH")
        packageManifestFile := path.Join(goPath, "src", packageName, LOCK_FILE)
        if _, err := os.Stat(packageManifestFile); os.IsNotExist(err) {
            continue
        }
        fmt.Fprintf(os.Stdout, "getting dependencies of %s...\n", packageName)
        packageManifest := parseManifest(packageManifestFile)
        installPackages(&packageManifest, update)
        fmt.Fprintf(os.Stdout, "done with dependencies of %s\n", packageName)
    }
}

// Installs the given package. If the package has a locked revision,
// use the locked revision. Otherwise, update the package (by checking out the tip of the specified branch).
// If updateRevision is set to true and the package was updated, update the Revision field
// of the packageInfo struct.
func installPackage(packageName string, packageInfo *Package, updateRevision bool) {
    goPath := os.Getenv("GOPATH")
    packageDir := path.Join(goPath, "src", packageName)
    fmt.Fprintf(os.Stdout, "updating %s", packageName)

    git := GitRepository{
        repoUrl: packageInfo.Source,
        repoPath: packageDir,
    }

    // If package directory does not exist, create the directory.
    if _, err := os.Stat(packageDir); os.IsNotExist(err) {
        _, execErr := executeCommand("mkdir", "-p", packageDir)
        if execErr != nil {
            log.Fatal(execErr)
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
        if updateRevision {
            packageInfo.Revision = git.getCurrentRevision()
        }
    }
}

func usage() {
    fmt.Fprintf(os.Stderr, "Deliver is a package manager for Go\n\n")
    fmt.Fprintf(os.Stderr, "Usage:\n\n  deliver command [arguments] [flags]\n\n")
    fmt.Fprintf(os.Stderr, "The commands are:\n\n")
    fmt.Fprintf(os.Stderr, "  install: recursively installs all packages in packages.json\n")
    fmt.Fprintf(os.Stderr, "  update [package]: gets the latest revision of the package and updates packages.lock\n\n")
    fmt.Fprintf(os.Stderr, "The flags are:\n\n")
    flag.PrintDefaults()
    fmt.Fprintf(os.Stderr, "\n")
    os.Exit(2)
}

func main() {
    flag.Usage = usage
    flag.Parse()

    args := flag.Args()
    if len(args) < 1 {
        usage()
    }

    switch args[0] {
    case "path":
        // Return the deliver gopath.
        workspacePath := getWorkspacePath()
        fmt.Fprintf(os.Stdout, "%s", workspacePath)
        os.Exit(0)
    case "install":
        // Get the locked versions of all packages.
        lockManifest := parseManifest(LOCK_FILE)
        installPackages(&lockManifest, false)
    case "update":
        manifest := parseManifest(PACKAGE_FILE)
        if len(args) == 2 {
            lockManifest := parseManifest(LOCK_FILE)

            // Get latest version of a single package
            // and updates the single package in the lockfile.
            packageName := args[1]
            packageInfo, ok := manifest.Packages[packageName]
            if !ok {
                log.Fatalf("package not in packages.json")
            }
            installPackage(packageName, &packageInfo, true)
            lockManifest.Packages[packageName] = packageInfo
            lockManifest.writeToFile(LOCK_FILE)
        } else {

            // Get latest versions of all packages,
            // and updates the entire lockfile.
            installPackages(&manifest, true)
            manifest.writeToFile(LOCK_FILE)
        }
    }
}
