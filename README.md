# dockerpc

`dockerpc` enables JSON-RPC over Docker plugins.

This project was inspired by the [pie plugin architecture](https://github.com/natefinch/pie).

Like `pie`, it enables you to create an JSON-RPC plugin communication over standard in/standard out,
however, it uses Docker containers as `pie` plugin providers. 

stdin/out/error is still used for RPC; however, all of the communication occurs over
the Docker API (via the `/attach` API).

Currently, only plugin `providers` are supported (not consumers) and you must run Docker on a 
non-unix socket port.

## Installation

```
go get github.com/jandre/dockerpc
```

## How to use

  1. Put your `pie` plugin provider in a Docker container as the entrypoint.  

 Example `plugin.go`:
 ```go
// Command example_plugin is an example of a very simple plugin.
//
// example_plugin provides two APIs that communicate via JSON-RPC.  It is
// expected to be started by example_master.
package main

import (
        "log"
        "net/rpc/jsonrpc"

        "github.com/natefinch/pie"
)

func main() {
        log.SetPrefix("[plugin log] ")

        p := pie.NewProvider()
        if err := p.RegisterName("Plugin", api{}); err != nil {
                log.Fatalf("failed to register Plugin: %s", err)
        }
        if err := p.RegisterName("Plugin2", api2{}); err != nil {
                log.Fatalf("failed to register Plugin2: %s", err)
        }
        p.ServeCodec(jsonrpc.NewServerCodec)
}

type api struct{}

func (api) SayHi(name string, response *string) error {
        log.Printf("got call for SayHi with name %q", name)
        *response = "Hi " + name
        return nil
}

type api2 struct{}

func (api2) SayBye(name string, response *string) error {
        log.Printf("got call for SayBye with name %q", name)
        *response = ""
        for i := 0; i < 500; i++ {
                *response += "asdf asdf adsf a dsf adsf asdf asdf sdaffasdf adsf asdf  asdf asdf asd fas df adfs asdf asdf asdf asd fas df asdf sad fjdaskfjldaksjfklajsdflkdjsalkfjlkdasjfdlaj Bye " + name
        }
        return nil
}
```

 Example `Dockerfile`:

 ```
from golang:1.5

RUN mkdir -p /tmp
COPY ./plugin.go /tmp/plugin.go
WORKDIR /tmp
RUN go get -d -v
RUN go build -o bin/plugin plugin.go
RUN cp bin/plugin /bin


ENTRYPOINT ["/bin/plugin"]
 ```

  2. Build the docker image.

  ```bash
  docker build -t docker-plugin:latest --no-cache .
  ```

  3. In your plugin caller, use `dockerpc.NewClient` to start the `docker-plugin` container
  and connect to the RPC service via Docker's API. 
 
  The example below will execute the 'Plugin.SayHi' method in the container!

  ```go
  image := "docker-plugin:latest"
  host := "tcp://127.0.0.1:4500"
  name := "my-container-name"
  client := dockerpc.NewClient(name, image, host)
  
  ...
  var result string
  err := client.Call("Plugin.SayHi", "Jen", &result)
  ```

## Example

See the [./example](example/) directory for an example of a [plugin]('example/plugin/'),
and a [caller]('example/caller/') that launches the plugin via Docker.

```bash
$ cd $GOPATH/src/github.com/jandre/dockerpc/example
$ cd ./plugin && make  # may need `sudo make` if you aren't in the docker user group.
$ cd ../caller && make
$ ./caller

2015/10/12 08:15:01 [plugin log] 2015/10/12 12:09:23 got call for SayHi with name "jen"
2015/10/12 08:15:01 Plugin.SayHi jen Returned: Hi jen
2015/10/12 08:15:01 [plugin log] 2015/10/12 12:09:23 got call for SayHi with name "bob"
2015/10/12 08:15:01 Plugin.SayHi bob Returned: Hi bob
2015/10/12 08:15:01
2015/10/12 08:15:01 Error: rpc: can't find method Plugin.SayHi2
``` 

## TODO

 * [ ] Tests
 * [ ] Implement consumer plugin
 * [ ] Support unix socket connection to Docker.

## Credits

  * [github.com/natefinch/pie](https://github.com/natefinch/pie)
  * [github.com/fsouza/go-dockerclient](https://github.com/fsouza/go-dockerclient) (for queryString function) 
