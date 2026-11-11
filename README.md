# tinyFaaS: A Lightweight FaaS Platform for Edge Environments

tinyFaaS is a lightweight FaaS (Function-as-a-Service) platform for edge
environment with a focus on performance in constrained environments.

This is a fork of the original tinyFaaS, created by Tobias Pfandzelter and David
Bermbach. Most of the original code and design is preserved in this fork. The
build system has been changed from Makefile to Just, and a few other minor
things have been updated. Another difference is that this fork supports only
Linux and CoAP support is removed.

For the original tinyFaaS, please refer to its official [GitHub
repository](https://github.com/OpenFogStack/tinyFaaS).

## Instructions

> [!CAUTION]
> tinyFaaS uses the local Docker instance to manage containers, and it allows
> anyone on your network to start Docker containers running arbitrary code. If
> you don't understand what this means, you should not run this software on your
> computer. Also note that this is a research prototype and is not
> production-ready.

### About

tinyFaaS comprises the _management service_, the _reverse proxy_, and a number
of _function handlers_. In order to run tinyFaaS, the management service has to
be deployed. It will then automatically start the reverse proxy. Once a function
is deployed to tinyFaaS, function handlers are created automatically.

### Prerequisites

Before you get started, make sure you have the following dependencies installed:

- Go to compile management service and reverse proxy
- Docker
- [just](https://github.com/casey/just)
- curl and unzip, only for `just install-protoc`
- [uv](https://docs.astral.sh/uv/), used to run the Python tooling
- a writable directory (tinyFaaS writes temporary files to a `./tmp` directory)

The original tinyFaaS should also work on macOS and on arm64 Linux, but I have
only tested it on Linux x86_64. Also, the modifications I made in this fork do
not take macOS and arm64 into account.

#### One-Time Setup

Building tinyFaaS generates the gRPC code from the protocol buffer definition,
which requires `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc`. If you do
not have them yet, install them once with:

```sh
just install-protoc
```

Make sure that directory (usually `~/go/bin`) is in your `PATH`, otherwise
`protoc` will not find the plugins.

### Getting Started

Start tinyFaaS with:

```sh
just start
```

This builds tinyFaaS for your host platform if necessary and runs it with
`sudo`, so you may be asked for your password. To cross-compile instead, use
`just tinyfaas {OS} {ARCH}`, for example `just tinyfaas linux arm64`.

The reverse proxy will be started automatically. Please note that you cannot use
tinyFaaS until the reverse proxy is running.

### Managing Functions

To manage functions on tinyFaaS, use the `./tinyfaas-cli.py` script. Run
`./tinyfaas-cli.py --help` to see all commands, and use `--url` to target a
management service other than the default `http://localhost:8080`.

#### Deploy a function

There are two ways to deploy a function:

```sh
$ ./tinyfaas-cli.py upload {FOLDER} {NAME} {ENV} {THREADS}
$ ./tinyfaas-cli.py upload-url {URL} {SUBFOLDER_PATH} {NAME} {ENV} {THREADS}
```

The main difference is that the `upload` command takes a local folder that
contains your function code, whereas `upload-url` deploys a function from a zip
file that the tinyFaaS manager can reach at the given URL. In that case, you
also need to give the folder that contains the code inside the zip file (use `/`
if the code is at the top level).

The other available options are:

* `{NAME}`: the name of the function, which also identifies it. You need this
  name to call the function.
* `{ENV}`: the runtime you want to use (`python3`, `go`, `nodejs`, `binary`).
* `{THREADS}`: the number of function handlers, which is how many identical
  containers are created for the function. Each container can already handle
  requests in parallel on its own, and the reverse proxy forwards a call to a
  random function handler. If you want to change this value, you need to
  re-deploy the function.

You can pass environment variables to the function with `-e KEY=VALUE` (you can
repeat this option).

Examples:

```sh
$ ./tinyfaas-cli.py upload "./test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1
$ ./tinyfaas-cli.py upload-url \
    "https://github.com/OpenFogStack/tinyFaaS/archive/main.zip" \
    "tinyFaaS-main/test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1
```

#### Other commands

To get a list of existing functions, run `./tinyfaas-cli.py list`.

To delete a function, run `./tinyfaas-cli.py delete {NAME}`. To remove all
functions, run `./tinyfaas-cli.py wipe`.

To read the logs of all functions, run `./tinyfaas-cli.py logs`, or
`./tinyfaas-cli.py logs {NAME}` for a single function.

### Writing Functions

This tinyFaaS prototype supports functions written for Go 1.27, Node.js 26,
Python 3.13, and binary functions. A good place to start is the collection of
test functions in [`./test/fns`](./test/fns/). In Node.js and Python functions,
HTTP headers and gRPC metadata are available as key-value pairs. See the
"show-headers" test functions for more information.

#### Go

Your function must be supplied as a file named `fn.go` that exposes the package
`main` with the function `fn` that is invoked for every function invocation.
This method must accept two arguments: `data` of type `string`, and `headers` of
type `map[string]string`. Must return a string and an error as return values.
You may also provide a `go.mod` file from which dependencies will be installed
alongside your function.

#### Node.js

Your function must be supplied as a Node module with the name `fn` that exports
a single function that takes the `req` and `res` parameters for request and
response, respectively. `res` supports the `send()` function that has one
parameter, a string that is passed to the client as-is.

#### Python

Your function must be supplied as a file named `fn.py` that exposes a method
`fn` that is invoked for every function invocation. This method must accept a
string as an input (that can also be `None`) and must provide a string as a
return value. You may also provide a `requirements.txt` file from which
dependencies will be installed alongside your function. Any other data you
provide will be available.

#### Binary

Your function must be provided as a `fn.sh` shell script that is invoked for
every function call. This shell script may also call other binaries as needed.
Input data is provided from `stdin`. Output responses should be provided on
`stdout`.

### Calling Functions

tinyFaaS supports different application layer protocols at its reverse proxy.
Different protocols are useful for different use-cases: HTTP to support
traditional web applications, GRPC for inter-process communication.

#### HTTP

To call a tinyFaaS function using its HTTP endpoint, make a GET or POST request
to `http://{HOST}:{PORT}/{NAME}` where `{HOST}` is the address of the tinyFaaS
host, `{PORT}` is the port for the tinyFaaS HTTP endpoint (default is `80`), and
`{NAME}` is the name of your function. You may include data in any form you
want, it will be passed to your function.

TLS is not supported (but contributions are welcome).

To make an asynchronous request, pass the `X-tinyFaaS-Async` header with any
value. An asynchronous request means the client will receive a `202` response
code immediately and no function results will be sent back.

```bash
curl --header "X-tinyFaaS-Async: true" "http://localhost:8000/sieve"
```

#### gRPC

To use the gRPC endpoint, compile the `tinyfaas` protocol buffer (included in
[`./pkg/grpc/tinyfaas`](./pkg/grpc/tinyfaas)) for your programming language and
import it into your application. We already provide compiled versions for Go and
Python in that directory. Specify the tinyFaaS host and port (default is `9000`)
for the GRPC endpoint and use the `Request` function with the
`functionIdentifier` being your function's name and the `data` field including
data in any form you want.

### Removing tinyFaaS

When you stop the management service with `SIGINT` (`Ctrl+C`), the reverse proxy
and all function handlers should be stopped. You can also use `just
clean-docker` to delete generated Docker containers, networks and images, or
`just clean` to delete also generated files in the repository.

### Specifying Ports

By default, tinyFaaS will use the following ports:

| Port | Protocol | Description        |
| ---- | -------- | ------------------ |
| 8080 | TCP      | Management Service |
| 8081 | TCP      | Reverse Proxy      |
| 8000 | TCP      | HTTP Endpoint      |
| 9000 | TCP      | GRPC Endpoint      |

The reverse proxy exposes Prometheus metrics at `http://<host>:8081/metrics`:

* `tinyfaas_function_duration_seconds` (histogram, label `function_name`):
  duration of function invocations.
* `tinyfaas_function_invocation_total` (counter, labels `function_name`,
  `status` and `activation_type`, either `sync` or `async`): number of
  function invocations. `status` is the HTTP status code returned by the
  function handler (regardless of the protocol used to invoke the function), or
  `502` if the function handler could not be reached.

To change or deactivate the endpoints of tinyFaaS, you can use the `HTTP_PORT`
and `GRPC_PORT` environment variables. Specify `-1` to deactivate a specific
endpoint. For example, to use `6000` as the port for HTTP and deactivate GRPC,
build tinyFaaS and run it with this command:

```sh
$ sudo env HTTP_PORT=6000 GRPC_PORT=-1 ./tinyfaas-linux-amd64
```

### Tests

The tests in [`./test`](./test) test the end-to-end functionality of tinyFaaS
and are expected to complete successfully. We use these tests during development
to ensure no patches break any functionality. The tests can also serve as
documentation on the expected behavior of tinyFaaS.

Running the tests requires:

- Python >3.10
- `just`, `uv` and Docker
- `sudo` access, as tinyFaaS needs it to manage Docker containers

Run the tests from the repository root with `just test`. This builds tinyFaaS
for your host and runs the tests with `uv`. The tests start their own tinyFaaS
instance, so make sure no other instance is running. You will be able to see the
output and result for each test, and tinyFaaS output will be written to
`tf_test.out`.

### Known limitations

#### HTTP status code and headers

Any function, in any runtime, cannot control the HTTP status code or the HTTP
headers that are returned. Only the body is sent back to the client. If the
function returns an error or throws an exception (except in the binary runtime),
the error is included in the body and the HTTP status is 500.

To recap, for function requests, tinyFaaS returns the following HTTP status
codes to the client:

* `200 OK`: The container responded with any status: success, a Python
  exception, a Go error, a non-zero exit from a binary, or a Node.js
  `res.status(...)`.
* `202 Accepted`: The request had the `X-tinyFaaS-Async` header. The proxy
  replies straight away and never waits for the function.
* `404 Not Found`: No function is deployed under that name.
* `500 Internal Server Error`: The proxy couldn't read the client's request
  body, couldn't reach the container (connection refused, or the container
  crashed mid-request), or couldn't read the container's response.

#### Function timeout

The reverse proxy does not set a timeout for a function request. A synchronous
function that hangs keeps the client waiting forever instead of getting an error
code.

### License

The code in this repository is licensed under the terms of the [MIT](./LICENSE)
license.
