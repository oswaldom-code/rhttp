# Installation

## Requirements

Go 1.21 or later. Nothing else: rhttp depends only on the standard library.

## Install

```sh
go get github.com/oswaldom-code/rhttp
```

## Import

```go
import "github.com/oswaldom-code/rhttp"
```

The package name is `rhttp`.

## Verify

```go
package main

import (
    "context"
    "fmt"

    "github.com/oswaldom-code/rhttp"
)

func main() {
    client := rhttp.New()

    resp, err := client.R().
        Context(context.Background()).
        Get("https://httpbin.org/get")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    fmt.Println(resp.Status)
}
```

`rhttp.New()` without options gives you an optimized transport with sane defaults.
Add resiliency with middleware — see the [Quickstart](quickstart.md).
