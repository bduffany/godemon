<p align="center">
  <img src="https://user-images.githubusercontent.com/2414826/154606331-751abc5a-2c92-40d0-bc89-bd772397a14f.png"
       width="192"
       height="192"
       alt="Golang gopher mascot with little horns" />
</p>

# godemon

godemon lets you run a command that is restarted whenever files change.

It is great for:

- Restarting a development server whenever you change source code
- Running tests whenever you change source code
- Mirroring a directory to another location, using a tool like `rsync`
- Lots of other things!

It is like [nodemon](https://github.com/remy/nodemon), but with a few key
differences:

- Written in Go
- General purpose: works with any command, and gives no special treatment
  to nodejs
- Portable: ships as a single binary with no external dependencies

Tested on Mac and Linux. It has not yet been tested on Windows.

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
