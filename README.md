# tinyFaaS: A Lightweight FaaS Platform for Edge Environments

tinyFaaS is a lightweight FaaS (Function-as-a-Service) platform for edge
environment with a focus on performance in constrained environments.

## Research

To use tinyFaaS in the version used in our paper, use `git checkout v0.1`.
If you use this software in a publication, please cite it as:

### Text

T. Pfandzelter and D. Bermbach, **tinyFaaS: A Lightweight FaaS Platform for Edge
Environments**, Proceedings of the 2020 IEEE International Conference on Fog
Computing (ICFC '20), Sydney, Australia, 2020, pp. 17-24, DOI:
10.1109/ICFC49376.2020.00011.

### BibTeX

```bibtex
@inproceedings{pfandzelter_tinyfaas:_2020,
    author = "Pfandzelter, Tobias and Bermbach, David",
    title = "tinyFaaS: A Lightweight FaaS Platform for Edge Environments",
    booktitle = "Proceedings of the 2020 IEEE International Conference on Fog Computing (ICFC '20)",
    year = 2020,
    publisher = "IEEE",
    pages = "17--24",
    doi = "10.1109/ICFC49376.2020.00011"
}
```

For a full list of publications, please see [our
website](https://www.tu.berlin/en/3s/research/publications).

### License

The code in this repository is licensed under the terms of the [MIT](./LICENSE)
license.

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

- Go (>=v1.22) to compile management service and reverse proxy
- Docker (>=v24)
- [just](https://github.com/casey/just)
- curl and unzip, only for `just install-protoc`
- [uv](https://docs.astral.sh/uv/), used to run the Python tooling
- a writable directory (tinyFaaS writes temporary files to a `./tmp` directory)

Note that tinyFaaS is intended for Linux hosts (`x86_64` and `arm64`). Due to
limitations of Docker Desktop for Mac, installing and running
[`docker-mac-net-connect`](https://github.com/chipmk/docker-mac-net-connect) is
necessary to run tinyFaaS on macOS hosts. Running tinyFaaS on Windows computers
(native or through WSL) is probably possible but has not been tested and is thus
not recommended.

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
`./tinyfaas-cli.py --help` for all commands, and `--url` to target a management
service other than `http://localhost:8080`.

To upload a function, run `./tinyfaas-cli.py upload {FOLDER} {NAME} {ENV}
{THREADS}`, where `{FOLDER}` is the path to your function code, `{NAME}` is the
name for your function, `{ENV}` is the environment you want to use (`python3`,
`nodejs`, or `binary`), and `{THREADS}` is the number of function handlers for
your function. For example, you can run `./tinyfaas-cli.py upload
"./test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1` to upload the _Sieve of
Eratosthenes_ example function included in this repository. Environment
variables for the function can be passed with `-e KEY=VALUE` (repeatable).

Alternatively, you can upload a function from a zip file available at a URL:
`./tinyfaas-cli.py upload-url {URL} {SUBFOLDER_PATH} {NAME} {ENV} {THREADS}`,
where `{URL}` is the URL of a zip file containing your function code and
`{SUBFOLDER_PATH}` is the folder containing the code within the zip (use `/` if
the code is at the top level). For example, you can run `./tinyfaas-cli.py
upload-url "https://github.com/OpenFogStack/tinyFaaS/archive/main.zip"
"tinyFaaS-main/test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1`.

To get a list of existing functions, run `./tinyfaas-cli.py list`.

To delete a function, run `./tinyfaas-cli.py delete {NAME}`.

To read the logs of all functions, run `./tinyfaas-cli.py logs`, or
`./tinyfaas-cli.py logs {NAME}` for a single function. To remove all functions,
run `./tinyfaas-cli.py wipe`.

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
Different protocols are useful for different use-cases: CoAP for lightweight
communication, e.g., for IoT devices; HTTP to support traditional web
applications; GRPC for inter-process communication.

#### CoAP

To call a tinyFaaS function using its CoAP endpoint, make a GET or POST request
to `coap://{HOST}:{PORT}/{NAME}` where `{HOST}` is the address of the tinyFaaS
host, `{PORT}` is the port for the tinyFaaS CoAP endpoint (default is `5683`),
and `{NAME}` is the name of your function. You may include data in any form you
want, it will be passed to your function.

Unfortunately, [`curl` does not yet support
CoAP](https://curl.se/mail/lib-2018-05/0017.html), but [a
number](https://github.com/coapjs/coap-cli) [of
other](https://aiocoap.readthedocs.io/en/latest/tools.html) [tools are
available](https://fitbit.github.io/golden-gate/tools/coap_client.html).

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
| 5683 | UDP      | CoAP Endpoint      |
| 8000 | TCP      | HTTP Endpoint      |
| 9000 | TCP      | GRPC Endpoint      |

To change the port of the management service, change the port binding in the
`docker run` command.

To change or deactivate the endpoints of tinyFaaS, you can use the `COAP_PORT`,
`HTTP_PORT`, and `GRPC_PORT` environment variables, which must be passed to the
management service Docker container. Specify `-1` to deactivate a specific
endpoint. For example, to use `6000` as the port for the CoAP and deactivate
GRPC, run the management service with this command:

```bash
docker run --env COAP_PORT=6000 --env GRPC_PORT=-1 \
    -v /var/run/docker.sock:/var/run/docker.sock -p 8080:8080 \
    --name tinyfaas-mgmt -d tinyfaas-mgmt tinyfaas-mgmt
```

### Tests

The tests in [`./test`](./test) test the end-to-end functionality of tinyFaaS
and are expected to complete successfully. We use these tests during development
to ensure no patches break any functionality. The tests can also serve as
documentation on the expected behavior of tinyFaaS.

Running the tests requires:

- Python >3.10
- a tinyFaaS binary built for your host

The Python dependencies for CoAP and gRPC are installed automatically by `uv`
when you run `just test`. Run the tests with `just test`. You will be able to
see the output and result for each test. tinyFaaS output will be written to
`tf_test.out`.

On macOS,
[`docker-mac-net-connect`](https://github.com/chipmk/docker-mac-net-connect) is
necessary to run tinyFaaS. There is a [known issue in
`docker-mac-net-connect`](https://github.com/chipmk/docker-mac-net-connect/issues/36)
that silently breaks the tunnel when Docker Desktop enters its resource saver
mode. If you find that tinyFaaS does not start properly on your macOS host, try
restarting the tunnel:

```sh
sudo brew services restart docker-mac-net-connect
```
