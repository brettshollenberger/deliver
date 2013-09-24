deliver: a Go package manager
=======

deliver is a command-line tool to fetch and update package dependencies in a Go project.
Each package must be mapped to a Git repository. A package declares its dependencies in a `packages.json` file in the repository root:

```
{
    "packages": [
        {
            "location": "github.com/edmodo/minion",
            "source": "git@github.com:edmodo/minion.git"
        },
        {
            "location": "git.apache.org/thrift.git",
            "source": "git@github.com:apache/thrift.git"
        }
    ]
}
```

For each package dependency, `location` defines the package directory relative to the `src/` directory of the Go workspace.
`source` defines the Git repository from which to fetch and update the package. Note that unlike with the `go get` tool,
the source does not have to match the location. This allows us, for example, to download the Apache thrift package from Github
mirror rather than git.apache.org (which is quite slow).

Running `deliver` will fetch or update all packages specified in `./packages.json`. If any of the packages themselves include a `packages.json` file, deliver will recursively fetch or update all of those dependencies as well.


Binary
------
You can download the latest `deliver` binary for Linux from [https://s3-us-west-2.amazonaws.com/nodemodo/users/adam/deliver](https://s3-us-west-2.amazonaws.com/nodemodo/users/adam/deliver).


Boostrapping Edmodo projects
----------------------------
You can use deliver to fetch an entire Go project into your workspace by finding the project manifest file.
For example, The project manifest for the Planner backend can be downloaded from[https://s3-us-west-2.amazonaws.com/nodemodo/users/adam/planner_manifest.json](https://s3-us-west-2.amazonaws.com/nodemodo/users/adam/planner_manifest.json).

Running `./deliver --file=planner_manifest.json` will fetch the Planner project and all necessary packages.


To-do
-----
- Allow specifying git branches and tags for packages
- Detect circular dependencies (current deliver will get caught in a loop)
- Expand support for other source control systems
