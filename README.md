# ![cuddlefs](https://user-images.githubusercontent.com/343539/53932129-3f0a6b00-4066-11e9-848a-5660014aaa4c.png)

[![Go Report Card](https://goreportcard.com/badge/github.com/jzelinskie/cuddlefs?style=flat-square)](https://goreportcard.com/report/github.com/jzelinskie/cuddlefs)
[![Build Status Travis](https://img.shields.io/travis/jzelinskie/cuddlefs.svg?style=flat-square&&branch=master)](https://travis-ci.org/jzelinskie/cuddlefs)
[![Godoc](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)](https://godoc.org/github.com/jzelinskie/cuddlefs)
[![Releases](https://img.shields.io/github/release/jzelinskie/cuddlefs/all.svg?style=flat-square)](https://github.com/jzelinskie/cuddlefs/releases)
[![LICENSE](https://img.shields.io/github/license/jzelinskie/cuddlefs.svg?style=flat-square)](https://github.com/jzelinskie/cuddlefs/blob/master/LICENSE)

**Note**: The `master` branch may be in an *unstable or even broken state* during development. Please use [releases](https://github.com/jzelinskie/cuddlefs/releases) instead of the `master` branch in order to get stable binaries.

cuddlefs is a userspace filesystem for Kubernetes.

## Development

In order to compile the project, the [latest stable version of Go] and knowledge of a [working Go environment] are required.

```sh
git clone git@github.com:jzelinskie/cuddlefs.git
cd cuddlefs
GO111MODULE=on go build ./cmd/cuddlefs
```

[latest stable version of Go]: https://golang.org/dl
[working Go environment]: https://golang.org/doc/code.html

## License

cuddlefs is made available under the Apache 2.0 license.
See the [LICENSE](LICENSE) file for details.
