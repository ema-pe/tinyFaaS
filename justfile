project_name := "tinyFaaS"
pkg          := "github.com/OpenFogStack/" + project_name
test_dir     := "./test"
proto_dir    := "pkg/grpc/tinyfaas"

supported_arch := "amd64 arm arm64"

os   := `go env GOOS`
arch := `go env GOARCH`

requirements := proto_dir + "/requirements.txt"
py           := "uv run --with-requirements " + requirements + " python3"

default: build

build: (tinyfaas os arch)

start: build
    sudo ./tinyfaas-{{os}}-{{arch}}

test: build proto-py
    sudo env "PATH=$PATH" {{py}} {{test_dir}}/test_all.py

clean:
    sudo sh clean.sh
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
    tag=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
        https://github.com/protocolbuffers/protobuf/releases/latest | sed 's|.*/||')
    version="${tag#v}"
    echo "Installing protoc ${version} (${platform})"

    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    curl -fsSL -o "$tmp/protoc.zip" "https://github.com/protocolbuffers/protobuf/releases/download/${tag}/protoc-${version}-${platform}.zip"
    unzip -q "$tmp/protoc.zip" -d "$tmp/protoc"

    sudo install -m 0755 "$tmp/protoc/bin/protoc" /usr/local/bin/protoc
    sudo mkdir -p /usr/local/include
    sudo cp -r "$tmp/protoc/include/." /usr/local/include/

    # Required Go plugins
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

    protoc --version
    echo "Make sure $(go env GOPATH)/bin is in your PATH."

# Generate Protocol Buffers code for Go
proto-go:
    protoc -I {{proto_dir}} {{proto_dir}}/tinyfaas.proto \
        --go_out={{proto_dir}} --go_opt=paths=source_relative \
        --go-grpc_out={{proto_dir}} --go-grpc_opt=require_unimplemented_servers=false,paths=source_relative

# Generate Protocol Buffers code for Python
proto-py:
    {{py}} -m grpc_tools.protoc -I {{proto_dir}} \
        --python_out={{proto_dir}} --grpc_python_out={{proto_dir}} --mypy_out={{proto_dir}} \
        {{proto_dir}}/tinyfaas.proto

# Build the reverse proxy for a given target (`just rproxy linux arm64`).
rproxy goos goarch: proto-go proto-py
    GOOS={{goos}} GOARCH={{goarch}} go build -o cmd/manager/rproxy-{{goos}}-{{goarch}}.bin -v {{pkg}}/cmd/rproxy

# Build the runtime images/blobs for one architecture
runtimes a:
    #!/usr/bin/env bash
    set -euo pipefail
    for df in $(find pkg/docker/runtimes -name Dockerfile); do
        src=$(dirname "$df")
        rt=$(basename "$src")
        out="pkg/docker/runtimes-{{a}}/$rt"
        mkdir -p "$out"
        cp -r "$src/Dockerfile" "$out/Dockerfile"
        # skip if blob is newer than its build.Dockerfile
        if [ -f "$out/blob.tar.gz" ] && [ "$out/blob.tar.gz" -nt "$src/build.Dockerfile" ]; then
            continue
        fi
        (cd "$src" && sudo docker build --platform=linux/{{a}} -t "tf-build-{{a}}-$rt" -f build.Dockerfile .)
        sudo docker run -d -t --platform=linux/{{a}} --name "{{project_name}}-$rt" --rm "tf-build-{{a}}-$rt"
        sudo docker export "{{project_name}}-$rt" | gzip > "$out/blob.tar.gz"
        sudo docker kill "{{project_name}}-$rt"
    done

# Build the runtimes for every supported architecture
runtimes-all:
    #!/usr/bin/env bash
    set -euo pipefail
    for a in {{supported_arch}}; do just runtimes "$a"; done

# Build the management service for a given target (`just tinyfaas linux arm64`).
tinyfaas goos goarch: (rproxy goos goarch) (runtimes goarch)
    GOOS={{goos}} GOARCH={{goarch}} go build -o tinyfaas-{{goos}}-{{goarch}} -v {{pkg}}/cmd/manager
