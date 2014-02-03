deliver: a Go package manager
=======

Deliver is a command-line tool to fetch and update package dependencies in a Go project.
It can also be used to automatically manage different Go workspaces for different projects.

Each package must be mapped to a Git repository. A package declares its dependencies in a `packages.json` file in the repository root:

```
{
    "repository": "github.com/edmodo/auth",
    "packages": {
        "github.com/edmodo/minion": {
            "source": "git@github.com:edmodo/minion.git",
        },
        "git.apache.org/thrift.git": {
            "source": "git@github.com:edmodo/thrift.git",
            "branch": "edmodo-0.9.1"
        }
    }
}
```

Each package also contains a `packages.lock` file, which is a copy of `packages.json` with "lock" information about the revision of each package.

### Using Deliver

1. Install deliver on your machine. Download it from here and place it in a directory on your PATH.
2. Add `alias go='GOPATH=$(deliver path) go'` to ~/.bashrc. Run `source ~/.bashrc`.

### Checking out a project

#### Check out your Go project from Git.
```
> cd ~
> git clone git@github.com:edmodo/auth.git
> cd auth
> ls
models/
main/
users_handler.go
packages.json
packages.lock
```

#### Run `deliver install`
This command will go a few things:
    - create a `workspace/` directory in the repository (ignored by git).
    - download the locked versions of all packages listed in `packages.lock` into `workspace/src`.
    - recursively download any dependencies of the packages into `workspace/src`.
    - create a symlink for `auth` if "repository" is present in `packages.json`.
```
> deliver install
installing github.com/edmodo/minion
getting dependencies of github.com/edmodo/minion
installing github.com/coopernuse/gorp
done with dependencies of github.com/edmodo/minion
installing git.apache.org/thrift.git
done.
> ls
models/
main/
users_handler.go
packages.json
packages.lock
workspace/
    bin/
    pkg/
    src/
        github.com/
            edmodo/
                auth/ -> ~/auth
                minion/
                thrift-services/
            coopernurse/
                gorp/
        git.apache.org/
            thrift.git/
```

#### Build/test/install with `go`
The alias for `go` will dynamically set GOPATH to the appropriate workspace. Thus, when you switch between projects, you automatically switch Go workspaces as well.

### Updating or adding a package
#### Modify `packages.json`.
Lets say we want to switch to the "stable" branch of minion, and add a new dependency:
```diff
{
    "repository": "github.com/edmodo/auth",
    "packages": {
        "github.com/edmodo/minion": {
            "source": "git@github.com:edmodo/minion.git",
+           "branch": "stable"
        },
+       "github.com/bradfitz/gomemcache": {
+           "source": "git@github.com:bradfitz/gomemcache.git"
+       },
        "git.apache.org/thrift.git": {
            "source": "git@github.com:edmodo/thrift.git",
            "branch": "edmodo-0.9.1"
        }
    }
}
```

#### Run `deliver update github.com/edmodo/auth`.
Deliver will download the tip of the "stable" branch, and update `packages.lock` with the new lock information:
```diff
    "packages": {
        "github.com/edmodo/minion": {
            "Source": "git@github.com:edmodo/minion.git",
-            "Branch": "master",
-            "Revision": "7083fb7612a8bc8ef9a48c35b8364fd06fadf9ad"
+            "Branch": "stable",
+            "Revision": "e0dbf3dfaa5531d50c37ccd39d6798c8cc7d4a78"
        },
+        "github.com/bradfitz/gomemcache": {
+           "Source": "git@github.com:bradfitz/gomemcache.git",
+           "Branch": "master",
+           "Revision": "08e1d8ca19c74cb69e01ff7a8f332a2e46448c47"
+       }
}
```
