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

var noRun *bool = flag.Bool("n", false, "print the commands but do not run them")
var verbose *bool = flag.Bool("v", false, "print the commands while running them")

// Entire package definition
type Manifest struct {
    Repository string
    Packages map[string]Package
}

// Packages defined in the manifest
type Package struct {
    Source   string
    Branch   string
    Revision string
}

func (p *Package) getBranch() string {
    if p.Branch == "" {
        return "master"
    } else {
        return p.Branch
    }
}

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

// Gets or updates all packages specified in the given file.
// Fetches packages recursively if one of the referenced packages
// has a manifest.
func installPackages(manifest *Manifest, update bool) {
    packages := manifest.Packages
    subPackagesToInstall := make([]Manifest, 0)
    for packageName, packageInfo := range packages {
        subPackageFile := installPackage(packageName, &packageInfo, update)

        // Package revision may have been updated
        if update {
            packages[packageName] = packageInfo
        }

        // Package may have own dependencies.
        if subPackageFile != "" {
            subPackageManifest := parseManifest(subPackageFile)
            subPackagesToInstall = append(subPackagesToInstall, subPackageManifest)
        }
    }

    //for _, manifest := range subPackagesToInstall {
    //    installPackages(&manifest, update)
    //}
}

// Installs the given package. If the package has a locked revision,
// use the locked revision. Otherwise, fetch and checkout the specified branch.
func installPackage(packageName string, packageInfo *Package, updateRevision bool) (packageManifest string) {
    goRoot := os.Getenv("GOPATH")
    packageDir := path.Join(goRoot, "src", packageName)
    log.Printf("updating %s", packageName)

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

    if packageInfo.Revision != "" {
        git.checkoutRevision(packageInfo.Revision)
    } else {
        git.checkoutBranch(packageInfo.getBranch())
        git.pullBranch(packageInfo.getBranch())

        if updateRevision {
            packageInfo.Branch = packageInfo.getBranch()
            packageInfo.Revision = git.getCurrentRevision()
        }
    }

    // Return true if the package has a manifest
    packageLockFile := path.Join(packageDir, LOCK_FILE)
    if _, err := os.Stat(packageLockFile); os.IsNotExist(err) {
        return ""
    }
    return packageLockFile
}

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

// Same as checkoutRevision.
func (g *GitRepository) checkoutBranch(branch string) {
    g.checkoutRevision(branch)
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

func (g *GitRepository) fetch() {
    runInDirectory(g.repoPath, func() (string, error) {
        return executeCommand("git", "fetch")
    })
}

type commandFunction func() (string, error)
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
        log.Println(logArgs...)
    }

    if !*noRun {
        cmd := exec.Command(args[0], args[1:]...)
        outBytes, err = cmd.Output()
    }
    return string(outBytes), err
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
    case "install":
        lockManifest := parseManifest(LOCK_FILE)
        installPackages(&lockManifest, false)
    case "update":
        manifest := parseManifest(PACKAGE_FILE)
        if len(args) == 2 {
            // update single package
            packageName := args[1]
            packageInfo, ok := manifest.Packages[packageName]
            if !ok {
                log.Fatalf("package not in packages.json")
            }
            installPackage(packageName, &packageInfo, true)
        } else {
            // update all packages
            installPackages(&manifest, true)
        }
        lockFileData, err := json.Marshal(manifest)
        if err != nil {
            log.Fatal(err)
        }
        ioutil.WriteFile(LOCK_FILE, lockFileData, 0644)
    }
}
