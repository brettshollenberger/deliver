package main

import (
    "encoding/json"
    "io/ioutil"
    "flag"
    "log"
    "os"
    "path"
    "bytes"
    "os/exec"
)

var rootPackageFile *string = flag.String("file", "packages.json", "JSON describing the app packages")
var noRun *bool = flag.Bool("n", false, "print the commands but do not run them")
var verbose *bool = flag.Bool("v", false, "print the commands while running them")

// Entire package definition
type Manifest struct {
    Packages []Package `json:"packages"`
}

// Packages defined in the manifest
type Package struct {
    Location string `json:"location"`
    Source   string `json:"source"`
}

// Gets or updates all packages specified in the given file.
// Fetches packages recursively if one of the referenced packages
// has a manifest.
func updatePackagesFromFile(packageManifestFile string) {
    fileBytes, err := ioutil.ReadFile(packageManifestFile)
    if err != nil {
        log.Fatal(err)
    }

    var manifest Manifest
    err = json.Unmarshal(fileBytes, &manifest)

    if err != nil {
        log.Fatal(err)
    }

    packages := manifest.Packages
    for _, p := range packages {
        subPackageManifestFile := updatePackage(&p)
        if subPackageManifestFile != "" {
            updatePackagesFromFile(subPackageManifestFile)
        }
    }
}

// Gets or updates the given package.
// Returns a string path to the package's manifest file if it
// contains one, otherwise returns an empty string.
func updatePackage(p *Package) (packageManifest string) {
    goRoot := os.Getenv("GOPATH")
    packageDir := path.Join(goRoot, "src", p.Location)
    log.Println("PACKAGE: ", p.Location)

    _, err := os.Stat(packageDir);
    if os.IsNotExist(err) {
        // Package directory does not exist yet. Create it.
        err = executeCommand("mkdir", []string{"-p", packageDir})
        if err != nil {
            log.Fatal(err)
        }
        cloneRepo(p.Source, packageDir)
    } else {
        // Package directory exists. Check if there's a git repository.
        gitInfoPath := path.Join(packageDir, ".git")
        if _, err := os.Stat(gitInfoPath); os.IsNotExist(err) {
            // Git repo does not exist. Clone it.
            cloneRepo(p.Source, packageDir)
        } else {
            // Git repo exists. Pull latest.
            pullRepo(packageDir)
        }
    }

    // Return true if the package has a manifest
    packageManifestFile := path.Join(packageDir, "packages.json")
    if _, err = os.Stat(packageManifestFile); os.IsNotExist(err) {
        return ""
    } else {
        return packageManifestFile
    }
}

// Pulls the git repo from origin in the given repo path.
func pullRepo(repoPath string) {
    currentDir, err := os.Getwd()
    if err != nil {
        log.Fatal(err)
    }

    err = os.Chdir(repoPath);
    if err != nil {
        log.Fatal(err)
    }

    err = executeCommand("git", []string{"pull"})
    if err != nil {
        log.Fatal(err)
    }

    err = os.Chdir(currentDir)
    if err != nil {
        log.Fatal(err)
    }
}

// Clones the git repo into the given directory.
func cloneRepo(repoUrl string, destinationPath string) {
    err := executeCommand("git", []string{"clone", repoUrl, destinationPath})
    if err != nil {
        log.Fatal(err)
    }
}

// Executes a shell command. Depending on the flags,
// it may just print the command to run, or both print and
// run the command.
func executeCommand(name string, args []string) (err error) {
    if *noRun || *verbose {
        log.Println(name, args)
    }

    if !*noRun {
        cmd := exec.Command(name, args...)
        var out bytes.Buffer
        cmd.Stdout = &out
        err = cmd.Run()
    }
    return
}

func main() {
    flag.Parse()
    updatePackagesFromFile(*rootPackageFile)
}
