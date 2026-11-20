# tinyFaaS: A Lightweight FaaS Platform for Edge Environments

tinyFaaS is a lightweight FaaS (Function-as-a-Service) platform for edge
environments with a focus on performance in constrained environments.

This is a fork of the original
[tinyFaaS](https://github.com/OpenFogStack/tinyFaaS) by Tobias Pfandzelter and
David Bermbach. See [Differences from upstream](#differences-from-upstream) for
what changed.

> [!CAUTION]
> tinyFaaS uses the local Docker instance to manage containers, and it allows
> anyone on your network to start Docker containers running arbitrary code. If
> you don't understand what this means, you should not run this software on your
> computer. Also note that this is a research prototype and is not
> production-ready.

## Quick Start

The fastest way to run tinyFaaS is the published Docker image. You need a Linux
host, Docker, and [uv](https://docs.astral.sh/uv/) to run the CLI.

To start tinyFaaS:

```console
$ sudo docker pull ghcr.io/ema-pe/tinyfaas:dev
$ sudo docker run --detach --name tinyfaas --network host \
     --volume /var/run/docker.sock:/var/run/docker.sock \
     ghcr.io/ema-pe/tinyfaas:dev
```

Both options are required:

* `--volume`: tinyFaaS manages function containers through the host Docker
  daemon, so it needs its socket.
* `--network host`: each function runs on its own Docker network, and the
  management service and the reverse proxy reach the function containers
  directly on their IPs. From the host network namespace they are reachable,
  whereas from a container on a regular Docker network they are not.

Check that it is running with `sudo docker logs --follow tinyfaas`.

Functions are managed with the `tinyfaas-cli.py` script from this repository.
Cloning the repository also gives you the example functions in
[`./test/fns`](./test/fns/):

```console
$ git clone https://github.com/ema-pe/tinyfaas.git
$ cd tinyfaas
$ ./tinyfaas-cli.py --help
```

Deploy and call a function:

```console
$ ./tinyfaas-cli.py upload "./test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1
$ curl "http://localhost:8000/sieve"
```

At the end you can stop tinyFaaS:

```console
$ sudo docker stop tinyfaas
$ sudo docker rm tinyfaas # Warning: do not do "docker rm -f"!
```

Stopping the container also stops the reverse proxy and removes all function
handlers.

## How It Works

tinyFaaS comprises the _management service_, the _reverse proxy_, and a number
of _function handlers_. Only the management service has to be started: it then
starts the reverse proxy automatically. When a function is deployed, its
function handlers (one or more containers) are created automatically.

## Managing Functions

Use `./tinyfaas-cli.py` to manage functions. Run `./tinyfaas-cli.py --help` to
see all commands, and use `--url` to target a management service other than the
default `http://localhost:8080`.

### Deploy a function

There are two ways to deploy a function:

```sh
./tinyfaas-cli.py upload {FOLDER} {NAME} {ENV} {THREADS}
./tinyfaas-cli.py upload-url {URL} {SUBFOLDER_PATH} {NAME} {ENV} {THREADS}
```

`upload` takes a local folder that contains your function code. `upload-url`
deploys a function from a zip file that the tinyFaaS manager can reach at the
given URL; `{SUBFOLDER_PATH}` is the folder inside the zip that contains the
code (use `/` if the code is at the top level).

The other options are:

* `{NAME}`: the name of the function, which also identifies it. You need this
  name to call the function.
* `{ENV}`: the runtime to use (`python3`, `go`, `nodejs`, `binary`).
* `{THREADS}`: the number of function handlers, i.e. how many identical
  containers are created for the function. Each container can already handle
  requests in parallel, and the reverse proxy forwards each call to a random
  handler. To change this value, re-deploy the function.

Pass environment variables to the function with `-e KEY=VALUE` (repeatable).

Examples:

```sh
./tinyfaas-cli.py upload "./test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1
./tinyfaas-cli.py upload-url \
    "https://github.com/OpenFogStack/tinyFaaS/archive/main.zip" \
    "tinyFaaS-main/test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1
```

### Other commands

* `./tinyfaas-cli.py list`: list deployed functions.
* `./tinyfaas-cli.py delete {NAME}`: delete a function.
* `./tinyfaas-cli.py wipe`: delete all functions.
* `./tinyfaas-cli.py logs`: show the logs of all functions.
* `./tinyfaas-cli.py logs {NAME}`: show the logs of one function.

## Writing Functions

tinyFaaS supports functions written for Go 1.27, Node.js 26, Python 3.13, and
binary functions. A good place to start is the collection of test functions in
[`./test/fns`](./test/fns/). In Node.js and Python functions, HTTP headers and
gRPC metadata are available as key-value pairs; see the "show-headers" test
functions for details.

### Go

Provide a file named `fn.go` in package `main` with a function `fn`, invoked for
every call. It must accept two arguments, `data` (`string`) and `headers`
(`map[string]string`), and return a `string` and an `error`. You may also
provide a `go.mod` file from which dependencies will be installed.

### Node.js

Provide a Node module named `fn` that exports a single function taking `req`
and `res` (request and response). `res` supports `send()`, which takes one
string parameter that is passed to the client as-is.

### Python

Provide a file named `fn.py` that exposes a function `fn`, invoked for every
call. It must accept a string (which can be `None`) and return a string. You
may also provide a `requirements.txt` file from which dependencies will be
installed. Any other files you provide will be available to the function.

### Binary

Provide a `fn.sh` shell script, invoked for every call. It may call other
binaries as needed. Input is provided on `stdin`; the response is read from
`stdout`.

## Calling Functions

The reverse proxy supports HTTP (for traditional web applications) and gRPC
(for inter-process communication).

### HTTP

Make a GET or POST request to `http://{HOST}:{PORT}/{NAME}`, where `{PORT}` is
the HTTP endpoint port (default `8000`) and `{NAME}` is the function name. Any
request body is passed to the function. TLS is not supported (contributions
are welcome).

For an asynchronous request, pass the `X-tinyFaaS-Async` header with any value.
The client immediately receives a `202` response and no function result.

```sh
curl --header "X-tinyFaaS-Async: true" "http://localhost:8000/sieve"
```

### gRPC

Compile the `tinyfaas` protocol buffer in
[`./pkg/grpc/tinyfaas`](./pkg/grpc/tinyfaas) for your language (compiled
versions for Go and Python are already provided there) and import it into your
application. Connect to the tinyFaaS host on the gRPC port (default `9000`) and
call `Request` with `functionIdentifier` set to the function name and `data` set
to your payload.

## Configuration

### Ports

| Port | Protocol | Description        | Variable    |
| ---- | -------- | ------------------ | ----------- |
| 8080 | TCP      | Management Service |             |
| 8081 | TCP      | Reverse Proxy      |             |
| 8000 | TCP      | HTTP Endpoint      | `HTTP_PORT` |
| 9000 | TCP      | gRPC Endpoint      | `GRPC_PORT` |

Set `HTTP_PORT` or `GRPC_PORT` to change an endpoint port, or to `-1` to disable
it. For example, to use port `6000` for HTTP and disable gRPC:

```sh
sudo docker run --detach --name tinyfaas --network host \
    --volume /var/run/docker.sock:/var/run/docker.sock \
    --env HTTP_PORT=6000 --env GRPC_PORT=-1 \
    ghcr.io/ema-pe/tinyfaas:dev
```

When running a local build instead: `sudo env HTTP_PORT=6000 GRPC_PORT=-1 ./tinyfaas-linux-amd64`.

### Metrics

The reverse proxy exposes Prometheus metrics at `http://{HOST}:8081/metrics`:

* `tinyfaas_function_duration_seconds` (histogram, label `function_name`):
  duration of function invocations.
* `tinyfaas_function_invocation_total` (counter, labels `function_name`,
  `status` and `activation_type`, either `sync` or `async`): number of function
  invocations. `status` is the HTTP status code returned by the function
  handler (regardless of the protocol used to invoke the function), or `502` if
  the function handler could not be reached.

## Known Limitations

### HTTP status codes and headers

Functions cannot control the HTTP status code or headers returned to the
client; only the body is sent back. For function requests, tinyFaaS returns:

* `200 OK`: the container responded, whatever the outcome: success, a Python
  exception, a Go error, a non-zero exit from a binary, or a Node.js
  `res.status(...)`. Error messages are included in the body.
* `202 Accepted`: the request had the `X-tinyFaaS-Async` header. The proxy
  replies straight away and never waits for the function.
* `404 Not Found`: no function is deployed under that name.
* `500 Internal Server Error`: the proxy couldn't read the client's request
  body, couldn't reach the container (connection refused, or the container
  crashed mid-request), or couldn't read the container's response.

### Function timeout

The reverse proxy does not set a timeout for function requests. A synchronous
function that hangs keeps the client waiting forever instead of returning an
error.

## Development

This section is only needed if you want to build tinyFaaS yourself instead of
using the published image.

### Prerequisites

- Go, to compile the management service and reverse proxy
- Docker
- [just](https://github.com/casey/just)
- [uv](https://docs.astral.sh/uv/), to run the Python tooling
- curl and unzip, only for `just install-protoc`
- a writable working directory (tinyFaaS writes temporary files to `./tmp`)

Building generates the gRPC code from the protocol buffer definition, which
requires `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc`. Install them once
with:

```sh
just install-protoc
```

Make sure the install directory (usually `~/go/bin`) is in your `PATH`,
otherwise `protoc` will not find the plugins.

### Building and running

The provided [justfile](./justfile) contains several recipes for building and
running tinyFaaS. You can see the full list by running `just --list`. If you
just want to run tinyFaaS locally, use `just start`.

You can also build tinyFaaS with Docker. In that case a locally image is run
like the published one, replacing `ghcr.io/ema-pe/tinyfaas:dev` with
`tinyfaas:dev`.

The management service stops the reverse proxy and all function handlers when
it receives `SIGINT` (`Ctrl+C`) or `SIGTERM` (e.g. `kill` or `docker stop`).

### Tests

The tests in [`./test`](./test) check the end-to-end functionality of tinyFaaS
and also serve as documentation of its expected behavior. They require Python,
`just`, `uv`, Docker, and `sudo` access.

Run them from the repository root with `just test`. The tests start their own
tinyFaaS instance, so make sure no other instance is running. tinyFaaS output
is written to `tf_test.out`.

## Differences from upstream

Most of the original code and design is preserved. Changes in this fork:

* The build system uses Just instead of Make.
* Only Linux is supported, and it is only tested on x86_64. The upstream version
  should also work on macOS and arm64 Linux, but the changes in this fork do not
  take them into account.
* CoAP support is removed.

## License

The code in this repository is licensed under the terms of the [MIT](./LICENSE)
license.
