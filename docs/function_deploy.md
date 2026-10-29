# Function deployment

Function deployment in tinyFaaS works in two phases: one when tinyFaaS itself
is built, and one on every upload.

## Phase 1 (build time)

`just build` runs the `runtimes` recipe in the [`justfile`](../justfile). This
recipe prepares the runtimes (for all supported languages) for the host
architecture only (e.g. `amd64`). In particular, for each
[`pkg/docker/runtimes/<env>/`](../pkg/docker/runtimes) folder, it:

1. Builds `build.Dockerfile`.
2. Starts a container from that image and runs `docker export` on it to save
   its whole filesystem to `pkg/docker/runtimes-<arch>/<env>/blob.tar.gz`.
3. Copies the second Dockerfile (`Dockerfile`) next to the blob.

The blobs for all runtimes are then embedded in the manager binary (the
`tinyfaas-<os>-<arch>` executable), along with the reverse proxy binary. The
blob for a specific runtime is then used each time you deploy a function.

## Phase 2 (upload time)

The CLI zips the function folder, base64-encodes it and sends it to the
manager. The manager then uses the Docker API to:

1. Create a build context in `./tmp/<name>-<uuid>/`.
2. Copy the embedded runtime files (`Dockerfile` and `blob.tar.gz`) into it.
3. Copy the function code into `fn/`.
4. Tar the context and build it.
5. Create one bridge network per function.
6. Create `THREADS` containers on that network, with the `-e` environment
   variables.

Every Docker object is tagged with the labels `tinyfaas-function=<name>` and
`tinyFaaS=<instance id>`. [`clean.sh`](../clean.sh) uses these labels to clean
up.

The function's `Dockerfile` starts with `FROM scratch` and
`ADD blob.tar.gz /`, which rebuilds the runtime filesystem. It then adds the
function code and installs its dependencies. For example, this is the `python3`
runtime:

```dockerfile
FROM scratch
ADD blob.tar.gz /
WORKDIR /usr/src/app
COPY fn/* ./
RUN uv pip install --system --no-cache --link-mode=copy -r requirements.txt
CMD [ "python3", "functionhandler.py" ]
```

The other runtimes work as follows:

- `nodejs` runs `npm install ./fn`.
- `go` compiles the function together with the handler. It uses
  `FROM golang` instead of `scratch`, so the base image does get pulled. This
  is because the function must be compiled before use.
- `binary` runs the function's `fn.sh`.

### Redeploy and delete

When you upload a function with a name that already exists, the manager first
builds and starts the new version, then updates the rproxy, and only then
deletes the old containers, network and image.
