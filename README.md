This tool takes as input:

- a source directory
- a list of Go packages
- a "with tests" flag

and removes from the sources everything that is not needed to build any of the provided Go packages. If the "with tests" flag is provided, then all tests corresponding to code that remains also remain.

The way it works:
- it uses static code analysis to identify all the dependencies for all the provided packages
    - including things like resources defined using "embed"
    - including test dependencies if "with tests" is enabled
- it calculates a list of files to keep in the repository
- then it proceeds to delete everything else

Note that going forward, code deletion will only be one of the outcomes. Using the same information, we'll want to generate things like Dockerignore files, or even git sparse checkout specifications. So the code is architected in a way that makes it possible.