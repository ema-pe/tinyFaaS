project_name := "tinyFaaS"
pkg          := "github.com/OpenFogStack/" + project_name
test_dir     := "./test"
proto_dir    := "pkg/grpc/tinyfaas"

os   := `go env GOOS`
arch := `go env GOARCH`

requirements := proto_dir + "/requirements.txt"
py           := "uv run --with-requirements " + requirements + " python3"

default: build

build: (tinyfaas os arch)

start: build
    sudo ./tinyfaas-{{os}}-{{arch}}

test: build proto-py
    sudo env "PATH=$PATH" {{py}} {{test_dir}}/test_all.py -v

# Remove only Docker objects.
clean-docker:
    sudo sh clean.sh

# Remove Docker objects and also generated files in the git repository.
clean: clean-docker
    rm --recursive --force --verbose pkg/docker/runtimes-*/
    rm --force --verbose tinyfaas-*-* cmd/manager/rproxy-*.bin

# Install protoc to /usr/local, protoc-gen-go and protoc-gen-go-grpc (via Go). Go, curl and unzip must be already installed.
install-protoc:
    #!/usr/bin/env bash
    set -euo pipefail

    # Pick the release asset matching this machine
    case "$(uname -s)-$(uname -m)" in
        Linux-x86_64)          platform="linux-x86_64" ;;
        Linux-aarch64|Linux-arm64) platform="linux-aarch_64" ;;
        Darwin-x86_64)         platform="osx-x86_64" ;;
        Darwin-arm64)          platform="osx-aarch_64" ;;
        *) echo "unsupported platform: $(uname -s)-$(uname -m)" >&2; exit 1 ;;
    esac

    # Get the latest version tag.
    tag=$(curl --fail --silent --show-error --location --head --output /dev/null \
        --write-out '%{url_effective}' \
        https://github.com/protocolbuffers/protobuf/releases/latest | sed 's|.*/||')
    version="${tag#v}"
    echo "Installing protoc ${version} (${platform})"

    tmp=$(mktemp --directory)

    # Make sure to remove the tmp dir on exit (also on errors).
    trap 'rm --recursive --force "$tmp"' EXIT

    # Get the binary.
    curl --fail --silent --show-error --location --output "$tmp/protoc.zip" \
        "https://github.com/protocolbuffers/protobuf/releases/download/${tag}/protoc-${version}-${platform}.zip"
    unzip -q "$tmp/protoc.zip" -d "$tmp/protoc"

    sudo install --mode=0755 "$tmp/protoc/bin/protoc" /usr/local/bin/protoc
    sudo mkdir --parents /usr/local/include
    sudo cp --recursive "$tmp/protoc/include/." /usr/local/include/

    # Required Go plugins.
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

    protoc --version
    echo "Make sure $(go env GOPATH)/bin is in your PATH."

# Generate Protocol Buffers code for Go
proto-go:
    protoc --proto_path={{proto_dir}} {{proto_dir}}/tinyfaas.proto \
        --go_out={{proto_dir}} --go_opt=paths=source_relative \
        --go-grpc_out={{proto_dir}} --go-grpc_opt=require_unimplemented_servers=false,paths=source_relative

# Generate Protocol Buffers code for Python
proto-py:
    {{py}} -m grpc_tools.protoc --proto_path={{proto_dir}} \
        --python_out={{proto_dir}} --grpc_python_out={{proto_dir}} --mypy_out={{proto_dir}} \
        {{proto_dir}}/tinyfaas.proto

# Build the reverse proxy for a given os/arch target (`just rproxy linux arm64`).
[arg("arch", pattern="amd64|arm|arm64")]
rproxy os arch: proto-go proto-py
    GOOS={{os}} GOARCH={{arch}} go build -o cmd/manager/rproxy-{{os}}-{{arch}}.bin -v {{pkg}}/cmd/rproxy

# Build the runtime images/blobs for one architecture.
[arg("arch", pattern="amd64|arm|arm64")]
runtimes arch:
    #!/usr/bin/env bash
    set -euo pipefail
    for df in $(find pkg/docker/runtimes -name Dockerfile); do
        src=$(dirname "$df")
        rt=$(basename "$src")
        out="pkg/docker/runtimes-{{arch}}/$rt"
        blob="$out/blob.tar.gz"
        image="tf-build-{{arch}}-$rt"
        name="{{project_name}}-{{arch}}-$rt"

        mkdir --parents "$out"
        cp "$src/Dockerfile" "$out/Dockerfile"

        # Skip build if the blob exists and no file in the build context is
        # newer.
        if [ -f "$blob" ] && [ -z "$(find "$src" -type f -newer "$blob" -print -quit)" ]; then
            continue
        fi

        (cd "$src" && sudo docker build --platform=linux/{{arch}} --tag "$image" --file build.Dockerfile .)

        # Make sure there was not a container with the same name.
        sudo docker rm --force "$name" >/dev/null 2>&1 || true

        # Make sure that on exit or error the stale container and the
        # half-written temporary blob are deleted!
        trap 'sudo docker kill "$name" >/dev/null 2>&1 || true; rm --force "$blob.tmp"' EXIT

        # Finally we can build the runtime.
        sudo docker run --detach --tty --platform=linux/{{arch}} --name "$name" --rm "$image"
        sudo docker export "$name" | gzip > "$blob.tmp"
        sudo docker kill "$name" >/dev/null
        mv "$blob.tmp" "$blob"
    done

# Build the runtimes for every supported architecture (amd64, arm, and arm64).
runtimes-all: (runtimes "amd64") (runtimes "arm") (runtimes "arm64")

# Build the management service for a given os/arch target (`just tinyfaas linux arm64`).
[arg("arch", pattern="amd64|arm|arm64")]
tinyfaas os arch: (rproxy os arch) (runtimes arch)
    GOOS={{os}} GOARCH={{arch}} go build -o tinyfaas-{{os}}-{{arch}} -v {{pkg}}/cmd/manager
