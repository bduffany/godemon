# godemon

Like [nodemon](https://github.com/remy/nodemon), but written in Go.

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

To watch the current directory (recursively) and run a command on any change:

```bash
godemon echo 'Something changed in ./'
```

To watch multiple directories, use `--watch` or `-w`. When using `--watch`,
the current directory needs to be passed explicitly:

```bash
godemon --watch . --watch /some/other/dir echo 'Watching ./ or in /some/other/dir'
```

To only restart when certain files patterns change, use `--only` or `-o`:

```bash
godemon --only '**/*.ts' --only '**/*.tsx' echo 'Watching ts/tsx files in ./'
```

To ignore certain directories, use `--ignore` or `-i`:

```bash
godemon --ignore '**/server/**' echo 'Something changed, but not in the /server/ dir'
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
