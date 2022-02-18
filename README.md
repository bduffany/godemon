<p align="center">
  <img src="https://user-images.githubusercontent.com/2414826/154606331-751abc5a-2c92-40d0-bc89-bd772397a14f.png"
       width="256"
       height="256"
       alt="Golang gopher mascot with little horns" />
</p>

# godemon

godemon lets you restart a command when files change. It is useful for
things like automatically rebuilding your application when you edit source
files.

Like [nodemon](https://github.com/remy/nodemon), but written in Go.

**STATUS: WIP**. Signal handling, child process management, etc. is not quite
battle-tested and may have some bugs.

## Install

```bash
go install github.com/bduffany/godemon@latest
```

## Basic usage

The `godemon` CLI is used in the following way:

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

To watch multiple directories, use `--watch` or `-w`. When using this
flag, the current directory needs to be passed explicitly:

```bash
godemon --watch . --watch /some/other/dir command
```

To only restart on changes to certain paths or patterns, use `--only` or
`-o`:

```bash
godemon --only '**/*.ts' --only '**/*.tsx' command
```

To ignore changes to certain paths or patterns, use `--ignore` or `-i`:

```bash
godemon --ignore '**/build/**' --ignore '**/dist/**' command
```

## Default ignored dirs

The default ignored patterns are:

```
**/__pycache__/**
**/.git/**
**/.hg/**
**/.svn/**
**/*.swp
**/*.swx
**/bazel-*/**
**/CVS/**
**/node_modules/**
```

To disable the default ignore list, pass `--no-default-ignore`.
