<p align="center">
  <img src="https://user-images.githubusercontent.com/2414826/154606331-751abc5a-2c92-40d0-bc89-bd772397a14f.png"
       width="192"
       height="192"
       alt="Golang gopher mascot with little horns" />
</p>

# godemon

![go test](https://github.com/bduffany/godemon/actions/workflows/test.yaml/badge.svg)
[![License: Unlicense](https://img.shields.io/badge/license-Unlicense-blue.svg)](http://unlicense.org/)

godemon is a tool that runs a command, monitors for changes to files,
and restarts the command automatically whenever files change.

It is great for:

- Restarting a development server whenever you change source code
- Running tests whenever you change source code
- Mirroring a directory to another location, using a tool like `rsync`
- Lots of other things!

It is inspired by [nodemon](https://github.com/remy/nodemon) which
is a similar tool written in JavaScript.

## Goals

- **Generic**: Does not give preference to any language or framework (even
  Go). Can run arbitrary comands when changing arbitrary files.
- **Configurable**: There is no "one size fits all" approach to file
  watchers, so godemon gives you fine-grained control over the paths that
  are included or ignored. For more advanced users, it also lets you
  control which signals are used to restart your process, since different
  processes respond differently to different signals.
- **Strong defaults**: godemon ships with a default ignore list and also
  treats `.gitignore` files as additional ignore patterns. These defaults
  can be turned off via config options.

## Status

godemon has been tested on Mac and Linux. It has not yet been tested on
Windows.

The command line interface and config file format are not yet stable.

## Installation

The easiest way to install godemon is with the `go` CLI:

```bash
go install github.com/bduffany/godemon/cmd/godemon@latest
```

## Basic usage

The `godemon` command can be used as follows:

```
godemon [options ...] <command> [args ...]
```

By default, the given command is executed whenever any file in the current
directory changes.

## Examples

To watch the current directory (recursively) and run a command on any
change:

```bash
godemon command
```

To watch multiple directories, use `--watch` or `-w`. **When using this
flag, the current directory needs to be passed explicitly,** otherwise
it's assumed that you only want to watch `/some/other/dir` (in this
example).

```bash
godemon --watch . --watch /some/other/dir command
```

To only restart on changes to certain paths or patterns, use `--only` or
`-o`:

```bash
godemon --only '*.ts' --only '*.tsx' command
```

To ignore changes to certain paths or patterns, use `--ignore` or `-i`:

```bash
godemon --ignore '**/build/**' --ignore '**/dist/**' command
```

## Default ignored patterns

The default ignored patterns are listed in
[default_ignore.go](https://github.com/bduffany/godemon/tree/master/default_ignore.go).

To disable the default ignore list, pass `--no-default-ignore`.

By default, godemon also ignores patterns that are listed in `.gitignore`
files for any watched paths which are contained in a Git repository. To
disable this behavior, pass `--no-gitignore`.
