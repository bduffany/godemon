<p align="center">
  <img src="https://user-images.githubusercontent.com/2414826/154606331-751abc5a-2c92-40d0-bc89-bd772397a14f.png"
       width="192"
       height="192"
       alt="Golang gopher mascot with little horns" />
</p>

# godemon

godemon is a tool that runs a command, monitors for changes to files,
and restarts the command automatically whenever files change.

It is great for:

- Restarting a development server whenever you change source code
- Running tests whenever you change source code
- Mirroring a directory to another location, using a tool like `rsync`
- Lots of other things!

It is inspired by [nodemon](https://github.com/remy/nodemon) which
is a similar tool written in JavaScript.

## Status

godemon has been tested on Mac and Linux. It has not yet been tested on
Windows.

The command line interface and config file format are not yet stable.

## Installation

The easiest way to install godemon is with the `go` CLI:

```bash
go install github.com/bduffany/godemon@latest
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

godemon also ignores patterns in the top-level `.gitignore` file of each
watched directory by default. To disable this behavior, pass
`--no-gitignore`.
