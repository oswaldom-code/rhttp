# Instalación

## Requisitos

Go 1.21 o superior. Nada más: rhttp solo depende de la biblioteca estándar.

## Instalar

```sh
go get github.com/oswaldom-code/rhttp
```

## Importar

```go
import "github.com/oswaldom-code/rhttp"
```

El nombre del paquete es `rhttp`.

## Verificar

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

`rhttp.New()` sin opciones te da un transport optimizado con valores por defecto
razonables. La resiliencia se añade con middleware — mira el
[Inicio rápido](quickstart.md).
