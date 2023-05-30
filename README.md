# binr #

[![GoDoc](https://godoc.org/github.com/lkingland/binr?status.svg)](https://godoc.org/github.com/lkingland/binr)

Binr is a Go package that implements a very simply way to for acquire
command-line programs.

While developed with binaries in mind, this will also work well for any
command line application which can be sourced programmatically.

!!!! Warning !!!

This is a work in progress.

This is pre-1.0.  Please expect bugs and for the API to change.  Use
at your own risk, and please open issues with questions or bugs.

## Usage

Import the `binr` library and call `.Get` for the absolute path to a locally-
installed command-line-utility.  The provided `Source` will be utilized if
the command is not yet available.

Commands are downloaded to `~/.config/binr` by default, though
`XDG_CONFIG_HOME` can be used to alter the location of `~/.config`.

See the Godocs for more.


## Roadmap

- [X] Simple Fetching
- [X] Content-addressed Caching
- [X] Namespaces
- [ ] Checksum Validation
- [ ] Nonstandard Source Resolution
- [ ] Convenience Source Implementations
- [ ] Command Execution Environment Helper
- [ ] CI
- [ ] Releases

## Architecture

See [[ARCHITECTURE.md]] for how it works and how the code is structured.

## Contributing

See [[CONTRIBUTING.md]] for how to hack on binr.

